package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/storage"
	"github.com/sirupsen/logrus"
)

// ConversationTurn is kept for database persistence callbacks
type ConversationTurn struct {
	UserMessage      string `json:"user_message"`
	AssistantMessage string `json:"assistant_message"`
}

// Memory stores user-related information for context building
type Memory struct {
	UserID        string   `json:"user_id"`
	Profile       string   `json:"profile"`
	Preferences   string   `json:"preferences"`
	Facts         []string `json:"facts"`
	RecentSummary string   `json:"recent_summary"` // LLM-compressed rolling summary
	UpdatedAt     string   `json:"updated_at"`
}

const (
	memoryKeyPrefix  = "memory:"
	memoryCacheTTL   = 24 * time.Hour
	maxSummaryTokens = 500
	maxFacts         = 30
)

// addFactIfNew appends fact only if not already present (case-insensitive).
// Evicts the oldest fact when at capacity.
func addFactIfNew(facts []string, newFact string) []string {
	newFact = strings.TrimSpace(newFact)
	if newFact == "" {
		return facts
	}
	for _, f := range facts {
		if strings.EqualFold(f, newFact) {
			return facts
		}
	}
	if len(facts) >= maxFacts {
		facts = facts[1:]
	}
	return append(facts, newFact)
}

// MemoryService manages user memory with Redis cache + MySQL persistence
type MemoryService struct {
	storage           storage.KVStore
	tokenBudget       int
	logger            *logrus.Logger
	determiner        *MemoryDeterminer
	dbSaveCallback    func(userID string, turn ConversationTurn) error
	memLoadCallback   func(userID string) (*Memory, error)
	memSaveCallback   func(mem *Memory) error
	memDeleteCallback func(userID string) error
}

// NewMemoryService creates a new memory service
func NewMemoryService(store storage.KVStore, tokenBudget int) *MemoryService {
	return &MemoryService{
		storage:     store,
		tokenBudget: tokenBudget,
		logger:      logger.New(),
	}
}

// SetDeterminer sets the memory determiner for async analysis
func (ms *MemoryService) SetDeterminer(d *MemoryDeterminer) {
	ms.determiner = d
}

// SetDatabaseCallbacks sets the save callback for conversation turn persistence
func (ms *MemoryService) SetDatabaseCallbacks(saveFunc func(userID string, turn ConversationTurn) error) {
	ms.dbSaveCallback = saveFunc
}

// SetMemoryPersistence wires MySQL load/save/delete for the Memory struct itself.
// loadFn: called on Redis miss to restore from DB (return nil,nil if row not found).
// saveFn: called after any in-memory update to persist the full snapshot.
// deleteFn: called by ClearMemory to hard-delete the row.
func (ms *MemoryService) SetMemoryPersistence(
	loadFn func(userID string) (*Memory, error),
	saveFn func(mem *Memory) error,
	deleteFn func(userID string) error,
) {
	ms.memLoadCallback = loadFn
	ms.memSaveCallback = saveFn
	ms.memDeleteCallback = deleteFn
}

