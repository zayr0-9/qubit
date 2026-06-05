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

func loadThemeConfig(qubitDir string) (themeConfig, error) {
	path, err := themeConfigPath()
	if err != nil {
		return themeConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return themeConfig{}, fmt.Errorf("read theme config: %w", err)
		}
		if qubitDir == "" {
			return themeConfig{}, nil
		}
		data, err = os.ReadFile(legacyThemeConfigPath(qubitDir))
		if err != nil {
			if os.IsNotExist(err) {
				return themeConfig{}, nil
			}
			return themeConfig{}, fmt.Errorf("read legacy theme config: %w", err)
		}
	}
	var theme themeConfig
	if err := json.Unmarshal(data, &theme); err != nil {
		return themeConfig{}, fmt.Errorf("parse theme config: %w", err)
	}
	resolved := resolveThemeConfig(theme)
	if resolved.Background != "" && resolved.Text != "" && !fileExists(path) {
		_ = saveThemeConfig(qubitDir, resolved)
	}
	return resolved, nil
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
