package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return newAppView("loading...")
	}

	content := appStyle.Width(m.width).Height(m.height).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderHeader(),
			m.renderChatArea(),
			m.renderInput(),
			m.renderFooter(),
		),
	)
	return newAppView(content)
}

func newAppView(content string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeNone
	return view
}

func (m model) renderHeader() string {
	provider := fallback(m.provider, "...")
	modelName := fallback(m.model, "...")
	sessionTitle := fallback(m.title, m.currentSessionTitle())
	sessionTitle = fallback(sessionTitle, "untitled")

	activity := okSt.Render(m.status)
	if m.busy {
		activity = m.spinner.View() + " " + lipgloss.NewStyle().Foreground(accent).Render(m.status)
	} else if strings.Contains(m.status, "error") || m.err != "" {
		activity = errSt.Render(m.status)
	}

	appName := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("qubit")
	meta := mutedSt.Render(fmt.Sprintf("%s · %s · %s", provider, modelName, short(m.session, 12)))
	headerLeft := fmt.Sprintf("%s  %s", appName, activity)
	headerRight := oneLine(sessionTitle, max(12, m.width-lipgloss.Width(headerLeft)-lipgloss.Width(meta)-8))
	headerText := fmt.Sprintf("%s  %s  %s", headerLeft, mutedSt.Render(headerRight), meta)
	return headerStyle.Width(m.width).Render(headerText)
}

func (m model) renderChatArea() string {
	chatContent := m.viewport.View()
	if m.mode == modeSessionPicker {
		chatContent = m.renderSessionPicker()
	} else if m.showSlashPalette() {
		chatContent = lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), "", m.renderSlashPalette())
	}
	return chatStyle.Width(m.width).Height(max(1, m.height-5)).Render(chatContent)
}

func (m model) renderInput() string {
	return inputStyle.Width(m.width).Render(m.input.View())
}

func (m model) renderFooter() string {
	footer := "enter send · / commands · pgup/pgdn scroll · esc quit"
	if m.mode == modeSessionPicker {
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
	var b strings.Builder
	for i, message := range m.messages {
		if i > 0 {
			b.WriteString("\n\n")
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
		b.WriteString(wrap(message.Content, max(20, m.viewport.Width())))
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}
