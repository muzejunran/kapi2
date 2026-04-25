package memory

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// fakeKVStore is an in-memory KVStore for testing.
type fakeKVStore struct {
	data map[string][]byte
}

func newFakeKVStore() *fakeKVStore {
	return &fakeKVStore{data: make(map[string][]byte)}
}

func (f *fakeKVStore) Get(key string) ([]byte, error) {
	v, ok := f.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (f *fakeKVStore) Set(key string, value []byte, _ time.Duration) error {
	f.data[key] = value
	return nil
}

func (f *fakeKVStore) Delete(key string) error {
	delete(f.data, key)
	return nil
}

func newTestService() *MemoryService {
	return NewMemoryService(newFakeKVStore(), 1500)
}

// ── CalculateContextTokens ────────────────────────────────────────────────────

func TestCalculateContextTokens(t *testing.T) {
	ms := newTestService()
	mem := &Memory{
		Profile:       "程序员",        // ~4 tokens
		Preferences:   "简洁回答",       // ~6 tokens
		Facts:         []string{"在减肥"}, // ~3 tokens
		RecentSummary: "用户是程序员",     // ~6 tokens
	}
	got := ms.CalculateContextTokens(mem)
	if got <= 0 {
		t.Fatalf("expected positive token count, got %d", got)
	}
}

// ── TrimToBudget ──────────────────────────────────────────────────────────────

func TestTrimToBudget_WithinBudget(t *testing.T) {
	ms := NewMemoryService(newFakeKVStore(), 10000)
	mem := &Memory{Profile: "short", Preferences: "short", Facts: []string{"f1"}, RecentSummary: "short"}
	trimmed := ms.TrimToBudget(mem)
	if trimmed.Profile != mem.Profile || trimmed.Preferences != mem.Preferences {
		t.Error("should not trim when within budget")
	}
}

func TestTrimToBudget_DropsInPriorityOrder(t *testing.T) {
	// Budget so tight that only RecentSummary fits
	longText := "这是一段很长很长的文本用来撑满token预算让系统不得不裁剪内容以满足要求"
	ms := NewMemoryService(newFakeKVStore(), 5)
	mem := &Memory{
		Profile:       longText,
		Preferences:   longText,
		Facts:         []string{longText},
		RecentSummary: longText,
	}
	trimmed := ms.TrimToBudget(mem)
	// When everything is too long the trimmer drops all fields
	if trimmed.Profile != "" {
		t.Error("Profile should be dropped")
	}
	if trimmed.Preferences != "" {
		t.Error("Preferences should be dropped")
	}
	if len(trimmed.Facts) != 0 {
		t.Error("Facts should be dropped")
	}
}

func TestTrimToBudget_DropsProfileFirst(t *testing.T) {
	longProfile := "很长的用户画像" + "x很长的用户画像x"
	ms := NewMemoryService(newFakeKVStore(), 3)
	mem := &Memory{
		Profile:       longProfile,
		Preferences:   "",
		Facts:         []string{},
		RecentSummary: "",
	}
	trimmed := ms.TrimToBudget(mem)
	if trimmed.Profile != "" {
		t.Error("Profile should be dropped first")
	}
}

// ── GetUserMemory ─────────────────────────────────────────────────────────────

func TestGetUserMemory_CacheMiss_ReturnsEmpty(t *testing.T) {
	ms := newTestService()
	mem, err := ms.GetUserMemory("new_user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.UserID != "new_user" {
		t.Errorf("expected user_id=new_user, got %q", mem.UserID)
	}
	if mem.Profile != "" || mem.Preferences != "" || len(mem.Facts) != 0 {
		t.Error("new user should have empty memory")
	}
}

func TestGetUserMemory_CacheHit(t *testing.T) {
	store := newFakeKVStore()
	ms := NewMemoryService(store, 1500)

	stored := &Memory{
		UserID:  "user1",
		Profile: "程序员",
		Facts:   []string{"在减肥"},
	}
	data, _ := json.Marshal(stored)
	store.Set("memory:user1", data, 0)

	mem, err := ms.GetUserMemory("user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.Profile != "程序员" {
		t.Errorf("expected profile from cache, got %q", mem.Profile)
	}
	if len(mem.Facts) != 1 || mem.Facts[0] != "在减肥" {
		t.Errorf("unexpected facts: %v", mem.Facts)
	}
}

// ── AddFact / RemoveFact ────────────────────��─────────────────────────────────

func TestAddFact_DeduplicatesAndPersists(t *testing.T) {
	ms := newTestService()
	if err := ms.AddFact("u1", "有房贷"); err != nil {
		t.Fatal(err)
	}
	if err := ms.AddFact("u1", "有房贷"); err != nil { // duplicate
		t.Fatal(err)
	}
	mem, _ := ms.GetUserMemory("u1")
	if len(mem.Facts) != 1 {
		t.Fatalf("expected 1 fact after dedup, got %d: %v", len(mem.Facts), mem.Facts)
	}
}

func TestRemoveFact_ValidIndex(t *testing.T) {
	ms := newTestService()
	ms.AddFact("u2", "事实A")
	ms.AddFact("u2", "事实B")
	if err := ms.RemoveFact("u2", 0); err != nil {
		t.Fatal(err)
	}
	mem, _ := ms.GetUserMemory("u2")
	if len(mem.Facts) != 1 || mem.Facts[0] != "事实B" {
		t.Errorf("unexpected facts after remove: %v", mem.Facts)
	}
}

func TestRemoveFact_OutOfRange(t *testing.T) {
	ms := newTestService()
	ms.AddFact("u3", "唯一事实")
	if err := ms.RemoveFact("u3", 5); err == nil {
		t.Error("expected error for out-of-range index")
	}
}

// ── ClearMemory ───────────────────────────────────────────────────────────────

func TestClearMemory(t *testing.T) {
	ms := newTestService()
	ms.AddFact("u4", "待清除事实")
	if err := ms.ClearMemory("u4"); err != nil {
		t.Fatal(err)
	}
	mem, _ := ms.GetUserMemory("u4")
	if mem.Profile != "" || len(mem.Facts) != 0 {
		t.Errorf("memory should be empty after clear, got %+v", mem)
	}
}
