package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) openKeyPicker() (tea.Model, tea.Cmd) {
	m.mode = modeKeyPicker
	m.ensureApiKeyCursorInBounds()
	m.busy = true
	m.status = "loading api keys"
	return m, sendRuntime(m.runtime, map[string]any{"type": "key.list"})
}

func (m model) updateKeyPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		m.moveApiKeyCursor(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveApiKeyCursor(1)
		return m, nil
	case "a":
		return m.openKeyEntry(), nil
	case "d", "delete":
		selected, ok := m.selectedApiKey()
		if !ok {
			m.status = "no api key selected"
			return m, nil
		}
		if selected.Readonly || selected.Source == "env" {
			m.status = "env api keys are read-only"
			return m, nil
		}
		return m.openDeleteApiKeyConfirm(selected), nil
	case "enter":
		selected, ok := m.selectedApiKey()
		if !ok {
			return m, nil
		}
		if selected.Active {
			m.mode = modeChat
			m.status = "ready"
			return m, nil
		}
		m.busy = true
		m.status = "activating api key"
		return m, sendRuntime(m.runtime, map[string]any{"type": "key.use", "provider": selected.Provider, "alias": selected.Alias})
	}
	return m, nil
}

func (m model) openDeleteApiKeyConfirm(selected apiKeyInfo) model {
	m.previousMode = m.mode
	m.mode = modeModal
	m.modal = &modalState{
		ID:          "delete_api_key",
		Kind:        modalKindConfirm,
		Title:       "Delete API key?",
		Description: fmt.Sprintf("Delete stored key %s/%s from the OS keychain? This cannot be undone.", fallback(selected.Provider, "glm"), selected.Alias),
		Fields: []modalField{
			{Label: "Provider", Value: fallback(selected.Provider, "glm")},
			{Label: "Alias", Value: selected.Alias},
			{Label: "Source", Value: selected.Source},
		},
		Actions: []modalAction{
			{ID: "cancel", Label: "Cancel", Default: true},
			{ID: "delete", Label: "Delete", Style: "danger"},
		},
		Payload: map[string]any{
			"action":   "key.delete",
			"provider": selected.Provider,
			"alias":    selected.Alias,
		},
	}
	m.status = "confirm api key delete"
	return m
}

func (m model) openKeyEntry() model {
	providers := apiKeyProviderOptions()
	providerID := fallback(m.activeProvider, fallback(m.provider, providers[0].ID))
	provider := newKeyEntryComposer(providerID, "provider")
	alias := newKeyEntryComposer("", "alias")
	secret := newKeyEntryComposer("", "api key")
	provider.SetCharLimit(32)
	alias.SetCharLimit(64)
	secret.SetCharLimit(4096)
	m.previousMode = m.mode
	m.mode = modeKeyEntry
	m.keyEntry = &keyEntryState{Step: keyEntryProvider, ProviderCursor: providerOptionIndex(providers, providerID), Providers: providers, Provider: provider, Alias: alias, Secret: secret}
	m.status = "choose provider"
	return m
}

func newKeyEntryComposer(value string, placeholder string) composerModel {
	c := newComposer()
	c.SetPlaceholder(placeholder)
	c.SetMinHeight(1)
	c.SetMaxHeight(3)
	c.SetValue(value)
	return c
}

func (m model) updateKeyEntry(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.keyEntry == nil {
		m.mode = modeKeyPicker
		return m, nil
	}
	if isNewlineKey(msg) {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		if m.keyEntry.Step == keyEntryProvider {
			m.moveKeyEntryProvider(-1)
			m.layout()
			return m, nil
		}
	case "down", "j", "ctrl+n":
		if m.keyEntry.Step == keyEntryProvider {
			m.moveKeyEntryProvider(1)
			m.layout()
			return m, nil
		}
	case "esc":
		m.keyEntry = nil
		m.mode = modeKeyPicker
		m.status = "ready"
		return m, nil
	case "enter":
		return m.advanceKeyEntry()
	}
	composer := m.activeKeyEntryComposer()
	if composer == nil {
		return m, nil
	}
	handled, cmd := composer.UpdateKey(msg)
	if handled {
		m.layout()
		return m, cmd
	}
	return m, nil
}

