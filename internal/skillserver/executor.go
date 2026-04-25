package skillserver

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
	"github.com/sirupsen/logrus"
)

// Executor 负责执行工具并返回原始结果
type Executor struct {
	billRepo   *repository.BillRepository
	budgetRepo *repository.BudgetRepository
}

// NewExecutor 创建执行器，repo 为 nil 时降级为 mock 数据
func NewExecutor(billRepo *repository.BillRepository, budgetRepo *repository.BudgetRepository) *Executor {
	return &Executor{billRepo: billRepo, budgetRepo: budgetRepo}
}

// Execute 执行指定工具，返回原始结果（不含 action_type，由 handler 层附加）
func (e *Executor) Execute(ctx context.Context, toolName, userID string, args map[string]interface{}) (map[string]interface{}, error) {
	log := logger.FromContext(ctx)
	log.WithFields(logrus.Fields{
		"tool":    toolName,
		"user_id": userID,
	}).Info("[executor] execute start")

	start := time.Now()
	var result map[string]interface{}
	var err error

	switch toolName {
	case "add_bill":
		result, err = e.addBill(ctx, userID, args)
	case "query_bills":
		result, err = e.queryBills(ctx, userID, args)
	case "query_budget":
		result, err = e.queryBudget(ctx, userID, args)
	case "update_budget":
		result, err = e.updateBudget(ctx, userID, args)
	default:
		return nil, fmt.Errorf("tool %q not implemented", toolName)
	}

	log.WithFields(logrus.Fields{
		"tool":     toolName,
		"duration": time.Since(start).String(),
		"ok":       err == nil,
	}).Info("[executor] execute done")

	return result, err
}

// ── 账单 ──────────────────────────────────────────────────────────────────────

func (e *Executor) addBill(ctx context.Context, userID string, args map[string]interface{}) (map[string]interface{}, error) {
	logger.FromContext(ctx).WithFields(logrus.Fields{"user_id": userID, "args": args}).Info("[executor] addBill")
	amount := getFloat(args, "amount")
	if amount <= 0 {
		return nil, fmt.Errorf("金额必须大于0")
	}
	category := getString(args, "category")
	if category == "" {
		category = "其他"
	}
	if userID == "" {
		userID = "default_user"
	}

	bill := &model.Bill{
		UserID:      userID,
		Amount:      amount,
		Category:    category,
		Description: getString(args, "description"),
		Timestamp:   getString(args, "date"),
	}

	if e.billRepo != nil {
		if err := e.billRepo.AddBill(bill); err != nil {
			return nil, fmt.Errorf("写入账单失败: %w", err)
		}
	} else {
		bill.ID = fmt.Sprintf("MOCK-%06d", rand.Intn(1000000))
		if bill.Timestamp == "" {
			bill.Timestamp = time.Now().Format("2006-01-02")
		}
	}

	return map[string]interface{}{
		"bill_id":     bill.ID,
		"amount":      bill.Amount,
		"category":    bill.Category,
		"description": bill.Description,
		"date":        bill.Timestamp,
	}, nil
}

func (e *Executor) queryBills(ctx context.Context, userID string, args map[string]interface{}) (map[string]interface{}, error) {
	logger.FromContext(ctx).WithFields(logrus.Fields{"user_id": userID, "args": args}).Info("[executor] queryBills")
	if userID == "" {
		userID = "default_user"
	}

	if e.billRepo == nil {
		return mockQueryBills(args)
	}

	now := time.Now()
	filter := &model.BillFilter{
		UserID:    userID,
		Category:  getString(args, "category"),
		StartDate: getString(args, "start_date"),
		EndDate:   getString(args, "end_date"),
	}
	if filter.StartDate == "" {
		filter.StartDate = now.Format("2006-01") + "-01"
	}
	if filter.EndDate == "" {
		filter.EndDate = now.Format("2006-01-02")
	}

	bills, summary, err := e.billRepo.QueryBills(filter)
	if err != nil {
		return nil, fmt.Errorf("查询账单失败: %w", err)
	}
	if bills == nil {
		bills = []*model.Bill{}
	}

	return map[string]interface{}{
		"bills":        bills,
		"count":        summary.TotalCount,
		"total_amount": summary.TotalAmount,
		"summary":      summary.ByCategory,
	}, nil
}

// ── 预算 ──────────────────────────────────────────────────────────────────────

