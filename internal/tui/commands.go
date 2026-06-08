package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var slashCommands = []slashCommand{
	{Name: "new", Usage: "/new [title]", Description: "Create a new chat session", NeedsArg: false},
	{Name: "fork", Usage: "/fork [title|message-number]", Description: "Fork here or edit a numbered user message", NeedsArg: false},
	{Name: "compact", Usage: "/compact", Description: "Summarize this session into a compact continuation fork", NeedsArg: false},
	{Name: "tree", Usage: "/tree", Description: "Open the fork tree", NeedsArg: false, OpensOnSelect: true},
	{Name: "sessions", Usage: "/sessions", Description: "Open the session picker", NeedsArg: false, OpensOnSelect: true},
	{Name: "md-editor", Usage: "/md-editor", Description: "Edit project Markdown docs", NeedsArg: false, OpensOnSelect: true},
	{Name: "favourite-session", Usage: "/favourite-session", Description: "Favourite the current session", NeedsArg: false},
	{Name: "keys", Usage: "/keys", Description: "Manage provider API keys", NeedsArg: false, OpensOnSelect: true},
	{Name: "models", Usage: "/models", Description: "Choose the active model", NeedsArg: false, OpensOnSelect: true},
	{Name: "providers", Usage: "/providers", Description: "Choose the active provider", NeedsArg: false, OpensOnSelect: true},
	{Name: "subagents", Usage: "/subagents", Description: "Choose the default subagent model", NeedsArg: false, OpensOnSelect: true},
	{Name: "mcp", Usage: "/mcp", Description: "Manage Model Context Protocol servers", NeedsArg: false, OpensOnSelect: true},
	{Name: "codex-login", Usage: "/codex-login", Description: "Sign in to ChatGPT Codex", NeedsArg: false},
	{Name: "codex-status", Usage: "/codex-status", Description: "Show ChatGPT Codex sign-in status", NeedsArg: false},
	{Name: "codex-logout", Usage: "/codex-logout", Description: "Sign out of ChatGPT Codex", NeedsArg: false},
	{Name: "theme", Usage: "/theme", Description: "Customize terminal colors", NeedsArg: false, OpensOnSelect: true},
	{Name: "rename", Usage: "/rename <title>", Description: "Rename current session", NeedsArg: true},
	{Name: "terminal-setup", Usage: "/terminal-setup", Description: "Install Windows Terminal keyboard and appearance setup", NeedsArg: false},
	{Name: "permission", Usage: "/permission <plan|edit|allow-all>", Description: "Set tool permission mode", NeedsArg: true},
	{Name: "cwd-remove-block", Usage: "/cwd-remove-block", Description: "Allow tools to access paths outside launch cwd", NeedsArg: false},
	{Name: "cwd-enable-block", Usage: "/cwd-enable-block", Description: "Restrict tools to launch cwd", NeedsArg: false},
	{Name: "reasoning", Usage: "/reasoning <none|low|medium|high>", Description: "Set model reasoning effort", NeedsArg: true},
	{Name: "permission-test", Usage: "/permission-test", Description: "Open a demo permission modal", NeedsArg: false},
	{Name: "help", Usage: "/help", Description: "Show command help", NeedsArg: false},
}

func (m model) updateSessionPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.sessionSearchMode {
		return m.updateSessionPickerSearch(msg)
	}

	switch msg.String() {
	case "esc":
		m.closeSessionPicker()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "s", "S":
		m.sessionSearchMode = true
		m.sessionSearchQuery = ""
		m.sessionCursor = 0
		m.status = "search sessions"
		return m, nil
	case "up", "k", "ctrl+p":
		m.moveSessionCursor(-1)
		return m, nil
	case "left":
		m.moveSessionCursor(-5)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveSessionCursor(1)
		return m, nil
	case "right":
		m.moveSessionCursor(5)
		return m, nil
	case "d", "delete":
		return m.openSessionDeleteConfirm()
	case "t", "T":
		return m.openSelectedSessionForkTree()
	case "enter":
		return m.activateSelectedSession()
	}
	return m, nil
}

