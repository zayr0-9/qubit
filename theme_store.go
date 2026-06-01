package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func themeConfigPath(appRoot string) string {
	return filepath.Join(appRoot, ".qubit", "theme.json")
}

func loadThemeConfig(appRoot string) (themeConfig, error) {
	if appRoot == "" {
		return themeConfig{}, nil
	}
	data, err := os.ReadFile(themeConfigPath(appRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return themeConfig{}, nil
		}
		return themeConfig{}, fmt.Errorf("read theme config: %w", err)
	}
	var theme themeConfig
	if err := json.Unmarshal(data, &theme); err != nil {
		return themeConfig{}, fmt.Errorf("parse theme config: %w", err)
	}
	return resolveThemeConfig(theme), nil
}

func saveThemeConfig(appRoot string, theme themeConfig) error {
	if appRoot == "" {
		return nil
	}
	path := themeConfigPath(appRoot)
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
	appRoot := ""
	if m.runtime != nil {
		appRoot = m.runtime.appRoot
	}
	if err := saveThemeConfig(appRoot, m.theme); err != nil {
		m.err = err.Error()
		m.status = "theme save failed"
	}
}
