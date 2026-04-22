package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ai-assistant-service/internal/config"

	"github.com/redis/go-redis/v9"
)

type Manager struct {
	redisClient *redis.Client
	config     *config.Config
}

type Session struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"user_id"`
	Messages    []map[string]interface{} `json:"messages"`
	CreatedAt   string                 `json:"created_at"`
	LastUpdated string                 `json:"last_updated"`
	ExpiresAt   string                 `json:"expires_at"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		redisClient: redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		}),
		config: cfg,
	}
}

func (m *Manager) GetOrCreateSession(userID string) (string, error) {
	// Check if session exists for user
	sessionID, err := m.getSessionIDForUser(userID)
	if err == nil && sessionID != "" {
		// Check if session is expired
		session, err := m.GetSession(sessionID)
		if err == nil && !m.isSessionExpired(session) {
			return sessionID, nil
		}
	}

	// Create new session
	sessionID, err = m.createSession(userID)
	if err != nil {
		return "", err
	}

	return sessionID, nil
}

func (m *Manager) GetSession(sessionID string) (*Session, error) {
	ctx := context.Background()
	sessionJSON, err := m.redisClient.Get(ctx, "session:"+sessionID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

func (m *Manager) UpdateSession(sessionID string, session *Session) error {
	ctx := context.Background()
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return err
	}

	return m.redisClient.Set(ctx, "session:"+sessionID, sessionJSON, 24*time.Hour).Err()
}

func (m *Manager) DeleteSession(sessionID string) error {
	ctx := context.Background()
	return m.redisClient.Del(ctx, "session:"+sessionID).Err()
}

func (m *Manager) createSession(userID string) (string, error) {
	sessionID := generateSessionID()
	now := time.Now().UTC().Format(time.RFC3339)

	session := &Session{
		ID:          sessionID,
		UserID:      userID,
		Messages:    make([]map[string]interface{}, 0),
		CreatedAt:   now,
		LastUpdated: now,
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		Data:       make(map[string]interface{}),
	}

	ctx := context.Background()
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	if err := m.redisClient.Set(ctx, "session:"+sessionID, sessionJSON, 24*time.Hour).Err(); err != nil {
		return "", err
	}

	m.indexUserSession(userID, sessionID)
	return sessionID, nil
}

func (m *Manager) getSessionIDForUser(userID string) (string, error) {
	ctx := context.Background()
	sessionID, err := m.redisClient.Get(ctx, "user:"+userID+":session").Result()
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func (m *Manager) indexUserSession(userID, sessionID string) {
	ctx := context.Background()
	m.redisClient.Set(ctx, "user:"+userID+":session", sessionID, 24*time.Hour).Err()
}

func (m *Manager) ListSessions() ([]*Session, error) {
	ctx := context.Background()

	// Get all session keys
	keys, err := m.redisClient.Keys(ctx, "session:*").Result()
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, key := range keys {
		sessionJSON, err := m.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var session Session
		if err := json.Unmarshal([]byte(sessionJSON), &session); err == nil {
			if !m.isSessionExpired(&session) {
				sessions = append(sessions, &session)
			}
		}
	}

	return sessions, nil
}

func (m *Manager) CleanupExpiredSessions() error {
	ctx := context.Background()

	// Get all session keys
	keys, err := m.redisClient.Keys(ctx, "session:*").Result()
	if err != nil {
		return err
	}

	for _, key := range keys {
		sessionJSON, err := m.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var session Session
		if err := json.Unmarshal([]byte(sessionJSON), &session); err == nil {
			if m.isSessionExpired(&session) {
				m.redisClient.Del(ctx, key).Err()
			}
		}
	}

	return nil
}

func (m *Manager) isSessionExpired(session *Session) bool {
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(expiresAt)
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}