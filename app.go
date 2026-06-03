package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func initialModel(rt *runtimeClient) model {
	composer := newComposer()
	theme := defaultTheme()
	inputHistory := []string(nil)
	if rt != nil {
		var err error
		if loadedTheme, err := loadThemeConfig(rt.qubitDir); err == nil && loadedTheme.Background != "" && loadedTheme.Text != "" {
			theme = loadedTheme
		}
		inputHistory, err = loadInputHistory(rt.qubitDir)
		if err != nil {
			inputHistory = nil
		}
	}
	applyTheme(theme)

	spin := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle))

	return model{
		viewport:             viewport.New(),
		composer:             composer,
		spinner:              spin,
		renderCache:          make(map[renderCacheKey]string),
		messages:             []chatMessage{{Role: "assistant", Content: "Ready. Try / for commands."}},
		status:               "starting runtime",
		permissionMode:       permissionModeAsk,
		theme:                theme,
		autoNewSessionOnChat: true,
		autoScroll:           true,
		inputHistory:         inputHistory,
		inputHistoryIndex:    len(inputHistory),
		notifier:             newPlatformNotifier(),
		runtime:              rt,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitRuntimeEvent(m.runtime), m.spinner.Tick, inputCursorPulseTick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil
	case tea.KeyboardEnhancementsMsg:
		m.keyboardEnhanced = msg.SupportsKeyDisambiguation()
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case tea.MouseWheelMsg:
		if m.mode == modeForkTree {
			return m.updateForkTreeMouseWheel(msg), nil
		}
		return m.updateMouseWheel(msg), nil
	case tea.MouseClickMsg:
		return m.updateMouseClick(msg), nil
	case runtimeMsg:
		return m.updateRuntime(runtimeEvent(msg))
	case runtimeErrMsg:
		return m.updateRuntimeError(msg.err), nil
	case sendDoneMsg:
		return m.updateSendDone(msg.err), nil
	case terminalSetupResultMsg:
		return m.updateTerminalSetupResult(terminalSetupResult(msg)), nil
	case fakeStreamTickMsg:
		return m.updateFakeStreamTick()
	case inputCursorPulseMsg:
		m.inputCursorPulse++
		if m.hasRunningToolGroup() {
			m.refreshViewport()
		}
		return m, inputCursorPulseTick()
	case toolCallRevealTickMsg:
		return m.updateToolCallRevealTick()
	case notificationResultMsg:
		return m.updateNotificationResult(msg), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.inputSpinnerActive() {
			return m, cmd
		}
		return m, nil
	case tea.PasteMsg:
		if m.mode == modeThemeEntry {
			return m.updateThemeEntryTeaPaste(msg), nil
		}
		return m.updateKeyEntryTeaPaste(msg), nil
	case composerPasteMsg:
		if m.mode == modeThemeEntry {
			return m.updateThemeEntryPaste(msg), nil
		}
		return m.updateKeyEntryPaste(msg), nil
	}

	return m.updateInputAndViewport(msg)
}

func (m model) inputSpinnerActive() bool {
	return m.busy || m.streaming || m.activeRunID != ""
}