func (m model) updateSessionPickerSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sessionSearchMode = false
		m.sessionSearchQuery = ""
		m.ensureSessionCursor()
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.activateSelectedSession()
	case "up", "ctrl+p":
		m.moveSessionCursor(-1)
		return m, nil
	case "left":
		m.moveSessionCursor(-5)
		return m, nil
	case "down", "ctrl+n":
		m.moveSessionCursor(1)
		return m, nil
	case "right":
		m.moveSessionCursor(5)
		return m, nil
	case "backspace", "ctrl+h":
		if m.sessionSearchQuery != "" {
			runes := []rune(m.sessionSearchQuery)
			m.sessionSearchQuery = string(runes[:len(runes)-1])
			m.sessionCursor = 0
		}
		return m, nil
	case "delete":
		return m, nil
	}
	if msg.Text != "" {
		m.sessionSearchQuery += msg.Text
		m.sessionCursor = 0
		return m, nil
	}
	return m, nil
}

func (m *model) closeSessionPicker() {
	m.mode = modeChat
	m.sessionSearchMode = false
	m.sessionSearchQuery = ""
	m.status = "ready"
}

func (m model) activateSelectedSession() (tea.Model, tea.Cmd) {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 {
		m.closeSessionPicker()
		return m, nil
	}
	m.ensureSessionCursorInBounds()
	session := sessions[m.sessionCursor]
	m.mode = modeChat
	m.sessionSearchMode = false
	m.sessionSearchQuery = ""
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

func (m model) openSelectedSessionForkTree() (tea.Model, tea.Cmd) {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 {
		return m, nil
	}
	m.ensureSessionCursorInBounds()
	return m.openForkTreeForSession(sessions[m.sessionCursor].ID)
}

func (m model) openSessionDeleteConfirm() (tea.Model, tea.Cmd) {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 || len(m.sessions) <= 1 {
		m.status = "cannot delete only session"
		return m, nil
	}
	m.ensureSessionCursorInBounds()
	session := sessions[m.sessionCursor]
	m.previousMode = modeSessionPicker
	m.mode = modeModal
	m.pendingDeleteSession = session
	m.modal = &modalState{
		ID:          "session_delete_confirm",
		Kind:        modalKindConfirm,
		Title:       "Delete session?",
		Description: "This removes the session from Qubit and deletes its stored transcript.",
		Fields:      []modalField{{Label: "Session", Value: session.Title}},
		Actions: []modalAction{
			{ID: "delete", Label: "Delete", Style: "danger"},
			{ID: "cancel", Label: "Cancel", Default: true},
		},
		Cursor:  1,
		Payload: map[string]any{"action": "session.delete", "sessionId": session.ID},
	}
	m.status = "confirm session delete"
	return m, nil
}

func (m model) sessionPickerSessions() []sessionInfo {
	favourites := make([]sessionInfo, 0, len(m.sessions))
	recent := make([]sessionInfo, 0, len(m.sessions))
	query := strings.ToLower(strings.TrimSpace(m.sessionSearchQuery))
	for _, session := range m.sessions {
		if session.Hidden {
			continue
		}
		if session.ForkedFromSessionID != "" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(session.Title), query) {
			continue
		}
		if strings.TrimSpace(session.FavouritedAt) != "" {
			favourites = append(favourites, session)
		} else {
			recent = append(recent, session)
		}
	}
	sortSessionsByRecentActivity(favourites)
	sortSessionsByRecentActivity(recent)
	return append(favourites, recent...)
}

func sortSessionsByRecentActivity(sessions []sessionInfo) {
	sort.SliceStable(sessions, func(i, j int) bool {
		left := sessionRecentTimestamp(sessions[i])
		right := sessionRecentTimestamp(sessions[j])
		if left == right {
			return false
		}
		return left > right
	})
}

func sessionRecentTimestamp(session sessionInfo) string {
	if strings.TrimSpace(session.UpdatedAt) != "" {
		return session.UpdatedAt
	}
	return session.CreatedAt
}

func (m *model) ensureSessionCursor() {
	m.ensureSessionCursorInBounds()
	m.setSessionCursorForSession(m.session)
}

func (m *model) setSessionCursorForSession(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	for i, session := range m.sessionPickerSessions() {
		if session.ID == sessionID {
			m.sessionCursor = i
			return true
		}
	}
	return false
}

func (m *model) ensureSessionCursorInBounds() {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	if m.sessionCursor < 0 || m.sessionCursor >= len(sessions) {
		m.sessionCursor = 0
	}
}

func (m *model) moveSessionCursor(delta int) {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	m.sessionCursor = moveListCursor(m.sessionCursor, len(sessions), delta)
}

