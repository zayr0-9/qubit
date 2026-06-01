package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSlashKeysOpensPickerAndRequestsList(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true

	updated, cmd := m.handleSlashCommand("/keys")
	got := updated.(model)

	if got.mode != modeKeyPicker {
		t.Fatalf("mode = %v, want key picker", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading keys")
	}
	if got.status != "loading api keys" {
		t.Fatalf("status = %q, want loading api keys", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "key.list", "")
}

func TestApplyKeyListUpdatesState(t *testing.T) {
	m := initialModel(nil)
	keys := []apiKeyInfo{{Provider: "glm", Alias: "work", Source: "keychain", Active: true, Masked: "zai_…abcd"}}

	m.applyKeyList(runtimeEvent{Type: "key.list", ActiveProvider: "glm", ActiveKeyAlias: "work", Keys: keys})

	if m.busy {
		t.Fatal("busy = true, want false")
	}
	if m.provider != "glm" || m.activeProvider != "glm" {
		t.Fatalf("provider metadata = %q/%q, want glm/glm", m.provider, m.activeProvider)
	}
	if m.activeKeyAlias != "work" {
		t.Fatalf("activeKeyAlias = %q, want work", m.activeKeyAlias)
	}
	if len(m.apiKeys) != 1 || m.apiKeys[0].Alias != "work" {
		t.Fatalf("apiKeys = %#v, want work key", m.apiKeys)
	}
}

func TestKeyPickerEnterActivatesSelectedKey(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.mode = modeKeyPicker
	m.ready = true
	m.apiKeys = []apiKeyInfo{
		{Provider: "glm", Alias: "env:ZAI_API_KEY", Source: "env", Active: true, Readonly: true},
		{Provider: "glm", Alias: "work", Source: "keychain"},
	}
	m.apiKeyCursor = 1

	updated, cmd := m.updateKeyPicker(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)

	if !got.busy {
		t.Fatal("busy = false, want true while activating key")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "key.use", "")
	if payload["provider"] != "glm" || payload["alias"] != "work" {
		t.Fatalf("payload = %#v, want glm/work", payload)
	}
}

func TestKeyPickerBlocksDeletingEnvKey(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeKeyPicker
	m.apiKeys = []apiKeyInfo{{Provider: "glm", Alias: "env:ZAI_API_KEY", Source: "env", Readonly: true}}

	updated, cmd := m.updateKeyPicker(tea.KeyPressMsg{Code: 'd', Text: "d"})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("delete env key returned command, want nil")
	}
	if got.status != "env api keys are read-only" {
		t.Fatalf("status = %q, want env api keys are read-only", got.status)
	}
}

func TestKeyEntrySendsSetWithoutRenderingSecret(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.width = 80
	m.height = 24
	m.layout()
	m = m.openKeyEntry()

	updated, cmd := m.updateKeyEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("provider enter returned command, want nil")
	}
	m = updated.(model)
	m.keyEntry.Alias.InsertString("work")
	updated, cmd = m.updateKeyEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("alias enter returned command, want nil")
	}
	m = updated.(model)
	m.keyEntry.Secret.InsertString("zai-secret-1234")

	view := m.renderKeyEntry(12)
	if strings.Contains(view, "zai-secret-1234") {
		t.Fatalf("rendered key entry leaked secret: %q", view)
	}
	if !strings.Contains(view, "••••") {
		t.Fatalf("rendered key entry did not show masked bullets: %q", view)
	}

	updated, cmd = m.updateKeyEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeKeyPicker {
		t.Fatalf("mode = %v, want key picker after save", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while saving key")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "key.set", "")
	if payload["provider"] != "glm" || payload["alias"] != "work" || payload["apiKey"] != "zai-secret-1234" {
		t.Fatalf("payload = %#v, want glm/work secret", payload)
	}
	if got.keyEntry != nil {
		t.Fatalf("keyEntry = %#v, want nil after save", got.keyEntry)
	}
}

func TestKeyUpdatedAppendsConfirmation(t *testing.T) {
	m := initialModel(nil)
	m.applyKeyUpdated(runtimeEvent{Type: "key.updated", ActiveProvider: "glm", ActiveKeyAlias: "work", Status: "Activated glm/work."})

	if m.activeKeyAlias != "work" {
		t.Fatalf("activeKeyAlias = %q, want work", m.activeKeyAlias)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Activated glm/work") {
		t.Fatalf("messages = %#v, want confirmation", m.messages)
	}
}

func TestPastingAPIKeyJumpsToSecretStepAndSaves(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.width = 80
	m.height = 24
	m.layout()
	m = m.openKeyEntry()
	m.keyEntry.Alias.SetValue("work")

	m = m.updateKeyEntryTeaPaste(tea.PasteMsg{Content: "zai-pasted-secret-1234567890"})

	if m.keyEntry == nil {
		t.Fatal("keyEntry nil after paste, want active entry")
	}
	if m.keyEntry.Step != keyEntrySecret {
		t.Fatalf("step = %v, want secret step after API key paste", m.keyEntry.Step)
	}
	if got := m.keyEntry.Secret.Value(); got != "zai-pasted-secret-1234567890" {
		t.Fatalf("secret = %q, want pasted key", got)
	}
	view := m.renderKeyEntry(12)
	if strings.Contains(view, "zai-pasted-secret-1234567890") {
		t.Fatalf("rendered key entry leaked pasted secret: %q", view)
	}

	updated, cmd := m.updateKeyEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if !got.busy {
		t.Fatal("busy = false, want true while saving pasted key")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "key.set", "")
	if payload["provider"] != "glm" || payload["alias"] != "work" || payload["apiKey"] != "zai-pasted-secret-1234567890" {
		t.Fatalf("payload = %#v, want pasted key save", payload)
	}
}

func TestKeyPickerDeleteOpensConfirmationModal(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeKeyPicker
	m.apiKeys = []apiKeyInfo{{Provider: "glm", Alias: "work", Source: "keychain", Masked: "zai-…abcd"}}

	updated, cmd := m.updateKeyPicker(tea.KeyPressMsg{Code: 'd', Text: "d"})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("delete key returned command, want confirmation modal only")
	}
	if got.mode != modeModal {
		t.Fatalf("mode = %v, want modal", got.mode)
	}
	if got.previousMode != modeKeyPicker {
		t.Fatalf("previousMode = %v, want key picker", got.previousMode)
	}
	if got.modal == nil || got.modal.Kind != modalKindConfirm {
		t.Fatalf("modal = %#v, want confirm modal", got.modal)
	}
	if got.selectedModalActionID() != "cancel" {
		t.Fatalf("selected action = %q, want cancel default", got.selectedModalActionID())
	}
}

func TestApiKeyDeleteConfirmationCancelDoesNotDelete(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeKeyPicker
	m = m.openDeleteApiKeyConfirm(apiKeyInfo{Provider: "glm", Alias: "work", Source: "keychain"})

	updated, cmd := m.resolveModalAction("cancel")
	got := updated.(model)

	if cmd != nil {
		t.Fatal("cancel delete returned command, want nil")
	}
	if got.mode != modeKeyPicker {
		t.Fatalf("mode = %v, want key picker", got.mode)
	}
	if got.status != "delete cancelled" {
		t.Fatalf("status = %q, want delete cancelled", got.status)
	}
}

func TestApiKeyDeleteConfirmationSendsDelete(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.mode = modeKeyPicker
	m = m.openDeleteApiKeyConfirm(apiKeyInfo{Provider: "glm", Alias: "work", Source: "keychain"})

	updated, cmd := m.resolveModalAction("delete")
	got := updated.(model)

	if got.mode != modeKeyPicker {
		t.Fatalf("mode = %v, want key picker", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while deleting")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "key.delete", "")
	if payload["provider"] != "glm" || payload["alias"] != "work" {
		t.Fatalf("payload = %#v, want glm/work delete", payload)
	}
}