func (m model) updateKeyEntryPaste(msg composerPasteMsg) model {
	if msg.Err != nil {
		return m.updateRuntimeError(msg.Err)
	}
	return m.insertKeyEntryPaste(msg.Text)
}

func (m model) updateKeyEntryTeaPaste(msg tea.PasteMsg) model {
	return m.insertKeyEntryPaste(msg.Content)
}

func (m model) insertKeyEntryPaste(text string) model {
	if m.mode != modeKeyEntry || m.keyEntry == nil {
		m.composer.InsertString(text)
		m.layout()
		return m
	}

	pasted := strings.TrimSpace(normalizeInputNewlines(text))
	if looksLikeAPIKey(pasted) && m.keyEntry.Step != keyEntrySecret {
		m.keyEntry.Secret.SetValue(pasted)
		m.keyEntry.Step = keyEntrySecret
		m.status = "api key pasted · press enter to save"
		m.layout()
		return m
	}

	if composer := m.activeKeyEntryComposer(); composer != nil {
		composer.InsertString(text)
		m.layout()
	}
	return m
}

func (m model) advanceKeyEntry() (tea.Model, tea.Cmd) {
	if m.keyEntry == nil {
		m.mode = modeKeyPicker
		return m, nil
	}
	switch m.keyEntry.Step {
	case keyEntryProvider:
		m.syncSelectedKeyEntryProvider()
		provider := strings.TrimSpace(m.keyEntry.Provider.Value())
		if provider == "" {
			m.status = "provider required"
			return m, nil
		}
		m.keyEntry.Step = keyEntryAlias
		m.status = "enter key alias"
		return m, nil
	case keyEntryAlias:
		alias := strings.TrimSpace(m.keyEntry.Alias.Value())
		if alias == "" {
			m.status = "alias required"
			return m, nil
		}
		m.keyEntry.Step = keyEntrySecret
		m.status = "paste api key"
		return m, nil
	case keyEntrySecret:
		provider := strings.TrimSpace(m.keyEntry.Provider.Value())
		alias := strings.TrimSpace(m.keyEntry.Alias.Value())
		secret := strings.TrimSpace(m.keyEntry.Secret.Value())
		if provider == "" || alias == "" || secret == "" {
			m.status = "provider, alias, and api key are required"
			return m, nil
		}
		m.keyEntry.Secret.Reset()
		m.keyEntry = nil
		m.mode = modeKeyPicker
		m.busy = true
		m.status = "saving api key securely"
		return m, sendRuntime(m.runtime, map[string]any{"type": "key.set", "provider": provider, "alias": alias, "apiKey": secret})
	}
	return m, nil
}

func (m *model) moveKeyEntryProvider(delta int) {
	if m.keyEntry == nil || len(m.keyEntry.Providers) == 0 {
		return
	}
	m.keyEntry.ProviderCursor = (m.keyEntry.ProviderCursor + delta + len(m.keyEntry.Providers)) % len(m.keyEntry.Providers)
	m.syncSelectedKeyEntryProvider()
	m.status = "choose provider"
}

func (m *model) syncSelectedKeyEntryProvider() {
	if m.keyEntry == nil || len(m.keyEntry.Providers) == 0 {
		return
	}
	cursor := clampInt(m.keyEntry.ProviderCursor, 0, len(m.keyEntry.Providers)-1)
	m.keyEntry.ProviderCursor = cursor
	m.keyEntry.Provider.SetValue(m.keyEntry.Providers[cursor].ID)
}

func (m *model) activeKeyEntryComposer() *composerModel {
	if m.keyEntry == nil {
		return nil
	}
	switch m.keyEntry.Step {
	case keyEntryProvider:
		return nil
	case keyEntryAlias:
		return &m.keyEntry.Alias
	case keyEntrySecret:
		return &m.keyEntry.Secret
	default:
		return nil
	}
}

func (m *model) applyKeyList(ev runtimeEvent) {
	m.apiKeys = ev.Keys
	m.applyActiveKeyMetadata(ev)
	m.busy = false
	m.status = "ready"
	m.err = ""
	m.ensureApiKeyCursorInBounds()
}

