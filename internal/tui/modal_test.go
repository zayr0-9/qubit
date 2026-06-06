package tui

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
	if len(m.modal.Actions) != 3 || m.modal.Actions[0].ID != "allow" || m.modal.Actions[1].ID != "allow_all" || m.modal.Actions[2].ID != "deny" {
		t.Fatalf("actions = %#v, want allow/allow_all/deny", m.modal.Actions)
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

func TestMoveModalOptionCursorWraps(t *testing.T) {
	m := model{modal: &modalState{Options: []modalOption{{ID: "alpha", Label: "Alpha"}, {ID: "beta", Label: "Beta"}}}}

	m.moveModalOptionCursor(-1)
	if got := m.selectedModalOptionID(); got != "beta" {
		t.Fatalf("selected option after -1 = %q, want beta", got)
	}
	m.moveModalOptionCursor(1)
	if got := m.selectedModalOptionID(); got != "alpha" {
		t.Fatalf("selected option after +1 = %q, want alpha", got)
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

func TestSlashSelectionRequestsModelList(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.composer.SetValue("/models")

	updated, cmd := m.acceptSlashSelection()
	got := updated.(model)
	if got.mode != modeModal {
		t.Fatalf("mode = %v, want modeModal while loading model selector", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading models")
	}
	if got.composer.Value() != "" {
		t.Fatalf("composer value = %q, want reset after opening selector", got.composer.Value())
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "model.list", "")
}

func TestModelSelectorSelectsModel(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeChat, runtime: rt}.openModelSelectorModal([]modelInfo{
		{ID: "glm-4.6", Name: "glm-4.6", Description: "Default", Active: true},
		{ID: "glm-4-air", Name: "glm-4-air", Description: "Fast"},
	})

	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil")
	}
	m = updated.(model)
	if got := m.selectedModalOptionID(); got != "glm-4-air" {
		t.Fatalf("selected option = %q, want glm-4-air", got)
	}

	updated, cmd = m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeChat || got.modal != nil {
		t.Fatalf("model selector not closed: mode=%v modal=%#v", got.mode, got.modal)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while switching model")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "model.use", "")
	if payload["model"] != "glm-4-air" {
		t.Fatalf("model.use model = %#v, want glm-4-air", payload["model"])
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

func TestAskPermissionModeOpensPermissionModal(t *testing.T) {
	m := initialModel(nil)
	m.permissionMode = permissionModeAsk

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "tool.permission.request", ID: "perm_ask", ToolName: "editFile"})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("permission request returned nil command, want waitRuntimeEvent command")
	}
	if got.mode != modeModal {
		t.Fatalf("mode = %v, want modeModal", got.mode)
	}
	if got.modal == nil || got.modal.ID != "perm_ask" || got.modal.Kind != modalKindPermission {
		t.Fatalf("modal = %#v, want permission modal for perm_ask", got.modal)
	}
}

func TestAlwaysAllowPermissionModeAutoApproves(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.permissionMode = permissionModeAlwaysAllow

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "tool.permission.request", ID: "perm_auto", ToolName: "editFile"})
	got := updated.(model)

	if got.modal != nil || got.mode == modeModal {
		t.Fatalf("always-allow opened modal: mode=%v modal=%#v", got.mode, got.modal)
	}
	if got.status != "thinking" {
		t.Fatalf("status = %q, want thinking", got.status)
	}
	payload := runBatchSendCommand(t, cmd, stdin, "tool.permission.response")
	if payload["type"] != "tool.permission.response" {
		t.Fatalf("payload type = %#v, want tool.permission.response; payload=%#v", payload["type"], payload)
	}
	if payload["id"] != "perm_auto" {
		t.Fatalf("payload id = %#v, want perm_auto; payload=%#v", payload["id"], payload)
	}
	if payload["allow"] != true {
		t.Fatalf("payload allow = %#v, want true; payload=%#v", payload["allow"], payload)
	}
}

func TestSetPermissionModeSlashCommand(t *testing.T) {
	m := initialModel(nil)
	m.ready = true

	updated, cmd := m.handleSlashCommand("/permission always")
	if cmd != nil {
		t.Fatal("/permission always returned command, want nil")
	}
	got := updated.(model)
	if got.permissionMode != permissionModeAlwaysAllow {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAlwaysAllow)
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}

	updated, cmd = got.handleSlashCommand("/permission allow-all")
	if cmd != nil {
		t.Fatal("/permission allow-all returned command, want nil")
	}
	got = updated.(model)
	if got.permissionMode != permissionModeAllowAll {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAllowAll)
	}
	if got.systemPromptMode() != "plan" {
		t.Fatalf("systemPromptMode = %q, want plan", got.systemPromptMode())
	}

	updated, cmd = got.handleSlashCommand("/permission ask")
	if cmd != nil {
		t.Fatal("/permission ask returned command, want nil")
	}
	got = updated.(model)
	if got.permissionMode != permissionModeAsk {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAsk)
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}
}

