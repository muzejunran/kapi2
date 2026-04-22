package main

import (
	"context"
	"fmt"
	"time"

	"ai-assistant-service/internal/plugin"
)

// BudgetAdvisorSkill 预算建议技能插件
type BudgetAdvisorSkill struct {
	deps *plugin.Dependencies
}

// NewSkill 插件导出的函数，必须存在
func NewSkill(deps *plugin.Dependencies) (plugin.Skill, error) {
	return &BudgetAdvisorSkill{deps: deps}, nil
}

func (s *BudgetAdvisorSkill) GetID() string {
	return "budget_advisor"
}

func (s *BudgetAdvisorSkill) GetName() string {
	return "预算建议"
}

func (s *BudgetAdvisorSkill) GetDescription() string {
	return "根据收入和支出提供预算建议"
}

func (s *BudgetAdvisorSkill) CanHandle(ctx plugin.AgentContext) float64 {
	// 只检查 page_context 是否匹配
	if ctx.PageContext == "budget" {
		return 1.0
	}
	return 0
}

func (s *BudgetAdvisorSkill) GetExtractionSchema(ctx plugin.AgentContext) string {
	return `{
		"type": "object",
		"properties": {
			"income": {
				"type": "number",
				"minimum": 0,
				"description": "月收入（元）"
			},
			"expenses": {
				"type": "number",
				"minimum": 0,
				"description": "月支出（元），可选"
			}
		},
		"required": ["income"],
		"additionalProperties": false
	}`
}

func (s *BudgetAdvisorSkill) GetTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Type: "function",
			Function: plugin.FunctionDef{
				Name:        "budget_advisor",
				Description: "根据收入和支出提供预算建议",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"income": map[string]interface{}{
							"type":        "number",
							"minimum":     0,
							"description": "月收入（元）",
						},
						"expenses": map[string]interface{}{
							"type":        "number",
							"minimum":     0,
							"description": "月支出（元），可选",
						},
					},
					"required":               []string{"income"},
					"additionalProperties": false,
				},
			},
		},
	}
}

func (s *BudgetAdvisorSkill) Execute(ctx context.Context, toolName string, params map[string]interface{}, contextData map[string]interface{}) (map[string]interface{}, error) {
	if toolName != "budget_advisor" {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	income, ok := params["income"].(float64)
	if !ok || income == 0 {
		return map[string]interface{}{
			"success": false,
			"message": "请提供收入金额",
		}, nil
	}

	expenses := 0.0
	if exp, ok := params["expenses"].(float64); ok {
		expenses = exp
	}

	userID, _ := contextData["user_id"].(string)
	if userID == "" {
		userID = "default_user"
	}

	var avgMonthlySpent float64 = 0
	var savingsRate float64 = 0

	if s.deps.BillRepo != nil {
		now := time.Now()
		lastMonth := now.AddDate(0, -1, 0)
		filter := &plugin.BillFilter{
			UserID:    userID,
			StartDate: lastMonth.Format("2006-01") + "-01",
			EndDate:   now.Format("2006-01-02"),
		}

		_, summary, err := s.deps.BillRepo.QueryBills(filter)
		if err == nil && summary != nil {
			avgMonthlySpent = summary.TotalAmount
		}
	}

	if expenses > 0 {
		avgMonthlySpent = expenses
	}

	if income > 0 {
		savingsRate = ((income - avgMonthlySpent) / income) * 100
	}

	var advice string
	savings := income - avgMonthlySpent

	if savings < 0 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，超过收入 %.2f 元，建议控制支出。", avgMonthlySpent, -savings)
	} else if savingsRate < 10 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%。", avgMonthlySpent, savingsRate)
	} else if savingsRate < 30 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%。", avgMonthlySpent, savingsRate)
	} else {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%。", avgMonthlySpent, savingsRate)
	}

	return map[string]interface{}{
		"success": true,
		"message": advice,
		"data": map[string]interface{}{
			"income":        income,
			"avg_spent":     avgMonthlySpent,
			"savings":       savings,
			"savings_rate":  savingsRate,
		},
	}, nil
}

func (s *BudgetAdvisorSkill) Cleanup() error {
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
