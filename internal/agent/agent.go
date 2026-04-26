package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/session"
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
	LLMEndpoint    string
	LLMApiKey      string
	ModelName      string
	MaxTokens      int
	Temperature    float64
	StreamEnabled  bool
	TokenBudget    int
	SkillTimeout   time.Duration
	SkillServerURL string
}

// Agent represents the conversational AI agent
type Agent struct {
	config         AgentConfig
	skillServerURL string
	httpClient     *http.Client
	memoryService  *memory.MemoryService
	sessionManager *session.Manager
	streamer       *streaming.Streamer
	logger         *logrus.Logger
	stateMutex     sync.RWMutex
}

// NewAgent creates a new agent instance
func NewAgent(cfg AgentConfig, memService *memory.MemoryService, sessionMgr *session.Manager, streamer *streaming.Streamer) *Agent {
	return &Agent{
		config:         cfg,
		skillServerURL: cfg.SkillServerURL,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		memoryService:  memService,
		sessionManager: sessionMgr,
		streamer:       streamer,
		logger:         logger.New(),
	}
}

// ProcessMessage processes a user message and returns the response
func (a *Agent) ProcessMessage(ctx context.Context, sessionID, userID, pageContext, message string) (<-chan streaming.StreamEvent, error) {
	sess, err := a.sessionManager.GetSession(sessionID)
	if err != nil {
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

	messageHistory := make([]Message, 0, len(sess.Messages))
	for _, msg := range sess.Messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role != "" {
			messageHistory = append(messageHistory, Message{
				Role:      role,
				Content:   content,
				Timestamp: time.Now(),
			})
		}
	}

	state := &AgentState{
		SessionID:      sessionID,
		UserID:         userID,
		PageContext:    pageContext,
		MessageHistory: messageHistory,
		CurrentUserMsg: message,
	}

	mem, err := a.memoryService.GetUserMemory(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load memory: %w", err)
	}
	state.Memory = a.memoryService.TrimToBudget(mem)

	state.MessageHistory = append(state.MessageHistory, Message{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	})

	streamChan := make(chan streaming.StreamEvent, 100)

	go func() {
		defer close(streamChan)
		if err := a.streamResponse(ctx, state, streamChan); err != nil {
			a.logger.WithError(err).Error("Error streaming response")
			streamChan <- streaming.StreamEvent{
				Type:    streaming.ErrorEvent,
				Content: "Error processing your request",
			}
		}
	}()

	return streamChan, nil
}

// streamResponse drives the full agent turn
func (a *Agent) streamResponse(ctx context.Context, state *AgentState, streamChan chan<- streaming.StreamEvent) error {
	streamChan <- streaming.StreamEvent{Type: streaming.StartEvent}

	response, err := a.runAgentLoop(ctx, state, streamChan)
	if err != nil {
		return err
	}

	if err := a.memoryService.AddConversationTurn(state.UserID, state.CurrentUserMsg, response.Content); err != nil {
		a.logger.WithError(err).Warn("Failed to save conversation to memory")
	}

	a.saveSession(state, response)

	streamChan <- streaming.StreamEvent{Type: streaming.DoneEvent}
	return nil
}

