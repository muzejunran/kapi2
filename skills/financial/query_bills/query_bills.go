package main

import (
	"context"
	"fmt"
	"time"

	"ai-assistant-service/internal/plugin"
)

// QueryBillsSkill 查询账单技能插件
type QueryBillsSkill struct {
	deps *plugin.Dependencies
}

// NewSkill 插件导出的函数，必须存在
func NewSkill(deps *plugin.Dependencies) (plugin.Skill, error) {
	return &QueryBillsSkill{deps: deps}, nil
}

func (s *QueryBillsSkill) GetID() string {
	return "query_bills"
}

func (s *QueryBillsSkill) GetName() string {
	return "查询账单"
}

func (s *QueryBillsSkill) GetDescription() string {
	return "查询指定时间段内的账单记录"
}

func (s *QueryBillsSkill) CanHandle(ctx plugin.AgentContext) float64 {
	// 只检查 page_context 是否匹配
	if ctx.PageContext == "query_bills" {
		return 1.0
	}
	return 0
}

func (s *QueryBillsSkill) GetExtractionSchema(ctx plugin.AgentContext) string {
	// 返回参数提取的 JSON Schema
	return `{
		"type": "object",
		"properties": {
			"start_date": {
				"type": "string",
				"format": "date",
				"pattern": "^\\d{4}-\\d{2}-\\d{2}$",
				"description": "开始日期，格式: YYYY-MM-DD"
			},
			"end_date": {
				"type": "string",
				"format": "date",
				"pattern": "^\\d{4}-\\d{2}-\\d{2}$",
				"description": "结束日期，格式: YYYY-MM-DD"
			},
			"category": {
				"type": "string",
				"description": "筛选分类（可选）"
			}
		},
		"required": [],
		"additionalProperties": false
	}`
}

func (s *QueryBillsSkill) GetTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Type: "function",
			Function: plugin.FunctionDef{
				Name:        "query_bills",
				Description: "查询指定时间段内的账单记录",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"start_date": map[string]interface{}{
							"type":        "string",
							"format":      "date",
							"pattern":     "^\\d{4}-\\d{2}-\\d{2}$",
							"description": "开始日期，格式: YYYY-MM-DD",
						},
						"end_date": map[string]interface{}{
							"type":        "string",
							"format":      "date",
							"pattern":     "^\\d{4}-\\d{2}-\\d{2}$",
							"description": "结束日期，格式: YYYY-MM-DD",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"description": "筛选分类（可选）",
						},
					},
					"required":             []string{},
					"additionalProperties": false,
				},
			},
		},
	}
}

func (s *QueryBillsSkill) Execute(ctx context.Context, toolName string, params map[string]interface{}, contextData map[string]interface{}) (map[string]interface{}, error) {
	if toolName != "query_bills" {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	// 提取参数
	startDate, _ := params["start_date"].(string)
	endDate, _ := params["end_date"].(string)
	category, _ := params["category"].(string)

	// 如果没有结束日期，默认为今天
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	// 获取用户 ID
	userID, _ := contextData["user_id"].(string)
	if userID == "" {
		userID = "default_user"
	}

	filter := &plugin.BillFilter{
		UserID:    userID,
		Category:  category,
		StartDate: startDate,
		EndDate:   endDate,
	}

	// 查询数据库
	bills, summary, err := s.deps.BillRepo.QueryBills(filter)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("查询账单失败: %v", err),
		}, nil
	}

	// 确保 bills 不为 nil
	if bills == nil {
		bills = []*plugin.Bill{}
	}

	return map[string]interface{}{
		"success": true,
		"message": "", // 留空，由 Agent 生成自然语言
		"data": map[string]interface{}{
			"total":   len(bills),
			"bills":   bills,
			"summary": summary,
			"query":   params,
		},
	}, nil
}

func (s *QueryBillsSkill) Cleanup() error {
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
