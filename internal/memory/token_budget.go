package memory

// TokenCategory 表示 token 使用类别
type TokenCategory string

const (
	TokenCategorySystemPrompt TokenCategory = "system_prompt"  // 系统提示词
	TokenCategoryUserInput   TokenCategory = "user_input"     // 用户输入
	TokenCategoryLLMOutput    TokenCategory = "llm_output"     // LLM 输出
	TokenCategoryToolRequest TokenCategory = "tool_request"   // Skill 调用请求
	TokenCategoryToolResponse TokenCategory = "tool_response"  // Skill 调用响应
	TokenCategoryAuxiliary    TokenCategory = "auxiliary"       // 辅助信息（loading 等）
)

// TokenUsage 表示一次 token 使用记录
type TokenUsage struct {
	Category    TokenCategory `json:"category"`
	TokenCount  int          `json:"token_count"`
	Description string        `json:"description,omitempty"`
	Timestamp   string        `json:"timestamp"`
}

// TokenBudget 跟踪 token 使用和预算
type TokenBudget struct {
	dailyLimit     int                    `json:"daily_limit"`
	currentDaily   int                    `json:"current_daily"`
	totalUsed      int                    `json:"total_used"`
	resetTime      string                 `json:"reset_time"`
}

// CalculateTokens 估算文本的 token 数量
func CalculateTokens(text string) int {
	// 简单估算：中文约1.5 token/字符，英文约0.3 token/字符
	charCount := len(text)
	var tokensPerChar float64

	for _, r := range text {
		if r >= 0x4e00 { // 中日韩字符
			tokensPerChar = 1.5
		} else { // 拉丁字符
			tokensPerChar = 0.3
		}
	}

	return int(float64(charCount) * tokensPerChar)
}
