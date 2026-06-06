package tui

import "testing"

func TestUpdateTerminalBellResultIgnoresSuccess(t *testing.T) {
	m := model{status: "ready", err: ""}

	got := m.updateTerminalBellResult(terminalBellResultMsg{})

	if got.status != "ready" || got.err != "" {
		t.Fatalf("model = status:%q err:%q, want unchanged success", got.status, got.err)
	}
}

func TestUpdateTerminalBellResultSurfacesFailure(t *testing.T) {
	m := model{status: "ready"}

	got := m.updateTerminalBellResult(terminalBellResultMsg{err: errTestTerminalBell})

	if got.status != "terminal bell failed" || got.err != errTestTerminalBell.Error() {
		t.Fatalf("model = status:%q err:%q, want terminal bell failure", got.status, got.err)
	}
}

type testTerminalBellError string

func (e testTerminalBellError) Error() string { return string(e) }

const errTestTerminalBell = testTerminalBellError("bell failed")
