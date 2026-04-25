package agent

import (
	"strings"
	"testing"
	"time"

	"ai-assistant-service/internal/memory"
)

// ── buildSystemPrompt ─────────────────────────────────────────────────────────

func newAgent() *Agent {
	return &Agent{config: AgentConfig{ModelName: "test-model"}}
}

func TestBuildSystemPrompt_EmptyMemory(t *testing.T) {
	a := newAgent()
	state := &AgentState{
		PageContext: "home",
		Memory:     &memory.Memory{},
	}
	prompt := a.buildSystemPrompt(state)
	if !strings.Contains(prompt, "KaPi") {
		t.Error("prompt should mention KaPi")
	}
	if !strings.Contains(prompt, "home") {
		t.Error("prompt should include page context")
	}
	// no memory sections when empty
	if strings.Contains(prompt, "[用户画像]") {
		t.Error("should not include empty profile section")
	}
}

func TestBuildSystemPrompt_FullMemory(t *testing.T) {
	a := newAgent()
	state := &AgentState{
		PageContext: "budget.index",
		Memory: &memory.Memory{
			Profile:       "30岁程序员",
			Preferences:   "简洁回答",
			Facts:         []string{"有房贷", "在减肥"},
			RecentSummary: "上次讨论了预算设置",
		},
	}
	prompt := a.buildSystemPrompt(state)
	for _, want := range []string{
		"[用户画像]", "30岁程序员",
		"[偏好]", "简洁回答",
		"[关键事实]", "有房贷", "在减肥",
		"[近期对话摘要]", "上次讨论了预算设置",
		"budget.index",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildSystemPrompt_ContainsToday(t *testing.T) {
	a := newAgent()
	state := &AgentState{Memory: &memory.Memory{}}
	prompt := a.buildSystemPrompt(state)
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(prompt, today) {
		t.Errorf("prompt should contain today's date %q", today)
	}
}

// ── stepStartMsg ──────────────────────────────────────────────────────────────

func TestStepStartMsg_KnownTools(t *testing.T) {
	cases := map[string]string{
		"add_bill":     "记录账单",
		"query_bills":  "查询账单",
		"query_budget": "查询预算",
		"update_budget": "更新预算",
	}
	for tool, want := range cases {
		msg := stepStartMsg(tool)
		if !strings.Contains(msg, want) {
			t.Errorf("stepStartMsg(%q) = %q, want to contain %q", tool, msg, want)
		}
	}
}

func TestStepStartMsg_UnknownTool(t *testing.T) {
	msg := stepStartMsg("some_custom_tool")
	if !strings.Contains(msg, "some_custom_tool") {
		t.Errorf("stepStartMsg for unknown tool should include tool name, got %q", msg)
	}
}

// ── stepDoneMsg ───────────────────────────────────────────────────────────────

func TestStepDoneMsg_QueryBills_WithCount(t *testing.T) {
	msg := stepDoneMsg("query_bills", map[string]interface{}{"count": float64(7)})
	if !strings.Contains(msg, "7") {
		t.Errorf("stepDoneMsg should contain count, got %q", msg)
	}
}

func TestStepDoneMsg_QueryBudget_WithBudgets(t *testing.T) {
	msg := stepDoneMsg("query_budget", map[string]interface{}{
		"budgets": []interface{}{"a", "b", "c"},
	})
	if !strings.Contains(msg, "3") {
		t.Errorf("stepDoneMsg should contain budget count, got %q", msg)
	}
}

func TestStepDoneMsg_UpdateBudget(t *testing.T) {
	msg := stepDoneMsg("update_budget", map[string]interface{}{
		"category":   "餐饮",
		"new_amount": float64(1200),
	})
	if !strings.Contains(msg, "餐饮") || !strings.Contains(msg, "1200") {
		t.Errorf("stepDoneMsg should contain category and amount, got %q", msg)
	}
}

func TestStepDoneMsg_Unknown(t *testing.T) {
	msg := stepDoneMsg("unknown_tool", map[string]interface{}{})
	if msg == "" {
		t.Error("stepDoneMsg should return non-empty string for unknown tool")
	}
}