// runAgentLoop implements the Function Calling loop with skill-server.
// Stage 1: skill-server filters tools by page_context (hard filter).
// Stage 2: LLM decides which tool to call (semantic selection).
// Stage 3: execute tool, branch on action_type, loop or return.
func (a *Agent) runAgentLoop(ctx context.Context, state *AgentState, streamChan chan<- streaming.StreamEvent) (*Message, error) {
	log := logger.FromContext(ctx)
	log.WithFields(logrus.Fields{
		"session_id": state.SessionID,
		"user_id":    state.UserID,
		"page":       state.PageContext,
	}).Info("[agent] loop started")

	tools, err := a.getSkillsFromServer(ctx, state.PageContext)
	if err != nil {
		log.WithError(err).Warn("[agent] skill-server unreachable, proceeding without tools")
	}

	systemPrompt := a.buildSystemPrompt(state)
	messages := make([]map[string]interface{}, 0, len(state.MessageHistory)+1)
	messages = append(messages, map[string]interface{}{"role": "system", "content": systemPrompt})
	for _, m := range state.MessageHistory {
		messages = append(messages, map[string]interface{}{"role": m.Role, "content": m.Content})
	}

	var finalContent strings.Builder

	toolsInRequest := len(tools) > 0

	// When tools are available, append a constraint listing what the LLM CAN do on
	// this page, so it gives a specific decline instead of a free-form answer.
	if toolsInRequest {
		messages[0]["content"] = messages[0]["content"].(string) +
			"\n\n[工具使用约束]\n当前页面支持的操作：\n" + buildToolCapabilitySummary(tools) +
			"\n若用户的请求不属于以上操作，请先告知用户该需求在当前页面无法完成，再说明当前页面能做什么，不要自行回答超出范围的内容。"
	}

	for iter := 0; iter < 5; iter++ {
		var content string
		var toolCalls []ToolCall
		var err error

		if toolsInRequest && iter == 0 {
			// Tool-selection phase: non-streaming — we only need to know which tool the
			// LLM chose. If it chose none, its content is already the decline message
			// (enforced by the system prompt above); we stream that directly below.
			req := a.buildLLMRequest(messages, false)
			req["tools"] = tools
			req["tool_choice"] = "auto"
			content, toolCalls, err = a.callLLMBlocking(ctx, req, state.SessionID)
		} else {
			// Response phase (no tools, or summarising tool results): stream directly.
			req := a.buildLLMRequest(messages, true)
			if toolsInRequest {
				req["tools"] = tools
				req["tool_choice"] = "auto"
			}
			content, toolCalls, err = a.callLLMStreamInternal(ctx, req, streamChan, state.SessionID)
		}

		if err != nil {
			a.logger.WithError(err).Warn("LLM call failed, using fallback")
			return a.getMockResponse(state, streamChan), nil
		}

		if len(toolCalls) == 0 {
			if toolsInRequest && iter == 0 {
				// No tool matched — stream the decline content from the blocking call.
				streamChan <- streaming.StreamEvent{Type: streaming.TextEvent, Content: content}
			}
			finalContent.WriteString(content)
			break
		}

		// Append assistant's tool_call turn to messages
		tcs := make([]map[string]interface{}, len(toolCalls))
		for i, tc := range toolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			tcs[i] = map[string]interface{}{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Function.Name,
					"arguments": string(argsJSON),
				},
			}
		}
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    "",
			"tool_calls": tcs,
		})

		// Execute each tool and collect results
		done := false
		for _, tc := range toolCalls {
			// 中间步骤：执行前通知前端
			streamChan <- streaming.StreamEvent{Type: streaming.StepEvent, Content: stepStartMsg(tc.Function.Name)}

			execResp, execErr := a.executeToolOnServer(ctx, tc.Function.Name, state.UserID, tc.Function.Arguments)

			var resultContent string
			if execErr != nil {
				resultContent = fmt.Sprintf("error: %s", execErr.Error())
			} else if !execResp.Success {
				resultContent = fmt.Sprintf("error: %s", execResp.Error)
			} else {
				switch execResp.ActionType {
				case "return_direct":
					// skill-server pre-rendered the reply; stream it and stop
					msg := execResp.Message
					if msg == "" {
						b, _ := json.Marshal(execResp.Result)
						msg = string(b)
					}
					streamChan <- streaming.StreamEvent{Type: streaming.TextEvent, Content: msg}
					finalContent.WriteString(msg)
					done = true

				case "next_step":
					// Chain next_tool result together for LLM summary
					b, _ := json.Marshal(execResp.Result)
					resultContent = string(b)
					if execResp.NextTool != "" {
						streamChan <- streaming.StreamEvent{Type: streaming.StepEvent, Content: stepStartMsg(execResp.NextTool)}
						nextResp, nextErr := a.executeToolOnServer(ctx, execResp.NextTool, state.UserID, map[string]interface{}{})
						if nextErr == nil && nextResp != nil && nextResp.Success {
							streamChan <- streaming.StreamEvent{Type: streaming.StepEvent, Content: stepDoneMsg(execResp.NextTool, nextResp.Result)}
							nb, _ := json.Marshal(nextResp.Result)
							resultContent += "\n[" + execResp.NextTool + "]: " + string(nb)
						}
					}
					streamChan <- streaming.StreamEvent{Type: streaming.StepEvent, Content: stepDoneMsg(tc.Function.Name, execResp.Result)}

				default: // llm_summary — feed raw result back to LLM
					b, _ := json.Marshal(execResp.Result)
					resultContent = string(b)
					streamChan <- streaming.StreamEvent{Type: streaming.StepEvent, Content: stepDoneMsg(tc.Function.Name, execResp.Result)}
				}
			}

			if !done {
				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc.ID,
					"content":      resultContent,
				})
			}
		}

		if done {
			break
		}
	}

	return &Message{
		Role:      "assistant",
		Content:   finalContent.String(),
		Timestamp: time.Now(),
	}, nil
}

