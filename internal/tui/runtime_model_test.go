package tui

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

func TestProviderUpdatedAppliesProviderModelAndModels(t *testing.T) {
	m := initialModel(nil)

	m.applyModelUpdated(runtimeEvent{
		Type:           "model.updated",
		ActiveProvider: "codex",
		ActiveKeyAlias: "chatgpt",
		Model:          "gpt-5.2-codex",
		Status:         "Using provider codex with model gpt-5.2-codex.",
		Models: []modelInfo{
			{ID: "gpt-5.5", Name: "GPT-5.5"},
			{ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex", Active: true},
			{ID: "gpt-5.2", Name: "GPT-5.2"},
		},
	})

	if m.activeProvider != "codex" || m.provider != "codex" {
		t.Fatalf("provider = %q activeProvider = %q, want codex", m.provider, m.activeProvider)
	}
	if m.activeKeyAlias != "chatgpt" {
		t.Fatalf("activeKeyAlias = %q, want chatgpt", m.activeKeyAlias)
	}
	if m.model != "gpt-5.2-codex" {
		t.Fatalf("model = %q, want gpt-5.2-codex", m.model)
	}
	if len(m.models) != 3 || m.models[0].ID != "gpt-5.5" || m.models[1].ID != "gpt-5.2-codex" {
		t.Fatalf("models = %#v, want codex model list", m.models)
	}
}

func TestProviderSelectorModal(t *testing.T) {
	m := initialModel(nil)
	m.activeProvider = "codex"

	got := m.openProviderSelectorModal()

	if got.mode != modeModal {
		t.Fatalf("mode = %v, want modeModal", got.mode)
	}
	if got.modal == nil || got.modal.ID != "provider_selector" {
		t.Fatalf("modal = %#v, want provider selector", got.modal)
	}
	if len(got.modal.Options) == 0 {
		t.Fatal("provider selector options empty")
	}
	if got.modal.Options[got.modal.OptionCursor].ID != "codex" {
		t.Fatalf("selected provider = %q, want codex", got.modal.Options[got.modal.OptionCursor].ID)
	}
}
