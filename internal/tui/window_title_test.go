package tui

import "testing"

func TestTerminalWindowTitleUsesLaunchCwd(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: "/home/me/project"})

	if got := m.terminalWindowTitle(); got != "qubit-/home/me/project" {
		t.Fatalf("terminalWindowTitle() = %q, want %q", got, "qubit-/home/me/project")
	}
}

func TestTerminalWindowTitleSupportsWindowsPath(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: `D:\qubit`})

	if got := m.terminalWindowTitle(); got != `qubit-D:\qubit` {
		t.Fatalf("terminalWindowTitle() = %q, want %q", got, `qubit-D:\qubit`)
	}
}

func TestViewSetsWindowTitle(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: "/home/me/project"})
	m.width = 80
	m.height = 24

	if got := m.View().WindowTitle; got != "qubit-/home/me/project" {
		t.Fatalf("View().WindowTitle = %q, want %q", got, "qubit-/home/me/project")
	}
}

func TestLoadingViewSetsWindowTitle(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: "/home/me/project"})

	if got := m.View().WindowTitle; got != "qubit-/home/me/project" {
		t.Fatalf("loading View().WindowTitle = %q, want %q", got, "qubit-/home/me/project")
	}
}

func TestFormatTerminalWindowTitleFallsBackToQubit(t *testing.T) {
	if got := formatTerminalWindowTitle(""); got != "qubit" {
		t.Fatalf("formatTerminalWindowTitle(\"\") = %q, want %q", got, "qubit")
	}
}