func (m model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeModal {
		return m.updateModal(msg)
	}
	if m.mode == modeForkTree {
		return m.updateForkTree(msg)
	}
	if m.mode == modeKeyEntry {
		return m.updateKeyEntry(msg)
	}
	if m.mode == modeThemeEntry {
		return m.updateThemeEntry(msg)
	}
	if m.mode == modeKeyPicker {
		return m.updateKeyPicker(msg)
	}
	if m.mode == modeSessionPicker {
		return m.updateSessionPicker(msg)
	}
	if m.forkSelector.Active {
		return m.updateForkSelector(msg)
	}
	if isOpenForkTreeShortcut(msg) {
		m.composer.Reset()
		m.layout()
		return m.openForkTree()
	}
	if isNewlineKey(msg) {
		return m.insertInputNewline()
	}

	switch msg.String() {
	case "ctrl+c":
		if m.composer.HasSelection() {
			m.status = "copied input"
			return m, copyClipboardCmd(m.composer.SelectedText())
		}
		return m, tea.Quit
	case "esc":
		if m.showFileMentionPalette() {
			m.fileMention.Cursor = 0
			return m, nil
		}
		if m.messageEdit.Active {
			m.messageEdit = messageEditState{}
			m.composer.Reset()
			m.status = "ready"
			m.layout()
			return m, nil
		}
		if m.composer.HasSelection() {
			m.composer.ClearSelection()
			m.layout()
			return m, nil
		}
		if m.streaming || (m.busy && m.activeRunID != "") {
			runID := m.activeRunID
			m.abortActiveRun()
			if runID != "" {
				return m, sendRuntime(m.runtime, map[string]any{"type": "chat.cancel", "runId": runID})
			}
			return m, nil
		}
		m.status = "ready"
		return m, nil
	case "up", "ctrl+p":
		if m.showSlashPalette() {
			m.moveSlashCursor(-1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(-1)
			return m, nil
		}
		if next, ok := m.cycleInputHistory(-1); ok {
			return next, nil
		}
	case "down", "ctrl+n":
		if m.showSlashPalette() {
			m.moveSlashCursor(1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(1)
			return m, nil
		}
		if next, ok := m.cycleInputHistory(1); ok {
			return next, nil
		}
	case "shift+tab":
		if m.showSlashPalette() {
			m.moveSlashCursor(-1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(-1)
			return m, nil
		}
		return m.cyclePermissionMode()
	case "tab":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
		if m.showFileMentionPalette() {
			if next, ok := m.acceptFileMentionSelection(); ok {
				next.layout()
				return next, nil
			}
		}
	case "enter":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
		if m.showFileMentionPalette() {
			if next, ok := m.acceptFileMentionSelection(); ok {
				next.layout()
				return next, nil
			}
		}
		return m.submitInput()
	case "pgup":
		m.autoScroll = false
		m.viewport.PageUp()
		return m, nil
	case "pgdown":
		m.viewport.PageDown()
		m.autoScroll = m.viewport.AtBottom()
		return m, nil
	}

	return m.updateComposerKey(msg)
}

func (m model) updateComposerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	handled, cmd := m.composer.UpdateKey(msg)
	if handled {
		m.inputHistoryActive = false
		m.inputHistoryIndex = len(m.inputHistory)
		if m.showFileMentionPalette() {
			m.ensureFileMentionIndex()
			matches := m.filteredFileMentionEntries()
			if len(matches) == 0 {
				m.fileMention.Cursor = 0
			} else if m.fileMention.Cursor >= len(matches) {
				m.fileMention.Cursor = 0
			}
		}
		m.layout()
		return m, cmd
	}
	return m, nil
}

func (m model) updateMouseWheel(msg tea.MouseWheelMsg) tea.Model {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.autoScroll = false
		m.viewport.ScrollUp(max(1, m.viewport.MouseWheelDelta))
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(max(1, m.viewport.MouseWheelDelta))
		m.autoScroll = m.viewport.AtBottom()
	}
	return m
}

func (m model) updateMouseClick(msg tea.MouseClickMsg) tea.Model {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || mouse.Y < m.chatTopY || mouse.Y >= m.chatTopY+m.viewport.Height() {
		return m
	}
	contentY := m.viewport.YOffset() + mouse.Y - m.chatTopY
	for _, hitbox := range m.toolHitboxes {
		if contentY >= hitbox.StartY && contentY <= hitbox.EndY && mouse.X >= hitbox.StartX && mouse.X <= hitbox.EndX {
			m.autoScroll = false
			m.toggleToolGroup(hitbox.GroupID)
			return m
		}
	}
	return m
}

func (m model) insertInputNewline() (tea.Model, tea.Cmd) {
	m.composer.InsertNewline()
	m.layout()
	return m, nil
}

func (m model) updateInputAndViewport(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	previousYOffset := m.viewport.YOffset()
	m.viewport, cmd = m.viewport.Update(msg)
	if m.viewport.YOffset() != previousYOffset {
		m.autoScroll = m.viewport.AtBottom()
	}
	return m, cmd
}

func isOpenForkTreeShortcut(msg tea.KeyPressMsg) bool {
	keyEvent := msg.Key()
	return keyEvent.Code == tea.KeySpace && keyEvent.Mod&tea.ModCtrl != 0
}

func isNewlineKey(msg tea.KeyPressMsg) bool {
	if key.Matches(msg, inputNewlineBinding) {
		return true
	}
	keyEvent := msg.Key()
	if keyEvent.Code != tea.KeyEnter {
		return false
	}
	return keyEvent.Mod&tea.ModShift != 0 || keyEvent.Mod&tea.ModAlt != 0
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(normalizeInputNewlines(m.composer.Value()))
	if input == "" || !m.ready {
		return m, nil
	}
	input = m.expandFileMentionsForSend(input)

	m.composer.Reset()
	m.layout()
	m.err = ""
	if strings.HasPrefix(input, "/") {
		return m.handleSlashCommand(input)
	}
	if m.messageEdit.Active {
		if m.busy || m.streaming || m.activeRunID != "" {
			m.appendLocalStatus("Cannot submit an edited message while a run is active.")
			return m, nil
		}
		return m.submitMessageEdit(input)
	}
	if m.busy || m.streaming || m.activeRunID != "" {
		m.queueUserMessage(input)
		m.layout()
		return m, nil
	}
	m.recordInputHistory(input)
	m.saveInputHistory()
	return m.startChatRun(input)
}

func (m model) startChatRun(input string) (model, tea.Cmd) {
	runID := newRunID()
	m.messages = append(m.messages, chatMessage{Role: "user", Content: input})
	m.busy = true
	m.activeRunID = runID
	m.status = "thinking"
	m.autoScroll = true
	m.refreshViewport()
	payload := map[string]any{"type": "chat", "input": input, "runId": runID, "systemPromptMode": m.systemPromptMode(), "reasoningLevel": m.reasoningLevelValue()}
	if m.autoNewSessionOnChat {
		payload["newSession"] = true
		payload["title"] = titleFromInput(input)
		m.autoNewSessionOnChat = false
	} else {
		payload["sessionId"] = m.session
		m.touchLocalSessionActivity(m.session, titleFromInput(input))
	}
	return m, tea.Batch(sendRuntime(m.runtime, payload), m.spinner.Tick)
}

func (m model) submitMessageEdit(input string) (tea.Model, tea.Cmd) {
	target := clampInt(m.messageEdit.MessageIndex, 0, len(m.messages))
	runID := newRunID()

	nextMessages := append([]chatMessage(nil), m.messages[:target]...)
	nextMessages = append(nextMessages, chatMessage{Role: "user", Content: input})
	m.messages = nextMessages
	m.messageEdit = messageEditState{}
	m.busy = true
	m.activeRunID = runID
	m.status = "thinking"
	m.autoScroll = true
	m.refreshViewport()

	payload := map[string]any{"type": "chat", "input": input, "runId": runID, "sessionId": m.session, "replaceFromMessageIndex": target, "title": "Edit: " + fallback(m.title, m.currentSessionTitle()), "systemPromptMode": m.systemPromptMode(), "reasoningLevel": m.reasoningLevelValue()}
	return m, tea.Batch(sendRuntime(m.runtime, payload), m.spinner.Tick)
}

func (m model) updateRuntime(ev runtimeEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "ready":
		m.ready = true
		m.provider = ev.Provider
		m.activeProvider = ev.ActiveProvider
		if m.activeProvider == "" {
			m.activeProvider = ev.Provider
		}
		m.activeKeyAlias = ev.ActiveKeyAlias
		m.model = ev.Model
		m.maxContext = ev.MaxContext
		m.reasoningLevel = normalizeReasoningLevel(ev.ReasoningLevel)
		if ev.WorkspaceCwd != "" {
			m.runtime.launchCwd = ev.WorkspaceCwd
		}
		m.session = ev.SessionID
		m.title = ""
		m.autoNewSessionOnChat = true
		m.status = "ready"
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}))
	case "run_started":
		m.busy = true
		if ev.RunID != "" {
			m.activeRunID = ev.RunID
		}
		m.status = "thinking"
		if ev.SessionID != "" {
			m.session = ev.SessionID
			m.lastRunStartedSession = ev.SessionID
			m.touchLocalSessionActivity(ev.SessionID, m.latestUserMessageTitle())
		}
	case "assistant":
		m.applyAssistantEvent(ev)
		return m, tea.Batch(waitRuntimeEvent(m.runtime), fakeStreamTick())
	case "run_finished":
		if ev.RunID != "" && m.activeRunID != "" && ev.RunID != m.activeRunID {
			return m, waitRuntimeEvent(m.runtime)
		}
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		finishStatus := ev.Status
		if finishStatus == "" {
			finishStatus = "ready"
		}
		var notifyCmd tea.Cmd
		if m.streaming {
			m.streamingFinished = true
			m.streamingFinishStatus = finishStatus
			m.status = "responding"
		} else {
			m.status = finishStatus
			notifyCmd = m.runCompleteNotificationCmd(finishStatus)
			m, notifyCmd = m.finishIdleAndMaybeStartQueuedUser(notifyCmd)
		}
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}), notifyCmd)
	case "session.list":
		m.applySessionList(ev)
	case "session.tree":
		m.applyForkTree(ev)
	case "key.list":
		m.applyKeyList(ev)
	case "model.list":
		if ev.Model != "" {
			m.model = ev.Model
		}
		m.maxContext = ev.MaxContext
		m.reasoningLevel = normalizeReasoningLevel(ev.ReasoningLevel)
		if len(ev.Models) > 0 {
			m.models = ev.Models
		}
		m.applyActiveKeyMetadata(ev)
		m = m.openModelSelectorModal(ev.Models)
	case "model.updated":
		m.applyModelUpdated(ev)
	case "reasoning.updated":
		m.applyReasoningUpdated(ev)
	case "key.updated":
		m.applyKeyUpdated(ev)
	case "codex.login.started", "codex.login.completed", "codex.login.cancelled", "codex.logout.completed", "codex.status", "codex.error":
		m.applyCodexEvent(ev)
	case "tool.permission.request":
		if m.shouldAutoAllowPermission(ev) {
			m.status = "thinking"
			return m, tea.Batch(sendRuntime(m.runtime, map[string]any{"type": "tool.permission.response", "id": ev.ID, "allow": true}), waitRuntimeEvent(m.runtime))
		}
		m = m.openToolPermissionModal(ev)
	case "tool.call.start":
		m.applyToolCallStart(ev)
		return m, tea.Batch(waitRuntimeEvent(m.runtime), toolCallRevealTick())
	case "tool.call.finish":
		m.applyToolCallFinish(ev)
	case "plan.view":
		m.applyPlanView(ev)
	case "session.created":
		m.clearFakeStream()
		m.activeRunID = ""
		m.autoScroll = true
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.autoNewSessionOnChat = false
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Started new session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
	case "session.activated", "session.forked":
		m.clearFakeStream()
		m.activeRunID = ""
		m.autoScroll = true
		m.busy = true
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.autoNewSessionOnChat = false
		verb := "Loading session"
		if ev.Type == "session.forked" {
			verb = "Loading fork"
		}
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("%s: %s (%s)", verb, m.title, short(m.session, 18))}}
		m.status = "loading transcript"
		m.refreshViewport()
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.messages", "sessionId": m.session}))
	case "session.messages":
		m.applySessionMessages(ev)
	case "session.deleted":
		m.applySessionDeleted(ev)
	case "session.renamed":
		m.busy = false
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		if ev.SessionTitle != "" {
			m.title = ev.SessionTitle
		}
		m.appendSystem("Renamed current session to: " + m.title)
	case "error":
		m.clearFakeStream()
		m.busy = false
		m.lastRunStartedSession = ""
		m.activeRunID = ""
		m.err = ev.Error
		m.status = "error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: ev.Error})
		m.refreshViewport()
	}
	return m, waitRuntimeEvent(m.runtime)
}

