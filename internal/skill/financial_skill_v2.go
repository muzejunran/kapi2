package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
)

type FinancialManagementSkillV2 struct {
	*BaseSkill
	billRepo *repository.BillRepository
	config   SkillConfig
}

// loadConfigFromJSON loads skill configuration from a JSON file
func loadConfigFromJSON(filename string) (*SkillConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// First parse to handle template values in optional fields
	var tempConfig struct {
		ID             string          `json:"id"`
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Version        string          `json:"version"`
		SupportedPages []string        `json:"supported_pages"`
		ErrorMessage   string `json:"error_message"`
		Operations     map[string]struct {
			Operation string                 `json:"operation"`
			Name      string                 `json:"name"`
			Params    json.RawMessage        `json:"params"`
			Required  []string               `json:"required"`
			Optional  map[string]interface{} `json:"optional"`
			NeedsLLM  bool `json:"needs_llm"`
		} `json:"operations"`
	}

	if err := json.Unmarshal(data, &tempConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Process template values in optional fields
	today := time.Now().Format("2006-01-02")
	processedOps := make(map[string]OperationSchema)

	for opKey, op := range tempConfig.Operations {
		processedOptional := make(map[string]interface{})
		for k, v := range op.Optional {
			if str, ok := v.(string); ok && strings.Contains(str, "{{") {
				tmpl, err := template.New("optional").Parse(str)
				if err == nil {
					var buf strings.Builder
					tmpl.Execute(&buf, map[string]string{"Today": today})
					processedOptional[k] = buf.String()
				} else {
					processedOptional[k] = v
				}
			} else {
				processedOptional[k] = v
			}
		}

		processedOps[opKey] = OperationSchema{
			Operation: op.Operation,
			Name:      op.Name,
			Params:    op.Params,
			Required:  op.Required,
			Optional:  processedOptional,
			NeedsLLM:  op.NeedsLLM,
		}
	}

	return &SkillConfig{
		ID:             tempConfig.ID,
		Name:           tempConfig.Name,
		Description:    tempConfig.Description,
		Version:        tempConfig.Version,
		SupportedPages: tempConfig.SupportedPages,
		ErrorMessage:   tempConfig.ErrorMessage,
		Operations:     processedOps,
	}, nil
}

// NewFinancialManagementSkillV2 creates a new financial management skill from config file
func NewFinancialManagementSkillV2(billRepo *repository.BillRepository) *FinancialManagementSkillV2 {
	// Load config from JSON file
	config, err := loadConfigFromJSON("internal/skill/configs/financial_management.json")
	if err != nil {
		// Fallback to default config if loading fails
		config = &SkillConfig{
			ID:             "financial_management",
			Name:           "财务管理",
			Description:    "管理用户的账单、预算和财务建议",
			Version:        "2.0.0",
			SupportedPages: []string{"add_bill", "query_bills"},
			ErrorMessage:   "抱歉，我没理解您的意思，请换种说法或补充信息",
			Operations:     make(map[string]OperationSchema),
		}
	}

	return &FinancialManagementSkillV2{
		BaseSkill: NewBaseSkill(config.ID, config.Name, config.Description),
		billRepo:  billRepo,
		config:    *config,
	}
}

func (s *FinancialManagementSkillV2) Initialize(config map[string]interface{}) error {
	// Register tools from configuration
	for opKey, opSchema := range s.config.Operations {
		handler, exists := s.getHandler(opKey)
		if !exists {
			continue
		}

		s.RegisterTool(Tool{
			ID:          opSchema.Operation,
			Name:        opSchema.Name,
			Description: fmt.Sprintf("执行%s操作", opSchema.Name),
			Parameters:  opSchema.Params,
			Handler:     handler,
		})
	}

	return nil
}

func (s *FinancialManagementSkillV2) getHandler(opKey string) (ToolHandler, bool) {
	switch opKey {
	case "add_bill":
		return s.executeAddBill, true
	case "query_bills":
		return s.executeQueryBills, true
	case "budget_advisor":
		return s.executeBudgetAdvisor, true
	default:
		return nil, false
	}
}

func (s *FinancialManagementSkillV2) CanHandle(ctx AgentContext) float64 {
	fmt.Printf("[DEBUG] CanHandle - pageContext: %s, supportedPages: %v\n", ctx.PageContext, s.config.SupportedPages)
	for _, page := range s.config.SupportedPages {
		if ctx.PageContext == page {
			fmt.Printf("[DEBUG] CanHandle matched page: %s, returning 1.0\n", page)
			return 1.0
		}
	}
	fmt.Printf("[DEBUG] CanHandle no match, returning 0\n")
	return 0
}

func (s *FinancialManagementSkillV2) GetExtractionSchema(ctx AgentContext) string {
	// 根据页面上下文返回对应的 Schema
	switch ctx.PageContext {
	case "add_bill":
		return `{
		"type": "object",
		"properties": {
			"amount": {"type": "number", "description": "金额"},
			"category": {"type": "string", "description": "分类（餐饮、购物、交通等）"},
			"description": {"type": "string", "description": "描述"}
		},
		"required": ["amount"]
	}`
	case "query_bills":
		return `{
		"type": "object",
		"properties": {
			"start_date": {"type": "string", "description": "开始日期，格式 YYYY-MM-DD"},
			"end_date": {"type": "string", "description": "结束日期，格式 YYYY-MM-DD"},
			"category": {"type": "string", "description": "分类筛选"}
		}
	}`
	default:
		// home 或其他页面，根据操作类型返回对应的 schema
		return `{
		"type": "object",
		"properties": {
			"amount": {"type": "number", "description": "金额"},
			"category": {"type": "string", "description": "分类（餐饮、购物、交通等）"},
			"description": {"type": "string", "description": "描述"},
			"start_date": {"type": "string", "description": "开始日期，格式 YYYY-MM-DD"},
			"end_date": {"type": "string", "description": "结束日期，格式 YYYY-MM-DD"},
			"income": {"type": "number", "description": "月收入"},
			"expenses": {"type": "number", "description": "月支出"}
		}
	}`
	}
}

func (s *FinancialManagementSkillV2) Execute(ctx context.Context, request SkillRequest) (*SkillResponse, error) {
	operation := s.detectOperationWithPage(request.Context.PageContext, request.Input)

	fmt.Printf("[DEBUG] Execute called - input: %s, detected operation: %s, args: %+v\n", request.Input, operation, request.Args)

	if operation == "" {
		return &SkillResponse{
			Message: s.config.ErrorMessage,
		}, nil
	}

	opSchema, exists := s.config.Operations[operation]
	if !exists {
		return &SkillResponse{
			Message: fmt.Sprintf("不支持的操作: %s", operation),
		}, nil
	}

	// 使用 Args 中 LLM 提取的参数
	params := request.Args
	if params == nil {
		// 如果没有提取到参数，使用简单提取作为 fallback
		extraction := s.simpleExtractParams(operation, request.Input)
		if !extraction.Success {
			return &SkillResponse{
				Message: extraction.Error,
			}, nil
		}
		params = extraction.Params
	}

	fmt.Printf("[DEBUG] Params before applyDefaults: %+v\n", params)

	// 应用默认值
	params = s.applyDefaults(params, opSchema.Optional)

	fmt.Printf("[DEBUG] Params after applyDefaults: %+v\n", params)

	result, err := s.executeOperation(ctx, operation, params)
	if err != nil {
		return nil, err
	}

	return &SkillResponse{
		Message: result.Message,
		Data:    result.Data,
		Context: &Context{
			Query:  request.Input,
			Answer: result.Message,
		},
	}, nil
}

// detectOperationWithPage 根据 page_context 和消息内容检测操作类型
func (s *FinancialManagementSkillV2) detectOperationWithPage(pageContext, message string) string {
	// 优先根据 page_context 判断
	switch pageContext {
	case "add_bill":
		return "add_bill"
	case "query_bills":
		return "query_bills"
	case "budget":
		return "budget_advisor"
	}
	// 如果是 home 或其他页面，根据消息内容判断
	return s.detectOperation(message)
}

func (s *FinancialManagementSkillV2) detectOperation(message string) string {
	keywords := map[string][]string{
		"add_bill":       {"记账", "记", "花了", "支付", "消费", "账单"},
		"query_bills":    {"查询", "查", "花了", "消费", "账单", "记录"},
		"budget_advisor": {"预算", "建议", "理财", "存钱", "省钱"},
	}

	for op, words := range keywords {
		for _, word := range words {
			if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(word)).MatchString(message) {
				return op
			}
		}
	}

	return ""
}