func (m model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(input)
	cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	arg := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	if (m.busy || m.streaming || m.activeRunID != "") && !slashCommandRunsDuringActiveRun(cmd) {
		m.appendLocalStatus("Command queued status only: /" + cmd + " cannot run while a model response is active.")
		return m, nil
	}

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
		if point, ok := m.forkPointByNumber(arg); ok {
			return m.startMessageEdit(point), nil
		}
		return m.requestFork(len(m.messages), arg)
	case "compact", "compress":
		return m.requestCompaction("manual", "")
	case "tree", "branches", "forks", "map":
		return m.openForkTree()
	case "sessions", "session", "ls":
		m.mode = modeSessionPicker
		m.sessionCursor = 0
		m.busy = true
		m.status = "loading sessions"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.list"})
	case "md-editor", "md", "docs":
		return m.openMdEditor()
	case "favourite-session", "favorite-session", "favourite", "favorite":
		if strings.TrimSpace(m.session) == "" {
			m.appendSystem("No active session to favourite.")
			return m, nil
		}
		m.busy = true
		m.status = "favouriting session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.favourite", "sessionId": m.session})
	case "keys", "key":
		return m.openKeyPicker()
	case "models", "model", "list":
		m.mode = modeModal
		m.busy = true
		m.status = "loading models"
		return m, sendRuntime(m.runtime, map[string]any{"type": "model.list"})
	case "providers", "provider":
		return m.openProviderSelectorModal(), nil
	case "subagents", "subagent":
		m.mode = modeModal
		m.busy = true
		m.status = "loading subagent models"
		return m, sendRuntime(m.runtime, map[string]any{"type": "subagent.config"})
	case "mcp", "mcps":
		return m.openMcpManager()
	case "codex-login", "codexlogin":
		m.status = "starting Codex login"
		m.appendSystemDirect("Starting ChatGPT Codex sign-in...")
		m.busy = true
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
		return m.openTerminalSetupConfirm(), nil
	case "permission", "permissions", "perm":
		return m.setPermissionMode(arg)
	case "cwd-remove-block", "cwd-unblock", "cwd-open":
		return m.setCwdBlockEnabled(false), nil
	case "cwd-enable-block", "cwd-block", "cwd-close":
		return m.setCwdBlockEnabled(true), nil
	case "reasoning", "reason", "effort":
		return m.setReasoningLevel(arg)
	case "permission-test", "modal-test":
		return m.openDemoPermissionModal(), nil
	case "help", "h":
		m.appendSystem("Commands:\n/new [title] - create a new chat\n/fork [title|message-number] - fork current chat or edit a numbered user message\n/compact - summarize this session into a compact continuation fork\n/tree - open the fork tree\n/sessions - open the session picker\n/favourite-session - favourite the current session\n/keys - manage provider API keys in the OS keychain\n/providers - choose the active provider\n/models - choose the active provider's model\n/codex-login - sign in to ChatGPT Codex\n/codex-status - show ChatGPT Codex sign-in status\n/codex-logout - sign out of ChatGPT Codex\n/theme - customize terminal colors\n/rename <title> - rename current chat\n/terminal-setup - install Windows Terminal keyboard and appearance setup\n/permission <plan|edit|allow-all> - switch tool permission mode\n/reasoning <none|low|medium|high> - set model reasoning effort\n/permission-test - open a demo permission modal\n/help - show this help")
		return m, nil
	default:
		m.appendSystem("Unknown command. Try /help")
		return m, nil
	}
}

func (m model) requestCompaction(reason string, pendingInput string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.session) == "" {
		m.appendSystem("No active session to compact.")
		return m, nil
	}
	runID := newRunID()
	m.busy = true
	m.compacting = true
	m.pendingCompactInput = pendingInput
	m.activeRunID = runID
	m.status = "compacting"
	m.autoNewSessionOnChat = false
	payload := map[string]any{"type": "chat.compact", "sessionId": m.session, "runId": runID, "reason": reason, "systemPromptMode": m.systemPromptMode(), "reasoningLevel": m.reasoningLevelValue(), "cwdBlockEnabled": m.cwdBlockEnabled}
	if strings.TrimSpace(pendingInput) != "" {
		payload["pendingInput"] = pendingInput
	}
	return m, tea.Batch(sendRuntime(m.runtime, payload), m.spinner.Tick)
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
		points = append(points, forkPoint{Number: messageDisplayNumber(m.messages, i), MessageIndex: i + 1, EditMessageIndex: i, Content: content})
	}
	return points
}

