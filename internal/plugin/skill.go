package plugin

import (
	"context"
)

// Skill 插件必须实现的接口
type Skill interface {
	// GetID 返回技能ID
	GetID() string

	// GetName 返回技能名称
	GetName() string

	// GetDescription 返回技能描述
	GetDescription() string

	// CanHandle 判断是否能处理当前上下文
	// 返回 0-1，1 表示完全匹配
	CanHandle(ctx AgentContext) float64

	// GetTools 返回工具定义（OpenAI Function Calling 格式）
	GetTools() []ToolDef

	// GetExtractionSchema 返回参数提取的 JSON Schema
	// Agent 会调用 LLM 根据 Schema 提取用户输入中的参数
	GetExtractionSchema(ctx AgentContext) string

	// Execute 执行工具调用
	// toolName: 要执行的工具名
	// params: LLM 提取的参数（已经结构化）
	// contextData: 上下文数据（如 userID, pageContext 等）
	Execute(ctx context.Context, toolName string, params map[string]interface{}, contextData map[string]interface{}) (map[string]interface{}, error)

	// Cleanup 清理资源
	Cleanup() error
}

// ToolDef 工具定义
type ToolDef struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// AgentContext 代理上下文
type AgentContext struct {
	UserID      string                 `json:"user_id"`
	PageContext string                 `json:"page_context"`
	Message     string                 `json:"message"`
	State       map[string]interface{} `json:"state"`
}

// Dependencies 插件依赖（通过 NewSkill 注入）
type Dependencies struct {
	LLMService  LLMService  // LLM 服务接口
	BillRepo    BillRepo    // 账单仓储接口
	// 未来可以添加更多依赖
}

// LLMService LLM 服务接口
type LLMService interface {
	Chat(ctx context.Context, messages []Message) (*ChatResponse, error)
	ChatStream(ctx context.Context, messages []Message) (<-chan ChatChunk, error)
}

// Message 聊天消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

// Usage Token 使用情况
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatChunk 流式响应块
type ChatChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   error  `json:"error,omitempty"`
}

// Bill 账单
type Bill struct {
	ID          string  `json:"id"`
	UserID      string  `json:"user_id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Timestamp   string  `json:"timestamp"`
}

// BillFilter 账单筛选
type BillFilter struct {
	UserID    string `json:"user_id"`
	Category  string `json:"category"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// BillSummary 账单汇总
type BillSummary struct {
	TotalAmount float64            `json:"total_amount"`
	ByCategory  map[string]float64 `json:"by_category"`
	TotalCount  int                `json:"total_count"`
}

// BillRepo 账单仓储接口
type BillRepo interface {
	AddBill(bill *Bill) error
	QueryBills(filter *BillFilter) ([]*Bill, *BillSummary, error)
}

// NewSkill 插件导出的函数签名
// 必须有这个导出函数，主程序才能调用
type NewSkillFunc func(deps *Dependencies) (Skill, error)