// GetUserMemory retrieves user memory: Redis (hot) → MySQL (warm) → empty (cold)
func (ms *MemoryService) GetUserMemory(userID string) (*Memory, error) {
	key := memoryKeyPrefix + userID

	// 1. Redis hit
	if cached, err := ms.storage.Get(key); err == nil {
		var mem Memory
		if err := json.Unmarshal(cached, &mem); err == nil {
			ms.logger.WithField("user_id", userID).Debug("[memory] loaded from Redis cache")
			return &mem, nil
		}
	}

	// 2. MySQL fallback
	if ms.memLoadCallback != nil {
		dbMem, dbErr := ms.memLoadCallback(userID)
		if dbErr != nil {
			ms.logger.WithError(dbErr).Warn("[memory] failed to load from MySQL")
		} else if dbMem != nil {
			ms.logger.WithField("user_id", userID).Info("[memory] loaded from MySQL, warming Redis cache")
			_ = ms.saveMemoryToCache(dbMem)
			return dbMem, nil
		}
	}

	// 3. Brand-new user
	ms.logger.WithField("user_id", userID).Info("[memory] no existing memory, creating empty")
	mem := &Memory{
		UserID:    userID,
		Facts:     []string{},
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	_ = ms.saveMemoryToCache(mem)
	return mem, nil
}

// AddConversationTurn persists the turn and triggers async memory determination
func (ms *MemoryService) AddConversationTurn(userID, userMsg, assistantMsg string) error {
	if ms.dbSaveCallback != nil {
		turn := ConversationTurn{UserMessage: userMsg, AssistantMessage: assistantMsg}
		if dbErr := ms.dbSaveCallback(userID, turn); dbErr != nil {
			ms.logger.WithError(dbErr).Warn("Failed to save turn to database")
		}
	}

	if ms.determiner == nil {
		return nil
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := ms.determiner.Analyze(ctx, userMsg, assistantMsg)
		if err != nil {
			ms.logger.WithError(err).Warn("Memory determination failed")
			return
		}

		// Re-fetch latest memory to avoid overwriting concurrent updates
		mem, err := ms.GetUserMemory(userID)
		if err != nil {
			ms.logger.WithError(err).Warn("Failed to reload memory for update")
			return
		}

		if result.ProfileUpdate != nil && *result.ProfileUpdate != "" {
			mem.Profile = *result.ProfileUpdate
		}
		if result.PreferenceUpdate != nil && *result.PreferenceUpdate != "" {
			mem.Preferences = *result.PreferenceUpdate
		}
		for _, f := range result.NewFacts {
			mem.Facts = addFactIfNew(mem.Facts, f)
		}

		if result.Summary != "" {
			if mem.RecentSummary == "" {
				mem.RecentSummary = result.Summary
			} else {
				combined := mem.RecentSummary + "\n" + result.Summary
				if CalculateTokens(combined) > maxSummaryTokens {
					compressed, compErr := ms.determiner.compressSummary(ctx, combined)
					if compErr != nil {
						ms.logger.WithError(compErr).Warn("Failed to compress summary")
						mem.RecentSummary = combined
					} else {
						mem.RecentSummary = compressed
					}
				} else {
					mem.RecentSummary = combined
				}
			}
		}

		mem.UpdatedAt = time.Now().Format(time.RFC3339)
		if err := ms.saveMemoryToCache(mem); err != nil {
			ms.logger.WithError(err).Warn("[memory] failed to save to Redis after determination")
			return
		}
		if ms.memSaveCallback != nil {
			if err := ms.memSaveCallback(mem); err != nil {
				ms.logger.WithError(err).Warn("[memory] failed to persist to MySQL after determination")
			}
		}
		ms.logger.WithFields(logrus.Fields{
			"user_id":        userID,
			"has_profile":    mem.Profile != "",
			"has_prefs":      mem.Preferences != "",
			"facts_count":    len(mem.Facts),
			"summary_tokens": CalculateTokens(mem.RecentSummary),
		}).Info("[memory] updated after determination")
	}()

	return nil
}

// UpdateProfile updates user profile in memory and persists to MySQL.
func (ms *MemoryService) UpdateProfile(userID, profile string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Profile = profile
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveAndPersist(mem)
}

// UpdatePreferences updates user preferences in memory and persists to MySQL.
func (ms *MemoryService) UpdatePreferences(userID, preferences string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Preferences = preferences
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveAndPersist(mem)
}

// AddFact appends a fact to user memory (deduped, capped at maxFacts) and persists.
func (ms *MemoryService) AddFact(userID, fact string) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	mem.Facts = addFactIfNew(mem.Facts, fact)
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveAndPersist(mem)
}

