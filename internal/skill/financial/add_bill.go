package financial

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/skill"
)

// AddBillSkill 记账技能
type AddBillSkill struct {
	*skill.BaseSkill
	billRepo *repository.BillRepository
	config   *skill.NewSkillConfig
}

// NewAddBillSkill 创建记账技能
func NewAddBillSkill(billRepo *repository.BillRepository) (*AddBillSkill, error) {
	// 加载配置
	configPath := "skills/financial/add_bill.json"
	config, err := skill.LoadSkillConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	base := skill.NewBaseSkill(config.ID, config.Name, config.Description)

	s := &AddBillSkill{
		BaseSkill: base,
		billRepo:  billRepo,
		config:    config,
	}

	// 注册工具
	for _, toolDef := range config.Tools {
		// 将 map[string]interface{} 转换为 json.RawMessage
		paramsJSON, _ := json.Marshal(toolDef.Function.Parameters)

		s.RegisterTool(skill.Tool{
			ID:          toolDef.Function.Name,
			Name:        toolDef.Function.Name,
			Description: toolDef.Function.Description,
			Parameters:  json.RawMessage(paramsJSON),
			Handler:     func(args map[string]interface{}) (interface{}, error) {
				result, err := s.execute(args)
				return result, err
			},
		})
	}

	return s, nil
}

// Initialize 初始化技能
func (s *AddBillSkill) Initialize(config map[string]interface{}) error {
	return nil
}

// CanHandle 判断是否能处理当前上下文
func (s *AddBillSkill) CanHandle(ctx skill.AgentContext) float64 {
	// 检查是否在支持的页面
	for _, page := range s.config.SupportedPages {
		if ctx.PageContext == page {
			return 1.0
		}
	}

	// 检查消息中是否包含记账关键词
	keywords := []string{"记账", "记", "花了", "支付", "消费", "账单"}
	for _, word := range keywords {
		if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(word)).MatchString(ctx.Message) {
			return 0.8
		}
	}

	return 0
}

// GetExtractionSchema 返回参数提取的 Schema
func (s *AddBillSkill) GetExtractionSchema(ctx skill.AgentContext) string {
	schema := `{
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
	return schema
}

// Execute 执行技能
func (s *AddBillSkill) Execute(ctx context.Context, request skill.SkillRequest) (*skill.SkillResponse, error) {
	// 使用 LLM 提取的参数
	params := request.Args
	if params == nil {
		// Fallback: 简单提取
		params = s.simpleExtractParams(request.Input)
	}

	// 执行操作
	result, err := s.execute(params)
	if err != nil {
		return &skill.SkillResponse{
			Success: false,
			Message: s.config.RuntimeConfig.ErrorMessage,
		}, nil
	}

	return &skill.SkillResponse{
		Success: result["success"].(bool),
		Message: result["message"].(string),
		Data:    result["data"],
	}, nil
}

// execute 执行记账操作
func (s *AddBillSkill) execute(args map[string]interface{}) (map[string]interface{}, error) {
	if s.billRepo == nil {
		return map[string]interface{}{
			"success": false,
			"message": "数据库未初始化",
		}, nil
	}

	// 提取参数
	amount := getFloatFromMap(args, "amount")
	if amount <= 0 {
		return map[string]interface{}{
			"success": false,
			"message": "请提供有效的金额",
		}, nil
	}

	category := getStringFromMap(args, "category")
	if category == "" {
		category = "其他"
	}

	description := getStringFromMap(args, "description")

	// 创建账单
	bill := &model.Bill{
		UserID:      "default_user",
		Amount:      amount,
		Category:    category,
		Description: description,
	}

	if err := s.billRepo.AddBill(bill); err != nil {
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

// simpleExtractParams 简单参数提取（fallback）
func (s *AddBillSkill) simpleExtractParams(message string) map[string]interface{} {
	re := regexp.MustCompile(`\d+(\.\d+)?`)
	match := re.FindString(message)
	if match == "" {
		return map[string]interface{}{
			"error": "未能提取金额信息",
		}
	}

	var amount float64
	fmt.Sscanf(match, "%f", &amount)

	return map[string]interface{}{
		"amount":   amount,
		"category": "其他",
	}
}

// Cleanup 清理资源
func (s *AddBillSkill) Cleanup() error {
	return nil
}
