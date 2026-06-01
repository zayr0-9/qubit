package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var slashCommands = []slashCommand{
	{Name: "new", Usage: "/new [title]", Description: "Create a new chat session", NeedsArg: false},
	{Name: "fork", Usage: "/fork [title]", Description: "Fork current chat from here or a previous user message", NeedsArg: false},
	{Name: "tree", Usage: "/tree", Description: "Open the fork tree", NeedsArg: false, OpensOnSelect: true},
	{Name: "sessions", Usage: "/sessions", Description: "Open the session picker", NeedsArg: false, OpensOnSelect: true},
	{Name: "keys", Usage: "/keys", Description: "Manage provider API keys", NeedsArg: false, OpensOnSelect: true},
	{Name: "models", Usage: "/models", Description: "Choose the active model", NeedsArg: false, OpensOnSelect: true},
	{Name: "providers", Usage: "/providers", Description: "Choose the active provider", NeedsArg: false, OpensOnSelect: true},
	{Name: "codex-login", Usage: "/codex-login", Description: "Sign in to ChatGPT Codex", NeedsArg: false},
	{Name: "codex-status", Usage: "/codex-status", Description: "Show ChatGPT Codex sign-in status", NeedsArg: false},
	{Name: "codex-logout", Usage: "/codex-logout", Description: "Sign out of ChatGPT Codex", NeedsArg: false},
	{Name: "theme", Usage: "/theme", Description: "Customize terminal colors", NeedsArg: false, OpensOnSelect: true},
	{Name: "rename", Usage: "/rename <title>", Description: "Rename current session", NeedsArg: true},
	{Name: "terminal-setup", Usage: "/terminal-setup", Description: "Install Windows Terminal Shift+Enter newline setup", NeedsArg: false},
	{Name: "permission", Usage: "/permission <plan|edit>", Description: "Set plan/edit mode", NeedsArg: true},
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
	case "fork", "branch":
		if arg == "" {
			return m.startForkSelector(), nil
		}
		return m.requestFork(len(m.messages), arg)
	case "tree", "branches", "forks", "map":
		m.mode = modeForkTree
		m.forkTree = newForkTreeState()
		m.busy = true
		m.status = "loading fork tree"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.tree", "sessionId": m.session})
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
	case "providers", "provider":
		return m.openProviderSelectorModal(), nil
	case "codex-login", "codexlogin":
		m.busy = true
		m.status = "starting Codex login"
		m.appendSystem("Starting ChatGPT Codex sign-in...")
		return m, sendRuntime(m.runtime, map[string]any{"type": "codex.login.start"})
	case "codex-status", "codexstatus":
		m.busy = true
		m.status = "checking Codex status"
		return m, sendRuntime(m.runtime, map[string]any{"type": "codex.status"})
	case "codex-logout", "codexlogout":
		m.busy = true
		m.status = "signing out of Codex"
		return m, sendRuntime(m.runtime, map[string]any{"type": "codex.logout"})
	case "theme", "themes", "colors", "color":
		return m.openThemeEntry()
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
		m.appendSystem("Commands:\n/new [title] - create a new chat\n/fork [title] - fork current chat from here or a previous user message\n/tree - open the fork tree\n/sessions - open the session picker\n/keys - manage provider API keys in the OS keychain\n/providers - choose the active provider\n/models - choose the active provider's model\n/codex-login - sign in to ChatGPT Codex\n/codex-status - show ChatGPT Codex sign-in status\n/codex-logout - sign out of ChatGPT Codex\n/theme - customize terminal colors\n/rename <title> - rename current chat\n/terminal-setup - install Windows Terminal Shift+Enter newline setup\n/permission <plan|edit> - switch between plan and edit mode\n/permission-test - open a demo permission modal\n/help - show this help")
		return m, nil
	default:
		m.appendSystem("Unknown command. Try /help")
		return m, nil
	}
}

func (m model) requestFork(messageIndex int, title string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.autoNewSessionOnChat = false
	m.forkSelector = forkSelectorState{}
	m.status = "forking session"
	payload := map[string]any{"type": "session.fork", "sessionId": m.session, "messageIndex": messageIndex}
	if title != "" {
		payload["title"] = title
	}
	return m, sendRuntime(m.runtime, payload)
}

func (m model) startForkSelector() model {
	m.forkSelector = forkSelectorState{Active: true, Points: m.forkPoints(), Cursor: -1}
	m.status = "fork point"
	m.composer.SetValue("/fork")
	m.layout()
	return m
}

func (m model) forkPoints() []forkPoint {
	points := make([]forkPoint, 0)
	for i, message := range m.messages {
		if message.Role != "user" {
			continue
		}
		content := strings.TrimSpace(normalizeInputNewlines(message.Content))
		if content == "" {
			continue
		}
		points = append(points, forkPoint{MessageIndex: i + 1, EditMessageIndex: i, Content: content})
	}
	return points
}

