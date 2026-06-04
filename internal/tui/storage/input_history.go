package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MaxInputHistoryEntries = 200

type InputHistoryStore struct {
	Entries []string `json:"entries"`
}

func InputHistoryPath(qubitDir string) string {
	return filepath.Join(qubitDir, "input-history.json")
}

func LoadInputHistory(qubitDir string) ([]string, error) {
	if qubitDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(InputHistoryPath(qubitDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read input history: %w", err)
	}
	var store InputHistoryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse input history: %w", err)
	}
	return SanitizeInputHistory(store.Entries), nil
}

func SaveInputHistory(qubitDir string, entries []string) error {
	if qubitDir == "" {
		return nil
	}
	path := InputHistoryPath(qubitDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create input history directory: %w", err)
	}
	data, err := json.MarshalIndent(InputHistoryStore{Entries: SanitizeInputHistory(entries)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode input history: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write input history: %w", err)
	}
	return nil
}

func SanitizeInputHistory(entries []string) []string {
	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		entry := strings.TrimSpace(NormalizeInputNewlines(entries[i]))
		if !ShouldStoreInputHistory(entry) || seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
		if len(out) >= MaxInputHistoryEntries {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func ShouldStoreInputHistory(input string) bool {
	input = strings.TrimSpace(NormalizeInputNewlines(input))
	return input != "" && !strings.HasPrefix(input, "/")
}

func NormalizeInputNewlines(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	return strings.ReplaceAll(input, "\r", "\n")
}
