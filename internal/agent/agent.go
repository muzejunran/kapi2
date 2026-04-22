package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/session"
	"ai-assistant-service/internal/skill"
	"ai-assistant-service/internal/streaming"

	"github.com/sirupsen/logrus"
)

// AgentState represents the state of a conversation
type AgentState struct {
	SessionID      string
	UserID         string
	PageContext    string
	Memory         *memory.Memory
	MessageHistory []Message
	CurrentUserMsg string
	CurrentState   string
}

// Message represents a conversation message
type Message struct {
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// ToolCall represents a tool call in the conversation
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents a function call
type ToolFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	Error      string `json:"error,omitempty"`
}

// AgentConfig holds agent configuration
type AgentConfig struct {
	LLMEndpoint   string
	LLMApiKey     string
	ModelName     string
	MaxTokens     int
	Temperature   float64
	StreamEnabled bool
	TokenBudget   int
	SkillTimeout  time.Duration
}

// Agent represents the conversational AI agent
type Agent struct {
	config         AgentConfig
	skillRegistry  *skill.SkillRegistry
	toolRegistry   *skill.ToolRegistry
	toolExecutor   *skill.ToolExecutor
	memoryService  *memory.MemoryService
	sessionManager *session.Manager
	streamer       *streaming.Streamer
	logger         *logrus.Logger
	stateMutex     sync.RWMutex
}

// NewAgent creates a new agent instance
func NewAgent(cfg AgentConfig, skillReg *skill.SkillRegistry, toolReg *skill.ToolRegistry, toolExec *skill.ToolExecutor, memService *memory.MemoryService, sessionMgr *session.Manager, streamer *streaming.Streamer) *Agent {
	return &Agent{
		config:         cfg,
		skillRegistry:  skillReg,
		toolRegistry:   toolReg,
		toolExecutor:   toolExec,
		memoryService:  memService,
		sessionManager: sessionMgr,
		streamer:       streamer,
		logger:         logrus.New(),
	}
}

// ProcessMessage processes a user message and returns the response
func (a *Agent) ProcessMessage(ctx context.Context, sessionID, userID, pageContext, message string) (<-chan streaming.StreamEvent, error) {
	// Load session history
	sess, err := a.sessionManager.GetSession(sessionID)
	if err != nil {
		// Session not found, return empty history
		a.logger.WithFields(logrus.Fields{
			"session_id": sessionID,
			"error":      err,
		}).Warn("Session not found, starting with empty history")
		sess = &session.Session{
			ID:       sessionID,
			UserID:   userID,
			Messages: []map[string]interface{}{},
		}
	}

	// Convert session messages to Agent messages
	messageHistory := make([]Message, 0, len(sess.Messages))
	for _, msg := range sess.Messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role != "" {
			messageHistory = append(messageHistory, Message{
				Role:      role,
				Content:   content,
				Timestamp: time.Now(), // Session doesn't store timestamp
			})
		}
	}

	// Create state
	state := &AgentState{
		SessionID:      sessionID,
		UserID:         userID,
		PageContext:    pageContext,
		MessageHistory: messageHistory,
		CurrentUserMsg: message,
	}

	// Load memory
	mem, err := a.memoryService.GetUserMemory(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load memory: %w", err)
	}
	state.Memory = mem

	// Add user message to history
	userMsg := Message{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	}
	state.MessageHistory = append(state.MessageHistory, userMsg)

	// Create stream channel
	streamChan := make(chan streaming.StreamEvent, 100)

	go func() {
		defer close(streamChan)

		// Stream the response
		err := a.streamResponse(ctx, state, streamChan)
		if err != nil {
			a.logger.WithError(err).Error("Error streaming response")
			streamChan <- streaming.StreamEvent{
				Type:    streaming.ErrorEvent,
				Content: "Error processing your request",
			}
		}
	}()

	return streamChan, nil
}