func (s *FinancialManagementSkillV2) simpleExtractParams(operation string, message string) *ExtractionResult {
	if operation == "add_bill" {
		amount := extractNumber(message)
		if amount == 0 {
			return &ExtractionResult{
				Success: false,
				Error:   "未能提取金额信息",
			}
		}
		return &ExtractionResult{
			Success: true,
			Params: map[string]interface{}{
				"amount": amount,
			},
		}
	}

	return &ExtractionResult{
		Success: true,
		Params:  map[string]interface{}{},
	}
}

func (s *FinancialManagementSkillV2) applyDefaults(params map[string]interface{}, defaults map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range params {
		result[k] = v
	}

	for k, v := range defaults {
		if _, exists := result[k]; !exists {
			if v != nil && v != "" {
				result[k] = v
			}
		}
	}

	return result
}

func (s *FinancialManagementSkillV2) executeOperation(ctx context.Context, operation string, params map[string]interface{}) (*SkillResponse, error) {
	switch operation {
	case "add_bill":
		return s.executeAddBillOp(params)
	case "query_bills":
		return s.executeQueryBillsOp(params)
	case "budget_advisor":
		return s.executeBudgetAdvisorOp(params)
	default:
		return &SkillResponse{
			Message: fmt.Sprintf("不支持的操作: %s", operation),
		}, nil
	}
}

