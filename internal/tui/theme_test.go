package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNormalizeHexColor(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "hash six digits", input: "#101112", want: "#101112"},
		{name: "bare six digits", input: "ABCDEF", want: "#abcdef"},
		{name: "invalid example", input: "#easd217", wantErr: true},
		{name: "invalid letters", input: "#xyzxyz", wantErr: true},
		{name: "shorthand rejected", input: "#fff", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeHexColor(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeHexColor(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeHexColor(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeHexColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestThemeSlashCommandOpensThemeEditor(t *testing.T) {
	m := initialModel(nil)
	m.ready = true

	updated, cmd := m.handleSlashCommand("/theme")
	if cmd != nil {
		t.Fatal("/theme returned command, want nil")
	}
	got := updated.(model)
	if got.mode != modeThemeEntry {
		t.Fatalf("mode = %v, want modeThemeEntry", got.mode)
	}
	if got.themeEntry == nil {
		t.Fatal("themeEntry = nil, want editor state")
	}
	if got.themeEntry.Background.Value() != defaultTheme().Background || got.themeEntry.Text.Value() != defaultTheme().Text {
		t.Fatalf("theme editor values = %q/%q, want defaults", got.themeEntry.Background.Value(), got.themeEntry.Text.Value())
	}
}

func TestThemePresetSelectionAppliesNeon(t *testing.T) {
	m := initialModel(nil)
	beforeSpinner := m.spinner.View()
	updated, _ := m.openThemeEntry()
	m = updated.(model)

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil")
	}
	m = updated.(model)
	updated, cmd = m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("second down returned command, want nil")
	}
	m = updated.(model)
	updated, cmd = m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter preset returned command, want nil")
	}
	got := updated.(model)
	if got.mode != modeChat || got.themeEntry != nil {
		t.Fatalf("theme editor not closed: mode=%v themeEntry=%#v", got.mode, got.themeEntry)
	}
	if got.theme.ID != "neon" || got.theme.Accent != "#ff2bd6" || got.theme.Cyan != "#00f5ff" || got.theme.Red != "#ff9aa2" || got.theme.Green != "#b8f7b1" {
		t.Fatalf("theme = %#v, want neon preset", got.theme)
	}
	if !strings.Contains(got.status, "Neon") {
		t.Fatalf("status = %q, want Neon applied", got.status)
	}
	if got.spinner.View() == beforeSpinner {
		t.Fatalf("spinner view did not change after theme apply: %q", got.spinner.View())
	}
}

func TestThemeCustomAppliesValidHexAndClearsCache(t *testing.T) {
	m := initialModel(nil)
	m.renderCache[renderCacheKey{Role: "assistant", Content: "cached", Width: 80}] = "old"
	updated, _ := m.openThemeEntry()
	m = updated.(model)
	m.themeEntry.Step = themeEntryText
	m.themeEntry.Background.SetValue("0a0b0c")
	m.themeEntry.Text.SetValue("#ABCDEF")

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("custom apply returned command, want nil")
	}
	got := updated.(model)
	if got.theme.ID != "custom" || got.theme.Background != "#0a0b0c" || got.theme.Text != "#abcdef" {
		t.Fatalf("theme = %#v, want normalized custom", got.theme)
	}
	if len(got.renderCache) != 0 {
		t.Fatalf("renderCache len = %d, want cleared", len(got.renderCache))
	}
}

func TestThemeCustomRejectsInvalidHex(t *testing.T) {
	m := initialModel(nil)
	original := m.theme
	updated, _ := m.openThemeEntry()
	m = updated.(model)
	m.themeEntry.Step = themeEntryText
	m.themeEntry.Background.SetValue("#easd217")
	m.themeEntry.Text.SetValue("#ffffff")

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invalid custom apply returned command, want nil")
	}
	got := updated.(model)
	if got.mode != modeThemeEntry || got.themeEntry == nil {
		t.Fatalf("invalid theme closed editor: mode=%v themeEntry=%#v", got.mode, got.themeEntry)
	}
	if got.theme != original {
		t.Fatalf("theme changed to %#v, want original %#v", got.theme, original)
	}
	if !strings.Contains(got.themeEntry.Err, "background") {
		t.Fatalf("themeEntry.Err = %q, want background validation error", got.themeEntry.Err)
	}
}

func TestThemeDefaultShortcutRestoresDefault(t *testing.T) {
	m := initialModel(nil).applyThemeConfig(builtinThemes[2])
	updated, _ := m.openThemeEntry()
	m = updated.(model)

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Text: "d", Code: 'd'})
	if cmd != nil {
		t.Fatal("default shortcut returned command, want nil")
	}
	got := updated.(model)
	if got.theme.ID != defaultTheme().ID || got.theme.Background != defaultTheme().Background || got.theme.Text != defaultTheme().Text {
		t.Fatalf("theme = %#v, want default %#v", got.theme, defaultTheme())
	}
}

func TestRenderThemeEntryShowsPresetsAndHexHint(t *testing.T) {
	m := initialModel(nil)
	m.width = 90
	m.height = 30
	updated, _ := m.openThemeEntry()
	m = updated.(model)

	rendered := plainText(m.renderThemeEntry(20))
	for _, want := range []string{"Theme", "Dark", "Light", "Neon", "Background", "Text", "#RRGGBB", "#easd217"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered theme editor missing %q: %q", want, rendered)
		}
	}
}

func TestThemeConfigPersistsAndLoadsSelectedPreset(t *testing.T) {
	qubitDir := t.TempDir()
	rt, _ := newTestRuntime(t)
	rt.qubitDir = qubitDir

	m := initialModel(rt).applyThemeConfig(builtinThemes[2])
	if m.err != "" {
		t.Fatalf("applyThemeConfig err = %q", m.err)
	}

	loaded := initialModel(rt)
	if loaded.theme.ID != "neon" || loaded.theme.Background != "#000000" || loaded.theme.Surface != "#000000" {
		t.Fatalf("loaded theme = %#v, want persisted neon preset", loaded.theme)
	}
}

func TestNeonThemeUsesBlackMessageRowBackground(t *testing.T) {
	neon := builtinThemes[2]
	if neon.ID != "neon" {
		t.Fatalf("builtinThemes[2].ID = %q, want neon", neon.ID)
	}
	if neon.Background != "#000000" || neon.Surface != "#000000" || neon.SurfaceHi != "#000000" {
		t.Fatalf("neon backgrounds = %q/%q/%q, want all black", neon.Background, neon.Surface, neon.SurfaceHi)
	}

	applyTheme(neon)
	defer applyTheme(defaultTheme())
	rendered := renderChat("hello", 20, 1)
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("renderChat emitted background ANSI sequence for chat row: %q", rendered)
	}
}

func TestThemeReasoningColorDefaultsAndCustomFallback(t *testing.T) {
	if defaultTheme().Reasoning != "#c7a0ff" {
		t.Fatalf("default reasoning color = %q, want #c7a0ff", defaultTheme().Reasoning)
	}
	custom, err := customThemeFrom("#010203", "#abcdef", themeConfig{Reasoning: "#123456", ToolSearch: "#654321"})
	if err != nil {
		t.Fatalf("customThemeFrom error = %v", err)
	}
	applyTheme(custom)
	defer applyTheme(defaultTheme())
	if got := colorToHex(reasoning); got != "#123456" {
		t.Fatalf("reasoning color = %q, want custom reasoning color", got)
	}

	custom.Reasoning = ""
	applyTheme(custom)
	if got := colorToHex(reasoning); got != "#654321" {
		t.Fatalf("reasoning color fallback = %q, want tool search fallback", got)
	}
}
