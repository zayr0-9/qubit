package tui

func (m model) terminalWindowTitle() string {
	return formatTerminalWindowTitle(m.cwdStatusText())
}

func formatTerminalWindowTitle(cwd string) string {
	if cwd == "" {
		return "qubit"
	}
	return "qubit-" + cwd
}