func (e *Executor) queryBudget(ctx context.Context, userID string, args map[string]interface{}) (map[string]interface{}, error) {
	logger.FromContext(ctx).WithFields(logrus.Fields{"user_id": userID, "args": args}).Info("[executor] queryBudget")
	if userID == "" {
		userID = "default_user"
	}
	category := getString(args, "category")
	yearMonth := time.Now().Format("2006-01")

	if e.budgetRepo == nil {
		return mockQueryBudget(category, yearMonth)
	}

	rows, err := e.budgetRepo.QueryBudget(userID, yearMonth, category)
	if err != nil {
		return nil, fmt.Errorf("查询预算失败: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(rows))
	for _, b := range rows {
		result = append(result, map[string]interface{}{
			"category":  b.Category,
			"budget":    b.Budget,
			"spent":     b.Spent,
			"remaining": b.Remaining,
		})
	}
	return map[string]interface{}{
		"budgets": result,
		"month":   yearMonth,
	}, nil
}

func (e *Executor) updateBudget(ctx context.Context, userID string, args map[string]interface{}) (map[string]interface{}, error) {
	logger.FromContext(ctx).WithFields(logrus.Fields{"user_id": userID, "args": args}).Info("[executor] updateBudget")
	category := getString(args, "category")
	amount := getFloat(args, "amount")
	if category == "" {
		return nil, fmt.Errorf("category 不能为空")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("预算金额必须大于0")
	}
	if userID == "" {
		userID = "default_user"
	}
	yearMonth := time.Now().Format("2006-01")

	if e.budgetRepo == nil {
		return map[string]interface{}{
			"category":   category,
			"old_amount": 0.0,
			"new_amount": amount,
			"month":      yearMonth,
			"updated_at": time.Now().Format("2006-01-02 15:04:05"),
		}, nil
	}

	oldAmount, err := e.budgetRepo.UpsertBudget(userID, yearMonth, category, amount)
	if err != nil {
		return nil, fmt.Errorf("更新预算失败: %w", err)
	}
	return map[string]interface{}{
		"category":   category,
		"old_amount": oldAmount,
		"new_amount": amount,
		"month":      yearMonth,
		"updated_at": time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

// ── Mock 降级数据 ─────────────────────────────────────────────────────────────

func mockQueryBudget(category, yearMonth string) (map[string]interface{}, error) {
	all := []map[string]interface{}{
		{"category": "餐饮", "budget": 1000.0, "spent": 0.0, "remaining": 1000.0},
		{"category": "购物", "budget": 800.0, "spent": 0.0, "remaining": 800.0},
		{"category": "娱乐", "budget": 500.0, "spent": 0.0, "remaining": 500.0},
		{"category": "交通", "budget": 400.0, "spent": 0.0, "remaining": 400.0},
		{"category": "医疗", "budget": 300.0, "spent": 0.0, "remaining": 300.0},
		{"category": "其他", "budget": 200.0, "spent": 0.0, "remaining": 200.0},
	}
	var result []map[string]interface{}
	for _, b := range all {
		if category == "" || b["category"].(string) == category {
			result = append(result, b)
		}
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	return map[string]interface{}{"budgets": result, "month": yearMonth}, nil
}

func mockQueryBills(args map[string]interface{}) (map[string]interface{}, error) {
	category := getString(args, "category")
	now := time.Now()
	all := []map[string]interface{}{
		{"date": now.AddDate(0, 0, -1).Format("2006-01-02"), "category": "餐饮", "amount": 45.0, "description": "午餐"},
		{"date": now.AddDate(0, 0, -2).Format("2006-01-02"), "category": "交通", "amount": 23.5, "description": "滴滴打车"},
		{"date": now.AddDate(0, 0, -3).Format("2006-01-02"), "category": "娱乐", "amount": 128.0, "description": "电影票"},
		{"date": now.AddDate(0, 0, -4).Format("2006-01-02"), "category": "餐饮", "amount": 68.0, "description": "外卖"},
		{"date": now.AddDate(0, 0, -5).Format("2006-01-02"), "category": "购物", "amount": 299.0, "description": "日用品"},
	}
	var filtered []map[string]interface{}
	summary := map[string]float64{}
	for _, b := range all {
		cat := b["category"].(string)
		if category == "" || cat == category {
			filtered = append(filtered, b)
		}
		summary[cat] += b["amount"].(float64)
	}
	if filtered == nil {
		filtered = []map[string]interface{}{}
	}
	total := 0.0
	for _, v := range summary {
		total += v
	}
	return map[string]interface{}{
		"bills": filtered, "count": len(filtered),
		"total_amount": total, "summary": summary,
	}, nil
}

// ── 参数提取工具函数 ────────────────────────────────────────────────────────��──

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return 0
}