func (m model) updateForkSelector(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.forkSelector.Cursor >= 0 && m.forkSelector.Cursor < len(m.forkSelector.Points) {
			point := m.forkSelector.Points[m.forkSelector.Cursor]
			return m.startMessageEdit(point), nil
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

func (m model) forkPointByNumber(arg string) (forkPoint, bool) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return forkPoint{}, false
	}
	number, err := strconv.Atoi(fields[0])
	if err != nil || number <= 0 {
		return forkPoint{}, false
	}
	for _, point := range m.forkPoints() {
		if point.Number == number {
			return point, true
		}
	}
	return forkPoint{}, false
}

func (m model) startMessageEdit(point forkPoint) model {
	m.forkSelector = forkSelectorState{}
	m.messageEdit = messageEditState{Active: true, MessageIndex: point.EditMessageIndex, Original: point.Content}
	m.composer.SetValue(point.Content)
	m.composer.MoveToEnd(false)
	m.status = fmt.Sprintf("editing message %d", point.Number)
	m.layout()
	return m
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
		m.composer.SetValue(fmt.Sprintf("/fork %d %s", point.Number, oneLine(point.Content, max(20, m.width-12))))
	}
	m.layout()
}

func (m model) setPermissionMode(arg string) (tea.Model, tea.Cmd) {
	mode := strings.ToLower(strings.TrimSpace(arg))
	switch mode {
	case "plan", "ask", "a", "p":
		m.permissionMode = permissionModeAsk
		m.status = "ready"
		m.appendSystem("Mode: plan. Tool permissions will ask before running gated tools, except planMd.")
	case "edit", "always", "always-allow", "always_allow", "allow", "auto", "auto-allow", "e":
		m.permissionMode = permissionModeAlwaysAllow
		m.status = "ready"
		m.appendSystem("Mode: edit. Gated tools will be allowed automatically for this session.")
	case "allow-all", "allow_all", "all", "approve-all", "approve_all":
		m.permissionMode = permissionModeAllowAll
		m.status = "ready"
		m.appendSystem("Mode: allow all. Gated tools will be allowed automatically for this TUI session while keeping the plan prompt.")
	case "":
		m.appendSystem(fmt.Sprintf("Current mode: %s. Usage: /permission <plan|edit|allow-all>", m.permissionModeLabel()))
	default:
		m.appendSystem("Usage: /permission <plan|edit|allow-all>")
	}
	return m, nil
}

func (m model) setCwdBlockEnabled(enabled bool) model {
	m.cwdBlockEnabled = enabled
	m.status = "ready"
	if enabled {
		m.appendSystem("CWD block enabled. Tools are restricted to the launch cwd.")
	} else {
		m.appendSystem("CWD block removed. Tools may access paths outside the launch cwd when otherwise permitted.")
	}
	return m
}

func (m model) setReasoningLevel(arg string) (tea.Model, tea.Cmd) {
	level := normalizeReasoningLevel(arg)
	if strings.TrimSpace(arg) == "" {
		m.appendSystem(fmt.Sprintf("Current reasoning: %s. Usage: /reasoning <none|low|medium|high>", m.reasoningLevelValue()))
		return m, nil
	}
	if level == "" {
		m.appendSystem("Usage: /reasoning <none|low|medium|high>")
		return m, nil
	}
	m.busy = true
	m.status = "setting reasoning"
	return m, sendRuntime(m.runtime, map[string]any{"type": "reasoning.set", "level": string(level)})
}

func normalizeReasoningLevel(value string) reasoningLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "off", "disable", "disabled", "0":
		return reasoningLevelNone
	case "low", "l":
		return reasoningLevelLow
	case "medium", "med", "m", "":
		return reasoningLevelMedium
	case "high", "h":
		return reasoningLevelHigh
	default:
		return ""
	}
}

func (m model) reasoningLevelValue() string {
	level := normalizeReasoningLevel(string(m.reasoningLevel))
	if level == "" {
		level = reasoningLevelMedium
	}
	return string(level)
}

func (m model) cyclePermissionMode() (tea.Model, tea.Cmd) {
	switch m.permissionMode {
	case permissionModeAsk:
		m.permissionMode = permissionModeAlwaysAllow
	case permissionModeAlwaysAllow:
		m.permissionMode = permissionModeAllowAll
	default:
		m.permissionMode = permissionModeAsk
	}
	return m, nil
}

