package model

type Bill struct {
	ID          string  `json:"id"`
	UserID      string  `json:"user_id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Timestamp   string  `json:"timestamp"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type BillFilter struct {
	UserID      string `json:"user_id"`
	Category    string `json:"category,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
}

type BillSummary struct {
	TotalAmount float64            `json:"total_amount"`
	ByCategory map[string]float64 `json:"by_category"`
	TotalCount int                `json:"total_count"`
}
