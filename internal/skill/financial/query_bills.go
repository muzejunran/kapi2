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

// QueryBillsSkill 查询账单技能
type QueryBillsSkill struct {
	*skill.BaseSkill
	billRepo *repository.BillRepository
	config   *skill.NewSkillConfig
}

// NewQueryBillsSkill 创建查询账单技能
func NewQueryBillsSkill(billRepo *repository.BillRepository) (*QueryBillsSkill, error) {
	// 加载配置
	configPath := "skills/financial/query_bills.json"
	config, err := skill.LoadSkillConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	base := skill.NewBaseSkill(config.ID, config.Name, config.Description)

	s := &QueryBillsSkill{
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
func (s *QueryBillsSkill) Initialize(config map[string]interface{}) error {
	return nil
}

// CanHandle 判断是否能处理当前上下文
func (s *QueryBillsSkill) CanHandle(ctx skill.AgentContext) float64 {
	// 检查是否在支持的页面
	for _, page := range s.config.SupportedPages {
		if ctx.PageContext == page {
			return 1.0
		}
	}

	// 检查消息中是否包含查询关键词
	keywords := []string{"查询", "查", "花了", "消费", "账单", "记录"}
	for _, word := range keywords {
		if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(word)).MatchString(ctx.Message) {
			return 0.8
		}
	}

	return 0
}

// GetExtractionSchema 返回参数提取的 Schema
func (s *QueryBillsSkill) GetExtractionSchema(ctx skill.AgentContext) string {
	schema := `{
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
	return schema
}

// Execute 执行技能
func (s *QueryBillsSkill) Execute(ctx context.Context, request skill.SkillRequest) (*skill.SkillResponse, error) {
	// 使用 LLM 提取的参数
	params := request.Args
	if params == nil {
		// Fallback: 空参数
		params = make(map[string]interface{})
	}

	// 应用默认值
	if s.config.RuntimeConfig.DefaultParams != nil {
		for k, v := range s.config.RuntimeConfig.DefaultParams {
			if _, exists := params[k]; !exists {
				params[k] = v
			}
		}
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

// execute 执行查询操作
func (s *QueryBillsSkill) execute(args map[string]interface{}) (map[string]interface{}, error) {
	if s.billRepo == nil {
		return map[string]interface{}{
			"success": false,
			"message": "数据库未初始化",
		}, nil
	}

	// 提取参数
	startDate := getStringFromMap(args, "start_date")
	endDate := getStringFromMap(args, "end_date")
	category := getStringFromMap(args, "category")

	// 如果没有结束日期，默认为今天
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	filter := &model.BillFilter{
		UserID:    "default_user",
		Category:  category,
		StartDate: startDate,
		EndDate:   endDate,
	}

	bills, summary, err := s.billRepo.QueryBills(filter)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("查询账单失败: %v", err),
		}, nil
	}

	// 确保 bills 不为 nil
	if bills == nil {
		bills = []*model.Bill{}
	}

	return map[string]interface{}{
		"success": true,
		"message": "", // 留空，由 Agent 生成自然语言
		"data": map[string]interface{}{
			"total":   len(bills),
			"bills":   bills,
			"summary": summary,
			"query":   args,
		},
	}, nil
}

// Cleanup 清理资源
func (s *QueryBillsSkill) Cleanup() error {
	return nil
}
