package tui

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
)

const terminalBell = "\a"

var terminalBellWriter io.Writer = os.Stderr

type terminalBellResultMsg struct{ err error }

func terminalBellCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := fmt.Fprint(terminalBellWriter, terminalBell)
		return terminalBellResultMsg{err: err}
	}
}

func (m model) updateTerminalBellResult(msg terminalBellResultMsg) model {
	if msg.err == nil {
		return m
	}
	m.err = msg.err.Error()
	m.status = "terminal bell failed"
	return m
}