func (m *model) applyCodexEvent(ev runtimeEvent) {
	m.clearFakeStream()
	m.busy = false
	m.err = ""
	m.status = fallback(ev.Status, "ready")
	if ev.Provider != "" {
		m.provider = ev.Provider
	}
	if ev.ActiveProvider != "" {
		m.activeProvider = ev.ActiveProvider
	}
	if ev.ActiveKeyAlias != "" {
		m.activeKeyAlias = ev.ActiveKeyAlias
	}
	if ev.Model != "" {
		m.model = ev.Model
	}
	switch ev.Type {
	case "codex.login.started":
		copyStatus := ""
		if ev.AuthURL != "" {
			if err := clipboard.WriteAll(ev.AuthURL); err == nil {
				copyStatus = "\n\nThe URL has also been copied to your clipboard. Paste it into your browser if your terminal does not support Ctrl+Click."
			} else {
				copyStatus = "\n\nCould not copy the URL to clipboard automatically; select/copy the full URL below and paste it into your browser."
			}
		}
		m.appendSystem(fmt.Sprintf("Open this URL to sign in to ChatGPT Codex:\n%s%s", ev.AuthURL, copyStatus))
	case "codex.error":
		m.err = ev.Error
		m.status = "Codex error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: fallback(ev.Error, "Codex operation failed")})
		m.refreshViewport()
	default:
		detail := fallback(ev.Status, ev.Type)
		if ev.AccountEmail != "" {
			detail += "\nAccount: " + ev.AccountEmail
		}
		if ev.Storage != "" {
			detail += "\nStorage: " + ev.Storage
		}
		m.appendSystem(detail)
	}
}