func TestShiftTabCyclesPermissionMode(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.permissionMode = permissionModeAsk

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if cmd != nil {
		t.Fatal("shift+tab returned command, want nil")
	}
	got := updated.(model)
	if got.permissionMode != permissionModeAlwaysAllow {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAlwaysAllow)
	}
	if strings.Contains(got.status, "permission") {
		t.Fatalf("status = %q, should not display permission mode in title/status", got.status)
	}

	updated, cmd = got.updateKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if cmd != nil {
		t.Fatal("second shift+tab returned command, want nil")
	}
	got = updated.(model)
	if got.permissionMode != permissionModeAllowAll {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAllowAll)
	}
	if strings.Contains(got.status, "permission") {
		t.Fatalf("status = %q, should not display permission mode in title/status", got.status)
	}

	updated, cmd = got.updateKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if cmd != nil {
		t.Fatal("third shift+tab returned command, want nil")
	}
	got = updated.(model)
	if got.permissionMode != permissionModeAsk {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAsk)
	}
}

func TestRenderInputStatusShowsPermissionMode(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.permissionMode = permissionModeAlwaysAllow
	m.cwdBlockEnabled = true

	status := plainText(m.renderInputStatus())
	if strings.Contains(status, "permissions:") {
		t.Fatalf("status section = %q, want minimal mode label without prefix", status)
	}
	if !strings.Contains(status, "edit") {
		t.Fatalf("status section = %q, want current mode", status)
	}
	if strings.Contains(status, "always allow") || strings.Contains(status, "ask") {
		t.Fatalf("status section = %q, want plan/edit label only", status)
	}
	if strings.Contains(status, "cwd block") || strings.Contains(status, "cwd open") {
		t.Fatalf("status section = %q, want cwd state hidden while blocked", status)
	}
}

func TestCwdBlockSlashCommandsAndStatus(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80

	updated, cmd := m.handleSlashCommand("/cwd-remove-block")
	if cmd != nil {
		t.Fatal("/cwd-remove-block returned command, want nil")
	}
	got := updated.(model)
	if got.cwdBlockEnabled {
		t.Fatal("cwdBlockEnabled = true, want false")
	}
	status := plainText(got.renderInputStatus())
	if !strings.Contains(status, "cwd open") || !strings.Contains(status, "plan") {
		t.Fatalf("status = %q, want plan and cwd open", status)
	}

	updated, cmd = got.handleSlashCommand("/cwd-enable-block")
	if cmd != nil {
		t.Fatal("/cwd-enable-block returned command, want nil")
	}
	got = updated.(model)
	if !got.cwdBlockEnabled {
		t.Fatal("cwdBlockEnabled = false, want true")
	}
	status = plainText(got.renderInputStatus())
	if strings.Contains(status, "cwd block") || strings.Contains(status, "cwd open") {
		t.Fatalf("status = %q, want cwd state hidden while blocked", status)
	}
}

func TestModelSelectorCanPersistDefaultModel(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeChat, runtime: rt}.openModelSelectorModal([]modelInfo{
		{ID: "glm-4.6", Name: "glm-4.6", Description: "Default", Active: true},
		{ID: "glm-4-air", Name: "glm-4-air", Description: "Fast"},
	})

	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd != nil {
		t.Fatal("right returned command, want nil")
	}
	m = updated.(model)
	updated, cmd = m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeChat || got.modal != nil {
		t.Fatalf("model selector not closed: mode=%v modal=%#v", got.mode, got.modal)
	}
	if got.status != "saving default model" {
		t.Fatalf("status = %q, want saving default model", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "model.use", "")
	if payload["model"] != "glm-4.6" {
		t.Fatalf("model.use model = %#v, want glm-4.6", payload["model"])
	}
	if payload["persistDefault"] != true {
		t.Fatalf("persistDefault = %#v, want true", payload["persistDefault"])
	}
}

