package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) openThemeEntry() (tea.Model, tea.Cmd) {
	current := m.theme
	if current.Background == "" || current.Text == "" {
		current = defaultTheme()
	}
	background := newThemeEntryComposer(current.Background, "#101112")
	textColor := newThemeEntryComposer(current.Text, "#e6e8eb")
	m.previousMode = m.mode
	m.mode = modeThemeEntry
	m.themeEntry = &themeEntryState{
		Step:       themeEntryPresets,
		Preset:     matchingThemePreset(current),
		Background: background,
		Text:       textColor,
	}
	m.status = "edit theme"
	return m, nil
}

func newThemeEntryComposer(value string, placeholder string) composerModel {
	c := newComposer()
	c.placeholder = placeholder
	c.minHeight = 1
	c.maxHeight = 1
	c.charLimit = 7
	c.SetValue(value)
	return c
}

func matchingThemePreset(theme themeConfig) int {
	for i, preset := range builtinThemes {
		if strings.EqualFold(theme.ID, preset.ID) || (strings.EqualFold(theme.Background, preset.Background) && strings.EqualFold(theme.Text, preset.Text)) {
			return i
		}
	}
	return len(builtinThemes)
}

func (m model) updateThemeEntry(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.themeEntry == nil {
		m.mode = modeChat
		return m, nil
	}
	if isNewlineKey(msg) {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.themeEntry = nil
		m.mode = modeChat
		m.status = "theme cancelled"
		return m, nil
	case "up", "k", "ctrl+p":
		m.moveThemePreset(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveThemePreset(1)
		return m, nil
	case "left", "shift+tab":
		m.moveThemeStep(-1)
		return m, nil
	case "right", "tab":
		m.moveThemeStep(1)
		return m, nil
	case "enter":
		return m.advanceThemeEntry()
	case "d":
		return m.applyThemePreset(0)
	}

	composer := m.activeThemeEntryComposer()
	if composer == nil {
		return m, nil
	}
	handled, cmd := composer.UpdateKey(msg)
	if handled {
		m.themeEntry.Preset = len(builtinThemes)
		m.themeEntry.Err = ""
		m.status = "edit custom theme"
		m.layout()
		return m, cmd
	}
	return m, nil
}

func (m model) updateThemeEntryTeaPaste(msg tea.PasteMsg) model {
	return m.insertThemeEntryPaste(msg.Content)
}

func (m model) updateThemeEntryPaste(msg composerPasteMsg) model {
	if msg.err != nil {
		return m.updateRuntimeError(msg.err)
	}
	return m.insertThemeEntryPaste(msg.text)
}

func (m model) insertThemeEntryPaste(text string) model {
	if m.mode != modeThemeEntry || m.themeEntry == nil {
		return m.insertKeyEntryPaste(text)
	}
	composer := m.activeThemeEntryComposer()
	if composer == nil {
		return m
	}
	composer.InsertString(strings.TrimSpace(text))
	m.themeEntry.Preset = len(builtinThemes)
	m.themeEntry.Err = ""
	m.status = "edit custom theme"
	m.layout()
	return m
}

func (m *model) moveThemeStep(delta int) {
	if m.themeEntry == nil {
		return
	}
	steps := int(themeEntryText) + 1
	m.themeEntry.Step = themeEntryStep((int(m.themeEntry.Step) + delta + steps) % steps)
	m.themeEntry.Err = ""
}

func (m *model) moveThemePreset(delta int) {
	if m.themeEntry == nil || m.themeEntry.Step != themeEntryPresets {
		return
	}
	count := len(builtinThemes) + 1
	m.themeEntry.Preset = (m.themeEntry.Preset + delta + count) % count
	m.themeEntry.Err = ""
	if m.themeEntry.Preset < len(builtinThemes) {
		m.setThemeEntryColors(builtinThemes[m.themeEntry.Preset])
	}
}

func (m *model) setThemeEntryColors(theme themeConfig) {
	if m.themeEntry == nil {
		return
	}
	m.themeEntry.Background.SetValue(theme.Background)
	m.themeEntry.Text.SetValue(theme.Text)
}

func (m model) advanceThemeEntry() (tea.Model, tea.Cmd) {
	if m.themeEntry == nil {
		m.mode = modeChat
		return m, nil
	}
	if m.themeEntry.Step == themeEntryPresets && m.themeEntry.Preset < len(builtinThemes) {
		return m.applyThemePreset(m.themeEntry.Preset)
	}
	if m.themeEntry.Step != themeEntryText {
		m.moveThemeStep(1)
		return m, nil
	}
	return m.applyCustomTheme()
}

func (m model) applyThemePreset(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(builtinThemes) {
		return m, nil
	}
	return m.applyThemeConfig(builtinThemes[index]), nil
}

func (m model) applyCustomTheme() (tea.Model, tea.Cmd) {
	if m.themeEntry == nil {
		m.mode = modeChat
		return m, nil
	}
	next, err := customThemeFrom(m.themeEntry.Background.Value(), m.themeEntry.Text.Value(), m.theme)
	if err != nil {
		m.themeEntry.Err = err.Error()
		m.status = "invalid theme color"
		return m, nil
	}
	return m.applyThemeConfig(next), nil
}

func (m model) applyThemeConfig(theme themeConfig) model {
	theme = resolveThemeConfig(theme)
	if theme.Background == "" || theme.Text == "" {
		theme = defaultTheme()
	}
	applyTheme(theme)
	m.spinner.Style = spinnerStyle
	m.theme = theme
	m.saveThemeConfig()
	m.themeEntry = nil
	m.mode = modeChat
	m.previousMode = modeChat
	m.status = fmt.Sprintf("theme applied: %s", theme.Name)
	m.err = ""
	m.renderCache = make(map[renderCacheKey]string)
	m.layout()
	m.refreshViewport()
	m.renderCache = make(map[renderCacheKey]string)
	return m
}

func (m *model) activeThemeEntryComposer() *composerModel {
	if m.themeEntry == nil {
		return nil
	}
	switch m.themeEntry.Step {
	case themeEntryBackground:
		return &m.themeEntry.Background
	case themeEntryText:
		return &m.themeEntry.Text
	default:
		return nil
	}
}

func (m model) renderThemeEntry(height int) string {
	if m.themeEntry == nil {
		return ""
	}
	panelWidth := min(max(56, m.width-12), 96)
	contentWidth := max(20, panelWidth-6)
	m.themeEntry.Background.SetWidth(12)
	m.themeEntry.Text.SetWidth(12)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("Theme"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("Choose a preset, return to default, or set custom #RRGGBB colors."))
	b.WriteString("\n\n")
	b.WriteString(mutedSt.Render("Presets:"))
	b.WriteString("\n")
	for i, preset := range builtinThemes {
		b.WriteString(renderThemePresetLine(i, m.themeEntry.Preset, preset))
		b.WriteString("\n")
	}
	customLabel := "Custom"
	if m.themeEntry.Preset == len(builtinThemes) {
		customLabel = selectSt.Render("› " + customLabel)
	} else {
		customLabel = mutedSt.Render("  ") + customLabel
	}
	b.WriteString(customLabel)
	b.WriteString("\n\n")

	b.WriteString(renderThemeEntryLine("Background", m.themeEntry.Background.View("", 0), m.themeEntry.Step == themeEntryBackground))
	b.WriteString("\n")
	b.WriteString(renderThemeEntryLine("Text", m.themeEntry.Text.View("", 0), m.themeEntry.Step == themeEntryText))
	b.WriteString("\n")
	preview := lipgloss.NewStyle().Background(lipgloss.Color(m.themeEntry.Background.Value())).Foreground(lipgloss.Color(m.themeEntry.Text.Value())).Render(" preview text ")
	b.WriteString(mutedSt.Render("Preview: ") + preview)
	if m.themeEntry.Err != "" {
		b.WriteString("\n\n")
		b.WriteString(errSt.Render(m.themeEntry.Err))
	}
	b.WriteString("\n\n")
	b.WriteString(mutedSt.Render("↑/↓ preset · enter apply/next · tab field · d default · esc cancel"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render(wrap("Custom colors accept #RRGGBB or RRGGBB. Example: #easd217 is invalid because hex uses only 0-9 and A-F.", contentWidth)))

	panel := lipgloss.NewStyle().Foreground(text).Padding(1, 2).Width(panelWidth).Render(b.String())
	return lipgloss.Place(max(1, m.width-4), max(1, height), lipgloss.Center, lipgloss.Bottom, panel)
}

func renderThemePresetLine(index int, selected int, theme themeConfig) string {
	marker := mutedSt.Render("  ")
	label := theme.Name
	if index == selected {
		marker = selectSt.Render("› ")
		label = selectSt.Render(label)
	}
	colors := mutedSt.Render(fmt.Sprintf("bg %s · text %s", theme.Background, theme.Text))
	return fmt.Sprintf("%s%-8s %s", marker, label, colors)
}

func renderThemeEntryLine(label string, value string, active bool) string {
	prefix := mutedSt.Render(fmt.Sprintf("%-12s", label+":"))
	if active {
		prefix = accentSt().Render(fmt.Sprintf("%-12s", label+":"))
	}
	return prefix + value
}