func (m *model) applyAssistantEvent(ev runtimeEvent) {
	m.clearFakeStream()
	if ev.RunID != "" {
		m.activeRunID = ev.RunID
	}
	content := strings.TrimSpace(ev.Content)
	if content == "" {
		content = "(empty response)"
	}
	if reasoningContent := strings.TrimSpace(ev.ReasoningContent); reasoningContent != "" {
		m.messages = append(m.messages, chatMessage{Role: "reasoning", Content: reasoningContent})
	}
	m.messages = append(m.messages, chatMessage{Role: "assistant", Content: "", ReasoningContent: ev.ReasoningContent})
	m.streaming = true
	m.streamingMessageIndex = len(m.messages) - 1
	m.streamingFullContent = content
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.Status != "" {
		m.status = ev.Status
	} else {
		m.status = "responding"
	}
	m.refreshViewport()
}

func (m model) updateFakeStreamTick() (tea.Model, tea.Cmd) {
	if !m.streaming {
		return m, nil
	}
	if m.streamingMessageIndex < 0 || m.streamingMessageIndex >= len(m.messages) {
		m.clearFakeStream()
		m.busy = false
		m.status = "ready"
		m.lastRunStartedSession = ""
		m.activeRunID = ""
		return m, nil
	}

	fullContent := []rune(m.streamingFullContent)
	m.streamingVisibleRunes = min(len(fullContent), m.streamingVisibleRunes+fakeStreamChunkSize(len(fullContent), m.streamingVisibleRunes))
	m.messages[m.streamingMessageIndex].Content = string(fullContent[:m.streamingVisibleRunes])
	m.refreshViewport()

	if m.streamingVisibleRunes < len(fullContent) {
		return m, fakeStreamTick()
	}

	finishStatus := m.streamingFinishStatus
	finished := m.streamingFinished
	m.clearFakeStream()
	var notifyCmd tea.Cmd
	if finished {
		m.status = fallback(finishStatus, "ready")
		notifyCmd = m.runCompleteNotificationCmd(finishStatus)
		m, notifyCmd = m.finishIdleAndMaybeStartQueuedUser(notifyCmd)
	} else {
		m.status = "responding"
	}
	return m, notifyCmd
}