func TestProviderSelectorCanPersistDefaultProvider(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.activeProvider = "codex"
	m = m.openProviderSelectorModal()

	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd != nil {
		t.Fatal("right returned command, want nil")
	}
	m = updated.(model)
	updated, cmd = m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeChat || got.modal != nil {
		t.Fatalf("provider selector not closed: mode=%v modal=%#v", got.mode, got.modal)
	}
	if got.status != "saving default provider" {
		t.Fatalf("status = %q, want saving default provider", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "provider.use", "")
	if payload["provider"] != "codex" {
		t.Fatalf("provider.use provider = %#v, want codex", payload["provider"])
	}
	if payload["persistDefault"] != true {
		t.Fatalf("persistDefault = %#v, want true", payload["persistDefault"])
	}
}

func TestRenderModalKeepsSelectedOptionVisible(t *testing.T) {
	m := model{width: 80, height: 12, mode: modeModal, modal: &modalState{
		Title:        "Choose item",
		Description:  "Pick one.",
		OptionCursor: 8,
		Options: []modalOption{
			{ID: "one", Label: "Option one"},
			{ID: "two", Label: "Option two"},
			{ID: "three", Label: "Option three"},
			{ID: "four", Label: "Option four"},
			{ID: "five", Label: "Option five"},
			{ID: "six", Label: "Option six"},
			{ID: "seven", Label: "Option seven"},
			{ID: "eight", Label: "Option eight"},
			{ID: "nine", Label: "Option nine"},
			{ID: "ten", Label: "Option ten"},
		},
		Actions: []modalAction{{ID: "select", Label: "Select"}},
	}}

	rendered := plainText(m.renderModal(8))
	if !strings.Contains(rendered, "Option nine") {
		t.Fatalf("rendered modal does not include selected option:\n%s", rendered)
	}
	if strings.Contains(rendered, "Option one") {
		t.Fatalf("rendered modal includes hidden first option:\n%s", rendered)
	}
	if !strings.Contains(rendered, "more above") {
		t.Fatalf("rendered modal missing above hint:\n%s", rendered)
	}
}

func TestPlanModeAutoApprovesPlanModeExceptionTools(t *testing.T) {
	for _, toolName := range []string{"planMd", "subagent"} {
		t.Run(toolName, func(t *testing.T) {
			rt, stdin := newTestRuntime(t)
			m := initialModel(rt)
			m.permissionMode = permissionModeAsk
			permissionID := "perm_" + toolName

			updated, cmd := m.updateRuntime(runtimeEvent{Type: "tool.permission.request", ID: permissionID, ToolName: toolName})
			got := updated.(model)
			if got.modal != nil || got.mode == modeModal {
				t.Fatalf("%s opened modal in plan mode: mode=%v modal=%#v", toolName, got.mode, got.modal)
			}
			payload := runBatchSendCommand(t, cmd, stdin, "tool.permission.response")
			if payload["id"] != permissionID || payload["allow"] != true {
				t.Fatalf("payload = %#v, want allow response for %s", payload, permissionID)
			}
		})
	}
}

func TestPlanModeAutoApprovesPlanScopedEditFile(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.permissionMode = permissionModeAsk

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "tool.permission.request", ID: "perm_plan_edit", ToolName: "editFile", Metadata: map[string]any{"planModeAutoAllowProjectPlansOnly": true}})
	got := updated.(model)
	if got.modal != nil || got.mode == modeModal {
		t.Fatalf("plan-scoped editFile opened modal in plan mode: mode=%v modal=%#v", got.mode, got.modal)
	}
	payload := runBatchSendCommand(t, cmd, stdin, "tool.permission.response")
	if payload["id"] != "perm_plan_edit" || payload["allow"] != true {
		t.Fatalf("payload = %#v, want allow response for perm_plan_edit", payload)
	}
}

func TestPlanModeStillPromptsForUnscopedEditFile(t *testing.T) {
	m := initialModel(nil)
	m.permissionMode = permissionModeAsk

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "tool.permission.request", ID: "perm_edit", ToolName: "editFile"})
	got := updated.(model)
	if cmd == nil {
		t.Fatal("permission request returned nil command, want waitRuntimeEvent command")
	}
	if got.mode != modeModal || got.modal == nil || got.modal.ID != "perm_edit" {
		t.Fatalf("unscoped editFile did not open modal in plan mode: mode=%v modal=%#v", got.mode, got.modal)
	}
}

