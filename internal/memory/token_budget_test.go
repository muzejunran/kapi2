package memory

import (
	"strings"
	"testing"
)

func TestCalculateTokens(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},         // 5 × 0.3 = 1.5 → 1
		{"你好", 3},            // 2 × 1.5 = 3
		{"hello你好", 4},       // 1.5 + 3 = 4.5 → 4
		{strings.Repeat("a", 100), 30}, // 100 × 0.3 = 30
	}
	for _, c := range cases {
		got := CalculateTokens(c.input)
		if got != c.want {
			t.Errorf("CalculateTokens(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestAddFactIfNew(t *testing.T) {
	t.Run("adds new fact", func(t *testing.T) {
		facts := addFactIfNew([]string{}, "有房贷")
		if len(facts) != 1 || facts[0] != "有房贷" {
			t.Fatalf("got %v", facts)
		}
	})

	t.Run("deduplicates exact match", func(t *testing.T) {
		facts := addFactIfNew([]string{"有房贷"}, "有房贷")
		if len(facts) != 1 {
			t.Fatalf("expected 1 fact, got %v", facts)
		}
	})

	t.Run("deduplicates case-insensitive", func(t *testing.T) {
		facts := addFactIfNew([]string{"abc"}, "ABC")
		if len(facts) != 1 {
			t.Fatalf("expected 1 fact, got %v", facts)
		}
	})

	t.Run("trims whitespace before dedup", func(t *testing.T) {
		facts := addFactIfNew([]string{"有房贷"}, "  有房贷  ")
		if len(facts) != 1 {
			t.Fatalf("expected 1 fact, got %v", facts)
		}
	})

	t.Run("ignores empty fact", func(t *testing.T) {
		facts := addFactIfNew([]string{"有房贷"}, "")
		if len(facts) != 1 {
			t.Fatalf("expected 1 fact, got %v", facts)
		}
	})

	t.Run("ignores whitespace-only fact", func(t *testing.T) {
		facts := addFactIfNew([]string{"有房贷"}, "   ")
		if len(facts) != 1 {
			t.Fatalf("expected 1 fact, got %v", facts)
		}
	})

	t.Run("evicts oldest when at capacity", func(t *testing.T) {
		facts := make([]string, maxFacts)
		for i := range facts {
			facts[i] = strings.Repeat("x", i+1)
		}
		oldest := facts[0]
		facts = addFactIfNew(facts, "brand new fact")
		if len(facts) != maxFacts {
			t.Fatalf("expected %d facts, got %d", maxFacts, len(facts))
		}
		for _, f := range facts {
			if f == oldest {
				t.Fatal("oldest fact should have been evicted")
			}
		}
		if facts[len(facts)-1] != "brand new fact" {
			t.Fatalf("new fact not appended: %v", facts)
		}
	})
}
