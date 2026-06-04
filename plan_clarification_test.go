package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func clarificationEvent() runtimeEvent {
	return runtimeEvent{
		Type: "plan.clarification.request",
		ID:   "clarify_1",
		Questions: []planClarificationQuestion{
			{ID: "scope", Question: "Which scope should the plan cover?", Options: []planClarificationOption{
				{ID: "ui", Label: "Go UI only"},
				{ID: "all", Label: "UI and runtime"},
			}},
			{ID: "detail", Question: "Any extra detail?", Options: []planClarificationOption{
				{ID: "brief", Label: "Keep it brief"},
				{ID: "manual", Label: "None of the above — tell Qubit what to do instead", Manual: true},
			}},
		},
	}
}

func TestPlanClarificationRequestOpensBottomOverlay(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	updated, cmd := m.updateRuntime(clarificationEvent())
	got := updated.(model)
	if cmd == nil {
		t.Fatal("clarification request returned nil command, want waitRuntimeEvent")
	}
	if got.mode != modeChat {
		t.Fatalf("mode = %v, want chat bottom overlay", got.mode)
	}
	if !got.hasPlanClarification() {
		t.Fatal("plan clarification not active")
	}
	view := plainText(got.View().Content)
	if !strings.Contains(view, "clarification · 1/2") || !strings.Contains(view, "Which scope should the plan cover?") || !strings.Contains(view, "Go UI only") {
		t.Fatalf("view missing clarification overlay:\n%s", view)
	}
}

func TestPlanClarificationAnswersMultipleQuestions(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt).openPlanClarification(clarificationEvent())

	updated, cmd := m.updatePlanClarificationKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("first answer returned command before final question, want nil")
	}
	m = updated.(model)
	if m.planClarification.Index != 1 || len(m.planClarification.Answers) != 1 {
		t.Fatalf("state after first answer = %#v", m.planClarification)
	}

	updated, cmd = m.updatePlanClarificationKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil")
	}
	m = updated.(model)
	m.planClarification.Manual.InsertString("Cover tests and docs")
	updated, cmd = m.updatePlanClarificationKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.hasPlanClarification() {
		t.Fatal("clarification still active after final answer")
	}
	payload := runBatchSendCommand(t, cmd, stdin, "plan.clarification.response")
	answers, ok := payload["answers"].([]any)
	if !ok || len(answers) != 2 {
		t.Fatalf("answers = %#v, want two answers; payload=%#v", payload["answers"], payload)
	}
	second, _ := answers[1].(map[string]any)
	if second["answer"] != "Cover tests and docs" || second["manual"] != true {
		t.Fatalf("second answer = %#v, want manual answer text", second)
	}
}

func TestPlanClarificationManualComposerGrowsToThreeLines(t *testing.T) {
	m := initialModel(nil)
	m.width = 44
	m.height = 24
	m = m.openPlanClarification(runtimeEvent{ID: "clarify_manual", Questions: []planClarificationQuestion{{ID: "manual", Question: "Tell me more.", Options: []planClarificationOption{{ID: "manual", Label: "None of the above — tell Qubit what to do instead", Manual: true}}}}})

	if got := m.planClarification.Manual.Height(); got != 1 {
		t.Fatalf("initial manual height = %d, want 1", got)
	}
	m.planClarification.Manual.SetWidth(10)
	m.planClarification.Manual.InsertString("one two three four five six seven eight nine ten")
	if got := m.planClarification.Manual.Height(); got < 2 || got > 3 {
		t.Fatalf("grown manual height = %d, want 2..3", got)
	}
	m.planClarification.Manual.InsertString("\nmore\nlines\nthan\nfit")
	if got := m.planClarification.Manual.Height(); got != 3 {
		t.Fatalf("capped manual height = %d, want 3", got)
	}
}

func TestPlanClarificationEscCancelsPendingRequest(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt).openPlanClarification(clarificationEvent())

	updated, cmd := m.updatePlanClarificationKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)
	if got.hasPlanClarification() {
		t.Fatal("clarification still active after esc")
	}
	payload := runBatchSendCommand(t, cmd, stdin, "plan.clarification.response")
	if payload["cancelled"] != true {
		t.Fatalf("payload = %#v, want cancelled response", payload)
	}
}

func TestPlanClarificationReducesViewportHeightInLayout(t *testing.T) {
	base := initialModel(nil)
	base.width = 100
	base.height = 30
	base.layout()

	with := base.openPlanClarification(clarificationEvent())
	if with.viewport.Height() >= base.viewport.Height() {
		t.Fatalf("viewport height with clarification = %d, without = %d; want overlay to reserve space", with.viewport.Height(), base.viewport.Height())
	}
}
