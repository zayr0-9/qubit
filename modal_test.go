package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestOpenToolPermissionModal(t *testing.T) {
	m := model{mode: modeChat, session: "sess_1234567890"}
	m = m.openToolPermissionModal(runtimeEvent{
		ID:          "perm_1",
		SessionID:   "sess_1234567890",
		Step:        2,
		ToolCallID:  "call_1234567890",
		ToolName:    "write_file",
		Description: "Writes a file.",
		Args:        map[string]any{"path": "demo.txt"},
	})

	if m.mode != modeModal {
		t.Fatalf("mode = %v, want modeModal", m.mode)
	}
	if m.modal == nil {
		t.Fatal("modal is nil")
	}
	if m.modal.Kind != modalKindPermission {
		t.Fatalf("modal kind = %q, want %q", m.modal.Kind, modalKindPermission)
	}
	if m.modal.ID != "perm_1" {
		t.Fatalf("modal id = %q, want perm_1", m.modal.ID)
	}
	if len(m.modal.Actions) != 2 || m.modal.Actions[0].ID != "allow" || m.modal.Actions[1].ID != "deny" {
		t.Fatalf("actions = %#v, want allow/deny", m.modal.Actions)
	}
	if !modalHasField(m.modal, "Tool", "write_file") {
		t.Fatalf("modal fields missing tool: %#v", m.modal.Fields)
	}
}

func TestMoveModalCursorWraps(t *testing.T) {
	m := model{modal: &modalState{Actions: []modalAction{{ID: "allow"}, {ID: "deny"}}}}

	m.moveModalCursor(-1)
	if got := m.selectedModalActionID(); got != "deny" {
		t.Fatalf("selected action after -1 = %q, want deny", got)
	}
	m.moveModalCursor(1)
	if got := m.selectedModalActionID(); got != "allow" {
		t.Fatalf("selected action after +1 = %q, want allow", got)
	}
}

func TestDemoPermissionModalResolvesAllowAndDeny(t *testing.T) {
	allowModel := model{mode: modeChat}.openDemoPermissionModal()
	allowUpdated, cmd := allowModel.resolveModalAction("allow")
	if cmd != nil {
		t.Fatal("demo allow returned command, want nil")
	}
	allow := allowUpdated.(model)
	if allow.mode != modeChat || allow.modal != nil {
		t.Fatalf("allow modal not closed: mode=%v modal=%#v", allow.mode, allow.modal)
	}
	if len(allow.messages) == 0 || !strings.Contains(allow.messages[len(allow.messages)-1].Content, "allowed") {
		t.Fatalf("allow did not append allowed message: %#v", allow.messages)
	}

	denyModel := model{mode: modeChat}.openDemoPermissionModal()
	denyUpdated, cmd := denyModel.updateModal(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("demo deny returned command, want nil")
	}
	deny := denyUpdated.(model)
	if deny.mode != modeChat || deny.modal != nil {
		t.Fatalf("deny modal not closed: mode=%v modal=%#v", deny.mode, deny.modal)
	}
	if len(deny.messages) == 0 || !strings.Contains(deny.messages[len(deny.messages)-1].Content, "denied") {
		t.Fatalf("deny did not append denied message: %#v", deny.messages)
	}
}

func TestCompactJSONTruncates(t *testing.T) {
	got := compactJSON(map[string]any{"long": strings.Repeat("x", 100)}, 30)
	if len([]rune(got)) > 30 {
		t.Fatalf("compactJSON length = %d, want <= 30", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("compactJSON = %q, want ellipsis suffix", got)
	}
}

func modalHasField(modal *modalState, label string, contains string) bool {
	for _, field := range modal.Fields {
		if field.Label == label && strings.Contains(field.Value, contains) {
			return true
		}
	}
	return false
}
