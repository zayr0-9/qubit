package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const windowsTerminalShiftEnterInput = "\x1b[13;2u"

type terminalSetupResult struct {
	SettingsPath string
	BackupPath   string
	Changed      bool
	Err          error
}

func runTerminalSetup() tea.Cmd {
	return func() tea.Msg {
		return terminalSetupResultMsg(patchWindowsTerminalSettings())
	}
}

func patchWindowsTerminalSettings() terminalSetupResult {
	settingsPath, err := findWindowsTerminalSettingsPath()
	if err != nil {
		return terminalSetupResult{Err: err}
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return terminalSetupResult{SettingsPath: settingsPath, Err: fmt.Errorf("read Windows Terminal settings: %w", err)}
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return terminalSetupResult{SettingsPath: settingsPath, Err: fmt.Errorf("parse Windows Terminal settings JSON: %w", err)}
	}

	changed := removeMisplacedShiftEnterBinding(settings)
	updatedActions, actionsChanged := upsertShiftEnterBinding(settings["actions"])
	if actionsChanged {
		settings["actions"] = updatedActions
		changed = true
	}

	if !changed {
		return terminalSetupResult{SettingsPath: settingsPath, Changed: false}
	}

	backupPath := settingsPath + ".qubit-backup-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return terminalSetupResult{SettingsPath: settingsPath, Err: fmt.Errorf("backup Windows Terminal settings: %w", err)}
	}

	patched, err := json.MarshalIndent(settings, "", "    ")
	if err != nil {
		return terminalSetupResult{SettingsPath: settingsPath, BackupPath: backupPath, Err: fmt.Errorf("write Windows Terminal settings JSON: %w", err)}
	}
	patched = append(patched, '\n')

	if err := os.WriteFile(settingsPath, patched, 0o600); err != nil {
		return terminalSetupResult{SettingsPath: settingsPath, BackupPath: backupPath, Err: fmt.Errorf("write Windows Terminal settings: %w", err)}
	}

	return terminalSetupResult{SettingsPath: settingsPath, BackupPath: backupPath, Changed: true}
}

func findWindowsTerminalSettingsPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not set; cannot locate Windows Terminal settings.json")
	}

	candidates := []string{
		filepath.Join(localAppData, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(localAppData, "Packages", "Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(localAppData, "Microsoft", "Windows Terminal", "settings.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find Windows Terminal settings.json in the standard LocalAppData locations")
}

func removeMisplacedShiftEnterBinding(settings map[string]any) bool {
	keys, hasKeys := settings["keys"].(string)
	if !hasKeys || !strings.EqualFold(keys, "shift+enter") {
		return false
	}
	delete(settings, "keys")
	delete(settings, "command")
	return true
}

func upsertShiftEnterBinding(value any) ([]any, bool) {
	binding := map[string]any{
		"command": map[string]any{
			"action": "sendInput",
			"input":  windowsTerminalShiftEnterInput,
		},
		"keys": "shift+enter",
	}

	actions, ok := value.([]any)
	if !ok {
		return []any{binding}, true
	}

	for i, action := range actions {
		actionMap, ok := action.(map[string]any)
		if !ok || !bindingKeysMatch(actionMap, "shift+enter") {
			continue
		}
		if isShiftEnterSendInputBinding(actionMap) {
			return actions, false
		}
		actions[i] = binding
		return actions, true
	}

	return append([]any{binding}, actions...), true
}

func bindingKeysMatch(binding map[string]any, want string) bool {
	keys, ok := binding["keys"].(string)
	return ok && strings.EqualFold(keys, want)
}

func isShiftEnterSendInputBinding(binding map[string]any) bool {
	if !bindingKeysMatch(binding, "shift+enter") {
		return false
	}
	command, ok := binding["command"].(map[string]any)
	if !ok {
		return false
	}
	action, _ := command["action"].(string)
	input, _ := command["input"].(string)
	return action == "sendInput" && input == windowsTerminalShiftEnterInput
}