func (m model) runCompleteNotificationCmd(status string) tea.Cmd {
	if !shouldNotifyRunComplete(status) {
		return nil
	}
	return notifyRunCompleteCmd(m.notifier, fallback(m.title, m.currentSessionTitle()))
}

func (m model) updateNotificationResult(msg notificationResultMsg) model {
	if msg.err == nil {
		return m
	}
	m.err = msg.err.Error()
	m.status = "notification failed"
	return m
}

func (m *model) abortActiveRun() {
	m.clearFakeStream()
	m.busy = false
	m.status = "aborted"
	m.lastRunStartedSession = ""
	m.activeRunID = ""
	m.refreshViewport()
}

func (m *model) clearFakeStream() {
	m.streaming = false
	m.streamingMessageIndex = 0
	m.streamingFullContent = ""
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
}

func newRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

func fakeStreamTick() tea.Cmd {
	return tea.Tick(18*time.Millisecond, func(time.Time) tea.Msg {
		return fakeStreamTickMsg{}
	})
}

func inputCursorPulseTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return inputCursorPulseMsg{}
	})
}

func fakeStreamChunkSize(totalRunes int, visibleRunes int) int {
	remaining := totalRunes - visibleRunes
	if remaining <= 0 {
		return 0
	}
	size := 3
	if totalRunes > 2000 {
		size = 24
	} else if totalRunes > 800 {
		size = 12
	} else if totalRunes > 240 {
		size = 6
	}
	return min(size, remaining)
}