// RemoveFact removes a fact by index from user memory and persists to MySQL.
func (ms *MemoryService) RemoveFact(userID string, index int) error {
	mem, err := ms.GetUserMemory(userID)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(mem.Facts) {
		return fmt.Errorf("fact index %d out of range (0–%d)", index, len(mem.Facts)-1)
	}
	mem.Facts = append(mem.Facts[:index], mem.Facts[index+1:]...)
	mem.UpdatedAt = time.Now().Format(time.RFC3339)
	return ms.saveAndPersist(mem)
}

// TrimToBudget trims memory to fit within token budget.
// Drop priority (lowest priority first): Profile → Preferences → Facts → RecentSummary
func (ms *MemoryService) TrimToBudget(mem *Memory) *Memory {
	if ms.CalculateContextTokens(mem) <= ms.tokenBudget {
		return mem
	}

	trimmed := &Memory{
		UserID:        mem.UserID,
		Profile:       mem.Profile,
		Preferences:   mem.Preferences,
		Facts:         append([]string{}, mem.Facts...),
		RecentSummary: mem.RecentSummary,
		UpdatedAt:     mem.UpdatedAt,
	}

	if ms.CalculateContextTokens(trimmed) > ms.tokenBudget {
		trimmed.Profile = ""
	}
	if ms.CalculateContextTokens(trimmed) > ms.tokenBudget {
		trimmed.Preferences = ""
	}
	for ms.CalculateContextTokens(trimmed) > ms.tokenBudget && len(trimmed.Facts) > 0 {
		trimmed.Facts = trimmed.Facts[1:]
	}
	if ms.CalculateContextTokens(trimmed) > ms.tokenBudget {
		trimmed.RecentSummary = ""
	}

	ms.logger.WithFields(logrus.Fields{
		"user_id":         mem.UserID,
		"original_tokens": ms.CalculateContextTokens(mem),
		"trimmed_tokens":  ms.CalculateContextTokens(trimmed),
	}).Info("Memory trimmed to fit token budget")

	return trimmed
}

// CalculateContextTokens calculates token count for all memory fields
func (ms *MemoryService) CalculateContextTokens(mem *Memory) int {
	total := CalculateTokens(mem.Profile) +
		CalculateTokens(mem.Preferences) +
		CalculateTokens(mem.RecentSummary)
	for _, fact := range mem.Facts {
		total += CalculateTokens(fact)
	}
	return total
}

// saveMemoryToCache writes memory to Redis.
func (ms *MemoryService) saveMemoryToCache(mem *Memory) error {
	key := memoryKeyPrefix + mem.UserID
	data, err := json.Marshal(mem)
	if err != nil {
		return err
	}
	return ms.storage.Set(key, data, memoryCacheTTL)
}

// saveAndPersist writes memory to Redis and, if configured, to MySQL.
func (ms *MemoryService) saveAndPersist(mem *Memory) error {
	if err := ms.saveMemoryToCache(mem); err != nil {
		return err
	}
	if ms.memSaveCallback != nil {
		if err := ms.memSaveCallback(mem); err != nil {
			ms.logger.WithError(err).Warn("[memory] failed to persist to MySQL")
		}
	}
	return nil
}

// ClearMemory deletes user memory from Redis and MySQL.
func (ms *MemoryService) ClearMemory(userID string) error {
	key := memoryKeyPrefix + userID
	if err := ms.storage.Delete(key); err != nil {
		return err
	}
	if ms.memDeleteCallback != nil {
		if err := ms.memDeleteCallback(userID); err != nil {
			ms.logger.WithError(err).Warn("[memory] failed to delete from MySQL")
		}
	}
	return nil
}

// FormatForDebug formats memory for debugging purposes
func (ms *MemoryService) FormatForDebug(mem *Memory) string {
	return fmt.Sprintf("User ID: %s\nProfile: %s\nPreferences: %s\nFacts: %v\nRecentSummary: %s\nLast Updated: %s",
		mem.UserID, mem.Profile, mem.Preferences, mem.Facts, mem.RecentSummary, mem.UpdatedAt)
}
