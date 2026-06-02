package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
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
		Background: "#101214",
		Surface:    "#16191c",
		SurfaceHi:  "#1e2327",
		Text:       "#e8e3d8",
		Muted:      "#8a8378",
		Accent:     "#e8a15d",
		Cyan:       "#89cdd6",
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

	appStyle             lipgloss.Style
	headerStyle          lipgloss.Style
	chatStyle            lipgloss.Style
	inputStyle           lipgloss.Style
	footerStyle          lipgloss.Style
	userIcon             lipgloss.Style
	aiIcon               lipgloss.Style
	errorIcon            lipgloss.Style
	mutedSt              lipgloss.Style
	errSt                lipgloss.Style
	okSt                 lipgloss.Style
	selectSt             lipgloss.Style
	inputSelectSt        lipgloss.Style
	composerCursorSt     lipgloss.Style
	composerCursorStyles []lipgloss.Style
	spinnerStyle         lipgloss.Style
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

	appStyle = lipgloss.NewStyle().Foreground(text)
	headerStyle = lipgloss.NewStyle().Foreground(text).Padding(0, 2)
	chatStyle = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	inputStyle = lipgloss.NewStyle().Foreground(text).PaddingRight(2)
	footerStyle = lipgloss.NewStyle().Foreground(muted).PaddingLeft(2)
	userIcon = lipgloss.NewStyle().Foreground(accent).Bold(true)
	aiIcon = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	errorIcon = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedSt = lipgloss.NewStyle().Foreground(muted)
	errSt = lipgloss.NewStyle().Foreground(red)
	okSt = lipgloss.NewStyle().Foreground(green)
	selectSt = lipgloss.NewStyle().Foreground(accent).Bold(true)
	inputSelectSt = lipgloss.NewStyle().Foreground(bg).Background(accent)
	composerCursorSt = lipgloss.NewStyle().Foreground(bg).Background(text)
	composerCursorStyles = smoothCursorStyles(theme.Background, fallback(theme.Muted, "#7c838a"), theme.Text)
	spinnerStyle = lipgloss.NewStyle().Foreground(accent)
}

func smoothCursorStyles(backgroundHex, lowHex, highHex string) []lipgloss.Style {
	const frames = 24
	styles := make([]lipgloss.Style, 0, frames)
	for frame := 0; frame < frames; frame++ {
		phase := float64(frame) / float64(frames)
		// Ease the pulse with a sine wave so brightness breathes in and out
		// gradually instead of stepping linearly between colors.
		amount := 0.34 + 0.52*(0.5-0.5*math.Cos(phase*2*math.Pi))
		cursorColor := lipgloss.Color(blendHexColor(lowHex, highHex, amount))
		styles = append(styles, lipgloss.NewStyle().Foreground(lipgloss.Color(backgroundHex)).Background(cursorColor))
	}
	return styles
}

func blendHexColor(fromHex, toHex string, amount float64) string {
	from, err := parseHexRGB(fromHex)
	if err != nil {
		from = [3]int{124, 131, 138}
	}
	to, err := parseHexRGB(toHex)
	if err != nil {
		to = [3]int{230, 232, 235}
	}
	amount = maxFloat(0, minFloat(1, amount))
	return fmt.Sprintf("#%02x%02x%02x",
		int(float64(from[0])+float64(to[0]-from[0])*amount+0.5),
		int(float64(from[1])+float64(to[1]-from[1])*amount+0.5),
		int(float64(from[2])+float64(to[2]-from[2])*amount+0.5),
	)
}

func parseHexRGB(value string) ([3]int, error) {
	normalized, err := normalizeHexColor(value)
	if err != nil {
		return [3]int{}, err
	}
	var rgb [3]int
	for i := 0; i < 3; i++ {
		part := normalized[1+i*2 : 3+i*2]
		v, err := strconv.ParseInt(part, 16, 0)
		if err != nil {
			return [3]int{}, err
		}
		rgb[i] = int(v)
	}
	return rgb, nil
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
