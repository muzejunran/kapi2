package skillserver

// ActionType 工具执行完成后 Agent 的后续动作
type ActionType string

const (
	// ActionReturnDirect 直接把渲染好的话术返回给用户，终止本轮循环
	// 适用场景：操作类工具，结果确定，无需 LLM 再加工（如 add_bill）
	ActionReturnDirect ActionType = "return_direct"

	// ActionLLMSummary 把工具原始结果回灌给 LLM，由 LLM 生成自然语言回复
	// 适用场景：查询类工具，结果需要 LLM 总结解读（如 query_bills、update_budget）
	ActionLLMSummary ActionType = "llm_summary"

	// ActionNextStep 执行完当前工具后，强制串行执行下一个指定工具
	// 适用场景：多步编排，工具间有依赖（如先 check_balance 再 budget_advisor）
	ActionNextStep ActionType = "next_step"
)

// SkillConfig 从 JSON 加载的 Skill 定义
type SkillConfig struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Layer          string     `json:"layer"`            // built-in | org | user
	SupportedPages []string   `json:"supported_pages"` // 空 = 全局可用
	Tools          []ToolSpec `json:"tools"`
}

// ToolSpec 工具定义（含后续动作声明）
type ToolSpec struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Parameters     map[string]interface{} `json:"parameters"`
	ActionType     ActionType             `json:"action_type"`
	ReturnTemplate string                 `json:"return_template,omitempty"` // ActionReturnDirect 时的话术模板
	NextTool       string                 `json:"next_tool,omitempty"`       // ActionNextStep 时的下一个工具名
}

// ExecuteRequest POST /execute 请求
type ExecuteRequest struct {
	ToolName string                 `json:"tool_name"`
	UserID   string                 `json:"user_id"`
	Args     map[string]interface{} `json:"args"`
}

// ExecuteResponse POST /execute 响应
type ExecuteResponse struct {
	Success    bool                   `json:"success"`
	Result     map[string]interface{} `json:"result,omitempty"`
	ActionType ActionType             `json:"action_type"`
	Message    string                 `json:"message,omitempty"` // ActionReturnDirect 时已渲染的话术
	NextTool   string                 `json:"next_tool,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// GetSkillsResponse GET /skills 响应（OpenAI Function Calling 格式）
type GetSkillsResponse struct {
	Tools []OpenAITool `json:"tools"`
}

// OpenAITool OpenAI Function Calling 格式的工具描述
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction 工具函数描述
type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
