package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return newAppView("loading...")
	}

	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	bottomHeight := lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	mainHeight := max(0, m.height-bottomHeight)
	content := appStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			renderFixedHeight(m.renderMainArea(mainHeight), mainHeight),
			"",
			input,
			status,
			footer,
		),
	)
	return newAppView(content)
}

func newAppView(content string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.KeyboardEnhancements.ReportEventTypes = true
	return view
}

func (m model) renderHeader() string {
	provider := fallback(m.provider, "...")
	if m.activeKeyAlias != "" && m.activeKeyAlias != "stub" {
		provider = provider + "/" + short(strings.TrimPrefix(m.activeKeyAlias, "env:"), 14)
	}
	modelName := fallback(m.model, "...")
	sessionTitle := fallback(m.title, m.currentSessionTitle())
	sessionTitle = fallback(sessionTitle, "untitled")

	activity := okSt.Render(m.status)
	if strings.Contains(m.status, "error") || m.err != "" {
		activity = errSt.Render(m.status)
	}

	appName := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("qubit")
	meta := mutedSt.Render(fmt.Sprintf("%s · %s · %s", provider, modelName, short(m.session, 12)))
	headerLeft := fmt.Sprintf("%s  %s", appName, activity)
	headerRight := oneLine(sessionTitle, max(12, m.width-lipgloss.Width(headerLeft)-lipgloss.Width(meta)-8))
	headerText := fmt.Sprintf("%s  %s  %s", headerLeft, mutedSt.Render(headerRight), meta)
	return headerStyle.Width(m.width).Render(headerText)
}

func (m model) renderMainArea(height int) string {
	if height <= 0 {
		return ""
	}
	header := m.renderHeader()
	bodyHeight := max(0, height-lipgloss.Height(header))
	chatContent := m.viewport.View()
	if m.mode == modeSessionPicker {
		chatContent = m.renderSessionPicker()
	} else if m.mode == modeKeyPicker {
		chatContent = m.renderKeyPicker()
	} else if m.mode == modeKeyEntry {
		chatContent = m.renderKeyEntry(bodyHeight)
	} else if m.mode == modeModal {
		chatContent = m.renderModal(bodyHeight)
	}
	chat := renderChat(chatContent, m.width, max(1, bodyHeight))

	if m.mode == modeModal || m.mode == modeKeyEntry || !m.showSlashPalette() {
		return lipgloss.JoinVertical(lipgloss.Left, header, chat)
	}
	palette := m.renderSlashPalette()
	paletteHeight := lipgloss.Height(palette)
	chatHeight := max(0, bodyHeight-paletteHeight)
	if chatHeight > 0 {
		chat = renderChat(chatContent, m.width, chatHeight)
	} else {
		chat = ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, chat, palette)
}

func (m model) renderInput() string {
	view := m.composer.View(m.inputPrompt())
	return inputStyle.Width(m.width).Height(m.composer.Height()).Render(view)
}

func (m model) inputPrompt() string {
	if m.busy || m.streaming {
		return m.spinner.View() + " "
	}
	return idleInputPrompt()
}

func idleInputPrompt() string {
	return lipgloss.NewStyle().Foreground(accent).Render("› ")
}

func (m model) renderInputStatus() string {
	mode := m.permissionModeLabel()
	style := lipgloss.NewStyle().Bold(true)
	if m.permissionMode == permissionModeAlwaysAllow {
		style = style.Foreground(green)
	} else {
		style = style.Foreground(accent)
	}

	return footerStyle.Width(m.width).Render(style.Render(mode))
}

