package repository

import (
	"database/sql"
	"fmt"
	"time"

	"ai-assistant-service/internal/model"

	_ "github.com/go-sql-driver/mysql"
)

type BillRepository struct {
	db *sql.DB
}

func NewBillRepository(dsn string) (*BillRepository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// 创建表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS bills (
		id VARCHAR(64) PRIMARY KEY,
		user_id VARCHAR(64) NOT NULL,
		amount DECIMAL(10,2) NOT NULL,
		category VARCHAR(50) NOT NULL,
		description TEXT,
		timestamp DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_user_id (user_id),
		INDEX idx_timestamp (timestamp)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &BillRepository{
		db: db,
	}, nil
}

func (r *BillRepository) AddBill(bill *model.Bill) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	if bill.CreatedAt == "" {
		bill.CreatedAt = now
	}

	bill.UpdatedAt = now

	if bill.Timestamp == "" {
		bill.Timestamp = now
	}

	if bill.ID == "" {
		bill.ID = fmt.Sprintf("bill_%d", time.Now().UnixNano())
	}

	_, err := r.db.Exec(
		"INSERT INTO bills (id, user_id, amount, category, description, timestamp, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		bill.ID, bill.UserID, bill.Amount, bill.Category, bill.Description, bill.Timestamp, bill.CreatedAt, bill.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *BillRepository) QueryBills(filter *model.BillFilter) ([]*model.Bill, *model.BillSummary, error) {
	var results []*model.Bill

	// 构建查询条件
	where := "WHERE user_id = ?"
	args := []interface{}{filter.UserID}

	if filter.Category != "" {
		where += " AND category = ?"
		args = append(args, filter.Category)
	}

	if filter.StartDate != "" {
		where += " AND timestamp >= ?"
		args = append(args, filter.StartDate+" 00:00:00")
	}

	if filter.EndDate != "" {
		where += " AND timestamp <= ?"
		args = append(args, filter.EndDate+" 23:59:59")
	}

	query := "SELECT id, user_id, amount, category, description, timestamp, created_at, updated_at FROM bills " + where + " ORDER BY timestamp DESC"
	fmt.Printf("[DEBUG] SQL Query: %s\n", query)
	fmt.Printf("[DEBUG] SQL Args: %+v\n", args)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bill model.Bill
		err := rows.Scan(&bill.ID, &bill.UserID, &bill.Amount, &bill.Category, &bill.Description, &bill.Timestamp, &bill.CreatedAt, &bill.UpdatedAt)
		if err != nil {
			continue
		}
		results = append(results, &bill)
	}

	// 计算汇总
	totalAmount := 0.0
	byCategory := make(map[string]float64)

	for _, bill := range results {
		totalAmount += bill.Amount
		byCategory[bill.Category] += bill.Amount
	}

	summary := &model.BillSummary{
		TotalAmount: totalAmount,
		ByCategory:  byCategory,
		TotalCount:  len(results),
	}

	return results, summary, nil
}

func (r *BillRepository) GetSummary(filter *model.BillFilter) (*model.BillSummary, error) {
	_, summary, err := r.QueryBills(filter)
	return summary, err
}

func (r *BillRepository) Close() error {
	return r.db.Close()
}
