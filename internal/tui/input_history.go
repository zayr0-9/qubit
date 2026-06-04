package tui

import (
	"strings"

	"github.com/qubit/graviton-cli/internal/tui/storage"
)

const maxInputHistoryEntries = storage.MaxInputHistoryEntries

func inputHistoryPath(qubitDir string) string            { return storage.InputHistoryPath(qubitDir) }
func loadInputHistory(qubitDir string) ([]string, error) { return storage.LoadInputHistory(qubitDir) }
func saveInputHistory(qubitDir string, entries []string) error {
	return storage.SaveInputHistory(qubitDir, entries)
}
func sanitizeInputHistory(entries []string) []string { return storage.SanitizeInputHistory(entries) }
func shouldStoreInputHistory(input string) bool      { return storage.ShouldStoreInputHistory(input) }

func (m *model) recordInputHistory(input string) {
	input = strings.TrimSpace(normalizeInputNewlines(input))
	if !shouldStoreInputHistory(input) {
		return
	}
	m.inputHistory = append(m.inputHistory, input)
	m.inputHistory = sanitizeInputHistory(m.inputHistory)
	m.inputHistoryIndex = len(m.inputHistory)
	m.inputHistoryActive = false
}

func (m *model) saveInputHistory() {
	qubitDir := ""
	if m.runtime != nil {
		qubitDir = m.runtime.qubitDir
	}
	if err := saveInputHistory(qubitDir, m.inputHistory); err != nil {
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
