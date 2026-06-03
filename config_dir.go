package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func qubitConfigDir() (string, error) {
	if override := os.Getenv("QUBIT_CONFIG_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	name := "qubit"
	if os.PathSeparator == '\\' {
		name = "Qubit"
	}
	return filepath.Join(base, name), nil
}
