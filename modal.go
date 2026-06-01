package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) openToolPermissionModal(ev runtimeEvent) model {
	fields := []modalField{
		{Label: "Tool", Value: fallback(ev.ToolName, "unknown")},
	}
	if len(ev.Args) > 0 {
		fields = append(fields, modalField{Label: "Args", Value: compactJSON(ev.Args, 900)})
	}
	if modalDevDetailsEnabled() {
		fields = append(fields,
			modalField{Label: "Tool call", Value: fallback(short(ev.ToolCallID, 36), "unknown")},
			modalField{Label: "Session", Value: fallback(short(ev.SessionID, 36), "unknown")},
		)
		if ev.Step > 0 {
			fields = append(fields, modalField{Label: "Step", Value: fmt.Sprintf("%d", ev.Step)})
		}
		if len(ev.Metadata) > 0 {
			fields = append(fields, modalField{Label: "Metadata", Value: compactJSON(ev.Metadata, 500)})
		}
	}

	m.previousMode = m.mode
	m.mode = modeModal
	m.modal = &modalState{
		ID:          ev.ID,
		Kind:        modalKindPermission,
		Title:       "Permission required",
		Description: fallback(ev.Description, "Qubit wants to run a tool."),
		Fields:      fields,
		Actions: []modalAction{
			{ID: "allow", Label: "Allow", Style: "primary", Default: true},
			{ID: "deny", Label: "Deny", Style: "danger"},
		},
		Payload: map[string]any{
			"type":       "tool.permission.request",
			"toolName":   ev.ToolName,
			"toolCallId": ev.ToolCallID,
			"sessionId":  ev.SessionID,
			"step":       ev.Step,
		},
	}
	m.status = "permission required"
	return m
}

func (m model) openDemoPermissionModal() model {
	return m.openToolPermissionModal(runtimeEvent{
		Type:        "tool.permission.request",
		ID:          "demo_permission",
		SessionID:   m.session,
		Step:        1,
		ToolCallID:  "demo-tool-call-1",
		ToolName:    "write_file",
		Description: "Demo modal: approve or deny a pretend filesystem write.",
		Args: map[string]any{
			"path":    "D:\\qubit\\demo.txt",
			"content": "hello from the reusable modal layer",
		},
		Metadata: map[string]any{
			"category": "filesystem",
			"severity": "high",
		},
	})
}

func (m model) updateModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modal == nil {
		m.mode = modeChat
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.resolveModalAction("deny")
	case "left", "shift+tab", "up", "ctrl+p":
		m.moveModalCursor(-1)
		return m, nil
	case "right", "tab", "down", "ctrl+n":
		m.moveModalCursor(1)
		return m, nil
	case "enter":
		return m.resolveModalAction(m.selectedModalActionID())
	}
	return m, nil
}

func (m *model) moveModalCursor(delta int) {
	if m.modal == nil || len(m.modal.Actions) == 0 {
		return
	}
	m.modal.Cursor = (m.modal.Cursor + delta + len(m.modal.Actions)) % len(m.modal.Actions)
}

func (m model) selectedModalActionID() string {
	if m.modal == nil || len(m.modal.Actions) == 0 {
		return ""
	}
	if m.modal.Cursor < 0 || m.modal.Cursor >= len(m.modal.Actions) {
		return m.modal.Actions[0].ID
	}
	return m.modal.Actions[m.modal.Cursor].ID
}

func (m model) resolveModalAction(actionID string) (tea.Model, tea.Cmd) {
	modal := m.modal
	if modal == nil {
		m.mode = modeChat
		return m, nil
	}

	m.modal = nil
	if m.previousMode == modeModal {
		m.mode = modeChat
	} else {
		m.mode = m.previousMode
	}
	m.previousMode = modeChat

	if modal.Kind == modalKindPermission {
		allow := actionID == "allow"
		if modal.ID == "demo_permission" {
			if allow {
				m.appendSystem("Demo permission allowed.")
			} else {
				m.appendSystem("Demo permission denied.")
			}
			m.status = "ready"
			return m, nil
		}

		payload := map[string]any{
			"type":  "tool.permission.response",
			"id":    modal.ID,
			"allow": allow,
		}
		if !allow {
			payload["reason"] = "Denied by user."
		}
		m.status = "thinking"
		return m, sendRuntime(m.runtime, payload)
	}

	m.status = "ready"
	return m, nil
}

func compactJSON(v any, maxLen int) string {
	if v == nil {
		return ""
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return oneLine(fmt.Sprintf("%v", v), maxLen)
	}
	return truncateRunes(string(data), maxLen)
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxLen {
		return string(runes)
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

func truncateModalLines(s string, width int, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	wrapped := wrap(s, width)
	lines := strings.Split(wrapped, "\n")
	if len(lines) <= maxLines {
		return wrapped
	}
	return strings.Join(append(lines[:maxLines], "…"), "\n")
}

func modalDevDetailsEnabled() bool {
	return os.Getenv("QUBIT_MODAL_DEV") == "1" || os.Getenv("QUBIT_DEV") == "1"
}
