package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var slashCommands = []slashCommand{
	{Name: "new", Usage: "/new [title]", Description: "Create a new chat session", NeedsArg: false},
	{Name: "sessions", Usage: "/sessions", Description: "Open the session picker", NeedsArg: false},
	{Name: "rename", Usage: "/rename <title>", Description: "Rename current session", NeedsArg: true},
	{Name: "terminal-setup", Usage: "/terminal-setup", Description: "Install Windows Terminal Shift+Enter newline setup", NeedsArg: false},
	{Name: "permission-test", Usage: "/permission-test", Description: "Open a demo permission modal", NeedsArg: false},
	{Name: "help", Usage: "/help", Description: "Show command help", NeedsArg: false},
}

func (m model) updateSessionPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		m.moveSessionCursor(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveSessionCursor(1)
		return m, nil
	case "enter":
		if len(m.sessions) == 0 {
			m.mode = modeChat
			return m, nil
		}
		m.ensureSessionCursor()
		session := m.sessions[m.sessionCursor]
		m.mode = modeChat
		m.busy = true
		m.status = "switching session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.activate", "sessionId": session.ID})
	}
	return m, nil
}

func (m *model) ensureSessionCursor() {
	if len(m.sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	if m.sessionCursor < 0 || m.sessionCursor >= len(m.sessions) {
		m.sessionCursor = 0
	}
	for i, session := range m.sessions {
		if session.ID == m.session {
			m.sessionCursor = i
			return
		}
	}
}

func (m *model) moveSessionCursor(delta int) {
	if len(m.sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	m.sessionCursor = (m.sessionCursor + delta + len(m.sessions)) % len(m.sessions)
}

func (m model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(input)
	cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	arg := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))

	switch cmd {
	case "new", "n":
		title := arg
		if title == "" {
			title = "New chat"
		}
		m.busy = true
		m.status = "creating session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.new", "title": title})
	case "sessions", "session", "ls":
		m.mode = modeSessionPicker
		m.ensureSessionCursor()
		m.busy = true
		m.status = "loading sessions"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.list"})
	case "rename", "title":
		if arg == "" {
			m.appendSystem("Usage: /rename <title>")
			return m, nil
		}
		m.busy = true
		m.status = "renaming session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.rename", "sessionId": m.session, "title": arg})
	case "terminal-setup", "terminal", "setup":
		m.busy = true
		m.status = "updating terminal settings"
		m.appendSystem("Updating Windows Terminal settings for Shift+Enter newline support...")
		return m, runTerminalSetup()
	case "permission-test", "modal-test":
		return m.openDemoPermissionModal(), nil
	case "help", "h":
		m.appendSystem("Commands:\n/new [title] - create a new chat\n/sessions - open the session picker\n/rename <title> - rename current chat\n/terminal-setup - install Windows Terminal Shift+Enter newline setup\n/permission-test - open a demo permission modal\n/help - show this help")
		return m, nil
	default:
		m.appendSystem("Unknown command. Try /help")
		return m, nil
	}
}

func (m model) showSlashPalette() bool {
	value := m.composer.Value()
	return strings.HasPrefix(value, "/") && !strings.Contains(value, " ") && m.mode == modeChat && !m.busy && m.ready
}

func (m model) filteredSlashCommands() []slashCommand {
	query := strings.ToLower(strings.TrimPrefix(m.composer.Value(), "/"))
	var matches []slashCommand
	for _, command := range slashCommands {
		if query == "" || strings.Contains(command.Name, query) || strings.Contains(strings.ToLower(command.Description), query) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (m *model) moveSlashCursor(delta int) {
	matches := m.filteredSlashCommands()
	if len(matches) == 0 {
		m.slashCursor = 0
		return
	}
	m.slashCursor = (m.slashCursor + delta + len(matches)) % len(matches)
}

func (m model) acceptSlashSelection() (tea.Model, tea.Cmd) {
	matches := m.filteredSlashCommands()
	if len(matches) == 0 {
		return m, nil
	}
	if m.slashCursor < 0 || m.slashCursor >= len(matches) {
		m.slashCursor = 0
	}
	command := matches[m.slashCursor]
	if command.Name == "sessions" {
		m.composer.Reset()
		return m.handleSlashCommand("/sessions")
	}
	value := "/" + command.Name + " "
	m.composer.SetValue(value)
	m.composer.MoveToEnd(false)
	return m, nil
}

func (m model) renderSlashPalette() string {
	matches := m.filteredSlashCommands()
	if len(matches) == 0 {
		return mutedSt.Render("no matching commands")
	}
	maxItems := min(6, len(matches))
	var b strings.Builder
	b.WriteString(mutedSt.Render("commands") + "\n")
	for i := 0; i < maxItems; i++ {
		command := matches[i]
		line := fmt.Sprintf("  %-16s %s", command.Usage, mutedSt.Render(command.Description))
		if i == m.slashCursor {
			line = selectSt.Render("  " + fmt.Sprintf("%-16s %s", command.Usage, command.Description))
		}
		b.WriteString(line)
		if i < maxItems-1 {
			b.WriteString("\n")
		}
	}
	return lipgloss.NewStyle().Background(surface).Padding(1, 2).Width(max(20, m.width-4)).Render(b.String())
}

func terminalSetupResultMessage(result terminalSetupResult) string {
	if result.Err != nil {
		return strings.TrimSpace(fmt.Sprintf(`Windows Terminal setup failed

%s

Qubit still supports Ctrl+J for a reliable newline.

Manual settings snippet:

`+"```json"+`
{
  "command": {
    "action": "sendInput",
    "input": "\\u001b[13;2u"
  },
  "keys": "shift+enter"
}
`+"```", result.Err))
	}

	if !result.Changed {
		return fmt.Sprintf("Windows Terminal Shift+Enter setup is already installed.\n\nSettings: %s\n\nRestart Qubit/Windows Terminal if Shift+Enter still does not work. Ctrl+J remains available as a reliable newline.", result.SettingsPath)
	}

	return fmt.Sprintf("Windows Terminal Shift+Enter setup installed.\n\nSettings: %s\nBackup: %s\n\nFully close and reopen Windows Terminal, then restart Qubit. Shift+Enter should insert a newline. Ctrl+J remains available as a reliable fallback.", result.SettingsPath, result.BackupPath)
}