// ── skill-server HTTP client ──────────────────────────────────────────────────

type skillServerExecuteResp struct {
	Success    bool                   `json:"success"`
	Result     map[string]interface{} `json:"result,omitempty"`
	ActionType string                 `json:"action_type"`
	Message    string                 `json:"message,omitempty"`
	NextTool   string                 `json:"next_tool,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

func (a *Agent) getSkillsFromServer(ctx context.Context, pageContext string) ([]map[string]interface{}, error) {
	log := logger.FromContext(ctx)
	url := a.skillServerURL + "/skills"
	if pageContext != "" {
		url += "?page_context=" + pageContext
	}

	log.Infof("[skill-server] GET %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if traceID := logger.IDFromContext(ctx); traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Errorf("[skill-server] GET %s error: %v", url, err)
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Tools []map[string]interface{} `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	toolNames := make([]string, 0, len(body.Tools))
	for _, t := range body.Tools {
		if fn, ok := t["function"].(map[string]interface{}); ok {
			toolNames = append(toolNames, fmt.Sprintf("%v", fn["name"]))
		}
	}
	log.Infof("[skill-server] GET %s → %d tools: %v", url, len(body.Tools), toolNames)

	return body.Tools, nil
}

func (a *Agent) executeToolOnServer(ctx context.Context, toolName, userID string, args map[string]interface{}) (*skillServerExecuteResp, error) {
	log := logger.FromContext(ctx)
	payload, err := json.Marshal(map[string]interface{}{
		"tool_name": toolName,
		"user_id":   userID,
		"args":      args,
	})
	if err != nil {
		return nil, err
	}

	log.Infof("[skill-server] POST /execute tool=%s user=%s args=%s", toolName, userID, string(payload))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.skillServerURL+"/execute", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if traceID := logger.IDFromContext(ctx); traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Errorf("[skill-server] POST /execute tool=%s error: %v", toolName, err)
		return nil, err
	}
	defer resp.Body.Close()

	var result skillServerExecuteResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	resultJSON, _ := json.Marshal(result)
	log.Infof("[skill-server] POST /execute tool=%s → success=%v action=%s result=%s",
		toolName, result.Success, result.ActionType, string(resultJSON))

	return &result, nil
}

// ── LLM helpers ───────────────────────────────────────────────────────────────

// buildSystemPrompt builds the system prompt with memory context.
// Tool descriptions are NOT included here — they're passed via the LLM `tools` field.
func (a *Agent) buildSystemPrompt(state *AgentState) string {
	var sb strings.Builder
	sb.WriteString("You are an AI assistant for 咔皮记账 (KaPi Accounting). Help users manage their finances.\n\n")
	sb.WriteString("Today's date: " + time.Now().Format("2006-01-02") + "\n\n")

	if state.Memory.Profile != "" {
		sb.WriteString("[用户画像]\n" + state.Memory.Profile + "\n\n")
	}
	if state.Memory.Preferences != "" {
		sb.WriteString("[偏好]\n" + state.Memory.Preferences + "\n\n")
	}
	if len(state.Memory.Facts) > 0 {
		sb.WriteString("[关键事实]\n")
		for _, fact := range state.Memory.Facts {
			sb.WriteString("- " + fact + "\n")
		}
		sb.WriteString("\n")
	}
	if state.Memory.RecentSummary != "" {
		sb.WriteString("[近期对话摘要]\n" + state.Memory.RecentSummary + "\n\n")
	}
	sb.WriteString("Current Page: " + state.PageContext + "\n")
	return sb.String()
}