func (s *FinancialManagementSkillV2) executeAddBillOp(params map[string]interface{}) (*SkillResponse, error) {
	fmt.Printf("[DEBUG] executeAddBillOp called with params: %+v\n", params)

	if s.billRepo == nil {
		fmt.Println("[DEBUG] billRepo is nil!")
		return &SkillResponse{
			Message: "数据库未初始化",
		}, nil
	}

	amount := getFloatFromMap(params, "amount")
	if amount <= 0 {
		return &SkillResponse{
			Message: "请提供有效的金额",
		}, nil
	}

	category := getStringFromMap(params, "category")
	if category == "" {
		category = "其他"
	}

	description := getStringFromMap(params, "description")

	bill := &model.Bill{
		UserID:      "default_user",
		Amount:      amount,
		Category:    category,
		Description: description,
	}

	fmt.Printf("[DEBUG] About to add bill: %+v\n", bill)

	err := s.billRepo.AddBill(bill)
	if err != nil {
		fmt.Printf("[DEBUG] AddBill failed: %v\n", err)
		return &SkillResponse{
			Message: fmt.Sprintf("添加账单失败: %v", err),
		}, nil
	}

	fmt.Printf("[DEBUG] Bill added successfully\n")

	return &SkillResponse{
		Message: fmt.Sprintf("已记录%s账单%.2f元", category, amount),
	}, nil
}

func (s *FinancialManagementSkillV2) executeQueryBillsOp(params map[string]interface{}) (*SkillResponse, error) {
	if s.billRepo == nil {
		return &SkillResponse{
			Message: "数据库未初始化",
		}, nil
	}

	startDate := getStringFromMap(params, "start_date")
	endDate := getStringFromMap(params, "end_date")
	category := getStringFromMap(params, "category")

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
		return &SkillResponse{
			Message: fmt.Sprintf("查询账单失败: %v", err),
		}, nil
	}

	// 返回数据，让 Agent 调用 LLM 生成自然语言
	// 确保 bills 不为 nil
	billsSlice := bills
	if billsSlice == nil {
		billsSlice = []*model.Bill{}
	}

	return &SkillResponse{
		Message: "", // 留空，由 Agent 生成
		Data: map[string]interface{}{
			"total":   len(billsSlice),
			"bills":   billsSlice,
			"summary": summary,
			"query":   params, // 记录查询条件
		},
	}, nil
}

func (s *FinancialManagementSkillV2) executeBudgetAdvisorOp(params map[string]interface{}) (*SkillResponse, error) {
	income := getFloatFromMap(params, "income")
	if income == 0 {
		return &SkillResponse{
			Message: "请提供收入金额",
		}, nil
	}

	return &SkillResponse{
		Message: "建议您将收入的20%用于储蓄",
		Data: map[string]interface{}{
			"income":       income,
			"savings_rate": 20.0,
		},
	}, nil
}

func (s *FinancialManagementSkillV2) executeAddBill(args map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func (s *FinancialManagementSkillV2) executeQueryBills(args map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func (s *FinancialManagementSkillV2) executeBudgetAdvisor(args map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func extractNumber(text string) float64 {
	re := regexp.MustCompile(`\d+(\.\d+)?`)
	match := re.FindString(text)
	if match == "" {
		return 0
	}
	var num float64
	fmt.Sscanf(match, "%f", &num)
	return num
}

func getFloatFromMap(params map[string]interface{}, key string) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case string:
			var f float64
			fmt.Sscanf(v, "%f", &f)
			return f
		}
	}
	return 0
}

func getStringFromMap(params map[string]interface{}, key string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
