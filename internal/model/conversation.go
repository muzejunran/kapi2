package model

type Conversation struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`      // user, assistant
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
