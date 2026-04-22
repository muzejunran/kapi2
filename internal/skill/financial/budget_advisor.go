package financial

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/skill"
)

// BudgetAdvisorSkill 预算建议技能
type BudgetAdvisorSkill struct {
	*skill.BaseSkill
	billRepo *repository.BillRepository
	config   *skill.NewSkillConfig
}

// NewBudgetAdvisorSkill 创建预算建议技能
func NewBudgetAdvisorSkill(billRepo *repository.BillRepository) (*BudgetAdvisorSkill, error) {
	// 加载配置
	configPath := "skills/financial/budget_advisor.json"
	config, err := skill.LoadSkillConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	base := skill.NewBaseSkill(config.ID, config.Name, config.Description)

	s := &BudgetAdvisorSkill{
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
func (s *BudgetAdvisorSkill) Initialize(config map[string]interface{}) error {
	return nil
}

// CanHandle 判断是否能处理当前上下文
func (s *BudgetAdvisorSkill) CanHandle(ctx skill.AgentContext) float64 {
	// 检查是否在支持的页面
	for _, page := range s.config.SupportedPages {
		if ctx.PageContext == page {
			return 1.0
		}
	}

	// 检查消息中是否包含预算建议关键词
	keywords := []string{"预算", "建议", "理财", "存钱", "省钱"}
	for _, word := range keywords {
		if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(word)).MatchString(ctx.Message) {
			return 0.8
		}
	}

	return 0
}

// GetExtractionSchema 返回参数提取的 Schema
func (s *BudgetAdvisorSkill) GetExtractionSchema(ctx skill.AgentContext) string {
	schema := `{
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
	return schema
}

// Execute 执行技能
func (s *BudgetAdvisorSkill) Execute(ctx context.Context, request skill.SkillRequest) (*skill.SkillResponse, error) {
	// 使用 LLM 提取的参数
	params := request.Args
	if params == nil {
		// Fallback: 空参数
		params = make(map[string]interface{})
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

// execute 执行预算建议操作
func (s *BudgetAdvisorSkill) execute(args map[string]interface{}) (map[string]interface{}, error) {
	// 提取参数
	income := getFloatFromMap(args, "income")
	if income == 0 {
		return map[string]interface{}{
			"success": false,
			"message": "请提供收入金额",
		}, nil
	}

	expenses := getFloatFromMap(args, "expenses")

	// 如果有账单仓库，获取历史数据
	var avgMonthlySpent float64 = 0
	var savingsRate float64 = 0

	if s.billRepo != nil {
		// 获取上个月的账单数据
		now := time.Now()
		lastMonth := now.AddDate(0, -1, 0)
		filter := &model.BillFilter{
			UserID:    "default_user",
			StartDate: lastMonth.Format("2006-01") + "-01",
			EndDate:   now.Format("2006-01-02"),
		}

		_, summary, err := s.billRepo.QueryBills(filter)
		if err == nil && summary != nil {
			avgMonthlySpent = summary.TotalAmount
		}
	}

	// 如果用户提供了支出，使用用户提供的
	if expenses > 0 {
		avgMonthlySpent = expenses
	}

	// 计算储蓄率
	if income > 0 {
		savingsRate = ((income - avgMonthlySpent) / income) * 100
	}

	// 生成建议
	var advice string
	savings := income - avgMonthlySpent

	if savings < 0 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，超过收入 %.2f 元，建议控制支出。可以考虑从餐饮、娱乐等非必要消费开始减少。",
			avgMonthlySpent, -savings)
	} else if savingsRate < 10 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%。建议将储蓄率提高到 20%%，可以通过减少不必要的开支来实现。",
			avgMonthlySpent, savingsRate)
	} else if savingsRate < 30 {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%，继续保持！建议将储蓄资金用于理财或建立紧急备用金。",
			avgMonthlySpent, savingsRate)
	} else {
		advice = fmt.Sprintf("您上月支出 %.2f 元，储蓄率 %.1f%%，非常棒！您已经建立了良好的储蓄习惯，可以考虑更多元化的理财方式。",
			avgMonthlySpent, savingsRate)
	}

	// 分类建议（简化版）
	categoryAdvice := map[string]string{
		"餐饮":   "建议控制在收入的 15% 以内",
		"购物":   "建议控制在收入的 10% 以内",
		"交通":   "建议控制在收入的 5% 以内",
		"娱乐":   "建议控制在收入的 10% 以内",
		"医疗":   "建议预留收入的 5-10% 作为医疗储备",
		"其他":   "建议控制在收入的 5% 以内",
	}

	return map[string]interface{}{
		"success": true,
		"message": advice,
		"data": map[string]interface{}{
			"income":       income,
			"avg_spent":    avgMonthlySpent,
			"savings":      savings,
			"savings_rate": savingsRate,
			"category_advice": categoryAdvice,
		},
	}, nil
}

// Cleanup 清理资源
func (s *BudgetAdvisorSkill) Cleanup() error {
	return nil
}
