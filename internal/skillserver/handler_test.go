package skillserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// handlerPagesJSON uses skill IDs ("billing", "budget_advisor") — not tool names.
var handlerPagesJSON = []byte(`{
  "home":         ["billing"],
  "budget.index": ["budget_advisor"]
}`)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	fs := fstest.MapFS{
		"configs/billing.json": &fstest.MapFile{Data: testSkillJSON},
		"configs/budget.json":  &fstest.MapFile{Data: testBudgetJSON},
		"configs/pages.json":   &fstest.MapFile{Data: handlerPagesJSON},
	}
	srv, err := NewServer(fs, "configs", NewExecutor(nil, nil))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestGetSkills_NoFilter_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp GetSkillsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(resp.Tools))
	}
}

func TestGetSkills_PageFilter(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/skills?page_context=home", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp GetSkillsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Tools) != 1 || resp.Tools[0].Function.Name != "add_bill" {
		t.Errorf("expected only add_bill for page=home, got %+v", resp.Tools)
	}
}

func TestGetSkills_PageFilter_NoMatch(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/skills?page_context=unknown.page", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp GetSkillsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Tools) != 0 {
		t.Errorf("expected 0 tools for unknown page, got %d", len(resp.Tools))
	}
}

func TestExecute_KnownTool(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body, _ := json.Marshal(ExecuteRequest{
		ToolName: "query_budget",
		UserID:   "test_user",
		Args:     map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp ExecuteResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Success {
		t.Errorf("expected success=true, got error=%q", resp.Error)
	}
	if resp.ActionType != ActionLLMSummary {
		t.Errorf("expected action_type=llm_summary, got %q", resp.ActionType)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body, _ := json.Marshal(ExecuteRequest{ToolName: "nonexistent", UserID: "u"})
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp ExecuteResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected success=false for unknown tool")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestExecute_InvalidBody(t *testing.T) {
	srv := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp ExecuteResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected failure on invalid body")
	}
}
