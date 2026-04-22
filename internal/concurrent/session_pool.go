package concurrent

import (
	"context"
	"sync"
	"time"
)

// SessionPool manages multiple concurrent sessions
type SessionPool struct {
	sessions      map[string]*ActiveSession
	mu            sync.RWMutex
	maxSessions   int
	activeCount  int
	logger        Logger
}

// ActiveSession represents an active session with context
type ActiveSession struct {
	ID           string
	UserID       string
	Context      *AgentContext
	CreatedAt    time.Time
	LastUsed     time.Time
	Cancel       context.CancelFunc
}

// AgentContext holds the agent context for a session
type AgentContext struct {
	Messages      []AgentMessage
	Memory       *Memory
	Skills       []*Skill
	TokenBudget  int
}

// AgentMessage represents a message in the conversation
type AgentMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// Memory represents user memory structure
type Memory struct {
	Profile     string
	Preferences string
	Facts      []string
	Summaries  []ConversationSummary
}

// ConversationSummary represents a summarized conversation
type ConversationSummary struct {
	ID        string
	Content   string
	CreatedAt time.Time
}

// Skill represents an available skill
type Skill struct {
	ID          string
	Name        string
	Type        string
	Description string
}

// Logger interface for logging
type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}

// NewSessionPool creates a new session pool
func NewSessionPool(maxSessions int, logger Logger) *SessionPool {
	return &SessionPool{
		sessions:    make(map[string]*ActiveSession),
		maxSessions:  maxSessions,
		logger:      logger,
	}
}

// CreateSession creates a new session in the pool
func (sp *SessionPool) CreateSession(sessionID, userID string) (*ActiveSession, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Check if we've reached max sessions
	if sp.activeCount >= sp.maxSessions {
		return nil, NewMaxSessionsError(sp.maxSessions)
	}

	// Check if session already exists
	if _, exists := sp.sessions[sessionID]; exists {
		return nil, NewSessionExistsError(sessionID)
	}

	_, cancel := context.WithCancel(context.Background())

	session := &ActiveSession{
		ID:        sessionID,
		UserID:    userID,
		Context:    &AgentContext{
			Messages:    make([]AgentMessage, 0),
			Memory:     &Memory{},
			Skills:     make([]*Skill, 0),
			TokenBudget: 1500,
		},
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Cancel:    cancel,
	}

	sp.sessions[sessionID] = session
	sp.activeCount++
	sp.logger.Info("Session created", "session_id", sessionID, "active_sessions", sp.activeCount)

	return session, nil
}

// GetSession retrieves a session from the pool
func (sp *SessionPool) GetSession(sessionID string) (*ActiveSession, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	session, exists := sp.sessions[sessionID]
	if !exists {
		return nil, NewSessionNotFoundError(sessionID)
	}

	session.LastUsed = time.Now()
	return session, nil
}

// RemoveSession removes a session from the pool
func (sp *SessionPool) RemoveSession(sessionID string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	session, exists := sp.sessions[sessionID]
	if !exists {
		return NewSessionNotFoundError(sessionID)
	}

	// Cancel the context
	session.Cancel()

	delete(sp.sessions, sessionID)
	sp.activeCount--
	sp.logger.Info("Session removed", "session_id", sessionID, "active_sessions", sp.activeCount)

	return nil
}

// UpdateSessionActivity updates the last used time of a session
func (sp *SessionPool) UpdateSessionActivity(sessionID string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	session, exists := sp.sessions[sessionID]
	if !exists {
		return NewSessionNotFoundError(sessionID)
	}

	session.LastUsed = time.Now()
	return nil
}

// GetActiveSessions returns all active sessions
func (sp *SessionPool) GetActiveSessions() []*ActiveSession {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	sessions := make([]*ActiveSession, 0, len(sp.sessions))
	for _, session := range sp.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// GetStats returns session pool statistics
func (sp *SessionPool) GetStats() SessionPoolStats {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	return SessionPoolStats{
		ActiveSessions: sp.activeCount,
		MaxSessions:    sp.maxSessions,
		TotalCreated:   len(sp.sessions),
	}
}

// SessionPoolStats holds pool statistics
type SessionPoolStats struct {
	ActiveSessions int
	MaxSessions    int
	TotalCreated   int
}

// GetIdleSessions returns sessions that haven't been used recently
func (sp *SessionPool) GetIdleSessions(idleTimeout time.Duration) []string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	idleSessions := make([]string, 0)
	now := time.Now()

	for sessionID, session := range sp.sessions {
		if now.Sub(session.LastUsed) > idleTimeout {
			idleSessions = append(idleSessions, sessionID)
		}
	}

	return idleSessions
}

// EvictIdleSessions removes sessions that have been idle for too long
func (sp *SessionPool) EvictIdleSessions(idleTimeout time.Duration) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	now := time.Now()
	evicted := 0

	for sessionID, session := range sp.sessions {
		if now.Sub(session.LastUsed) > idleTimeout {
			// Cancel the context
			session.Cancel()
			delete(sp.sessions, sessionID)
			evicted++
		}
	}

	sp.activeCount = len(sp.sessions)

	if evicted > 0 {
		sp.logger.Info("Sessions evicted", "count", evicted, "idle_timeout", idleTimeout)
	}

	return evicted
}

// Error types
type MaxSessionsError struct {
	MaxSessions int
}

type SessionExistsError struct {
	SessionID string
}

type SessionNotFoundError struct {
	SessionID string
}

func NewMaxSessionsError(max int) *MaxSessionsError {
	return &MaxSessionsError{MaxSessions: max}
}

func (e *MaxSessionsError) Error() string {
	return "maximum number of sessions reached"
}

func NewSessionExistsError(id string) *SessionExistsError {
	return &SessionExistsError{SessionID: id}
}

func (e *SessionExistsError) Error() string {
	return "session already exists"
}

func NewSessionNotFoundError(id string) *SessionNotFoundError {
	return &SessionNotFoundError{SessionID: id}
}

func (e *SessionNotFoundError) Error() string {
	return "session not found"
}