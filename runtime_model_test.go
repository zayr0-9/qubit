package main

import "testing"

func TestModelListOpensSelector(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeModal
	m.busy = true

	updated, cmd := m.updateRuntime(runtimeEvent{
		Type:  "model.list",
		Model: "glm-4.6",
		Models: []modelInfo{
			{ID: "glm-4.6", Name: "glm-4.6", Description: "Default", Active: true},
			{ID: "glm-4-air", Name: "glm-4-air", Description: "Fast"},
		},
	})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("model.list returned nil command, want waitRuntimeEvent command")
	}
	if got.mode != modeModal {
		t.Fatalf("mode = %v, want modeModal", got.mode)
	}
	if got.busy {
		t.Fatal("busy = true, want false after model list loads")
	}
	if got.modal == nil || got.modal.ID != "model_selector" || len(got.modal.Options) != 2 {
		t.Fatalf("modal = %#v, want model selector with two options", got.modal)
	}
	if got.modal.OptionCursor != 0 {
		t.Fatalf("option cursor = %d, want active model index 0", got.modal.OptionCursor)
	}
}

func TestModelUpdatedAppliesActiveModel(t *testing.T) {
	m := initialModel(nil)

	m.applyModelUpdated(runtimeEvent{Type: "model.updated", Model: "glm-4-air", Status: "Using model glm-4-air."})

	if m.model != "glm-4-air" {
		t.Fatalf("model = %q, want glm-4-air", m.model)
	}
	if m.busy {
		t.Fatal("busy = true, want false")
	}
	if m.status != "ready" {
		t.Fatalf("status = %q, want ready", m.status)
	}
}
