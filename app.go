package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func initialModel(rt *runtimeClient) model {
	input := textinput.New()
	input.Placeholder = "message qubit..."
	input.Focus()
	input.CharLimit = 4000
	input.Prompt = lipgloss.NewStyle().Foreground(accent).Render("› ")

	spin := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))

	return model{
		viewport: viewport.New(),
		input:    input,
		spinner:  spin,
		messages: []chatMessage{{Role: "assistant", Content: "Ready. Try / for commands."}},
		status:   "starting runtime",
		runtime:  rt,
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
	case tea.KeyMsg:
		return m.updateKey(msg)
	case runtimeMsg:
		return m.updateRuntime(runtimeEvent(msg))
	case runtimeErrMsg:
		return m.updateRuntimeError(msg.err), nil
	case sendDoneMsg:
		return m.updateSendDone(msg.err), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.busy {
			return m, cmd
		}
		return m, nil
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeSessionPicker {
		return m.updateSessionPicker(msg)
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
	}

	return m, nil
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" || m.busy || !m.ready {
		return m, nil
	}

	m.input.SetValue("")
	m.err = ""
	if strings.HasPrefix(input, "/") {
		return m.handleSlashCommand(input)
	}

	m.messages = append(m.messages, chatMessage{Role: "user", Content: input})
	m.busy = true
	m.status = "thinking"
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
	case "session.created":
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Started new session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
	case "session.activated":
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Switched to session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
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

func (m *model) layout() {
	chatH := max(1, m.height-5)
	chatW := max(20, m.width-4)
	m.viewport.SetWidth(chatW)
	m.viewport.SetHeight(chatH)
	m.input.SetWidth(max(10, m.width-6))
	m.refreshViewport()
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

func (m model) resolveSessionID(prefix string) string {
	for _, session := range m.sessions {
		if session.ID == prefix || strings.HasPrefix(session.ID, prefix) {
			return session.ID
		}
	}
	return prefix
}