func (m model) updateForkSelector(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.forkSelector.Cursor >= 0 && m.forkSelector.Cursor < len(m.forkSelector.Points) {
			point := m.forkSelector.Points[m.forkSelector.Cursor]
			m.forkSelector = forkSelectorState{}
			m.messageEdit = messageEditState{Active: true, MessageIndex: point.EditMessageIndex, Original: point.Content}
			m.composer.SetValue(point.Content)
			m.status = "editing message"
			m.layout()
			return m, nil
		}
		m.composer.Reset()
		m.layout()
		return m.requestFork(len(m.messages), "")
	case "up", "ctrl+p":
		m.moveForkSelector(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveForkSelector(1)
		return m, nil
	case "esc":
		m.forkSelector = forkSelectorState{}
		m.composer.Reset()
		m.status = "ready"
		m.layout()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	return m.updateComposerKey(msg)
}

func (m *model) moveForkSelector(delta int) {
	if !m.forkSelector.Active || len(m.forkSelector.Points) == 0 {
		return
	}
	if m.forkSelector.Cursor == -1 {
		if delta < 0 {
			m.forkSelector.Cursor = len(m.forkSelector.Points) - 1
		} else {
			return
		}
	} else {
		m.forkSelector.Cursor += delta
		if m.forkSelector.Cursor >= len(m.forkSelector.Points) {
			m.forkSelector.Cursor = -1
		} else if m.forkSelector.Cursor < 0 {
			m.forkSelector.Cursor = 0
		}
	}
	if m.forkSelector.Cursor == -1 {
		m.composer.SetValue("/fork")
	} else {
		point := m.forkSelector.Points[m.forkSelector.Cursor]
		m.composer.SetValue("/fork " + oneLine(point.Content, max(20, m.width-8)))
	}
	m.layout()
}

func (m model) setPermissionMode(arg string) (tea.Model, tea.Cmd) {
	mode := strings.ToLower(strings.TrimSpace(arg))
	switch mode {
	case "plan", "ask", "a", "p":
		m.permissionMode = permissionModeAsk
		m.status = "ready"
		m.appendSystem("Mode: plan. Tool permissions will ask before running gated tools.")
	case "edit", "always", "always-allow", "always_allow", "allow", "auto", "auto-allow", "e":
		m.permissionMode = permissionModeAlwaysAllow
		m.status = "ready"
		m.appendSystem("Mode: edit. Gated tools will be allowed automatically for this session.")
	case "":
		m.appendSystem(fmt.Sprintf("Current mode: %s. Usage: /permission <plan|edit>", m.permissionModeLabel()))
	default:
		m.appendSystem("Usage: /permission <plan|edit>")
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
		return "edit"
	}
	return "plan"
}

func (m model) systemPromptMode() string {
	if m.permissionMode == permissionModeAlwaysAllow {
		return "edit"
	}
	return "plan"
}

func (m model) showSlashPalette() bool {
	value := m.composer.Value()
	return strings.HasPrefix(value, "/") && !strings.Contains(value, " ") && m.mode == modeChat && !m.busy && m.ready && !m.forkSelector.Active
}

func (m model) filteredSlashCommands() []slashCommand {
	query := strings.ToLower(strings.TrimPrefix(m.composer.Value(), "/"))
	if query == "" {
		return append([]slashCommand(nil), slashCommands...)
	}

	var nameMatches []slashCommand
	var descriptionMatches []slashCommand
	for _, command := range slashCommands {
		name := strings.ToLower(command.Name)
		description := strings.ToLower(command.Description)
		if strings.Contains(name, query) {
			nameMatches = append(nameMatches, command)
		} else if strings.Contains(description, query) {
			descriptionMatches = append(descriptionMatches, command)
		}
	}
	return append(nameMatches, descriptionMatches...)
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

func (m model) slashCommandModalHeight(maxHeight int) int {
	matches := m.filteredSlashCommands()
	visibleOptions := min(7, len(matches))
	if visibleOptions == 0 {
		return min(max(4, maxHeight), 5)
	}
	return min(max(4, maxHeight), visibleOptions+8)
}

func (m model) renderSlashCommandModal(height int) string {
	matches := m.filteredSlashCommands()
	modal := modalState{
		Title:       "Commands",
		Description: "Tab/Enter to complete or open. Esc closes suggestions.",
		Options:     slashCommandModalOptions(matches),
	}
	if len(matches) == 0 {
		modal.Description = "No matching commands."
	}
	if m.slashCursor < 0 || m.slashCursor >= len(matches) {
		modal.OptionCursor = 0
	} else {
		modal.OptionCursor = m.slashCursor
	}
	return m.renderModalStateAligned(modal, height, lipgloss.Left, lipgloss.Bottom)
}

func slashCommandModalOptions(commands []slashCommand) []modalOption {
	options := make([]modalOption, 0, len(commands))
	for _, command := range commands {
		options = append(options, modalOption{ID: command.Name, Label: command.Usage, Description: command.Description})
	}
	return options
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