func TestPermissionModalAllowAllEnablesSessionAutoAllow(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m = m.openToolPermissionModal(runtimeEvent{ID: "perm_all", ToolName: "editFile"})

	updated, cmd := m.resolveModalAction("allow_all")
	got := updated.(model)
	if got.permissionMode != permissionModeAllowAll {
		t.Fatalf("permissionMode = %q, want %q", got.permissionMode, permissionModeAllowAll)
	}
	payload := runBatchSendCommand(t, cmd, stdin, "tool.permission.response")
	if payload["id"] != "perm_all" || payload["allow"] != true {
		t.Fatalf("payload = %#v, want allow response for perm_all", payload)
	}
}

func TestSlashSubagentsRequestsConfig(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.composer.SetValue("/subagents")

	updated, cmd := m.acceptSlashSelection()
	got := updated.(model)
	if got.mode != modeModal || !got.busy {
		t.Fatalf("mode=%v busy=%v, want loading modal state", got.mode, got.busy)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "subagent.config", "")
}

func TestSubagentModelSelectorSendsSubagentModelUse(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeChat, runtime: rt, subagentProvider: "openai", subagentModel: "gpt-5.2"}.openSubagentModelSelectorModal([]modelInfo{
		{ID: "gpt-5.2", Name: "GPT-5.2", Active: true},
		{ID: "gpt-5-mini", Name: "GPT-5 mini"},
	})
	updated, _ := m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(model)
	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeChat || got.modal != nil || !got.busy {
		t.Fatalf("mode=%v modal=%#v busy=%v, want closed busy", got.mode, got.modal, got.busy)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "subagent.model.use", "")
	if payload["model"] != "gpt-5-mini" {
		t.Fatalf("model = %#v, want gpt-5-mini", payload["model"])
	}
}

func TestSubagentProviderSelectorSendsSubagentProviderUse(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.subagentProvider = "codex"
	m = m.openSubagentProviderSelectorModal()
	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeChat || got.modal != nil || !got.busy {
		t.Fatalf("mode=%v modal=%#v busy=%v, want closed busy", got.mode, got.modal, got.busy)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "subagent.provider.use", "")
	if payload["provider"] != "codex" {
		t.Fatalf("provider = %#v, want codex", payload["provider"])
	}
}

func TestPermissionModalShowsFullArgsWithScrollableContent(t *testing.T) {
	m := model{width: 80, height: 12, mode: modeChat}
	m = m.openToolPermissionModal(runtimeEvent{
		ID:       "perm_full_args",
		ToolName: "editFile",
		Args: map[string]any{
			"path":        "demo.txt",
			"replacement": strings.Repeat("replacement-line\n", 30),
		},
	})

	args := ""
	for _, field := range m.modal.Fields {
		if field.Label == "Args" {
			args = field.Value
		}
	}
	if args == "" {
		t.Fatal("permission modal missing Args field")
	}
	if strings.HasSuffix(args, "…") || strings.Contains(args, "...") {
		t.Fatalf("args were truncated: %q", args)
	}
	if strings.Count(args, "replacement-line") != 30 {
		t.Fatalf("args did not preserve full replacement details: %q", args)
	}

	rendered := plainText(m.renderModal(8))
	if !strings.Contains(rendered, "more below") {
		t.Fatalf("rendered modal missing scroll hint:\n%s", rendered)
	}

	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil while scrolling")
	}
	m = updated.(model)
	if m.modal.ScrollOffset == 0 {
		t.Fatal("ScrollOffset = 0, want scrolled content")
	}
	rendered = plainText(m.renderModal(m.modalPanelAvailableHeight()))
	if !strings.Contains(rendered, "more above") {
		t.Fatalf("rendered modal missing above scroll hint after scrolling:\n%s", rendered)
	}
}

func TestPermissionModalFitsActionsAtBottomWhenArgsAreLarge(t *testing.T) {
	m := model{width: 80, height: 12, mode: modeChat}
	m = m.openToolPermissionModal(runtimeEvent{
		ID:       "perm_large_args",
		ToolName: "subagent",
		Args: map[string]any{
			"tasks": []map[string]any{{"prompt": strings.Repeat("find things\n", 40)}},
		},
	})

	rendered := plainText(m.renderModal(8))
	if !strings.Contains(rendered, "Allow") || !strings.Contains(rendered, "Deny") {
		t.Fatalf("rendered modal clipped permission actions:\n%s", rendered)
	}
	if !strings.Contains(rendered, "more below") {
		t.Fatalf("rendered modal missing scroll hint:\n%s", rendered)
	}
}
