package repository

import (
	"database/sql"
	"fmt"
	"time"

	"ai-assistant-service/internal/model"

	_ "github.com/go-sql-driver/mysql"
)

type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(dsn string) (*ConversationRepository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// 创建表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS conversations (
		id VARCHAR(64) PRIMARY KEY,
		user_id VARCHAR(64) NOT NULL,
		role VARCHAR(20) NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		INDEX idx_user_id (user_id),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &ConversationRepository{
		db: db,
	}, nil
}

func (r *ConversationRepository) AddMessage(conv *model.Conversation) error {
	if conv.ID == "" {
		conv.ID = fmt.Sprintf("conv_%d", time.Now().UnixNano())
	}

	if conv.CreatedAt == "" {
		conv.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	_, err := r.db.Exec(
		"INSERT INTO conversations (id, user_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)",
		conv.ID, conv.UserID, conv.Role, conv.Content, conv.CreatedAt,
	)
	return err
}

func (r *ConversationRepository) GetRecentMessages(userID string, limit int) ([]*model.Conversation, error) {
	query := "SELECT id, user_id, role, content, created_at FROM conversations WHERE user_id = ? ORDER BY created_at DESC LIMIT ?"

	rows, err := r.db.Query(query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*model.Conversation
	for rows.Next() {
		var conv model.Conversation
		err := rows.Scan(&conv.ID, &conv.UserID, &conv.Role, &conv.Content, &conv.CreatedAt)
		if err != nil {
			continue
		}
		results = append(results, &conv)
	}

	return results, nil
}

func (r *ConversationRepository) Close() error {
	return r.db.Close()
}
