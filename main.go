package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	rt, err := startRuntime()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start runtime: %v\n", err)
		os.Exit(1)
	}
	defer rt.shutdown()

	program := tea.NewProgram(initialModel(rt))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "qubit crashed: %v\n", err)
		os.Exit(1)
	}
}