func (m model) permissionModeLabel() string {
	switch m.permissionMode {
	case permissionModeAlwaysAllow:
		return "edit"
	case permissionModeAllowAll:
		return "allow all"
	default:
		return "plan"
	}
}

func (m *model) applyReasoningUpdated(ev runtimeEvent) {
	m.busy = false
	m.status = "ready"
	if ev.ReasoningLevel != "" {
		m.reasoningLevel = normalizeReasoningLevel(ev.ReasoningLevel)
	}
	m.appendSystem(fallback(ev.Status, "Reasoning: "+m.reasoningLevelValue()))
}

func (m model) systemPromptMode() string {
	if m.permissionMode == permissionModeAlwaysAllow {
		return "edit"
	}
	return "plan"
}

func (m model) shouldAutoAllowPermission(ev runtimeEvent) bool {
	if m.permissionMode == permissionModeAlwaysAllow || m.permissionMode == permissionModeAllowAll {
		return true
	}
	if m.permissionMode != permissionModeAsk {
		return false
	}
	if ev.ToolName == "planMd" || ev.ToolName == "subagent" {
		return true
	}
	return ev.ToolName == "editFile" && boolMetadata(ev.Metadata, "planModeAutoAllowProjectPlansOnly")
}

func boolMetadata(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	return false
}

func (m model) openForkTree() (tea.Model, tea.Cmd) {
	return m.openForkTreeForSession(m.session)
}

func (m model) openForkTreeForSession(sessionID string) (tea.Model, tea.Cmd) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = m.session
	}
	previousMode := m.mode
	if m.mode == modeForkTree && m.previousMode == modeSessionPicker {
		previousMode = modeSessionPicker
	}
	m.previousMode = previousMode
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.forkTree.FocalSessionID = sessionID
	m.busy = true
	m.status = "loading fork tree"
	return m, sendRuntime(m.runtime, map[string]any{"type": "session.tree", "sessionId": sessionID})
}

func (m model) showSlashPalette() bool {
	value := m.composer.Value()
	return strings.HasPrefix(value, "/") && !strings.Contains(value, " ") && m.mode == modeChat && m.ready && !m.forkSelector.Active
}

func slashCommandRunsDuringActiveRun(cmd string) bool {
	switch strings.ToLower(strings.TrimSpace(cmd)) {
	case "help", "h", "permission", "permissions", "perm", "cwd-remove-block", "cwd-enable-block", "cwd-unblock", "cwd-block", "cwd-open", "cwd-close", "theme", "themes", "colors", "color", "permission-test", "modal-test", "md-editor", "md", "docs", "subagents", "subagent", "mcp", "mcps":
		return true
	default:
		return false
	}
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
	m.slashCursor = moveListCursor(m.slashCursor, len(matches), delta)
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
	appearance := "Appearance defaults: JetBrains Mono, size 11, line height 1.05, padding 8."
	if result.Err != nil {
		return strings.TrimSpace(fmt.Sprintf(`Windows Terminal setup failed

%s

Qubit still supports Ctrl+J for a reliable newline.

%s

Manual settings snippets:

profiles.defaults:

`+"```json"+`
{
  "font": {
    "face": "JetBrains Mono",
    "size": 11,
    "lineHeight": 1.05
  },
  "padding": "8"
}
`+"```"+`

Shift+Enter action:

`+"```json"+`
{
  "command": {
    "action": "sendInput",
    "input": "\\u001b[13;2u"
  },
  "keys": "shift+enter"
}
`+"```", result.Err, appearance))
	}

	if !result.Changed {
		return fmt.Sprintf("Windows Terminal keyboard and appearance setup is already installed.\n\n%s\nSettings: %s\n\nRestart Qubit/Windows Terminal if Shift+Enter or font changes do not appear. Ctrl+J remains available as a reliable newline.", appearance, result.SettingsPath)
	}

	return fmt.Sprintf("Windows Terminal keyboard and appearance setup installed.\n\n%s\nSettings: %s\nBackup: %s\n\nFully close and reopen Windows Terminal, then restart Qubit. Shift+Enter should insert a newline; font changes require the font to be installed on Windows. Ctrl+J remains available as a reliable fallback.", appearance, result.SettingsPath, result.BackupPath)
}
