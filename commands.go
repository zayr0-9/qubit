package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var slashCommands = []slashCommand{
	{Name: "new", Usage: "/new [title]", Description: "Create a new chat session", NeedsArg: false},
	{Name: "sessions", Usage: "/sessions", Description: "Open the session picker", NeedsArg: false, OpensOnSelect: true},
	{Name: "keys", Usage: "/keys", Description: "Manage provider API keys", NeedsArg: false, OpensOnSelect: true},
	{Name: "models", Usage: "/models", Description: "Choose the active GLM model", NeedsArg: false, OpensOnSelect: true},
	{Name: "rename", Usage: "/rename <title>", Description: "Rename current session", NeedsArg: true},
	{Name: "terminal-setup", Usage: "/terminal-setup", Description: "Install Windows Terminal Shift+Enter newline setup", NeedsArg: false},
	{Name: "permission", Usage: "/permission <ask|always>", Description: "Set tool permission mode", NeedsArg: true},
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
		m.ensureSessionCursorInBounds()
		session := m.sessions[m.sessionCursor]
		m.mode = modeChat
		m.clearFakeStream()
		m.autoScroll = true
		m.busy = true
		m.session = session.ID
		m.title = session.Title
		m.autoNewSessionOnChat = false
		m.messages = nil
		m.status = "loading transcript"
		m.layout()
		m.refreshViewport()
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.activate", "sessionId": session.ID})
	}
	return m, nil
}

func (m *model) ensureSessionCursor() {
	m.ensureSessionCursorInBounds()
	for i, session := range m.sessions {
		if session.ID == m.session {
			m.sessionCursor = i
			return
		}
	}
}

func (m *model) ensureSessionCursorInBounds() {
	if len(m.sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	if m.sessionCursor < 0 || m.sessionCursor >= len(m.sessions) {
		m.sessionCursor = 0
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
		m.autoNewSessionOnChat = false
		m.status = "creating session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.new", "title": title})
	case "sessions", "session", "ls":
		m.mode = modeSessionPicker
		m.ensureSessionCursor()
		m.busy = true
		m.status = "loading sessions"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.list"})
	case "keys", "key":
		return m.openKeyPicker()
	case "models", "model", "list":
		m.mode = modeModal
		m.busy = true
		m.status = "loading models"
		return m, sendRuntime(m.runtime, map[string]any{"type": "model.list"})
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
	case "permission", "permissions", "perm":
		return m.setPermissionMode(arg)
	case "permission-test", "modal-test":
		return m.openDemoPermissionModal(), nil
	case "help", "h":
		m.appendSystem("Commands:\n/new [title] - create a new chat\n/sessions - open the session picker\n/keys - manage provider API keys in the OS keychain\n/models - choose the active GLM model\n/rename <title> - rename current chat\n/terminal-setup - install Windows Terminal Shift+Enter newline setup\n/permission <ask|always> - choose whether gated tools ask or auto-allow\n/permission-test - open a demo permission modal\n/help - show this help")
		return m, nil
	default:
		m.appendSystem("Unknown command. Try /help")
		return m, nil
	}
}

func (m model) setPermissionMode(arg string) (tea.Model, tea.Cmd) {
	mode := strings.ToLower(strings.TrimSpace(arg))
	switch mode {
	case "ask", "a":
		m.permissionMode = permissionModeAsk
		m.status = "ready"
		m.appendSystem("Tool permissions: ask before running gated tools.")
	case "always", "always-allow", "always_allow", "allow", "auto", "auto-allow":
		m.permissionMode = permissionModeAlwaysAllow
		m.status = "ready"
		m.appendSystem("Tool permissions: always allow gated tools for this session.")
	case "":
		m.appendSystem(fmt.Sprintf("Tool permissions are currently: %s. Usage: /permission <ask|always>", m.permissionModeLabel()))
	default:
		m.appendSystem("Usage: /permission <ask|always>")
	}
	return m, nil
}

func (m model) cyclePermissionMode() (tea.Model, tea.Cmd) {
	if m.permissionMode == permissionModeAlwaysAllow {
		m.permissionMode = permissionModeAsk
	} else {
		m.permissionMode = permissionModeAlwaysAllow
	}
	return m, nil
}

func (m model) permissionModeLabel() string {
	if m.permissionMode == permissionModeAlwaysAllow {
		return "always allow"
	}
	return "ask"
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
	if command.OpensOnSelect {
		m.composer.Reset()
		return m.handleSlashCommand("/" + command.Name)
	}
	value := "/" + command.Name + " "
	m.composer.SetValue(value)
	m.composer.MoveToEnd(false)
	return m, nil
}

func (m model) renderSlashPalette() string {
	matches := m.filteredSlashCommands()
	paletteStyle := lipgloss.NewStyle().Padding(1, 2).Width(max(20, m.width-4))
	if len(matches) == 0 {
		return paletteStyle.Render(errSt.Render("✦ no matching commands"))
	}

	maxItems := min(6, len(matches))
	cmdStyle := lipgloss.NewStyle().Foreground(cyan).Bold(true)
	badgeStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	selectedBadgeStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)

	var b strings.Builder
	b.WriteString(badgeStyle.Render("✦ commands") + "  " + mutedSt.Render("tab/enter to complete") + "\n")
	for i := 0; i < maxItems; i++ {
		command := matches[i]
		marker := "  "
		usage := cmdStyle.Render(fmt.Sprintf("%-16s", command.Usage))
		description := mutedSt.Render(command.Description)
		if i == m.slashCursor {
			marker = selectedBadgeStyle.Render("› ")
		}
		b.WriteString(fmt.Sprintf("%s%s %s", marker, usage, description))
		if i < maxItems-1 {
			b.WriteString("\n")
		}
	}
	return paletteStyle.Render(b.String())
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
