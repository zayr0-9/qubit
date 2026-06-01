package main

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

type themeConfig struct {
	ID         string
	Name       string
	Background string
	Surface    string
	SurfaceHi  string
	Text       string
	Muted      string
	Accent     string
	Cyan       string
	Red        string
	Green      string
}

var builtinThemes = []themeConfig{
	{
		ID:         "dark",
		Name:       "Dark",
		Background: "#101112",
		Surface:    "#17191b",
		SurfaceHi:  "#202326",
		Text:       "#e6e8eb",
		Muted:      "#7c838a",
		Accent:     "#f2a65a",
		Cyan:       "#8bd3dd",
		Red:        "#ff6b6b",
		Green:      "#9be28f",
	},
	{
		ID:         "light",
		Name:       "Light",
		Background: "#f7f3ea",
		Surface:    "#ebe3d4",
		SurfaceHi:  "#ddd0bd",
		Text:       "#24201a",
		Muted:      "#756b5f",
		Accent:     "#9a5b00",
		Cyan:       "#006d77",
		Red:        "#b42318",
		Green:      "#287a3e",
	},
	{
		ID:         "neon",
		Name:       "Neon",
		Background: "#000000",
		Surface:    "#000000",
		SurfaceHi:  "#000000",
		Text:       "#f8f7ff",
		Muted:      "#a39bff",
		Accent:     "#ff2bd6",
		Cyan:       "#00f5ff",
		Red:        "#ff3864",
		Green:      "#39ff14",
	},
}

var (
	bg        color.Color
	surface   color.Color
	surfaceHi color.Color
	muted     color.Color
	text      color.Color
	accent    color.Color
	cyan      color.Color
	red       color.Color
	green     color.Color

	appStyle         lipgloss.Style
	headerStyle      lipgloss.Style
	chatStyle        lipgloss.Style
	inputStyle       lipgloss.Style
	footerStyle      lipgloss.Style
	userIcon         lipgloss.Style
	aiIcon           lipgloss.Style
	errorIcon        lipgloss.Style
	mutedSt          lipgloss.Style
	errSt            lipgloss.Style
	okSt             lipgloss.Style
	selectSt         lipgloss.Style
	inputSelectSt    lipgloss.Style
	composerCursorSt lipgloss.Style
	spinnerStyle     lipgloss.Style
)

var (
	inputNewlineBinding = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
)

func init() {
	applyTheme(defaultTheme())
}

func defaultTheme() themeConfig {
	return builtinThemes[0]
}

func applyTheme(theme themeConfig) {
	bg = lipgloss.Color(theme.Background)
	surface = lipgloss.Color(fallback(theme.Surface, theme.Background))
	surfaceHi = lipgloss.Color(fallback(theme.SurfaceHi, theme.Surface))
	muted = lipgloss.Color(fallback(theme.Muted, "#7c838a"))
	text = lipgloss.Color(theme.Text)
	accent = lipgloss.Color(fallback(theme.Accent, "#f2a65a"))
	cyan = lipgloss.Color(fallback(theme.Cyan, "#8bd3dd"))
	red = lipgloss.Color(fallback(theme.Red, "#ff6b6b"))
	green = lipgloss.Color(fallback(theme.Green, "#9be28f"))

	appStyle = lipgloss.NewStyle().Background(bg).Foreground(text)
	headerStyle = lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 2)
	chatStyle = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	inputStyle = lipgloss.NewStyle().Background(surface).Foreground(text).PaddingRight(2)
	footerStyle = lipgloss.NewStyle().Background(bg).Foreground(muted).PaddingLeft(2)
	userIcon = lipgloss.NewStyle().Foreground(accent).Bold(true)
	aiIcon = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	errorIcon = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedSt = lipgloss.NewStyle().Foreground(muted)
	errSt = lipgloss.NewStyle().Foreground(red)
	okSt = lipgloss.NewStyle().Foreground(green)
	selectSt = lipgloss.NewStyle().Foreground(text).Background(surfaceHi).Bold(true)
	inputSelectSt = lipgloss.NewStyle().Foreground(bg).Background(accent)
	composerCursorSt = lipgloss.NewStyle().Foreground(bg).Background(text)
	spinnerStyle = lipgloss.NewStyle().Foreground(accent)
}

func customThemeFrom(background, textColor string, base themeConfig) (themeConfig, error) {
	background, err := normalizeHexColor(background)
	if err != nil {
		return themeConfig{}, fmt.Errorf("background: %w", err)
	}
	textColor, err = normalizeHexColor(textColor)
	if err != nil {
		return themeConfig{}, fmt.Errorf("text: %w", err)
	}
	base.ID = "custom"
	base.Name = "Custom"
	base.Background = background
	base.Surface = background
	base.SurfaceHi = background
	base.Text = textColor
	return base, nil
}

func normalizeHexColor(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "#") {
		trimmed = strings.TrimPrefix(trimmed, "#")
	}
	if len(trimmed) != 6 {
		return "", fmt.Errorf("use #RRGGBB")
	}
	for _, r := range trimmed {
		if !unicode.IsDigit(r) && (unicode.ToLower(r) < 'a' || unicode.ToLower(r) > 'f') {
			return "", fmt.Errorf("use hex digits 0-9 or A-F")
		}
	}
	return "#" + strings.ToLower(trimmed), nil
}
