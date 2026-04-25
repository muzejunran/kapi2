package model

type Budget struct {
	ID        int64   `json:"id"`
	UserID    string  `json:"user_id"`
	BudgetMonth string `json:"budget_month"` // YYYY-MM
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type BudgetWithSpent struct {
	Category  string  `json:"category"`
	Budget    float64 `json:"budget"`
	Spent     float64 `json:"spent"`
	Remaining float64 `json:"remaining"`
}
