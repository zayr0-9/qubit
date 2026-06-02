package main

import "testing"

func TestUpsertShiftEnterBindingAddsBinding(t *testing.T) {
	actions, changed := upsertShiftEnterBinding([]any{})
	if !changed {
		t.Fatal("expected empty actions to change")
	}
	if len(actions) != 1 {
		t.Fatalf("expected one action, got %d", len(actions))
	}
	binding, ok := actions[0].(map[string]any)
	if !ok || !isShiftEnterSendInputBinding(binding) {
		t.Fatalf("expected Shift+Enter sendInput binding, got %#v", actions[0])
	}
}

func TestUpsertShiftEnterBindingIsIdempotent(t *testing.T) {
	actions, changed := upsertShiftEnterBinding([]any{})
	if !changed {
		t.Fatal("expected initial insert to change")
	}
	updated, changed := upsertShiftEnterBinding(actions)
	if changed {
		t.Fatal("expected second upsert to be unchanged")
	}
	if len(updated) != 1 {
		t.Fatalf("expected one action after idempotent upsert, got %d", len(updated))
	}
}

func TestUpsertShiftEnterBindingReplacesWrongShiftEnterBinding(t *testing.T) {
	actions := []any{
		map[string]any{
			"command": "unbound",
			"keys":    "shift+enter",
		},
	}
	updated, changed := upsertShiftEnterBinding(actions)
	if !changed {
		t.Fatal("expected wrong binding to be replaced")
	}
	binding, ok := updated[0].(map[string]any)
	if !ok || !isShiftEnterSendInputBinding(binding) {
		t.Fatalf("expected replacement Shift+Enter sendInput binding, got %#v", updated[0])
	}
}

func TestRemoveMisplacedShiftEnterBinding(t *testing.T) {
	settings := map[string]any{
		"actions": []any{},
		"command": map[string]any{
			"action": "sendInput",
			"input":  windowsTerminalShiftEnterInput,
		},
		"keys": "shift+enter",
	}

	if !removeMisplacedShiftEnterBinding(settings) {
		t.Fatal("expected misplaced binding to be removed")
	}
	if _, ok := settings["command"]; ok {
		t.Fatal("expected top-level command to be removed")
	}
	if _, ok := settings["keys"]; ok {
		t.Fatal("expected top-level keys to be removed")
	}
	if _, ok := settings["actions"]; !ok {
		t.Fatal("expected unrelated settings to remain")
	}
}

func TestUpsertWindowsTerminalAppearanceAddsDefaults(t *testing.T) {
	settings := map[string]any{}
	if !upsertWindowsTerminalAppearance(settings, defaultTerminalSetupOptions()) {
		t.Fatal("expected appearance defaults to change empty settings")
	}
	profiles, ok := settings["profiles"].(map[string]any)
	if !ok {
		t.Fatalf("profiles = %#v, want object", settings["profiles"])
	}
	defaults, ok := profiles["defaults"].(map[string]any)
	if !ok {
		t.Fatalf("profiles.defaults = %#v, want object", profiles["defaults"])
	}
	font, ok := defaults["font"].(map[string]any)
	if !ok {
		t.Fatalf("font = %#v, want object", defaults["font"])
	}
	if font["face"] != defaultTerminalFontFace || font["size"] != defaultTerminalFontSize || font["lineHeight"] != defaultTerminalLineHeight {
		t.Fatalf("font = %#v, want Qubit defaults", font)
	}
	if defaults["padding"] != defaultTerminalPadding {
		t.Fatalf("padding = %#v, want %q", defaults["padding"], defaultTerminalPadding)
	}
}

func TestUpsertWindowsTerminalAppearanceIsIdempotent(t *testing.T) {
	settings := map[string]any{}
	if !upsertWindowsTerminalAppearance(settings, defaultTerminalSetupOptions()) {
		t.Fatal("expected initial appearance upsert to change settings")
	}
	if upsertWindowsTerminalAppearance(settings, defaultTerminalSetupOptions()) {
		t.Fatal("expected second appearance upsert to be unchanged")
	}
}

func TestUpsertWindowsTerminalAppearancePreservesExistingProfilesList(t *testing.T) {
	settings := map[string]any{
		"profiles": map[string]any{
			"list": []any{map[string]any{"name": "PowerShell"}},
		},
	}
	upsertWindowsTerminalAppearance(settings, terminalSetupOptions{FontFace: "Cascadia Mono", FontSize: 12, LineHeight: 1.1, Padding: "6"})
	profiles := settings["profiles"].(map[string]any)
	if _, ok := profiles["list"].([]any); !ok {
		t.Fatalf("profiles.list was not preserved: %#v", profiles)
	}
	defaults := profiles["defaults"].(map[string]any)
	font := defaults["font"].(map[string]any)
	if font["face"] != "Cascadia Mono" || font["size"] != 12.0 || font["lineHeight"] != 1.1 || defaults["padding"] != "6" {
		t.Fatalf("appearance = defaults %#v font %#v, want custom options", defaults, font)
	}
}

func TestTerminalSetupSlashCommandOpensConfirmationModal(t *testing.T) {
	m := initialModel(nil)
	m.ready = true

	updated, cmd := m.handleSlashCommand("/terminal-setup")
	if cmd != nil {
		t.Fatal("/terminal-setup returned command before confirmation, want nil")
	}
	got := updated.(model)
	if got.mode != modeModal || got.modal == nil || got.modal.Kind != modalKindConfirm {
		t.Fatalf("mode/modal = %v/%#v, want confirmation modal", got.mode, got.modal)
	}
	if got.modal.Payload["action"] != "terminal.setup" {
		t.Fatalf("modal payload = %#v, want terminal.setup", got.modal.Payload)
	}
}

func TestTerminalSetupConfirmationCancelDoesNotRun(t *testing.T) {
	m := initialModel(nil).openTerminalSetupConfirm()
	updated, cmd := m.resolveModalAction("cancel")
	if cmd != nil {
		t.Fatal("cancel returned command, want nil")
	}
	got := updated.(model)
	if got.busy {
		t.Fatal("busy = true, want false after cancel")
	}
	if got.status != "terminal setup cancelled" {
		t.Fatalf("status = %q, want terminal setup cancelled", got.status)
	}
}

func TestTerminalSetupConfirmationApplyRunsSetup(t *testing.T) {
	m := initialModel(nil).openTerminalSetupConfirm()
	updated, cmd := m.resolveModalAction("run")
	if cmd == nil {
		t.Fatal("apply returned nil command, want terminal setup command")
	}
	got := updated.(model)
	if !got.busy {
		t.Fatal("busy = false, want true while terminal setup runs")
	}
	if got.status != "updating terminal settings" {
		t.Fatalf("status = %q, want updating terminal settings", got.status)
	}
}
