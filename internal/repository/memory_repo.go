package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-assistant-service/internal/memory"

	_ "github.com/go-sql-driver/mysql"
)

type MemoryRepository struct {
	db *sql.DB
}

func NewMemoryRepository(dsn string) (*MemoryRepository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_memory (
			user_id        VARCHAR(64)  NOT NULL PRIMARY KEY,
			profile        TEXT         NOT NULL,
			preferences    TEXT         NOT NULL,
			facts          JSON         NOT NULL,
			recent_summary TEXT         NOT NULL,
			updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create user_memory table: %w", err)
	}

	return &MemoryRepository{db: db}, nil
}

// Load retrieves a user's full memory from MySQL.
// Returns (nil, nil) if the user has no persisted memory yet.
func (r *MemoryRepository) Load(userID string) (*memory.Memory, error) {
	row := r.db.QueryRow(
		"SELECT profile, preferences, facts, recent_summary, updated_at FROM user_memory WHERE user_id = ?",
		userID,
	)
	var profile, preferences, factsJSON, recentSummary, updatedAt string
	if err := row.Scan(&profile, &preferences, &factsJSON, &recentSummary, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var facts []string
	if err := json.Unmarshal([]byte(factsJSON), &facts); err != nil {
		facts = []string{}
	}

	return &memory.Memory{
		UserID:        userID,
		Profile:       profile,
		Preferences:   preferences,
		Facts:         facts,
		RecentSummary: recentSummary,
		UpdatedAt:     updatedAt,
	}, nil
}

// Save upserts the full memory record for a user.
func (r *MemoryRepository) Save(mem *memory.Memory) error {
	factsJSON, err := json.Marshal(mem.Facts)
	if err != nil {
		factsJSON = []byte("[]")
	}
	_, err = r.db.Exec(`
		INSERT INTO user_memory (user_id, profile, preferences, facts, recent_summary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			profile        = VALUES(profile),
			preferences    = VALUES(preferences),
			facts          = VALUES(facts),
			recent_summary = VALUES(recent_summary),
			updated_at     = VALUES(updated_at)
	`, mem.UserID, mem.Profile, mem.Preferences, string(factsJSON), mem.RecentSummary,
		time.Now().UTC().Format("2006-01-02 15:04:05"))
	return err
}

// Delete removes the user's memory record from MySQL entirely.
func (r *MemoryRepository) Delete(userID string) error {
	_, err := r.db.Exec("DELETE FROM user_memory WHERE user_id = ?", userID)
	return err
}

func (r *MemoryRepository) Close() error {
	return r.db.Close()
}