func (m *model) applySessionList(ev runtimeEvent) {
	wasStreaming := m.streaming
	m.sessions = mergeSessionActivity(m.sessions, ev.Sessions)
	if ev.SessionID != "" && !(m.busy && m.lastRunStartedSession != "" && ev.SessionID != m.lastRunStartedSession) {
		m.session = ev.SessionID
	}
	if !m.autoNewSessionOnChat {
		if ev.SessionTitle != "" && ev.SessionID == m.session {
			m.title = ev.SessionTitle
		} else if !m.busy {
			m.title = m.currentSessionTitle()
		}
	}
	if wasStreaming {
		m.streaming = true
		m.status = "responding"
	} else {
		m.status = "ready"
		m.busy = false
		m.lastRunStartedSession = ""
		m.activeRunID = ""
	}
	m.ensureSessionCursor()
}

func (m *model) touchLocalSessionActivity(sessionID string, title string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for i := range m.sessions {
		if m.sessions[i].ID != sessionID {
			continue
		}
		if shouldReplaceSessionTitle(m.sessions[i].Title) && strings.TrimSpace(title) != "" {
			m.sessions[i].Title = title
		}
		if m.sessions[i].CreatedAt == "" {
			m.sessions[i].CreatedAt = now
		}
		m.sessions[i].UpdatedAt = now
		return
	}
	m.sessions = append(m.sessions, sessionInfo{ID: sessionID, Title: fallback(title, "New chat"), CreatedAt: now, UpdatedAt: now})
}

func (m model) latestUserMessageTitle() string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "user" && strings.TrimSpace(m.messages[i].Content) != "" {
			return titleFromInput(m.messages[i].Content)
		}
	}
	return fallback(m.title, m.currentSessionTitle())
}

func mergeSessionActivity(local []sessionInfo, incoming []sessionInfo) []sessionInfo {
	if len(local) == 0 {
		return incoming
	}
	localByID := make(map[string]sessionInfo, len(local))
	for _, session := range local {
		if session.ID != "" {
			localByID[session.ID] = session
		}
	}
	merged := make([]sessionInfo, 0, max(len(local), len(incoming)))
	seen := make(map[string]bool, len(incoming))
	for _, session := range incoming {
		seen[session.ID] = true
		if localSession, ok := localByID[session.ID]; ok && sessionRecentTimestamp(localSession) > sessionRecentTimestamp(session) {
			if localSession.UpdatedAt != "" {
				session.UpdatedAt = localSession.UpdatedAt
			}
			if session.CreatedAt == "" {
				session.CreatedAt = localSession.CreatedAt
			}
			if shouldReplaceSessionTitle(session.Title) && localSession.Title != "" {
				session.Title = localSession.Title
			}
		}
		merged = append(merged, session)
	}
	for _, session := range local {
		if session.ID != "" && !seen[session.ID] {
			merged = append(merged, session)
		}
	}
	return merged
}

