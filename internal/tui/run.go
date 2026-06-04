package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Run starts the Qubit terminal UI and its runtime sidecar.
func Run() error {
	rt, err := startRuntime()
	if err != nil {
		return fmt.Errorf("failed to start runtime: %w", err)
	}
	defer rt.shutdown()

	program := tea.NewProgram(initialModel(rt))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("qubit crashed: %w", err)
	}
	return nil
}
