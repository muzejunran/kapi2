package main

import (
	"context"
	"fmt"
	"time"

	"ai-assistant-service/internal/plugin"
)

// AddBillSkill 记账技能插件
type AddBillSkill struct {
	deps *plugin.Dependencies
}

// NewSkill 插件导出的函数，必须存在
func NewSkill(deps *plugin.Dependencies) (plugin.Skill, error) {
	return &AddBillSkill{deps: deps}, nil
}

func (s *AddBillSkill) GetID() string {
	return "add_bill"
}

func (s *AddBillSkill) GetName() string {
	return "记账"
}

func (s *AddBillSkill) GetDescription() string {
	return "记录一笔新的支出或收入"
}

func (s *AddBillSkill) CanHandle(ctx plugin.AgentContext) float64 {
	// 只检查 page_context 是否匹配
	if ctx.PageContext == "add_bill" {
		return 1.0
	}
	return 0
}

func (s *AddBillSkill) GetExtractionSchema(ctx plugin.AgentContext) string {
	// 返回参数提取的 JSON Schema
	return `{
		"type": "object",
		"properties": {
			"amount": {
				"type": "number",
				"minimum": 0,
				"description": "金额"
			},
			"category": {
				"type": "string",
				"enum": ["餐饮", "购物", "交通", "娱乐", "医疗", "其他"],
				"description": "消费分类"
			},
			"description": {
				"type": "string",
				"maxLength": 200,
				"description": "消费描述（可选）"
			}
		},
		"required": ["amount", "category"],
		"additionalProperties": false
	}`
}

func (s *AddBillSkill) GetTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Type: "function",
			Function: plugin.FunctionDef{
				Name:        "add_bill",
				Description: "记录一笔新的支出或收入",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"amount": map[string]interface{}{
							"type":        "number",
							"minimum":     0,
							"description": "金额",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"餐饮", "购物", "交通", "娱乐", "医疗", "其他"},
							"description": "消费分类",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"maxLength":   200,
							"description": "消费描述（可选）",
						},
					},
					"required":               []string{"amount", "category"},
					"additionalProperties": false,
				},
			},
		},
	}
}

func (s *AddBillSkill) Execute(ctx context.Context, toolName string, params map[string]interface{}, contextData map[string]interface{}) (map[string]interface{}, error) {
	if toolName != "add_bill" {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	// 提取参数
	amount, ok := params["amount"].(float64)
	if !ok || amount <= 0 {
		return map[string]interface{}{
			"success": false,
			"message": "请提供有效的金额",
		}, nil
	}

	category, _ := params["category"].(string)
	if category == "" {
		category = "其他"
	}

	description, _ := params["description"].(string)

	// 获取用户 ID
	userID, _ := contextData["user_id"].(string)
	if userID == "" {
		userID = "default_user"
	}

	// 创建账单
	bill := &plugin.Bill{
		ID:          fmt.Sprintf("bill_%d", time.Now().UnixNano()),
		UserID:      userID,
		Amount:      amount,
		Category:    category,
		Description: description,
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
	}

	// 存入数据库
	if err := s.deps.BillRepo.AddBill(bill); err != nil {
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("添加账单失败: %v", err),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("已记录%s账单%.2f元", category, amount),
		"data":    bill,
	}, nil
}

func (s *AddBillSkill) Cleanup() error {
	return nil
}

// contains 检查字符串是否包含子串
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
