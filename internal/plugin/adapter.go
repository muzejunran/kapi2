package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"ai-assistant-service/internal/llm"
	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/skill"
)

// LLMServiceAdapter 适配 internal/llm 到 plugin.LLMService
type LLMServiceAdapter struct {
	client *llm.Client
}

func NewLLMServiceAdapter(client *llm.Client) *LLMServiceAdapter {
	return &LLMServiceAdapter{client: client}
}

func (a *LLMServiceAdapter) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	llmMessages := make([]llm.Message, len(messages))
	for i, m := range messages {
		llmMessages[i] = llm.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	resp, err := a.client.Chat(llmMessages, "")
	if err != nil {
		return nil, err
	}

	// 从 LLMResponse 中提取内容
	if len(resp.Choices) > 0 {
		return &ChatResponse{
			Content: resp.Choices[0].Message.Content,
			Usage: Usage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			},
		}, nil
	}

	return &ChatResponse{
		Content: "",
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

func (a *LLMServiceAdapter) ChatStream(ctx context.Context, messages []Message) (<-chan ChatChunk, error) {
	// 当前 llm.Client 不支持流式，返回错误
	return nil, fmt.Errorf("streaming not supported by current LLM client")
}

// BillRepoAdapter 适配 internal/repository 到 plugin.BillRepo
type BillRepoAdapter struct {
	repo *repository.BillRepository
}

func NewBillRepoAdapter(repo *repository.BillRepository) *BillRepoAdapter {
	return &BillRepoAdapter{repo: repo}
}

func (a *BillRepoAdapter) AddBill(bill *Bill) error {
	if a.repo == nil {
		return fmt.Errorf("bill repository not initialized")
	}

	return a.repo.AddBill(&model.Bill{
		ID:          bill.ID,
		UserID:      bill.UserID,
		Amount:      bill.Amount,
		Category:    bill.Category,
		Description: bill.Description,
		Timestamp:   bill.Timestamp,
	})
}

func (a *BillRepoAdapter) QueryBills(filter *BillFilter) ([]*Bill, *BillSummary, error) {
	if a.repo == nil {
		return nil, nil, fmt.Errorf("bill repository not initialized")
	}

	bills, summary, err := a.repo.QueryBills(&model.BillFilter{
		UserID:    filter.UserID,
		Category:  filter.Category,
		StartDate: filter.StartDate,
		EndDate:   filter.EndDate,
	})

	if err != nil {
		return nil, nil, err
	}

	// 转换类型
	pluginBills := make([]*Bill, len(bills))
	for i, b := range bills {
		pluginBills[i] = &Bill{
			ID:          b.ID,
			UserID:      b.UserID,
			Amount:      b.Amount,
			Category:    b.Category,
			Description: b.Description,
			Timestamp:   b.Timestamp,
		}
	}

	pluginSummary := &BillSummary{
		TotalAmount: summary.TotalAmount,
		ByCategory:  summary.ByCategory,
		TotalCount:  summary.TotalCount,
	}

	return pluginBills, pluginSummary, nil
}

// ========== PluginSkillAdapter 插件适配器 ==========

// PluginSkillAdapter 将 plugin.Skill 适配为 skill.Skill
// 这样插件可以被现有的 Agent 和 SkillRegistry 使用
type PluginSkillAdapter struct {
	plugin Skill
}

// NewPluginSkillAdapter 创建插件适配器
func NewPluginSkillAdapter(pluginSkill Skill) *PluginSkillAdapter {
	return &PluginSkillAdapter{plugin: pluginSkill}
}

func (a *PluginSkillAdapter) GetID() string {
	return a.plugin.GetID()
}

func (a *PluginSkillAdapter) GetName() string {
	return a.plugin.GetName()
}

func (a *PluginSkillAdapter) GetDescription() string {
	return a.plugin.GetDescription()
}

func (a *PluginSkillAdapter) CanHandle(ctx skill.AgentContext) float64 {
	// 转换上下文
	pluginCtx := AgentContext{
		UserID:      ctx.UserID,
		PageContext: ctx.PageContext,
		Message:     ctx.Message,
		State:       ctx.State,
	}
	return a.plugin.CanHandle(pluginCtx)
}

func (a *PluginSkillAdapter) GetExtractionSchema(ctx skill.AgentContext) string {
	// 转换上下文
	pluginCtx := AgentContext{
		UserID:      ctx.UserID,
		PageContext: ctx.PageContext,
		Message:     ctx.Message,
		State:       ctx.State,
	}
	return a.plugin.GetExtractionSchema(pluginCtx)
}

// Initialize 初始化技能（插件已通过 NewSkill 初始化，这里无需操作）
func (a *PluginSkillAdapter) Initialize(config map[string]interface{}) error {
	return nil
}

func (a *PluginSkillAdapter) GetTools() []skill.Tool {
	// 转换 tool 类型
	pluginTools := a.plugin.GetTools()
	tools := make([]skill.Tool, len(pluginTools))

	for i, pt := range pluginTools {
		paramsJSON, _ := json.Marshal(pt.Function.Parameters)
		tools[i] = skill.Tool{
			ID:          pt.Function.Name,
			Name:        pt.Function.Name,
			Description: pt.Function.Description,
			Parameters:  json.RawMessage(paramsJSON),
			Handler:     nil, // 插件不需要 Handler，直接调用 Execute
		}
	}
	return tools
}

func (a *PluginSkillAdapter) Execute(ctx context.Context, request skill.SkillRequest) (*skill.SkillResponse, error) {
	// 确定要调用哪个工具
	toolName := a.detectToolName()

	// 调用插件
	result, err := a.plugin.Execute(ctx, toolName, request.Args, map[string]interface{}{
		"user_id": request.Context.UserID,
		"page":    request.Context.PageContext,
	})

	if err != nil {
		return &skill.SkillResponse{
			Success: false,
			Message: fmt.Sprintf("执行失败: %v", err),
		}, nil
	}

	// 转换响应
	return &skill.SkillResponse{
		Success: getBoolFromMap(result, "success"),
		Message: getStringFromMap(result, "message"),
		Data:    result["data"],
	}, nil
}

func (a *PluginSkillAdapter) Cleanup() error {
	return a.plugin.Cleanup()
}

// detectToolName 检测要调用的工具名
// 对于只有一个工具的插件，直接返回第一个工具名
func (a *PluginSkillAdapter) detectToolName() string {
	tools := a.plugin.GetTools()
	if len(tools) > 0 {
		return tools[0].Function.Name
	}
	return ""
}

// ========== 辅助函数 ==========

func getBoolFromMap(params map[string]interface{}, key string) bool {
	if val, ok := params[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func getStringFromMap(params map[string]interface{}, key string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
