package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func themeConfigPath() (string, error) {
	configDir, err := qubitConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "theme.json"), nil
}

func legacyThemeConfigPath(qubitDir string) string {
	return filepath.Join(qubitDir, "theme.json")
}

type themeLoadResult struct {
	Theme      themeConfig
	Path       string
	LegacyPath string
	FromLegacy bool
}

func loadThemeConfig(qubitDir string) (themeConfig, error) {
	loaded, err := loadThemeConfigWithResult(qubitDir)
	return loaded.Theme, err
}

func loadThemeConfigWithResult(qubitDir string) (themeLoadResult, error) {
	path, err := themeConfigPath()
	if err != nil {
		return themeLoadResult{}, err
	}
	result := themeLoadResult{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return result, fmt.Errorf("read theme config %s: %w", path, err)
		}
		if qubitDir == "" {
			return result, nil
		}
		legacyPath := legacyThemeConfigPath(qubitDir)
		result.LegacyPath = legacyPath
		data, err = os.ReadFile(legacyPath)
		if err != nil {
			if os.IsNotExist(err) {
				return result, nil
			}
			return result, fmt.Errorf("read legacy theme config %s: %w", legacyPath, err)
		}
		result.FromLegacy = true
	}
	var theme themeConfig
	if err := json.Unmarshal(data, &theme); err != nil {
		return result, fmt.Errorf("parse theme config %s: %w", result.SourcePath(), err)
	}
	resolved := resolveThemeConfig(theme)
	if resolved.Background == "" || resolved.Text == "" {
		return result, fmt.Errorf("theme config %s did not resolve to a valid theme", result.SourcePath())
	}
	result.Theme = resolved
	if result.FromLegacy && !fileExists(path) {
		_ = saveThemeConfig(qubitDir, resolved)
	}
	return result, nil
}

func (r themeLoadResult) SourcePath() string {
	if r.FromLegacy && r.LegacyPath != "" {
		return r.LegacyPath
	}
	return r.Path
}

func saveThemeConfig(_ string, theme themeConfig) error {
	path, err := themeConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create theme config directory: %w", err)
	}
	data, err := json.MarshalIndent(resolveThemeConfig(theme), "", "  ")
	if err != nil {
		return fmt.Errorf("encode theme config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write theme config: %w", err)
	}
	return nil
}

func resolveThemeConfig(theme themeConfig) themeConfig {
	if theme.ID != "" {
		for _, preset := range builtinThemes {
			if theme.ID == preset.ID {
				return preset
			}
		}
	}
	if theme.Background == "" || theme.Text == "" {
		return themeConfig{}
	}
	base := defaultTheme()
	if theme.ID == "custom" {
		base = theme
	} else if matched := matchingThemePreset(theme); matched < len(builtinThemes) {
		return builtinThemes[matched]
	}
	next, err := customThemeFrom(theme.Background, theme.Text, base)
	if err != nil {
		return themeConfig{}
	}
	return next
}

func (m *model) saveThemeConfig() {
	if err := saveThemeConfig(runtimeQubitDir(m.runtime), m.theme); err != nil {
		m.err = err.Error()
		m.status = "theme save failed"
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func themeLoadErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