// streamResponse streams the response back to the client
func (a *Agent) streamResponse(ctx context.Context, state *AgentState, streamChan chan<- streaming.StreamEvent) error {
	// Send start event
	streamChan <- streaming.StreamEvent{
		Type: streaming.StartEvent,
	}

	// 1. 检查是否有对应的 Skill
	agentCtx := skill.AgentContext{
		UserID:      state.UserID,
		PageContext: state.PageContext,
		Message:     state.CurrentUserMsg,
	}

	a.logger.WithFields(logrus.Fields{
		"page_context": state.PageContext,
		"message":      state.CurrentUserMsg,
	}).Info("Checking for matching skill")

	skill, score := a.skillRegistry.GetBestSkillForContext(agentCtx)

	a.logger.WithFields(logrus.Fields{
		"score": score,
		"skill_id": func() string {
			if skill != nil {
				return skill.GetID()
			}
			return "nil"
		}(),
	}).Info("Skill match result")

	var response *Message
	var err error

	if score > 0 {
		// 2. 有对应的 Skill，让 Skill 处理
		a.logger.Info("Using skill to process request")
		response, err = a.processWithSkill(ctx, state, skill, streamChan)
	} else {
		// 3. 没有对应 Skill，普通聊天
		a.logger.Info("Using LLM to process request")
		response, err = a.processWithLLM(ctx, state, streamChan)
	}

	if err != nil {
		return err
	}

	// 4. Save conversation to memory
	err = a.memoryService.AddConversationTurn(state.UserID, state.CurrentUserMsg, response.Content)
	if err != nil {
		a.logger.WithError(err).Warn("Failed to save conversation to memory")
	}

	// 5. Save conversation to session
	a.saveSession(state, response)

	// 6. Send done event
	streamChan <- streaming.StreamEvent{
		Type: streaming.DoneEvent,
	}

	return nil
}

// preparePrompt prepares the prompt including memory context
func (a *Agent) preparePrompt(state *AgentState) (string, error) {
	// Build system prompt with memory
	systemPrompt := a.buildSystemPrompt(state)

	// Include conversation history
	prompt := systemPrompt + "\n\n"
	for _, msg := range state.MessageHistory {
		prompt += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}

	return prompt, nil
}

// buildSystemPrompt builds the system prompt with memory context
func (a *Agent) buildSystemPrompt(state *AgentState) string {
	prompt := "You are an AI assistant for 咔皮记账 (KaPi Accounting). Help users manage their finances.\n\n"

	// Add memory context
	if state.Memory.Profile != "" {
		prompt += fmt.Sprintf("User Profile: %s\n", state.Memory.Profile)
	}
	if state.Memory.Preferences != "" {
		prompt += fmt.Sprintf("User Preferences: %s\n", state.Memory.Preferences)
	}
	if len(state.Memory.Facts) > 0 {
		prompt += "User Facts:\n"
		for _, fact := range state.Memory.Facts {
			prompt += "- " + fact + "\n"
		}
	}

	// Add page context
	prompt += fmt.Sprintf("\nCurrent Page: %s\n", state.PageContext)

	// Get available tools from skills that can handle this context
	agentCtx := skill.AgentContext{
		UserID:      state.UserID,
		PageContext: state.PageContext,
		Message:     state.CurrentUserMsg,
	}

	tools := a.skillRegistry.GetToolsForContext(agentCtx)
	if len(tools) > 0 {
		prompt += "Available tools:\n"
		for _, tool := range tools {
			prompt += fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description)
		}
	}

	// Add explicit tool calling rules
	prompt += "\nTool Calling Rules:\n"

	// Check if add_bill tool is available
	for _, tool := range tools {
		if tool.Name == "add_bill" {
			prompt += "- When user wants to add a bill (mentions amount, expense, record, 记账, 账单, etc.), you MUST call the add_bill function.\n"
			prompt += "  Extract: amount (number), category (like 餐饮, 购物, etc.), description (what was bought), timestamp\n"
		}
		if tool.Name == "query_bills" {
			prompt += "- When user wants to query bills, search expenses, or ask about spending history, call the query_bills function.\n"
		}
		if tool.Name == "budget_advisor" {
			prompt += "- When user asks for budget advice, spending recommendations, or savings tips, call the budget_advisor function.\n"
			prompt += "  Extract: income, expenses, savings_goal if mentioned\n"
		}
	}

	prompt += "\nRemember to:"
	prompt += "1. Be concise and helpful"
	prompt += "2. PROACTIVELY use available tools when user requests match the above rules"
	prompt += "3. Remember user preferences"
	prompt += "4. Keep track of important facts"

	return prompt
}

