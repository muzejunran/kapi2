package skillserver

import (
	"testing"
	"testing/fstest"
)

var testPagesJSON = []byte(`{
  "*":           ["web_search"],
  "home":        ["add_bill", "query_bills"],
  "bills.add":   ["add_bill"],
  "bills.detail": ["query_bills", "add_bill"]
}`)

func TestMatchesPage(t *testing.T) {
	index := PageIndex{
		"*":           {"web_search"},
		"home":        {"add_bill", "query_bills"},
		"bills.add":   {"add_bill"},
		"bills.detail": {"query_bills", "add_bill"},
	}
	cases := []struct {
		skillID string
		page    string
		want    bool
	}{
		{"web_search", "anything", true},      // global via "*"
		{"add_bill", "home", true},
		{"query_bills", "home", true},
		{"add_bill", "bills.add", true},
		{"add_bill", "bills.detail", true},    // the bug-fix scenario
		{"query_bills", "bills.detail", true},
		{"add_bill", "budget.index", false},   // not available on budget pages
		{"query_bills", "bills.add", false},
		{"unknown", "home", false},
	}
	for _, c := range cases {
		got := MatchesPage(index, c.skillID, c.page)
		if got != c.want {
			t.Errorf("MatchesPage(index, %q, %q) = %v, want %v", c.skillID, c.page, got, c.want)
		}
	}
}

func TestLoadPages(t *testing.T) {
	fs := fstest.MapFS{
		"configs/pages.json": &fstest.MapFile{Data: testPagesJSON},
	}
	index, err := LoadPages(fs, "configs")
	if err != nil {
		t.Fatalf("LoadPages error: %v", err)
	}
	if len(index["*"]) != 1 || index["*"][0] != "web_search" {
		t.Errorf("expected [web_search] under *, got %v", index["*"])
	}
	if len(index["home"]) != 2 {
		t.Errorf("expected 2 skills under home, got %v", index["home"])
	}
	if len(index["bills.detail"]) != 2 {
		t.Errorf("expected 2 skills under bills.detail, got %v", index["bills.detail"])
	}
}

func TestLoadPages_MissingFile(t *testing.T) {
	_, err := LoadPages(fstest.MapFS{}, "configs")
	if err == nil {
		t.Error("expected error for missing pages.json, got nil")
	}
}

func TestLoadPages_InvalidJSON(t *testing.T) {
	fs := fstest.MapFS{
		"configs/pages.json": &fstest.MapFile{Data: []byte("{ invalid")},
	}
	_, err := LoadPages(fs, "configs")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

var testSkillJSON = []byte(`{
  "id": "billing",
  "name": "记账",
  "layer": "org",
  "supported_pages": ["home", "bills.add"],
  "tools": [
    {
      "name": "add_bill",
      "description": "记录账单",
      "action_type": "return_direct",
      "return_template": "已记录 {{.amount}} 元",
      "parameters": {"type": "object", "properties": {}, "required": []}
    }
  ]
}`)

var testBudgetJSON = []byte(`{
  "id": "budget_advisor",
  "name": "预算",
  "layer": "org",
  "supported_pages": ["budget.index"],
  "tools": [
    {
      "name": "query_budget",
      "description": "查询预算",
      "action_type": "llm_summary",
      "parameters": {"type": "object", "properties": {}, "required": []}
    }
  ]
}`)

func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"configs/billing.json": &fstest.MapFile{Data: testSkillJSON},
		"configs/budget.json":  &fstest.MapFile{Data: testBudgetJSON},
		"configs/pages.json":   &fstest.MapFile{Data: testPagesJSON},
		"configs/ignore.txt":   &fstest.MapFile{Data: []byte("not json")},
	}
}

func TestLoadSkills_LoadsValidFiles(t *testing.T) {
	skills, index, err := LoadSkills(newTestFS(), "configs")
	if err != nil {
		t.Fatalf("LoadSkills error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	if _, ok := index["add_bill"]; !ok {
		t.Error("add_bill not in tool index")
	}
	if _, ok := index["query_budget"]; !ok {
		t.Error("query_budget not in tool index")
	}
}

func TestLoadSkills_SkipsNonJSON(t *testing.T) {
	skills, _, err := LoadSkills(newTestFS(), "configs")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if s.ID == "" {
			t.Error("loaded a skill with empty ID (likely from ignore.txt)")
		}
	}
}

func TestLoadSkills_SkipsPagesJSON(t *testing.T) {
	skills, _, err := LoadSkills(newTestFS(), "configs")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if s.ID == "" {
			t.Errorf("pages.json was loaded as a skill (ID empty)")
		}
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills (pages.json must be skipped), got %d", len(skills))
	}
}

func TestLoadSkills_InvalidJSONSkipped(t *testing.T) {
	fs := fstest.MapFS{
		"configs/good.json":  &fstest.MapFile{Data: testSkillJSON},
		"configs/bad.json":   &fstest.MapFile{Data: []byte("{ invalid json")},
		"configs/pages.json": &fstest.MapFile{Data: testPagesJSON},
	}
	skills, _, err := LoadSkills(fs, "configs")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(skills))
	}
}