func shouldReplaceSessionTitle(title string) bool {
	switch strings.TrimSpace(title) {
	case "", "New chat", "Default chat", "Untitled chat":
		return true
	default:
		return false
	}
}

func (m *model) applySessionDeleted(ev runtimeEvent) {
	m.busy = false
	m.status = "ready"
	deletedID := ev.SessionID
	if deletedID == "" {
		deletedID = m.pendingDeleteSession.ID
	}
	filtered := m.sessions[:0]
	for _, session := range m.sessions {
		if session.ID != deletedID {
			filtered = append(filtered, session)
		}
	}
	m.sessions = filtered
	m.pendingDeleteSession = sessionInfo{}
	if m.session == deletedID {
		m.session = ""
		m.title = ""
		m.messages = []chatMessage{{Role: "assistant", Content: "Session deleted. Choose another session or start a new chat."}}
		m.autoNewSessionOnChat = true
		m.refreshViewport()
	}
	m.ensureSessionCursorInBounds()
}

func (m *model) applySessionMessages(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
	m.clearFakeStream()
	m.activeRunID = ""
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.SessionTitle != "" {
		m.title = ev.SessionTitle
	}
	m.busy = false
	m.status = "ready"
	m.err = ""
	m.autoScroll = true
	if len(ev.Messages) == 0 {
		m.messages = []chatMessage{{Role: "assistant", Content: "No messages in this session yet."}}
	} else {
		m.messages = ev.Messages
	}
	m.layout()
	m.refreshViewport()
}

func (m model) updateRuntimeError(err error) model {
	m.clearFakeStream()
	m.busy = false
	m.ready = false
	m.err = err.Error()
	m.status = "runtime error"
	detail := err.Error()
	if m.runtime != nil && m.runtime.logPath != "" {
		detail += "\n\nRuntime log: " + m.runtime.logPath
	}
	m.messages = append(m.messages, chatMessage{Role: "error", Content: detail})
	m.refreshViewport()
	return m
}

func (m model) updateSendDone(err error) model {
	if err == nil {
		return m
	}
	m.clearFakeStream()
	m.busy = false
	m.err = err.Error()
	m.status = "send failed"
	m.messages = append(m.messages, chatMessage{Role: "error", Content: err.Error()})
	m.refreshViewport()
	return m
}

func (m model) updateTerminalSetupResult(result terminalSetupResult) model {
	m.busy = false
	m.status = "ready"
	if result.Err != nil {
		m.err = result.Err.Error()
		m.status = "terminal setup failed"
	} else {
		m.err = ""
	}
	m.appendSystem(terminalSetupResultMessage(result))
	return m
}

func (m *model) layout() {
	chatW := max(20, m.width-4)
	inputW := max(10, m.width-6)
	promptW := lipgloss.Width(m.inputPrompt())
	m.composer.SetWidth(max(1, inputW-promptW))

	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	header := m.renderHeader()
	bottomHeight := 1 + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	mainHeight := max(1, m.height-bottomHeight)
	bodyHeight := max(1, mainHeight-lipgloss.Height(header))
	m.chatTopY = lipgloss.Height(header)
	previousYOffset := m.viewport.YOffset()
	previousWidth := m.viewport.Width()
	m.viewport.SetWidth(chatW)
	m.viewport.SetHeight(max(1, bodyHeight))
	if previousWidth != chatW {
		m.refreshViewport()
		return
	}
	m.restoreViewportPosition(previousYOffset)
}

func (m *model) appendSystem(content string) {
	m.appendLocalStatus(content)
}

func (m *model) applyPlanView(ev runtimeEvent) {
	name := fallback(ev.Name, "plan")
	m.messages = append(m.messages, chatMessage{Role: "view", ViewType: "plan", Title: "Plan: " + name, Path: ev.Path, Content: ev.Content})
	m.refreshViewport()
}

func (m model) currentSessionTitle() string {
	for _, session := range m.sessions {
		if session.ID == m.session {
			return session.Title
		}
	}
	return ""
}

func normalizeInputNewlines(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	return strings.ReplaceAll(input, "\r", "\n")
}
