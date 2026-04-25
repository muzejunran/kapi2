package skillserver

import (
	"testing"
	"testing/fstest"
)

func TestMatchesPage(t *testing.T) {
	cases := []struct {
		pages   []string
		page    string
		want    bool
	}{
		{[]string{}, "anything", true},           // empty = global
		{[]string{"*"}, "anything", true},         // wildcard
		{[]string{"home", "bills.add"}, "home", true},
		{[]string{"home", "bills.add"}, "bills.add", true},
		{[]string{"home"}, "budget.index", false},
		{[]string{"bills.list"}, "bills.detail", false},
	}
	for _, c := range cases {
		got := MatchesPage(c.pages, c.page)
		if got != c.want {
			t.Errorf("MatchesPage(%v, %q) = %v, want %v", c.pages, c.page, got, c.want)
		}
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

func TestLoadSkills_InvalidJSONSkipped(t *testing.T) {
	fs := fstest.MapFS{
		"configs/good.json": &fstest.MapFile{Data: testSkillJSON},
		"configs/bad.json":  &fstest.MapFile{Data: []byte("{ invalid json")},
	}
	skills, _, err := LoadSkills(fs, "configs")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(skills))
	}
}