// CallTool executes a tool with the given arguments
func (a *Agent) CallTool(toolName string, args map[string]interface{}) (interface{}, error) {
	tool, exists := a.toolRegistry.GetTool(toolName)
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// Execute the tool using the executor
	result, err := a.toolExecutor.Execute(context.Background(), tool, args)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	return result, nil
}

// processWithSkill 使用 Skill 处理请求
func (a *Agent) processWithSkill(ctx context.Context, state *AgentState, selectedSkill skill.Skill, streamChan chan<- streaming.StreamEvent) (*Message, error) {
	// 1. 从 Skill 获取参数提取 Schema
	agentCtx := skill.AgentContext{
		UserID:      state.UserID,
		PageContext: state.PageContext,
		Message:     state.CurrentUserMsg,
	}
	schema := selectedSkill.GetExtractionSchema(agentCtx)

	// 2. 调用 LLM 提取参数（非流式）
	var extractedParams map[string]interface{}
	if schema != "" {
		params, err := a.callLLMForParameterExtraction(ctx, state.CurrentUserMsg, schema)
		if err != nil {
			a.logger.WithError(err).Warn("Failed to extract parameters with LLM, using fallback")
			extractedParams = a.getFallbackParams(selectedSkill.GetID(), state.CurrentUserMsg)
		} else {
			extractedParams = params
		}
	} else {
		extractedParams = make(map[string]interface{})
	}

	// 3. 构建请求
	skillRequest := skill.SkillRequest{
		Context: agentCtx,
		Intent:  "",
		Input:   state.CurrentUserMsg,
		Args:    extractedParams,
	}

	// 4. 调用 Skill 处理
	skillResponse, err := selectedSkill.Execute(ctx, skillRequest)
	if err != nil {
		return nil, err
	}

	// 5. 如果 Message 为空但有 Data，调用 LLM 生成自然语言（流式）
	var finalMessage string
	if skillResponse.Message == "" && skillResponse.Data != nil {
		dataMap, ok := skillResponse.Data.(map[string]interface{})
		if !ok {
			a.logger.Warn("Failed to cast skillResponse.Data to map")
			finalMessage = "查询成功，但数据格式错误"
		} else {
			// 流式输出，不需要返回完整消息
			a.generateResponseFromDataStreaming(ctx, state.CurrentUserMsg, dataMap, streamChan)
			finalMessage = "" // 已流式发送，这里返回空
		}
	} else {
		finalMessage = skillResponse.Message
	}

	// 6. 发送流式响应
	if finalMessage != "" {
		streamChan <- streaming.StreamEvent{
			Type:    streaming.TextEvent,
			Content: finalMessage,
		}
	}

	return &Message{
		Role:      "assistant",
		Content:   finalMessage,
		Timestamp: time.Now(),
	}, nil
}

// generateResponseFromData 根据 skill 返回的数据生成自然语言回复
func (a *Agent) generateResponseFromData(ctx context.Context, userQuery string, data map[string]interface{}) (string, error) {
	// 打印完整数据用于调试
	a.logger.WithField("full_data", data).Info("Full data from skill")

	// 将数据序列化为 JSON 再解析，统一为 map[string]interface{} 类型
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	var unifiedData map[string]interface{}
	if err := json.Unmarshal(dataJSON, &unifiedData); err != nil {
		return "", err
	}

	// 构建更友好的数据展示格式
	dataDisplay := a.formatDataForLLM(unifiedData)

	// 构建请求
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "你是咔皮记账的 AI 助手。根据用户的问题和查询结果，用自然语言回答用户。",
		},
		{
			"role":    "user",
			"content": fmt.Sprintf("用户问题：%s\n\n查询结果：\n%s\n\n请根据查询结果用自然语言回答用户的问题。", userQuery, dataDisplay),
		},
	}

	req := map[string]interface{}{
		"model":       a.config.ModelName,
		"messages":    messages,
		"max_tokens":  2000,
		"temperature": 0.7,
		"stream":      false,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	a.logger.WithField("request", string(reqBody)).Info("Sending response generation request to LLM")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	type LLMChoice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	type LLMResponse struct {
		Choices []LLMChoice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", err
	}

	if llmResp.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", llmResp.Error.Message)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	return llmResp.Choices[0].Message.Content, nil
}

