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
