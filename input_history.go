package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxInputHistoryEntries = 200

type inputHistoryStore struct {
	Entries []string `json:"entries"`
}

func inputHistoryPath(appRoot string) string {
	return filepath.Join(appRoot, ".qubit", "input-history.json")
}

func loadInputHistory(appRoot string) ([]string, error) {
	if appRoot == "" {
		return nil, nil
	}
	data, err := os.ReadFile(inputHistoryPath(appRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read input history: %w", err)
	}
	var store inputHistoryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse input history: %w", err)
	}
	return sanitizeInputHistory(store.Entries), nil
}

func saveInputHistory(appRoot string, entries []string) error {
	if appRoot == "" {
		return nil
	}
	path := inputHistoryPath(appRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create input history directory: %w", err)
	}
	data, err := json.MarshalIndent(inputHistoryStore{Entries: sanitizeInputHistory(entries)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode input history: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write input history: %w", err)
	}
	return nil
}

func sanitizeInputHistory(entries []string) []string {
	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		entry := strings.TrimSpace(normalizeInputNewlines(entries[i]))
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
		if len(out) >= maxInputHistoryEntries {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (m *model) recordInputHistory(input string) {
	input = strings.TrimSpace(normalizeInputNewlines(input))
	if input == "" {
		return
	}
	m.inputHistory = append(m.inputHistory, input)
	m.inputHistory = sanitizeInputHistory(m.inputHistory)
	m.inputHistoryIndex = len(m.inputHistory)
	m.inputHistoryActive = false
}

func (m *model) saveInputHistory() {
	appRoot := ""
	if m.runtime != nil {
		appRoot = m.runtime.appRoot
	}
	if err := saveInputHistory(appRoot, m.inputHistory); err != nil {
		m.err = err.Error()
		m.status = "input history save failed"
	}
}

func (m model) cycleInputHistory(delta int) (model, bool) {
	if len(m.inputHistory) == 0 {
		return m, false
	}
	if !m.inputHistoryActive {
		if strings.TrimSpace(m.composer.Value()) != "" || delta >= 0 {
			return m, false
		}
		m.inputHistoryActive = true
		m.inputHistoryIndex = len(m.inputHistory)
	}

	m.inputHistoryIndex = clampInt(m.inputHistoryIndex+delta, 0, len(m.inputHistory))
	if m.inputHistoryIndex == len(m.inputHistory) {
		m.composer.Reset()
		m.inputHistoryActive = false
	} else {
		m.composer.SetValue(m.inputHistory[m.inputHistoryIndex])
		m.inputHistoryActive = true
	}
	m.layout()
	return m, true
}