func (m model) renderFooter() string {
	footer := "enter send · ctrl+a all · ctrl+c copy/quit · shift+arrows select · ctrl+j newline · pgup/pgdn scroll"
	if m.keyboardEnhanced {
		footer = "enter send · shift+enter newline · shift+arrows select · ctrl+shift+←/→ words · ctrl+a all · ctrl+c copy/quit"
	}
	if m.composer.HasSelection() {
		footer = "selection · ctrl+c copy · type replace · backspace/delete remove · esc clear"
	}
	if m.mode == modeModal {
		footer = "←/→ choose action · enter confirm · esc deny/cancel"
	} else if m.mode == modeKeyEntry {
		footer = "enter next/save · ctrl+v paste · esc cancel · secret input is masked"
	} else if m.mode == modeKeyPicker {
		footer = "↑/↓ choose key · enter activate · a add · d delete · esc close"
	} else if m.mode == modeSessionPicker {
		footer = "↑/↓ choose session · enter switch · esc close"
	} else if m.showSlashPalette() {
		footer = "↑/↓ choose command · enter/tab complete"
	}

	footerText := mutedSt.Render(footer)
	if m.err != "" {
		copyHint := ""
		if m.runtime != nil && m.runtime.logPath != "" {
			copyHint = " · log: " + m.runtime.logPath
		}
		footerText = errSt.Render(oneLine(m.err, max(20, m.width-20-len(copyHint))) + copyHint)
	}
	return footerStyle.Width(m.width).Render(footerText)
}

func (m model) renderModal(height int) string {
	if m.modal == nil {
		return ""
	}

	modal := *m.modal
	panelWidth := min(max(48, m.width-12), 92)
	contentWidth := max(20, panelWidth-6)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render(modal.Title))
	if modal.Description != "" {
		b.WriteString("\n")
		b.WriteString(wrap(modal.Description, contentWidth))
	}

	if len(modal.Fields) > 0 {
		b.WriteString("\n\n")
		for i, field := range modal.Fields {
			label := mutedSt.Render(field.Label + ":")
			value := field.Value
			if strings.Contains(value, "\n") {
				b.WriteString(label)
				b.WriteString("\n")
				b.WriteString(truncateModalLines(value, contentWidth, 12))
			} else {
				b.WriteString(fmt.Sprintf("%s %s", label, oneLine(value, max(8, contentWidth-lipgloss.Width(field.Label)-2))))
			}
			if i < len(modal.Fields)-1 {
				b.WriteString("\n")
			}
		}
	}

	if len(modal.Actions) > 0 {
		b.WriteString("\n\n")
		for i, action := range modal.Actions {
			button := "[ " + action.Label + " ]"
			if i == modal.Cursor {
				button = selectSt.Render(button)
			} else if action.Style == "danger" {
				button = errSt.Render(button)
			} else {
				button = mutedSt.Render(button)
			}
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(button)
		}
	}

	panel := lipgloss.NewStyle().Background(surface).Foreground(text).Padding(1, 2).Width(panelWidth).Render(b.String())
	return lipgloss.Place(max(1, m.width-4), max(1, height), lipgloss.Center, lipgloss.Bottom, panel)
}

func (m model) renderSessionPicker() string {
	if len(m.sessions) == 0 {
		return mutedSt.Render("no sessions yet · esc then /new to create one")
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("sessions") + "\n")
	b.WriteString(mutedSt.Render("↑/↓ select · enter activate · esc close") + "\n\n")
	for i, session := range m.sessions {
		active := " "
		if session.ID == m.session {
			active = "•"
		}
		line := fmt.Sprintf("%s %-28s %3d msgs  %s", active, oneLine(session.Title, 28), session.MessageCount, mutedSt.Render(short(session.ID, 14)))
		if i == m.sessionCursor {
			line = selectSt.Render("  " + line)
		} else {
			line = mutedSt.Render("  ") + line
		}
		b.WriteString(line)
		if i < len(m.sessions)-1 {
			b.WriteString("\n")
		}
	}
	return lipgloss.NewStyle().Background(surface).Padding(1, 2).Width(max(20, m.width-4)).Render(b.String())
}