func (m *model) applyKeyUpdated(ev runtimeEvent) {
	m.apiKeys = ev.Keys
	m.applyActiveKeyMetadata(ev)
	m.busy = false
	m.status = "ready"
	m.err = ""
	m.ensureApiKeyCursorInBounds()
	if ev.Status != "" {
		m.appendSystem(ev.Status)
	}
}

func (m *model) applyActiveKeyMetadata(ev runtimeEvent) {
	if ev.MaxContext > 0 {
		m.maxContext = ev.MaxContext
	}
	if ev.ActiveProvider != "" {
		m.activeProvider = ev.ActiveProvider
		m.provider = ev.ActiveProvider
	} else if ev.Provider != "" {
		m.activeProvider = ev.Provider
		m.provider = ev.Provider
	}
	if ev.ActiveKeyAlias != "" {
		m.activeKeyAlias = ev.ActiveKeyAlias
	}
}

func (m *model) ensureApiKeyCursorInBounds() {
	if len(m.apiKeys) == 0 {
		m.apiKeyCursor = 0
		return
	}
	if m.apiKeyCursor < 0 || m.apiKeyCursor >= len(m.apiKeys) {
		m.apiKeyCursor = 0
	}
}

func (m *model) moveApiKeyCursor(delta int) {
	if len(m.apiKeys) == 0 {
		m.apiKeyCursor = 0
		return
	}
	m.apiKeyCursor = (m.apiKeyCursor + delta + len(m.apiKeys)) % len(m.apiKeys)
}

func (m model) selectedApiKey() (apiKeyInfo, bool) {
	if len(m.apiKeys) == 0 {
		return apiKeyInfo{}, false
	}
	cursor := clampInt(m.apiKeyCursor, 0, len(m.apiKeys)-1)
	return m.apiKeys[cursor], true
}

func (m model) renderKeyPicker() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("api keys") + "\n")
	b.WriteString(mutedSt.Render("↑/↓ select · enter activate · a add · d delete · esc close") + "\n\n")
	if len(m.apiKeys) == 0 {
		b.WriteString(mutedSt.Render("no api keys configured · press a to add a secure OS keychain key"))
		return lipgloss.NewStyle().Padding(1, 2).Width(max(20, m.width-4)).Render(b.String())
	}
	for i, key := range m.apiKeys {
		active := " "
		if key.Active {
			active = "•"
		}
		lock := ""
		if key.Readonly || key.Source == "env" {
			lock = " readonly"
		}
		line := fmt.Sprintf("%s %-12s %-18s %-10s %s%s", active, key.Provider, oneLine(key.Alias, 18), key.Source, key.Masked, mutedSt.Render(lock))
		if i == m.apiKeyCursor {
			line = selectSt.Render("  " + line)
		} else {
			line = mutedSt.Render("  ") + line
		}
		b.WriteString(line)
		if i < len(m.apiKeys)-1 {
			b.WriteString("\n")
		}
	}
	return lipgloss.NewStyle().Padding(1, 2).Width(max(20, m.width-4)).Render(b.String())
}

