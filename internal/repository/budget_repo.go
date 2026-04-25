package repository

import (
	"database/sql"
	"fmt"
	"time"

	"ai-assistant-service/internal/model"

	_ "github.com/go-sql-driver/mysql"
)

var defaultBudgets = map[string]float64{
	"餐饮": 1000.0,
	"购物": 800.0,
	"娱乐": 500.0,
	"交通": 400.0,
	"医疗": 300.0,
	"其他": 200.0,
}

type BudgetRepository struct {
	db *sql.DB
}

func NewBudgetRepository(dsn string) (*BudgetRepository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS budgets (
			id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			user_id    VARCHAR(64)    NOT NULL,
			budget_month CHAR(7)        NOT NULL,
			category   VARCHAR(50)    NOT NULL,
			amount     DECIMAL(10,2)  NOT NULL,
			created_at TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_user_month_cat (user_id, budget_month, category),
			INDEX      idx_user_month  (user_id, budget_month)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create budgets table: %w", err)
	}

	return &BudgetRepository{db: db}, nil
}

// QueryBudget returns budget vs. actual spending for the given user and month.
// If no budget rows exist for the month, default amounts are seeded automatically.
// category is optional — empty string returns all categories.
func (r *BudgetRepository) QueryBudget(userID, yearMonth, category string) ([]*model.BudgetWithSpent, error) {
	if err := r.ensureDefaults(userID, yearMonth); err != nil {
		// non-fatal: log at caller level
		_ = err
	}

	// Compute month boundaries for the bills JOIN
	t, err := time.Parse("2006-01", yearMonth)
	if err != nil {
		return nil, fmt.Errorf("invalid budget_month %q: %w", yearMonth, err)
	}
	startDate := t.Format("2006-01-02") + " 00:00:00"
	endDate := t.AddDate(0, 1, 0).Add(-time.Second).Format("2006-01-02 15:04:05")

	query := `
		SELECT
			b.category,
			b.amount                            AS budget,
			COALESCE(s.spent, 0)                AS spent,
			b.amount - COALESCE(s.spent, 0)     AS remaining
		FROM budgets b
		LEFT JOIN (
			SELECT category, SUM(amount) AS spent
			FROM   bills
			WHERE  user_id = ? AND timestamp >= ? AND timestamp <= ?
			GROUP  BY category
		) s ON b.category = s.category
		WHERE b.user_id = ? AND b.budget_month = ?`

	args := []interface{}{userID, startDate, endDate, userID, yearMonth}

	if category != "" {
		query += " AND b.category = ?"
		args = append(args, category)
	}
	query += " ORDER BY b.category"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*model.BudgetWithSpent
	for rows.Next() {
		var b model.BudgetWithSpent
		if err := rows.Scan(&b.Category, &b.Budget, &b.Spent, &b.Remaining); err != nil {
			return nil, err
		}
		results = append(results, &b)
	}
	return results, rows.Err()
}

// UpsertBudget sets the budget for a user/month/category, creating or updating as needed.
func (r *BudgetRepository) UpsertBudget(userID, yearMonth, category string, amount float64) (float64, error) {
	// Fetch current amount before update (for "old_amount" in the response)
	var old float64
	row := r.db.QueryRow(
		"SELECT amount FROM budgets WHERE user_id = ? AND budget_month = ? AND category = ?",
		userID, yearMonth, category,
	)
	_ = row.Scan(&old) // ignore error — zero if not found

	_, err := r.db.Exec(`
		INSERT INTO budgets (user_id, budget_month, category, amount)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE amount = VALUES(amount)`,
		userID, yearMonth, category, amount,
	)
	if err != nil {
		return 0, err
	}
	return old, nil
}

// ensureDefaults seeds all six default categories for a user+month if none exist yet.
func (r *BudgetRepository) ensureDefaults(userID, yearMonth string) error {
	for cat, amt := range defaultBudgets {
		_, err := r.db.Exec(`
			INSERT IGNORE INTO budgets (user_id, budget_month, category, amount)
			VALUES (?, ?, ?, ?)`,
			userID, yearMonth, cat, amt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *BudgetRepository) Close() error {
	return r.db.Close()
}