// buildLLMRequest constructs the base LLM API request body
func (a *Agent) buildLLMRequest(messages []map[string]interface{}, stream bool) map[string]interface{} {
	return map[string]interface{}{
		"model":       a.config.ModelName,
		"messages":    messages,
		"max_tokens":  a.config.MaxTokens,
		"temperature": a.config.Temperature,
		"stream":      stream,
	}
}

// callLLMStreamInternal sends a streaming request to the LLM, forwards text chunks to
// streamChan, and returns the accumulated full content and any tool calls.
func (a *Agent) callLLMStreamInternal(ctx context.Context, req map[string]interface{}, streamChan chan<- streaming.StreamEvent, sessionID string) (string, []ToolCall, error) {
	log := logger.FromContext(ctx)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}
	msgCount := 0
	if msgs, ok := req["messages"].([]map[string]interface{}); ok {
		msgCount = len(msgs)
	}
	toolCount := 0
	if tools, ok := req["tools"].([]map[string]interface{}); ok {
		toolCount = len(tools)
	}
	log.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"model":         req["model"],
		"endpoint":      a.config.LLMEndpoint,
		"message_count": msgCount,
		"tools_count":   toolCount,
	}).Info("[llm] sending request")
	log.WithField("session_id", sessionID).Debugf("[llm] request body: %s", string(reqBody))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	log.WithFields(logrus.Fields{
		"session_id": sessionID,
		"status":     resp.StatusCode,
	}).Info("[llm] response received")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	// tcAccum accumulates streaming tool_call fragments before final parse
	type tcAccum struct {
		ID          string
		FuncName    string
		ArgsBuilder strings.Builder
	}

	type StreamChoice struct {
		Delta struct {
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
	type StreamResponse struct {
		Choices []StreamChoice `json:"choices"`
		Usage   struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage,omitempty"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullContent strings.Builder
	tcMap := make(map[int]*tcAccum)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		log.WithField("session_id", sessionID).Debugf("[llm] stream chunk: %s", data)

		var chunk StreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return "", nil, fmt.Errorf("LLM API error: %s", chunk.Error.Message)
		}
		if chunk.Usage.TotalTokens > 0 {
			streamChan <- streaming.StreamEvent{Type: streaming.TextEvent, TokenUsage: chunk.Usage.TotalTokens}
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			streamChan <- streaming.StreamEvent{Type: streaming.TextEvent, Content: delta.Content}
		}

		// Accumulate raw argument strings; parse once after [DONE]
		for _, tc := range delta.ToolCalls {
			acc, ok := tcMap[tc.Index]
			if !ok {
				acc = &tcAccum{}
				tcMap[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.FuncName = tc.Function.Name
			}
			acc.ArgsBuilder.WriteString(tc.Function.Arguments)
		}
	}

	// Assemble ToolCall slice in index order
	toolCalls := make([]ToolCall, 0, len(tcMap))
	for i := 0; i < len(tcMap); i++ {
		acc, ok := tcMap[i]
		if !ok {
			continue
		}
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(acc.ArgsBuilder.String()), &args); err != nil {
			args = make(map[string]interface{})
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:   acc.ID,
			Type: "function",
			Function: ToolFunction{
				Name:      acc.FuncName,
				Arguments: args,
			},
		})
	}

	log.WithFields(logrus.Fields{
		"session_id":       sessionID,
		"content_length":   fullContent.Len(),
		"tool_calls_count": len(toolCalls),
		"content":          fullContent.String(),
	}).Info("[llm] stream complete")

	return fullContent.String(), toolCalls, nil
}

// ── Step event helpers ────────────────────────────────────────────────────────

