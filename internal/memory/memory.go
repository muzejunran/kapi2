package memory

import (
	"encoding/json"
	"fmt"
	"time"

	"ai-assistant-service/internal/storage"
	"github.com/sirupsen/logrus"
)

// ConversationTurn represents a single conversation turn (user + assistant)
type ConversationTurn struct {
	UserMessage      string `json:"user_message"`
	AssistantMessage string `json:"assistant_message"`
}

// Memory stores user-related information for context building
type Memory struct {
	UserID      string             `json:"user_id"`
	Profile     string             `json:"profile"`
	Preferences string             `json:"preferences"`
	Facts       []string           `json:"facts"`
	RecentTurns []ConversationTurn `json:"recent_turns"`
	UpdatedAt   string             `json:"updated_at"`
}

const (
	memoryKeyPrefix     = "memory:"
	memoryCacheTTL      = 24 * time.Hour
	maxRecentTurns      = 30
	maxContextTokens    = 4000
)

// MemoryService manages user memory with Redis cache + MySQL persistence
type MemoryService struct {
	storage          *storage.RedisStorage
	tokenBudget      int
	logger           *logrus.Logger
	dbLoadCallback   func(userID string, limit int) ([]ConversationTurn, error)
	dbSaveCallback   func(userID string, turn ConversationTurn) error
}

// NewMemoryService creates a new memory service
func NewMemoryService(redisStorage *storage.RedisStorage, tokenBudget int) *MemoryService {
	return &MemoryService{
		storage:     redisStorage,
		tokenBudget: tokenBudget,
		logger:      logrus.New(),
	}
}

// SetDatabaseCallbacks sets callbacks for database operations
func (ms *MemoryService) SetDatabaseCallbacks(loadFunc func(userID string, limit int) ([]ConversationTurn, error), saveFunc func(userID string, turn ConversationTurn) error) {
	ms.dbLoadCallback = loadFunc
	ms.dbSaveCallback = saveFunc
}

// GetUserMemory retrieves or creates user memory
func (ms *MemoryService) GetUserMemory(userID string) (*Memory, error) {
	key := memoryKeyPrefix + userID

	// Try to load from Redis cache
	cached, err := ms.storage.Get(key)
	if err == nil {
		var mem Memory
		if err := json.Unmarshal(cached, &mem); err == nil {
			ms.logger.WithField("user_id", userID).Debug("Memory loaded from cache")
			return &mem, nil
		}
	}

	// Load from database
	turns := []ConversationTurn{}
	if ms.dbLoadCallback != nil {
		loadedTurns, dbErr := ms.dbLoadCallback(userID, maxRecentTurns)
		if dbErr != nil {
			ms.logger.WithError(dbErr).Warn("Failed to load from database, using empty memory")
		} else {
			turns = loadedTurns
		}
	}

	// Create memory with existing turns
	mem := ms.createMemoryWithTurns(userID, turns)

	// Save to cache
	if err := ms.saveMemoryToCache(mem); err != nil {
		ms.logger.WithError(err).Warn("Failed to cache memory")
	}

	return mem, nil
}

// createMemoryWithTurns creates a new memory with existing conversation turns
func (ms *MemoryService) createMemoryWithTurns(userID string, turns []ConversationTurn) *Memory {
	return &Memory{
		UserID:      userID,
		Profile:     "",
		Preferences: "",
		Facts:       []string{},
		RecentTurns: turns,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}
}

// AddConversationTurn adds a new conversation turn to memory
func (ms *MemoryService) AddConversationTurn(userID, userMsg, assistantMsg string) error {
	// Get current memory
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}

	// Add new turn
	turn := ConversationTurn{
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
	}

	// Keep only recent turns
	mem.RecentTurns = append(mem.RecentTurns, turn)
	if len(mem.RecentTurns) > maxRecentTurns {
		mem.RecentTurns = mem.RecentTurns[len(mem.RecentTurns)-maxRecentTurns:]
	}
	mem.UpdatedAt = time.Now().Format(time.RFC3339)

	// Save to cache
	if err := ms.saveMemoryToCache(mem); err != nil {
		return err
	}

	// Save to database
	if ms.dbSaveCallback != nil {
		if dbErr := ms.dbSaveCallback(userID, turn); dbErr != nil {
			ms.logger.WithError(dbErr).Warn("Failed to save to database")
		}
	}

	return nil
}