// processWithLLM 使用 LLM 处理普通聊天（带上下文）
func (a *Agent) processWithLLM(ctx context.Context, state *AgentState, streamChan chan<- streaming.StreamEvent) (*Message, error) {
	// 构建系统提示词
	systemPrompt := a.buildSystemPrompt(state)

	// 构建消息列表
	messages := make([]map[string]interface{}, len(state.MessageHistory)+1)
	messages[0] = map[string]interface{}{
		"role":    "system",
		"content": systemPrompt,
	}
	for i, msg := range state.MessageHistory {
		messages[i+1] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// 准备 LLM 请求
	req := a.buildLLMRequest(messages, true) // 流式

	// 调用 LLM（流式）
	fullContent, toolCalls, err := a.callLLMStreamInternal(ctx, req, streamChan, state.SessionID)
	if err != nil {
		a.logger.WithError(err).Warn("LLM 调用失败，使用兜底响应")
		return a.getMockResponse(state, streamChan), nil
	}

	return &Message{
		Role:      "assistant",
		Content:   fullContent,
		ToolCalls: toolCalls,
		Timestamp: time.Now(),
	}, nil
}

// getMockResponse 返回兜底响应
func (a *Agent) getMockResponse(state *AgentState, streamChan chan<- streaming.StreamEvent) *Message {
	// 获取用户最后一条消息
	userMessage := ""
	if len(state.MessageHistory) > 0 {
		for i := len(state.MessageHistory) - 1; i >= 0; i-- {
			if state.MessageHistory[i].Role == "user" {
				userMessage = state.MessageHistory[i].Content
				break
			}
		}
	}

	// 根据 userMessage 返回不同的 mock 响应
	mockContent := "抱歉，我现在无法连接到 AI 服务。请稍后再试。"

	// 简单的关键词匹配来返回更友好的响应
	switch {
	case containsChinese(userMessage, "记账") || containsChinese(userMessage, "花费") || containsChinese(userMessage, "支出"):
		mockContent = "我帮您记录这笔支出。请问您想记录什么金额？或者我可以帮您查询最近的账单记录。"
	case containsChinese(userMessage, "查询") || containsChinese(userMessage, "账单") || containsChinese(userMessage, "记录"):
		mockContent = "我可以帮您查询账单记录。请告诉我您想查询的时间范围，比如本月或最近一周。"
	case containsChinese(userMessage, "预算") || containsChinese(userMessage, "理财") || containsChinese(userMessage, "存钱"):
		mockContent = "关于预算建议，建议您按照 50/30/20 法则：50% 用于必需品，30% 用于个人消费，20% 用于储蓄。您想了解哪方面的预算建议？"
	case containsChinese(userMessage, "你好") || containsChinese(userMessage, "hi") || containsChinese(userMessage, "hello"):
		mockContent = "您好！我是您的财务助手，我可以帮您记账、查询账单、提供预算建议。请问有什么可以帮您？"
	case userMessage == "":
		mockContent = "您好！我是您的财务助手，有什么可以帮您？"
	}

	// 发送流式事件
	fullContent := mockContent
	for _, char := range []rune(mockContent) {
		if streamChan != nil {
			streamChan <- streaming.StreamEvent{
				Type:    streaming.TextEvent,
				Content: string(char),
			}
		}
	}

	return &Message{
		Role:      "assistant",
		Content:   fullContent,
		ToolCalls: nil,
		Timestamp: time.Now(),
	}
}

// containsChinese 检查字符串是否包含中文关键词
func containsChinese(s, keyword string) bool {
	for i := 0; i <= len(s)-len(keyword); i++ {
		if s[i:i+len(keyword)] == keyword {
			return true
		}
	}
	return false
}

// getFallbackParams 为参数提取失败时提供兜底策略
func (a *Agent) getFallbackParams(skillID, userMsg string) map[string]interface{} {
	params := make(map[string]interface{})

	switch skillID {
	case "add_bill":
		// add_bill 兜底：从用户消息中提取金额和分类
		// 默认值
		params["amount"] = 100.0
		params["category"] = "其他"
		params["description"] = ""

		// 简单的金额提取（查找数字）
		amount := extractNumber(userMsg)
		if amount > 0 {
			params["amount"] = amount
		}

		// 分类提取
		if containsChinese(userMsg, "餐") || containsChinese(userMsg, "吃") || containsChinese(userMsg, "饭") {
			params["category"] = "餐饮"
		} else if containsChinese(userMsg, "买") || containsChinese(userMsg, "购") || containsChinese(userMsg, "衣服") {
			params["category"] = "购物"
		} else if containsChinese(userMsg, "车") || containsChinese(userMsg, "地铁") || containsChinese(userMsg, "公交") {
			params["category"] = "交通"
		} else if containsChinese(userMsg, "玩") || containsChinese(userMsg, "电影") || containsChinese(userMsg, "游戏") {
			params["category"] = "娱乐"
		} else if containsChinese(userMsg, "药") || containsChinese(userMsg, "医院") || containsChinese(userMsg, "看病") {
			params["category"] = "医疗"
		}

	case "query_bills":
		// query_bills 兜底：默认查询本月数据
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		params["start_date"] = startOfMonth.Format("2006-01-02")
		params["end_date"] = now.Format("2006-01-02")
		params["category"] = ""

		// 尝试从消息中提取日期关键词
		if containsChinese(userMsg, "今天") {
			today := time.Now()
			params["start_date"] = today.Format("2006-01-02")
			params["end_date"] = today.Format("2006-01-02")
		} else if containsChinese(userMsg, "昨天") {
			yesterday := time.Now().AddDate(0, 0, -1)
			params["start_date"] = yesterday.Format("2006-01-02")
			params["end_date"] = yesterday.Format("2006-01-02")
		} else if containsChinese(userMsg, "本周") || containsChinese(userMsg, "这周") {
			now := time.Now()
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			startOfWeek := now.AddDate(0, 0, -weekday+1)
			params["start_date"] = startOfWeek.Format("2006-01-02")
			params["end_date"] = now.Format("2006-01-02")
		} else if containsChinese(userMsg, "上周") {
			now := time.Now()
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			startOfLastWeek := now.AddDate(0, 0, -weekday-6)
			endOfLastWeek := now.AddDate(0, 0, -weekday)
			params["start_date"] = startOfLastWeek.Format("2006-01-02")
			params["end_date"] = endOfLastWeek.Format("2006-01-02")
		} else if containsChinese(userMsg, "本月") || containsChinese(userMsg, "这个月") {
			now := time.Now()
			startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			params["start_date"] = startOfMonth.Format("2006-01-02")
			params["end_date"] = now.Format("2006-01-02")
		} else if containsChinese(userMsg, "上月") || containsChinese(userMsg, "上个月") {
			now := time.Now()
			startOfLastMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
			endOfLastMonth := startOfLastMonth.AddDate(0, 1, -1)
			params["start_date"] = startOfLastMonth.Format("2006-01-02")
			params["end_date"] = endOfLastMonth.Format("2006-01-02")
		}

		// 分类筛选
		if containsChinese(userMsg, "餐") || containsChinese(userMsg, "吃") {
			params["category"] = "餐饮"
		} else if containsChinese(userMsg, "购") || containsChinese(userMsg, "买") {
			params["category"] = "购物"
		}

	default:
		// 其他 skill 返回空参数
	}

	return params
}

// formatTimestamp 格式化时间戳，只返回日期
func formatTimestamp(timestamp string) string {
	if timestamp == "" {
		return ""
	}
	// 尝试解析各种时间格式
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return timestamp
}

// getKeys 获取 map 的所有键（用于调试）
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// extractNumber 从字符串中提取第一个数字
func extractNumber(s string) float64 {
	var numStr string
	hasDecimal := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' {
			if c == '.' {
				if hasDecimal {
					break
				}
				hasDecimal = true
			}
			numStr += string(c)
		} else if numStr != "" {
			break
		}
	}

	if numStr == "" {
		return 0
	}

	var num float64
	_, err := fmt.Sscanf(numStr, "%f", &num)
	if err != nil {
		return 0
	}
	return num
}

