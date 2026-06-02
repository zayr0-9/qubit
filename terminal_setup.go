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

const (
	defaultTerminalFontFace   = "JetBrains Mono"
	defaultTerminalFontSize   = 11.0
	defaultTerminalLineHeight = 1.05
	defaultTerminalPadding    = "8"
)

type terminalSetupOptions struct {
	FontFace   string
	FontSize   float64
	LineHeight float64
	Padding    string
}

type terminalSetupResult struct {
	SettingsPath string
	BackupPath   string
	Changed      bool
	Err          error
}

func runTerminalSetup() tea.Cmd {
	return func() tea.Msg {
		return terminalSetupResultMsg(patchWindowsTerminalSettings(defaultTerminalSetupOptions()))
	}
}

func defaultTerminalSetupOptions() terminalSetupOptions {
	return terminalSetupOptions{
		FontFace:   defaultTerminalFontFace,
		FontSize:   defaultTerminalFontSize,
		LineHeight: defaultTerminalLineHeight,
		Padding:    defaultTerminalPadding,
	}
}

func patchWindowsTerminalSettings(options terminalSetupOptions) terminalSetupResult {
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
	if upsertWindowsTerminalAppearance(settings, options) {
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

func upsertWindowsTerminalAppearance(settings map[string]any, options terminalSetupOptions) bool {
	options = normalizeTerminalSetupOptions(options)
	profiles, ok := settings["profiles"].(map[string]any)
	if !ok {
		profiles = map[string]any{}
		settings["profiles"] = profiles
	}

	defaults, ok := profiles["defaults"].(map[string]any)
	if !ok {
		defaults = map[string]any{}
		profiles["defaults"] = defaults
	}

	changed := false
	font, ok := defaults["font"].(map[string]any)
	if !ok {
		font = map[string]any{}
		defaults["font"] = font
		changed = true
	}
	if setStringValue(font, "face", options.FontFace) {
		changed = true
	}
	if setNumberValue(font, "size", options.FontSize) {
		changed = true
	}
	if setNumberValue(font, "lineHeight", options.LineHeight) {
		changed = true
	}
	if setStringValue(defaults, "padding", options.Padding) {
		changed = true
	}
	return changed
}

func normalizeTerminalSetupOptions(options terminalSetupOptions) terminalSetupOptions {
	if strings.TrimSpace(options.FontFace) == "" {
		options.FontFace = defaultTerminalFontFace
	} else {
		options.FontFace = strings.TrimSpace(options.FontFace)
	}
	if options.FontSize <= 0 {
		options.FontSize = defaultTerminalFontSize
	}
	if options.LineHeight <= 0 {
		options.LineHeight = defaultTerminalLineHeight
	}
	if strings.TrimSpace(options.Padding) == "" {
		options.Padding = defaultTerminalPadding
	} else {
		options.Padding = strings.TrimSpace(options.Padding)
	}
	return options
}

func setStringValue(target map[string]any, key string, value string) bool {
	if current, ok := target[key].(string); ok && current == value {
		return false
	}
	target[key] = value
	return true
}

func setNumberValue(target map[string]any, key string, value float64) bool {
	if current, ok := terminalNumberValue(target[key]); ok && current == value {
		return false
	}
	target[key] = value
	return true
}

func terminalNumberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (m model) openTerminalSetupConfirm() model {
	m.previousMode = m.mode
	m.mode = modeModal
	m.modal = &modalState{
		ID:          "terminal_setup",
		Kind:        modalKindConfirm,
		Title:       "Update Windows Terminal settings?",
		Description: "Qubit will edit Windows Terminal settings.json after creating a timestamped backup. This installs Shift+Enter newline support and appearance defaults.",
		Fields: []modalField{
			{Label: "Font", Value: "JetBrains Mono, size 11"},
			{Label: "Line height", Value: "1.05"},
			{Label: "Padding", Value: "8"},
			{Label: "Keyboard", Value: "Shift+Enter sends newline input"},
		},
		Actions: []modalAction{
			{ID: "cancel", Label: "Cancel", Default: true},
			{ID: "run", Label: "Apply", Style: "primary"},
		},
		Payload: map[string]any{"action": "terminal.setup"},
	}
	m.status = "confirm terminal setup"
	return m
}