// UpdateProfile updates user profile in memory
func (ms *MemoryService) UpdateProfile(userID, profile string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Profile = profile
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveMemoryToCache(mem)
}

// UpdatePreferences updates user preferences in memory
func (ms *MemoryService) UpdatePreferences(userID, preferences string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Preferences = preferences
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveMemoryToCache(mem)
}

// AddFact adds a fact to user memory
func (ms *MemoryService) AddFact(userID, fact string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Facts = append(mem.Facts, fact)
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveMemoryToCache(mem)
}

// saveMemoryToCache saves memory to Redis cache
func (ms *MemoryService) saveMemoryToCache(mem *Memory) error {
	key := memoryKeyPrefix + mem.UserID
	data, err := json.Marshal(mem)
	if err != nil {
		return err
	}
	return ms.storage.Set(key, data, memoryCacheTTL)
}

// ClearMemory clears user memory from cache
func (ms *MemoryService) ClearMemory(userID string) error {
	key := memoryKeyPrefix + userID
	return ms.storage.Delete(key)
}

// CalculateContextTokens calculates token count for memory context
func (ms *MemoryService) CalculateContextTokens(mem *Memory) int {
	var total int

	// Profile tokens
	if mem.Profile != "" {
		total += CalculateTokens(mem.Profile)
	}

	// Preferences tokens
	if mem.Preferences != "" {
		total += CalculateTokens(mem.Preferences)
	}

	// Facts tokens
	for _, fact := range mem.Facts {
		total += CalculateTokens(fact)
	}

	// Conversation history tokens
	for _, turn := range mem.RecentTurns {
		if turn.UserMessage != "" {
			total += CalculateTokens(turn.UserMessage)
		}
		if turn.AssistantMessage != "" {
			total += CalculateTokens(turn.AssistantMessage)
		}
	}

	return total
}

// TrimToBudget trims memory to fit within token budget
// Removes older conversation turns first to preserve recent context
func (ms *MemoryService) TrimToBudget(mem *Memory) *Memory {
	currentTokens := ms.CalculateContextTokens(mem)

	if currentTokens <= ms.tokenBudget {
		return mem
	}

	trimmed := &Memory{
		UserID:      mem.UserID,
		Profile:     mem.Profile,
		Preferences: mem.Preferences,
		Facts:       mem.Facts,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}

	// Trim conversation history from the oldest turns
	// Keep the most recent turns
	totalTokens := ms.CalculateContextTokens(trimmed)

	startIdx := 0
	for totalTokens > ms.tokenBudget && startIdx < len(mem.RecentTurns) {
		// Skip older turn
		startIdx++

		// Recalculate with remaining turns
		remainingTurns := mem.RecentTurns[startIdx:]
		testMem := &Memory{
			RecentTurns: remainingTurns,
		}
		totalTokens = ms.CalculateContextTokens(trimmed) + ms.CalculateContextTokens(testMem)
	}

	trimmed.RecentTurns = mem.RecentTurns[startIdx:]

	ms.logger.WithFields(logrus.Fields{
		"user_id":           mem.UserID,
		"original_tokens":   currentTokens,
		"trimmed_tokens":    ms.CalculateContextTokens(trimmed),
		"turns_removed":     startIdx,
		"turns_remaining":   len(trimmed.RecentTurns),
	}).Info("Memory trimmed to fit token budget")

	return trimmed
}

// FormatForDebug formats memory for debugging (NOT sent to LLM)
func (ms *MemoryService) FormatForDebug(mem *Memory) string {
	return fmt.Sprintf("User ID: %s\nProfile: %s\nPreferences: %s\nFacts: %v\nConversation Turns: %d\nLast Updated: %s",
		mem.UserID,
		mem.Profile,
		mem.Preferences,
		mem.Facts,
		len(mem.RecentTurns),
		mem.UpdatedAt,
	)
}