// buildLLMRequest 构建 LLM 请求
func (a *Agent) buildLLMRequest(messages []map[string]interface{}, stream bool) map[string]interface{} {
	return map[string]interface{}{
		"model":       a.config.ModelName,
		"messages":    messages,
		"max_tokens":  a.config.MaxTokens,
		"temperature": a.config.Temperature,
		"stream":      stream,
	}
}

// callLLMForParameterExtraction 调用 LLM 提取参数（非流式）
func (a *Agent) callLLMForParameterExtraction(ctx context.Context, prompt string, schema string) (map[string]interface{}, error) {
	// 构建请求
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "你是一个参数提取助手。请根据用户输入和给定的 Schema 提取参数，返回纯 JSON 格式，不要包含 markdown 代码块标记。",
		},
		{
			"role":    "user",
			"content": fmt.Sprintf("用户输入：%s\n\nSchema：\n%s\n\n请提取参数并返回纯 JSON。", prompt, schema),
		},
	}

	req := map[string]interface{}{
		"model":       a.config.ModelName,
		"messages":    messages,
		"max_tokens":  2000,
		"temperature": 0,
		"stream":      false,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	a.logger.WithField("request", string(reqBody)).Info("Sending parameter extraction request to LLM")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	type LLMChoice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	type LLMResponse struct {
		Choices []LLMChoice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, err
	}

	if llmResp.Error != nil {
		return nil, fmt.Errorf("LLM API error: %s", llmResp.Error.Message)
	}

	if len(llmResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in LLM response")
	}

	rawContent := llmResp.Choices[0].Message.Content
	a.logger.WithField("response", rawContent).Info("Received LLM response for parameter extraction")

	// 清理可能存在的 markdown 代码块标记
	cleanedContent := strings.TrimSpace(rawContent)
	if strings.HasPrefix(cleanedContent, "```") {
		// 去掉第一行 ```json 或 ```
		lines := strings.SplitN(cleanedContent, "\n", 2)
		if len(lines) > 1 {
			cleanedContent = lines[1]
			// 去掉结尾的 ```
			if idx := strings.LastIndex(cleanedContent, "```"); idx != -1 {
				cleanedContent = cleanedContent[:idx]
			}
			cleanedContent = strings.TrimSpace(cleanedContent)
		}
	}

	if cleanedContent != rawContent {
		a.logger.WithField("cleaned_response", cleanedContent).Info("Cleaned markdown from LLM response")
	}

	// 解析 JSON 内容
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(cleanedContent), &params); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return params, nil
}

