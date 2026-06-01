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

func (m model) openProviderSelectorModal() model {
	m.previousMode = modeChat
	m.mode = modeModal
	providers := apiKeyProviderOptions()
	options := make([]modalOption, 0, len(providers))
	activeIndex := 0
	activeProvider := fallback(m.activeProvider, m.provider)
	for i, provider := range providers {
		if provider.ID == activeProvider {
			activeIndex = i
		}
		options = append(options, modalOption{ID: provider.ID, Label: provider.Label, Description: provider.Description})
	}
	m.modal = &modalState{
		ID:           "provider_selector",
		Kind:         modalKindCustom,
		Title:        "Choose provider",
		Description:  "Select the provider Qubit should use for new runs. Use Set default to remember it for future launches.",
		Options:      options,
		OptionCursor: activeIndex,
		Actions: []modalAction{
			{ID: "select", Label: "Use now", Style: "primary", Default: true},
			{ID: "default", Label: "Set default"},
			{ID: "cancel", Label: "Cancel"},
		},
		Payload: map[string]any{"action": "provider.select"},
	}
	m.busy = false
	m.status = "choose provider"
	return m
}

func (m model) openModelSelectorModal(models []modelInfo) model {
	m.previousMode = modeChat
	m.mode = modeModal
	m.models = models
	options := make([]modalOption, 0, len(models))
	activeIndex := 0
	for i, info := range models {
		label := fallback(info.Name, info.ID)
		if info.Active {
			activeIndex = i
		}
		options = append(options, modalOption{ID: info.ID, Label: label, Description: info.Description})
	}
	if len(options) == 0 {
		options = []modalOption{{ID: fallback(m.model, "glm-4.6"), Label: fallback(m.model, "glm-4.6"), Description: "Current runtime model"}}
	}
	title := "Choose model"
	if m.activeProvider != "" {
		title = fmt.Sprintf("Choose %s model", m.activeProvider)
	}
	m.modal = &modalState{
		ID:          "model_selector",
		Kind:        modalKindCustom,
		Title:       title,
		Description: "Select the model Qubit should use for new runs. Use Set default to remember it for this provider.",
		Options:     options,
		Actions: []modalAction{
			{ID: "select", Label: "Use now", Style: "primary", Default: true},
			{ID: "default", Label: "Set default"},
			{ID: "cancel", Label: "Cancel"},
		},
		OptionCursor: activeIndex,
		Payload:      map[string]any{"action": "model.select"},
	}
	m.busy = false
	m.status = "choose model"
	return m
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
		if len(m.modal.Options) > 0 {
			return m.resolveModalAction("cancel")
		}
		return m.resolveModalAction("deny")
	case "up", "ctrl+p":
		if len(m.modal.Options) > 0 {
			m.moveModalOptionCursor(-1)
		} else {
			m.moveModalCursor(-1)
		}
		return m, nil
	case "down", "ctrl+n":
		if len(m.modal.Options) > 0 {
			m.moveModalOptionCursor(1)
		} else {
			m.moveModalCursor(1)
		}
		return m, nil
	case "left", "shift+tab":
		m.moveModalCursor(-1)
		return m, nil
	case "right", "tab":
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

func (m *model) moveModalOptionCursor(delta int) {
	if m.modal == nil || len(m.modal.Options) == 0 {
		return
	}
	m.modal.OptionCursor = (m.modal.OptionCursor + delta + len(m.modal.Options)) % len(m.modal.Options)
}

func (m model) selectedModalOptionID() string {
	if m.modal == nil || len(m.modal.Options) == 0 {
		return ""
	}
	if m.modal.OptionCursor < 0 || m.modal.OptionCursor >= len(m.modal.Options) {
		return m.modal.Options[0].ID
	}
	return m.modal.Options[m.modal.OptionCursor].ID
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

	if modal.Kind == modalKindConfirm {
		if modal.Payload["action"] == "key.delete" {
			if actionID != "delete" {
				m.status = "delete cancelled"
				return m, nil
			}
			provider, _ := modal.Payload["provider"].(string)
			alias, _ := modal.Payload["alias"].(string)
			m.busy = true
			m.status = "deleting api key"
			return m, sendRuntime(m.runtime, map[string]any{"type": "key.delete", "provider": provider, "alias": alias})
		}
	}

	if modal.Payload["action"] == "model.select" {
		if actionID == "select" || actionID == "default" {
			selected := modalSelectedOption(modal)
			m.busy = true
			payload := map[string]any{"type": "model.use", "model": selected.ID}
			if actionID == "default" {
				payload["persistDefault"] = true
				m.status = "saving default model"
			} else {
				m.status = "switching model"
			}
			return m, sendRuntime(m.runtime, payload)
		}
		m.status = "model selection cancelled"
		return m, nil
	}

	if modal.Payload["action"] == "provider.select" {
		if actionID == "select" || actionID == "default" {
			selected := modalSelectedOption(modal)
			m.busy = true
			payload := map[string]any{"type": "provider.use", "provider": selected.ID}
			if actionID == "default" {
				payload["persistDefault"] = true
				m.status = "saving default provider"
			} else {
				m.status = "switching provider"
			}
			return m, sendRuntime(m.runtime, payload)
		}
		m.status = "provider selection cancelled"
		return m, nil
	}

	m.status = "ready"
	return m, nil
}

func modalSelectedOption(modal *modalState) modalOption {
	if modal == nil || len(modal.Options) == 0 {
		return modalOption{}
	}
	if modal.OptionCursor < 0 || modal.OptionCursor >= len(modal.Options) {
		return modal.Options[0]
	}
	return modal.Options[modal.OptionCursor]
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
