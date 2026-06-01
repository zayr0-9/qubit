package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type chatMessage struct {
	Role    string
	Content string
}

type sessionInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	MessageCount int    `json:"messageCount"`
}

type uiMode int

const (
	modeChat uiMode = iota
	modeSessionPicker
)

type slashCommand struct {
	Name        string
	Usage       string
	Description string
	NeedsArg    bool
}

type runtimeEvent struct {
	Type             string        `json:"type"`
	ID               string        `json:"id,omitempty"`
	SessionID        string        `json:"sessionId,omitempty"`
	SessionTitle     string        `json:"sessionTitle,omitempty"`
	Session          *sessionInfo  `json:"session,omitempty"`
	Sessions         []sessionInfo `json:"sessions,omitempty"`
	Provider         string        `json:"provider,omitempty"`
	Model            string        `json:"model,omitempty"`
	StoragePath      string        `json:"storagePath,omitempty"`
	IndexPath        string        `json:"indexPath,omitempty"`
	Status           string        `json:"status,omitempty"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoningContent,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type runtimeMsg runtimeEvent
type runtimeErrMsg struct{ err error }
type sendDoneMsg struct{ err error }

type runtimeClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	events  chan runtimeEvent
	errs    chan error
	appRoot string
	logPath string
}

type model struct {
	width  int
	height int

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	messages []chatMessage
	sessions []sessionInfo
	busy     bool
	ready    bool
	provider string
	model    string
	session  string
	title    string
	status   string
	err      string

	mode          uiMode
	sessionCursor int
	slashCursor   int

	runtime *runtimeClient
}

var (
	bg        = lipgloss.Color("#101112")
	surface   = lipgloss.Color("#17191b")
	surfaceHi = lipgloss.Color("#202326")
	muted     = lipgloss.Color("#7c838a")
	text      = lipgloss.Color("#e6e8eb")
	accent    = lipgloss.Color("#f2a65a")
	cyan      = lipgloss.Color("#8bd3dd")
	red       = lipgloss.Color("#ff6b6b")
	green     = lipgloss.Color("#9be28f")

	appStyle    = lipgloss.NewStyle().Background(bg).Foreground(text)
	headerStyle = lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 2)
	chatStyle   = lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 2)
	inputStyle  = lipgloss.NewStyle().Background(surface).Foreground(text).Padding(0, 2)
	footerStyle = lipgloss.NewStyle().Background(bg).PaddingLeft(2)
	userName    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	aiName      = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	mutedSt     = lipgloss.NewStyle().Foreground(muted)
	errSt       = lipgloss.NewStyle().Foreground(red)
	okSt        = lipgloss.NewStyle().Foreground(green)
	selectSt    = lipgloss.NewStyle().Foreground(text).Background(surfaceHi).Bold(true)
)

var slashCommands = []slashCommand{
	{Name: "new", Usage: "/new [title]", Description: "Create a new chat session", NeedsArg: false},
	{Name: "sessions", Usage: "/sessions", Description: "Open the session picker", NeedsArg: false},
	{Name: "use", Usage: "/use <id-prefix>", Description: "Switch to a session by id prefix", NeedsArg: true},
	{Name: "rename", Usage: "/rename <title>", Description: "Rename current session", NeedsArg: true},
	{Name: "help", Usage: "/help", Description: "Show command help", NeedsArg: false},
}

func main() {
	rt, err := startRuntime()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start runtime: %v\n", err)
		os.Exit(1)
	}
	defer rt.shutdown()

	m := initialModel(rt)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "qubit crashed: %v\n", err)
		os.Exit(1)
	}
}

func initialModel(rt *runtimeClient) model {
	ti := textinput.New()
	ti.Placeholder = "message qubit..."
	ti.Focus()
	ti.CharLimit = 4000
	ti.Prompt = lipgloss.NewStyle().Foreground(accent).Render("› ")

	vp := viewport.New()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))

	return model{
		viewport: vp,
		input:    ti,
		spinner:  sp,
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

	case runtimeMsg:
		ev := runtimeEvent(msg)
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

	case runtimeErrMsg:
		m.busy = false
		m.ready = false
		m.err = msg.err.Error()
		m.status = "runtime error"
		detail := msg.err.Error()
		if m.runtime != nil && m.runtime.logPath != "" {
			detail += "\n\nRuntime log: " + m.runtime.logPath
		}
		m.messages = append(m.messages, chatMessage{Role: "error", Content: detail})
		m.refreshViewport()
		return m, nil

	case sendDoneMsg:
		if msg.err != nil {
			m.busy = false
			m.err = msg.err.Error()
			m.status = "send failed"
			m.messages = append(m.messages, chatMessage{Role: "error", Content: msg.err.Error()})
			m.refreshViewport()
		}
		return m, nil

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

func (m model) updateSessionPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case "use", "switch":
		if arg == "" {
			m.appendSystem("Usage: /use <session-id-or-prefix>")
			return m, nil
		}
		sessionID := m.resolveSessionID(arg)
		m.busy = true
		m.status = "switching session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.activate", "sessionId": sessionID})
	case "rename", "title":
		if arg == "" {
			m.appendSystem("Usage: /rename <title>")
			return m, nil
		}
		m.busy = true
		m.status = "renaming session"
		return m, sendRuntime(m.runtime, map[string]any{"type": "session.rename", "sessionId": m.session, "title": arg})
	case "help", "h":
		m.appendSystem("Commands:\n/new [title] - create a new chat\n/sessions - list chats\n/use <id-prefix> - switch chat\n/rename <title> - rename current chat\n/help - show this help")
		return m, nil
	default:
		m.appendSystem("Unknown command. Try /help")
		return m, nil
	}
}

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		v.MouseMode = tea.MouseModeNone
		return v
	}

	provider := m.provider
	if provider == "" {
		provider = "..."
	}
	modelName := m.model
	if modelName == "" {
		modelName = "..."
	}
	sessionTitle := m.title
	if sessionTitle == "" {
		sessionTitle = m.currentSessionTitle()
	}
	if sessionTitle == "" {
		sessionTitle = "untitled"
	}

	activity := okSt.Render(m.status)
	if m.busy {
		activity = m.spinner.View() + " " + lipgloss.NewStyle().Foreground(accent).Render(m.status)
	} else if strings.Contains(m.status, "error") || m.err != "" {
		activity = errSt.Render(m.status)
	}

	title := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("qubit")
	meta := mutedSt.Render(fmt.Sprintf("%s · %s · %s", provider, modelName, short(m.session, 12)))
	headerLeft := fmt.Sprintf("%s  %s", title, activity)
	headerRight := oneLine(sessionTitle, max(12, m.width-lipgloss.Width(headerLeft)-lipgloss.Width(meta)-8))
	headerText := fmt.Sprintf("%s  %s  %s", headerLeft, mutedSt.Render(headerRight), meta)
	header := headerStyle.Width(m.width).Render(headerText)

	chatContent := m.viewport.View()
	if m.mode == modeSessionPicker {
		chatContent = m.renderSessionPicker()
	} else if m.showSlashPalette() {
		chatContent = lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), "", m.renderSlashPalette())
	}
	chat := chatStyle.Width(m.width).Height(max(1, m.height-5)).Render(chatContent)
	input := inputStyle.Width(m.width).Render(m.input.View())

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
	footerView := footerStyle.Width(m.width).Render(footerText)

	content := appStyle.Width(m.width).Height(m.height).Render(lipgloss.JoinVertical(lipgloss.Left, header, chat, input, footerView))
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeNone
	return v
}

func (m model) showSlashPalette() bool {
	value := m.input.Value()
	return strings.HasPrefix(value, "/") && !strings.Contains(value, " ") && m.mode == modeChat && !m.busy && m.ready
}

func (m model) filteredSlashCommands() []slashCommand {
	query := strings.ToLower(strings.TrimPrefix(m.input.Value(), "/"))
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
		m.input.SetValue("")
		return m.handleSlashCommand("/sessions")
	}
	value := "/" + command.Name
	if command.NeedsArg {
		value += " "
	} else {
		value += " "
	}
	m.input.SetValue(value)
	m.input.SetCursor(len(value))
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

func startRuntime() (*runtimeClient, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, err
	}
	appRoot, err := findAppRoot()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(appRoot, ".qubit", "runtime.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	_ = os.WriteFile(logPath, []byte(""), 0644)

	runtimePath := filepath.Join(appRoot, "runtime.mjs")
	cmd := exec.Command(node, runtimePath)
	cmd.Dir = appRoot
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	rt := &runtimeClient{cmd: cmd, stdin: stdin, events: make(chan runtimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, logPath: logPath}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go rt.readStdout(stdout)
	go rt.readStderr(stderr)
	go func() {
		if err := cmd.Wait(); err != nil {
			rt.errs <- fmt.Errorf("runtime exited: %w", err)
		}
		close(rt.events)
	}()

	return rt, nil
}

func (r *runtimeClient) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		r.appendLog("stdout", string(line))
		var ev runtimeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			r.errs <- fmt.Errorf("bad runtime event: %s", string(line))
			continue
		}
		r.events <- ev
	}
	if err := scanner.Err(); err != nil {
		r.errs <- err
	}
}

func (r *runtimeClient) readStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			r.appendLog("stderr", line)
			r.errs <- fmt.Errorf("%s", line)
		}
	}
}

func (r *runtimeClient) appendLog(stream string, line string) {
	if r.logPath == "" {
		return
	}
	entry := fmt.Sprintf("[%s] %s\n", stream, line)
	f, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

func (r *runtimeClient) shutdown() {
	_ = r.send(map[string]any{"type": "shutdown"})
	_ = r.stdin.Close()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}

func (r *runtimeClient) send(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdin, "%s\n", payload)
	return err
}

func waitRuntimeEvent(r *runtimeClient) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-r.events:
			if !ok {
				return runtimeErrMsg{err: fmt.Errorf("runtime stopped")}
			}
			return runtimeMsg(ev)
		case err := <-r.errs:
			return runtimeErrMsg{err: err}
		}
	}
}

func sendRuntime(r *runtimeClient, payload map[string]any) tea.Cmd {
	return func() tea.Msg {
		if _, ok := payload["id"]; !ok {
			payload["id"] = fmt.Sprintf("msg_%d", time.Now().UnixNano())
		}
		err := r.send(payload)
		return sendDoneMsg{err: err}
	}
}

func sessionListText(sessions []sessionInfo, activeID string) string {
	if len(sessions) == 0 {
		return "No sessions yet. Use /new to create one."
	}
	var b strings.Builder
	b.WriteString("Sessions:\n")
	for _, session := range sessions {
		marker := "  "
		if session.ID == activeID {
			marker = "→ "
		}
		b.WriteString(fmt.Sprintf("%s%s  %s  (%d msgs)\n", marker, short(session.ID, 22), session.Title, session.MessageCount))
	}
	b.WriteString("\nUse /use <id-prefix> to switch.")
	return strings.TrimRight(b.String(), "\n")
}

func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range words {
			if len(line) == 0 {
				line = word
			} else if len([]rune(line))+1+len([]rune(word)) <= width {
				line += " " + word
			} else {
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func short(s string, n int) string {
	if s == "" || len([]rune(s)) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func findAppRoot() (string, error) {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if _, err := os.Stat(filepath.Join(abs, "runtime.mjs")); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf("could not find runtime.mjs. Run qubit from D:\\qubit or keep bin\\qubit.exe next to the project root")
}

func oneLine(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if width <= 0 || len([]rune(s)) <= width {
		return s
	}
	runes := []rune(s)
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
