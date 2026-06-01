package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func initialModel(rt *runtimeClient) model {
	input := textarea.New()
	input.Placeholder = "message qubit..."
	input.CharLimit = 4000
	input.Prompt = lipgloss.NewStyle().Foreground(accent).Render("› ")
	input.ShowLineNumbers = false
	input.EndOfBufferCharacter = ' '
	input.DynamicHeight = true
	input.MinHeight = 1
	input.MaxHeight = 8
	input.MaxContentHeight = 80
	input.Focus()

	spin := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))

	return model{
		viewport:   viewport.New(),
		input:      input,
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
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.busy {
			return m, cmd
		}
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
	case "ctrl+c", "esc":
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
	case "pgup", "ctrl+u":
		m.autoScroll = false
		m.viewport.ScrollUp(max(1, m.viewport.Height()/2))
		return m, nil
	case "pgdown", "ctrl+d":
		m.viewport.ScrollDown(max(1, m.viewport.Height()/2))
		m.autoScroll = false
		return m, nil
	case "home":
		m.autoScroll = false
		m.viewport.GotoTop()
		return m, nil
	case "end":
		m.autoScroll = true
		m.viewport.GotoBottom()
		return m, nil
	}

	return m.updateInputAndViewport(msg)
}

func (m model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.layout()
	return m, cmd
}

func (m model) insertInputNewline() (tea.Model, tea.Cmd) {
	m.input.InsertString("\n")
	m.layout()
	return m, nil
}

func (m model) updateInputAndViewport(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	previousYOffset := m.viewport.YOffset()
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	if m.viewport.YOffset() != previousYOffset {
		m.autoScroll = m.viewport.AtBottom()
	}
	m.layout()
	return m, tea.Batch(cmds...)
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
	input := strings.TrimSpace(normalizeInputNewlines(m.input.Value()))
	if input == "" || m.busy || !m.ready {
		return m, nil
	}

	m.input.SetValue("")
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
	case "run_finished":
		m.busy = false
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		if ev.Status != "" {
			m.status = ev.Status
		} else {
			m.status = "ready"
		}
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}))
	case "session.list":
		m.applySessionList(ev)
	case "tool.permission.request":
		m = m.openToolPermissionModal(ev)
	case "session.created":
		m.autoScroll = true
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Started new session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
	case "session.activated":
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
		m.busy = false
		m.err = ev.Error
		m.status = "error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: ev.Error})
		m.refreshViewport()
	}
	return m, waitRuntimeEvent(m.runtime)
}

func (m *model) applyAssistantEvent(ev runtimeEvent) {
	content := strings.TrimSpace(ev.Content)
	if content == "" {
		content = "(empty response)"
	}
	m.messages = append(m.messages, chatMessage{Role: "assistant", Content: content})
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	m.status = ev.Status
	m.refreshViewport()
}

func (m *model) applySessionList(ev runtimeEvent) {
	m.sessions = ev.Sessions
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.SessionTitle != "" {
		m.title = ev.SessionTitle
	} else {
		m.title = m.currentSessionTitle()
	}
	m.status = "ready"
	m.busy = false
	m.ensureSessionCursor()
}

func (m *model) applySessionMessages(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
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
	m.input.SetWidth(inputW)

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
