package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"plain text", "plain text"},
		{"```json\n{\"k\":1}\n```", `{"k":1}`},
		{"```\n{\"k\":1}\n```", `{"k":1}`},
		{"  ```json\n{}\n```  ", "{}"},
		// no closing fence — still strips opening
		{"```json\n{}", "{}"},
	}
	for _, c := range cases {
		got := stripCodeFence(c.input)
		if got != c.want {
			t.Errorf("stripCodeFence(%q)\n  got  %q\n  want %q", c.input, got, c.want)
		}
	}
}

func TestExtractContent(t *testing.T) {
	t.Run("returns content when present", func(t *testing.T) {
		m := llmMessage{Content: "hello", ReasoningContent: "reason"}
		if got := extractContent(m); got != "hello" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("falls back to reasoning_content when content empty", func(t *testing.T) {
		m := llmMessage{Content: "", ReasoningContent: "from reasoning"}
		if got := extractContent(m); got != "from reasoning" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("strips code fence from content", func(t *testing.T) {
		m := llmMessage{Content: "```json\n{}\n```"}
		if got := extractContent(m); got != "{}" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("returns empty when both fields empty", func(t *testing.T) {
		m := llmMessage{}
		if got := extractContent(m); got != "" {
			t.Errorf("got %q", got)
		}
	})
}

func TestAnalyze_MockServer(t *testing.T) {
	payload := `{"profile_update":"程序员","preference_update":null,"new_facts":["在减肥"],"summary":"用户是程序员"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": payload, "reasoning_content": ""}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewMemoryDeterminer(srv.URL, "test-key", "test-model")
	result, err := d.Analyze(context.Background(), "我是程序员在减肥", "好的，我记住了")
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if result.ProfileUpdate == nil || *result.ProfileUpdate != "程序员" {
		t.Errorf("unexpected ProfileUpdate: %v", result.ProfileUpdate)
	}
	if len(result.NewFacts) != 1 || result.NewFacts[0] != "在减肥" {
		t.Errorf("unexpected NewFacts: %v", result.NewFacts)
	}
	if result.Summary != "用户是程序员" {
		t.Errorf("unexpected Summary: %q", result.Summary)
	}
}

func TestAnalyze_FallbackOnBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "not json at all", "reasoning_content": ""}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewMemoryDeterminer(srv.URL, "test-key", "test-model")
	result, err := d.Analyze(context.Background(), "hi", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "not json at all" {
		t.Errorf("expected raw content as summary fallback, got %q", result.Summary)
	}
}
