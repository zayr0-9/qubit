package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func initialModel(rt *runtimeClient) model {
	composer := newComposer()

	spin := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))

	return model{
		viewport:   viewport.New(),
		composer:   composer,
		spinner:    spin,
		messages:   []chatMessage{{Role: "assistant", Content: "Ready. Try / for commands."}},
		status:     "starting runtime",
		autoScroll: true,
		runtime:    rt,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitRuntimeEvent(m.runtime), m.spinner.Tick)
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
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.busy {
			return m, cmd
		}
		return m, nil
	case tea.PasteMsg:
		m.composer.InsertString(msg.Content)
		m.layout()
		return m, nil
	case composerPasteMsg:
		if msg.err != nil {
			return m.updateRuntimeError(msg.err), nil
		}
		m.composer.InsertString(msg.text)
		m.layout()
		return m, nil
	}

	return m.updateInputAndViewport(msg)
}

func (m model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeModal {
		return m.updateModal(msg)
	}
	if m.mode == modeSessionPicker {
		return m.updateSessionPicker(msg)
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
		if m.composer.HasSelection() {
			m.composer.ClearSelection()
			m.layout()
			return m, nil
		}
		return m, tea.Quit
	case "up", "ctrl+p":
		if m.showSlashPalette() {
			m.moveSlashCursor(-1)
			return m, nil
		}
	case "down", "ctrl+n":
		if m.showSlashPalette() {
			m.moveSlashCursor(1)
			return m, nil
		}
	case "tab":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
	case "enter":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
		return m.submitInput()
	case "pgup":
		m.autoScroll = false
		m.viewport.ScrollUp(max(1, m.viewport.Height()/2))
		return m, nil
	case "pgdown":
		m.viewport.ScrollDown(max(1, m.viewport.Height()/2))
		m.autoScroll = false
		return m, nil
	}

	return m.updateComposerKey(msg)
}

func (m model) updateComposerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	handled, cmd := m.composer.UpdateKey(msg)
	if handled {
		m.layout()
		return m, cmd
	}
	return m, nil
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
	m.layout()
	return m, cmd
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
	if input == "" || m.busy || !m.ready {
		return m, nil
	}

	m.composer.Reset()
	m.layout()
	m.err = ""
	if strings.HasPrefix(input, "/") {
		return m.handleSlashCommand(input)
	}

	m.messages = append(m.messages, chatMessage{Role: "user", Content: input})
	m.busy = true
	m.status = "thinking"
	m.autoScroll = true
	m.refreshViewport()
	return m, tea.Batch(sendRuntime(m.runtime, map[string]any{"type": "chat", "sessionId": m.session, "input": input}), m.spinner.Tick)
}

func (m model) updateRuntime(ev runtimeEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "ready":
		m.ready = true
		m.provider = ev.Provider
		m.model = ev.Model
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.status = "ready"
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}))
	case "run_started":
		m.busy = true
		m.status = "thinking"
	case "assistant":
		m.applyAssistantEvent(ev)
		return m, tea.Batch(waitRuntimeEvent(m.runtime), fakeStreamTick())
	case "run_finished":
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		finishStatus := ev.Status
		if finishStatus == "" {
			finishStatus = "ready"
		}
		if m.streaming {
			m.streamingFinished = true
			m.streamingFinishStatus = finishStatus
			m.status = "responding"
		} else {
			m.busy = false
			m.status = finishStatus
		}
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}))
	case "session.list":
		m.applySessionList(ev)
	case "tool.permission.request":
		m = m.openToolPermissionModal(ev)
	case "session.created":
		m.clearFakeStream()
		m.autoScroll = true
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Started new session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
	case "session.activated":
		m.clearFakeStream()
		m.autoScroll = true
		m.busy = true
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Loading session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "loading transcript"
		m.refreshViewport()
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.messages", "sessionId": m.session}))
	case "session.messages":
		m.applySessionMessages(ev)
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
		m.err = ev.Error
		m.status = "error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: ev.Error})
		m.refreshViewport()
	}
	return m, waitRuntimeEvent(m.runtime)
}

func (m *model) applyAssistantEvent(ev runtimeEvent) {
	m.clearFakeStream()
	content := strings.TrimSpace(ev.Content)
	if content == "" {
		content = "(empty response)"
	}
	m.messages = append(m.messages, chatMessage{Role: "assistant", Content: ""})
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
	if finished {
		m.busy = false
		m.status = fallback(finishStatus, "ready")
	} else {
		m.status = "responding"
	}
	return m, nil
}

func (m *model) clearFakeStream() {
	m.streaming = false
	m.streamingMessageIndex = 0
	m.streamingFullContent = ""
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
}

func fakeStreamTick() tea.Cmd {
	return tea.Tick(18*time.Millisecond, func(time.Time) tea.Msg {
		return fakeStreamTickMsg{}
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
	m.sessions = ev.Sessions
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.SessionTitle != "" {
		m.title = ev.SessionTitle
	} else {
		m.title = m.currentSessionTitle()
	}
	if wasStreaming {
		m.streaming = true
		m.status = "responding"
	} else {
		m.status = "ready"
		m.busy = false
	}
	m.ensureSessionCursor()
}

func (m *model) applySessionMessages(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
	m.clearFakeStream()
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
	footer := m.renderFooter()
	header := m.renderHeader()
	bottomHeight := 1 + lipgloss.Height(input) + lipgloss.Height(footer)
	mainHeight := max(1, m.height-bottomHeight)
	bodyHeight := max(1, mainHeight-lipgloss.Height(header))
	paletteHeight := 0
	if m.showSlashPalette() {
		paletteHeight = lipgloss.Height(m.renderSlashPalette())
	}

	m.viewport.SetWidth(chatW)
	m.viewport.SetHeight(max(1, bodyHeight-paletteHeight))
	if m.autoScroll {
		m.refreshViewport()
	}
}

func (m *model) appendSystem(content string) {
	m.messages = append(m.messages, chatMessage{Role: "assistant", Content: content})
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