// buildToolCapabilitySummary formats tool names and descriptions into a concise
// list for inclusion in the system prompt constraint block.
func buildToolCapabilitySummary(tools []map[string]interface{}) string {
	var sb strings.Builder
	for _, t := range tools {
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		sb.WriteString("- ")
		sb.WriteString(name)
		if desc != "" {
			sb.WriteString("：")
			sb.WriteString(desc)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func stepStartMsg(toolName string) string {
	switch toolName {
	case "add_bill":
		return "正在记录账单..."
	case "query_bills":
		return "正在查询账单..."
	case "query_budget":
		return "正在查询预算..."
	case "update_budget":
		return "正在更新预算..."
	default:
		return fmt.Sprintf("正在执行 %s...", toolName)
	}
}

func stepDoneMsg(toolName string, result map[string]interface{}) string {
	switch toolName {
	case "query_bills":
		if count, ok := result["count"].(float64); ok {
			return fmt.Sprintf("查询到 %d 条账单", int(count))
		}
		return "账单查询完成"
	case "query_budget":
		if budgets, ok := result["budgets"].([]interface{}); ok {
			return fmt.Sprintf("已获取 %d 个分类预算数据", len(budgets))
		}
		return "预算查询完成"
	case "update_budget":
		cat, _ := result["category"].(string)
		if amt, ok := result["new_amount"].(float64); ok && cat != "" {
			return fmt.Sprintf("%s 预算已更新为 %.0f 元", cat, amt)
		}
		return "预算更新完成"
	default:
		return "执行完成"
	}
}

// ── Fallback & persistence ────────────────────────────────────────────────────

// callLLMBlocking sends a non-streaming request to the LLM and returns the full
// response at once. Used for the tool-selection phase so we can inspect tool_calls
// before deciding whether to stream a decline or proceed with tool execution.
func (a *Agent) callLLMBlocking(ctx context.Context, req map[string]interface{}, sessionID string) (string, []ToolCall, error) {
	log := logger.FromContext(ctx)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}

	msgCount := 0
	if msgs, ok := req["messages"].([]map[string]interface{}); ok {
		msgCount = len(msgs)
	}
	toolCount := 0
	if tools, ok := req["tools"].([]map[string]interface{}); ok {
		toolCount = len(tools)
	}
	log.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"model":         req["model"],
		"message_count": msgCount,
		"tools_count":   toolCount,
	}).Info("[llm] blocking request")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.LLMEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.LLMApiKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}
	if result.Error != nil {
		return "", nil, fmt.Errorf("LLM API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("LLM returned no choices")
	}

	msg := result.Choices[0].Message
	toolCalls := make([]ToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = make(map[string]interface{})
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: ToolFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}

	log.WithFields(logrus.Fields{
		"session_id":       sessionID,
		"tool_calls_count": len(toolCalls),
	}).Info("[llm] blocking response")
	log.WithField("session_id", sessionID).Debugf("[llm] blocking body: %s", string(body))

	return msg.Content, toolCalls, nil
}

// getMockResponse returns a keyword-matched reply when the LLM is unreachable
func (a *Agent) getMockResponse(state *AgentState, streamChan chan<- streaming.StreamEvent) *Message {
	userMessage := ""
	for i := len(state.MessageHistory) - 1; i >= 0; i-- {
		if state.MessageHistory[i].Role == "user" {
			userMessage = state.MessageHistory[i].Content
			break
		}
	}

	content := "抱歉，我现在无法连接到 AI 服务。请稍后再试。"
	switch {
	case strings.Contains(userMessage, "记账") || strings.Contains(userMessage, "花费") || strings.Contains(userMessage, "支出"):
		content = "我帮您记录这笔支出。请问您想记录什么金额？"
	case strings.Contains(userMessage, "查询") || strings.Contains(userMessage, "账单") || strings.Contains(userMessage, "记录"):
		content = "我可以帮您查询账单记录。请告诉我您想查询的时间范围。"
	case strings.Contains(userMessage, "预算") || strings.Contains(userMessage, "理财") || strings.Contains(userMessage, "存钱"):
		content = "关于预算建议，建议您按照 50/30/20 法则。您想了解哪方面的预算建议？"
	case strings.Contains(userMessage, "你好") || strings.Contains(userMessage, "hello"):
		content = "您好！我是您的财务助手，有什么可以帮您？"
	}

	for _, ch := range []rune(content) {
		if streamChan != nil {
			streamChan <- streaming.StreamEvent{Type: streaming.TextEvent, Content: string(ch)}
		}
	}

	return &Message{Role: "assistant", Content: content, Timestamp: time.Now()}
}

// saveSession persists the current turn to the session store
func (a *Agent) saveSession(state *AgentState, response *Message) {
	sess, err := a.sessionManager.GetSession(state.SessionID)
	if err != nil {
		sessionID, err := a.sessionManager.GetOrCreateSession(state.UserID)
		if err != nil {
			a.logger.WithError(err).Warn("Failed to create session")
			return
		}
		state.SessionID = sessionID
		sess, _ = a.sessionManager.GetSession(sessionID)
	}

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