// callLLMStreamInternal 内部调用 LLM（流式）
func (a *Agent) callLLMStreamInternal(ctx context.Context, req map[string]interface{}, streamChan chan<- streaming.StreamEvent, sessionID string) (string, []ToolCall, error) {
	// 创建请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}

	// 打印 LLM 请求
	a.logger.WithFields(logrus.Fields{
		"model":    req["model"],
		"messages": req["messages"],
	}).Info("Sending LLM stream request")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	a.logger.Infof("[LLM Request] %s - Session:%s - Sending to: %s", time.Now().Format("15:04:05.000"), sessionID, a.config.LLMEndpoint)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	a.logger.Infof("[LLM Response] %s - Session:%s - Status: %d", time.Now().Format("15:04:05.000"), sessionID, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	// 解析 SSE 流
	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	var toolCalls []ToolCall
	toolCallMap := make(map[int]*ToolCall)

	type StreamChoice struct {
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	}

	type StreamUsage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}

	type StreamResponse struct {
		Choices []StreamChoice `json:"choices"`
		Usage   StreamUsage    `json:"usage,omitempty"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			a.logger.Infof("[LLM Stream] %s - Session:%s - [DONE]", time.Now().Format("15:04:05.000"), sessionID)
			continue
		}

		// Log raw LLM response with timestamp
		a.logger.Infof("[LLM Stream] %s - Session:%s - %s", time.Now().Format("15:04:05.000"), sessionID, data)

		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if streamResp.Error != nil {
			return "", nil, fmt.Errorf("LLM API error: %s", streamResp.Error.Message)
		}

		// Check for token usage and send it to client
		if streamResp.Usage.TotalTokens > 0 {
			streamChan <- streaming.StreamEvent{
				Type:       streaming.TextEvent,
				Content:    "",
				TokenUsage: streamResp.Usage.TotalTokens,
			}
		}

		if len(streamResp.Choices) > 0 {
			delta := streamResp.Choices[0].Delta
			if delta.Content != "" {
				fullContent.WriteString(delta.Content)
				streamChan <- streaming.StreamEvent{
					Type:    streaming.TextEvent,
					Content: delta.Content,
				}
			}

			// Handle tool calls in delta
			for _, tc := range delta.ToolCalls {
				call, exists := toolCallMap[tc.Index]
				if !exists {
					call = &ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: ToolFunction{
							Name:      tc.Function.Name,
							Arguments: make(map[string]interface{}),
						},
					}
					toolCallMap[tc.Index] = call
				}

				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Type != "" {
					call.Type = tc.Type
				}
				if tc.Function.Name != "" {
					call.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
						for k, v := range args {
							call.Function.Arguments[k] = v
						}
					}
				}
			}
		}
	}

	// Convert tool call map to slice in order
	for i := 0; i < len(toolCallMap); i++ {
		if call, exists := toolCallMap[i]; exists {
			toolCalls = append(toolCalls, *call)
		}
	}

	// 打印 LLM 响应
	a.logger.WithFields(logrus.Fields{
		"content_length":   len(fullContent.String()),
		"tool_calls_count": len(toolCalls),
		"content":          fullContent.String(),
	}).Info("Received LLM stream response")

	return fullContent.String(), toolCalls, nil
}

// formatDataForLLM 格式化数据供 LLM 理解（不包含用户敏感信息）
func (a *Agent) formatDataForLLM(data map[string]interface{}) string {
	var sb strings.Builder

	// 调试：打印原始数据
	a.logger.WithField("raw_data", data).Debug("Formatting data for LLM")

	// 处理账单查询结果 - 支持多种类型，过滤敏感信息
	billsValue, hasBills := data["bills"]
	if hasBills {
		switch bills := billsValue.(type) {
		case []interface{}:
			if len(bills) > 0 {
				sb.WriteString("账单记录：\n")
				for i, bill := range bills {
					if billMap, ok := bill.(map[string]interface{}); ok {
						// 格式化时间戳，只保留日期
						timestamp, _ := billMap["timestamp"].(string)
						timestamp = formatTimestamp(timestamp)
						category, _ := billMap["category"].(string)
						amount, _ := billMap["amount"].(float64)
						description, _ := billMap["description"].(string)
						sb.WriteString(fmt.Sprintf("%d. %s %s %.2f元", i+1, timestamp, category, amount))
						if description != "" {
							sb.WriteString(fmt.Sprintf(" (%s)", description))
						}
						sb.WriteString("\n")
					}
				}
			}
		case []map[string]interface{}:
			if len(bills) > 0 {
				sb.WriteString("账单记录：\n")
				for i, bill := range bills {
					// 格式化时间戳，只保留日期
					timestamp, _ := bill["timestamp"].(string)
					timestamp = formatTimestamp(timestamp)
					category, _ := bill["category"].(string)
					amount, _ := bill["amount"].(float64)
					description, _ := bill["description"].(string)
					sb.WriteString(fmt.Sprintf("%d. %s %s %.2f元", i+1, timestamp, category, amount))
					if description != "" {
						sb.WriteString(fmt.Sprintf(" (%s)", description))
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	// 处理汇总信息
	if summaryValue, hasSummary := data["summary"]; hasSummary {
		switch summary := summaryValue.(type) {
		case map[string]interface{}:
			if totalAmount, ok := summary["total_amount"].(float64); ok {
				sb.WriteString(fmt.Sprintf("\n总金额：%.2f元\n", totalAmount))
			}
			if totalCount, ok := summary["total_count"].(int); ok {
				sb.WriteString(fmt.Sprintf("总笔数：%d\n", totalCount))
			}
			if byCategory, ok := summary["by_category"].(map[string]interface{}); ok {
				sb.WriteString("\n分类统计：\n")
				for category, amount := range byCategory {
					if amt, ok := amount.(float64); ok {
						sb.WriteString(fmt.Sprintf("  %s: %.2f元\n", category, amt))
					}
				}
			}
		}
	}

	// 处理其他字段
	if total, ok := data["total"].(int); ok {
		sb.WriteString(fmt.Sprintf("\n总记录数：%d\n", total))
	}

	// 处理成功/失败消息
	if message, ok := data["message"].(string); ok && message != "" {
		sb.WriteString(fmt.Sprintf("\n系统消息：%s\n", message))
	}

	// 如果什么都没输出，输出原始数据用于调试
	if sb.Len() == 0 {
		a.logger.Warn("No data formatted, outputting raw data")
		dataJSON, _ := json.MarshalIndent(data, "", "  ")
		return string(dataJSON)
	}

	return sb.String()
}

// generateResponseFromDataStreaming 流式生成自然语言回复
func (a *Agent) generateResponseFromDataStreaming(ctx context.Context, userQuery string, data map[string]interface{}, streamChan chan<- streaming.StreamEvent) {
	// 将数据序列化为 JSON 再解析，统一类型
	dataJSON, err := json.Marshal(data)
	if err != nil {
		streamChan <- streaming.StreamEvent{
			Type:    streaming.ErrorEvent,
			Content: "数据格式错误",
		}
		return
	}
	var unifiedData map[string]interface{}
	if err := json.Unmarshal(dataJSON, &unifiedData); err != nil {
		streamChan <- streaming.StreamEvent{
			Type:    streaming.ErrorEvent,
			Content: "数据解析错误",
		}
		return
	}

	// 构建更友好的数据展示格式
	dataDisplay := a.formatDataForLLM(unifiedData)

	// 构建请求（流式）
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "你是咔皮记账的 AI 助手。根据用户的问题和查询结果，用自然语言回答用户。",
		},
		{
			"role":    "user",
			"content": fmt.Sprintf("用户问题：%s\n\n查询结果：\n%s\n\n请根据查询结果用自然语言回答用户的问题。", userQuery, dataDisplay),
		},
	}

	req := map[string]interface{}{
		"model":       a.config.ModelName,
		"messages":    messages,
		"max_tokens":  2000,
		"temperature": 0.7,
		"stream":      true,
	}

	// 调用流式 LLM
	reqBody, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		streamChan <- streaming.StreamEvent{
			Type:    streaming.ErrorEvent,
			Content: "调用 LLM 失败",
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		streamChan <- streaming.StreamEvent{
			Type:    streaming.ErrorEvent,
			Content: fmt.Sprintf("LLM 错误: %d", resp.StatusCode),
		}
		return
	}

	// 解析 SSE 流并流式发送
	scanner := bufio.NewScanner(resp.Body)
	type StreamChoice struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	}
	type StreamResponse struct {
		Choices []StreamChoice `json:"choices"`
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}

		if len(streamResp.Choices) > 0 && streamResp.Choices[0].Delta.Content != "" {
			streamChan <- streaming.StreamEvent{
				Type:    streaming.TextEvent,
				Content: streamResp.Choices[0].Delta.Content,
			}
		}
	}
}

// saveSession saves the current conversation to the session
func (a *Agent) saveSession(state *AgentState, response *Message) {
	sess, err := a.sessionManager.GetSession(state.SessionID)
	if err != nil {
		// Session doesn't exist, create a new one
		sessionID, err := a.sessionManager.GetOrCreateSession(state.UserID)
		if err != nil {
			a.logger.WithError(err).Warn("Failed to create session")
			return
		}
		state.SessionID = sessionID
		sess, _ = a.sessionManager.GetSession(sessionID)
	}

	// Add user and assistant messages to session
	now := time.Now().UTC().Format(time.RFC3339)
	sess.Messages = append(sess.Messages, map[string]interface{}{
		"role":      "user",
		"content":   state.MessageHistory[len(state.MessageHistory)-1].Content,
		"timestamp": now,
	})
	sess.Messages = append(sess.Messages, map[string]interface{}{
		"role":      response.Role,
		"content":   response.Content,
		"timestamp": now,
	})
	sess.LastUpdated = now

	if err := a.sessionManager.UpdateSession(state.SessionID, sess); err != nil {
		a.logger.WithError(err).Warn("Failed to save session")
	}
}