func (m *model) refreshViewport() {
	previousYOffset := m.viewport.YOffset()
	m.toolHitboxes = nil
	var b strings.Builder
	contentLine := 0
	for i, message := range m.messages {
		if i > 0 {
			b.WriteString("\n\n")
			contentLine += 2
		}
		if message.Role == "tool" {
			startLine := contentLine
			rendered := m.renderToolGroup(message.ToolGroup, max(20, m.viewport.Width()))
			b.WriteString(rendered)
			lineCount := renderedLineCount(rendered)
			if message.ToolGroup != nil {
				// Only the tool row header is clickable. Expanded detail rows often contain
				// selectable text/previews and should not toggle the group accidentally.
				m.toolHitboxes = append(m.toolHitboxes, toolHitbox{GroupID: message.ToolGroup.ID, StartY: startLine, EndY: startLine})
			}
			contentLine += lineCount
			continue
		}
		switch message.Role {
		case "user":
			b.WriteString(userName.Render("You"))
		case "error":
			b.WriteString(errSt.Bold(true).Render("Error"))
		default:
			b.WriteString(aiName.Render("Qubit"))
		}
		b.WriteString("\n")
		contentLine++
		cacheable := !(m.streaming && i == m.streamingMessageIndex)
		rendered := m.renderMessageContent(message, cacheable)
		b.WriteString(rendered)
		contentLine += renderedLineCount(rendered)
	}
	m.viewport.SetContent(b.String())
	m.restoreViewportPosition(previousYOffset)
}

func (m *model) restoreViewportPosition(yOffset int) {
	if m.autoScroll {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(clampInt(yOffset, 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
}

func (m *model) renderMessageContent(message chatMessage, cacheable bool) string {
	width := max(20, m.viewport.Width())
	key := renderCacheKey{Role: message.Role, Content: message.Content, Width: width}
	if cacheable && m.renderCache != nil {
		if cached, ok := m.renderCache[key]; ok {
			return cached
		}
	}

	rendered := ""
	if message.Role == "error" {
		rendered = wrap(message.Content, width)
	} else {
		markdown, err := renderMarkdown(message.Content, width)
		if err != nil {
			rendered = wrap(message.Content, width)
		} else {
			rendered = strings.TrimRight(stripBackgroundANSI(markdown), "\n")
		}
	}
	if cacheable && m.renderCache != nil {
		m.renderCache[key] = rendered
	}
	return rendered
}

func renderMarkdown(markdown string, width int) (string, error) {
	renderWidth := max(20, width)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(noBackgroundMarkdownStyle()),
		glamour.WithWordWrap(renderWidth),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return "", fmt.Errorf("create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return rendered, nil
}

func noBackgroundMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	style.H1.BackgroundColor = nil
	style.Code.BackgroundColor = nil
	if style.CodeBlock.Chroma != nil {
		style.CodeBlock.Chroma.Error.BackgroundColor = nil
		style.CodeBlock.Chroma.Background.BackgroundColor = nil
	}
	return style
}

func stripBackgroundANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\x1b' || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(s[i])
			i++
			continue
		}

		end := i + 2
		for end < len(s) && s[end] != 'm' {
			end++
		}
		if end >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}

		sequence := s[i+2 : end]
		kept := keepNonBackgroundSGR(sequence)
		if len(kept) > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strings.Join(kept, ";"))
			b.WriteByte('m')
		}
		i = end + 1
	}
	return b.String()
}

func keepNonBackgroundSGR(sequence string) []string {
	if sequence == "" {
		return []string{"0"}
	}

	parts := strings.Split(sequence, ";")
	kept := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		switch part {
		case "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "100", "101", "102", "103", "104", "105", "106", "107":
			if part == "48" {
				i = skipExtendedSGR(parts, i)
			}
			continue
		default:
			kept = append(kept, part)
		}
	}
	return kept
}

func skipExtendedSGR(parts []string, i int) int {
	if i+1 >= len(parts) {
		return i
	}
	switch parts[i+1] {
	case "5":
		if i+2 < len(parts) {
			return i + 2
		}
	case "2":
		if i+4 < len(parts) {
			return i + 4
		}
	}
	return i + 1
}

func renderChat(content string, width int, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = chatStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

func renderFixedHeight(content string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