func (m model) renderKeyEntry(height int) string {
	if m.keyEntry == nil {
		return ""
	}
	panelWidth := min(max(54, m.width-12), 96)
	contentWidth := max(20, panelWidth-6)
	setEntryWidths(m.keyEntry, contentWidth-12)

	stepName := "provider"
	if m.keyEntry.Step == keyEntryAlias {
		stepName = "alias"
	} else if m.keyEntry.Step == keyEntrySecret {
		stepName = "api key"
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("Add API key"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("Stored in your OS keychain. Qubit never writes the raw key to .qubit files or runtime events."))
	b.WriteString("\n\n")
	b.WriteString(mutedSt.Render("Step: ") + stepName + "\n\n")
	b.WriteString(renderKeyProviderList(m.keyEntry))
	b.WriteString("\n")
	b.WriteString(renderKeyEntryLine("Alias", m.keyEntry.Alias.View("", 0), m.keyEntry.Step == keyEntryAlias, false))
	b.WriteString("\n")
	secretView := maskComposerView(m.keyEntry.Secret)
	b.WriteString(renderKeyEntryLine("API key", secretView, m.keyEntry.Step == keyEntrySecret, true))
	b.WriteString("\n\n")
	b.WriteString(mutedSt.Render("provider: ↑/↓ choose · enter next/save · ctrl+v paste · esc cancel"))

	panel := lipgloss.NewStyle().Foreground(text).Padding(1, 2).Width(panelWidth).Render(b.String())
	return lipgloss.Place(max(1, m.width-4), max(1, height), lipgloss.Center, lipgloss.Bottom, panel)
}

func renderKeyProviderList(entry *keyEntryState) string {
	if entry == nil || len(entry.Providers) == 0 {
		return renderKeyEntryLine("Provider", "", false, false)
	}
	var b strings.Builder
	label := mutedSt.Render(fmt.Sprintf("%-10s", "Provider:"))
	if entry.Step == keyEntryProvider {
		label = accentSt().Render(fmt.Sprintf("%-10s", "Provider:"))
	}
	b.WriteString(label)
	if entry.Step != keyEntryProvider {
		b.WriteString(entry.Provider.View("", 0))
		return b.String()
	}
	b.WriteString("\n")
	for i, provider := range entry.Providers {
		prefix := "  "
		line := fmt.Sprintf("%s%-12s %s", prefix, provider.ID, provider.Description)
		if i == entry.ProviderCursor {
			line = selectSt.Render("  • " + fmt.Sprintf("%-12s %s", provider.ID, provider.Description))
		} else {
			line = mutedSt.Render("    ") + fmt.Sprintf("%-12s %s", provider.ID, provider.Description)
		}
		b.WriteString(line)
		if i < len(entry.Providers)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func setEntryWidths(entry *keyEntryState, width int) {
	if entry == nil {
		return
	}
	w := max(12, width)
	entry.Provider.SetWidth(w)
	entry.Alias.SetWidth(w)
	entry.Secret.SetWidth(w)
}

func renderKeyEntryLine(label string, value string, active bool, secret bool) string {
	prefix := mutedSt.Render(fmt.Sprintf("%-10s", label+":"))
	if active {
		prefix = accentSt().Render(fmt.Sprintf("%-10s", label+":"))
	}
	if secret && strings.TrimSpace(value) == "" {
		value = mutedSt.Render("api key")
	}
	return prefix + value
}

func maskComposerView(c composerModel) string {
	masked := c
	if value := c.Value(); value != "" {
		masked.SetValue(strings.Repeat("•", len([]rune(value))))
	}
	return masked.View("", 0)
}

func looksLikeAPIKey(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, " ") || len([]rune(trimmed)) < 16 {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "zai-") || strings.HasPrefix(lower, "zai_") || strings.HasPrefix(lower, "key-") || strings.Contains(lower, "api") || len([]rune(trimmed)) >= 32
}

func accentSt() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(accent).Bold(true)
}

func (m *model) applyModelUpdated(ev runtimeEvent) {
	if ev.Model != "" {
		m.model = ev.Model
	}
	m.maxContext = ev.MaxContext
	if len(ev.Models) > 0 {
		m.models = ev.Models
	}
	m.applyActiveKeyMetadata(ev)
	m.busy = false
	m.status = "ready"
	m.err = ""
	if ev.Status != "" {
		m.appendSystem(ev.Status)
	}
}

func apiKeyProviderOptions() []apiKeyProviderOption {
	return []apiKeyProviderOption{
		{ID: "glm", Label: "GLM", Description: "Z.ai GLM models"},
		{ID: "hyperrouter", Label: "Hyperrouter", Description: "OpenAI-compatible Hyperrouter gateway"},
		{ID: "openai", Label: "OpenAI", Description: "OpenAI API"},
		{ID: "bedrock", Label: "Amazon Bedrock", Description: "AWS Bedrock via Vercel AI SDK"},
		{ID: "openrouter", Label: "OpenRouter", Description: "OpenRouter models and tools"},
		{ID: "codex", Label: "Codex", Description: "ChatGPT Codex OAuth"},
	}
}

func providerOptionIndex(options []apiKeyProviderOption, provider string) int {
	provider = strings.TrimSpace(strings.ToLower(provider))
	for i, option := range options {
		if option.ID == provider {
			return i
		}
	}
	return 0
}
