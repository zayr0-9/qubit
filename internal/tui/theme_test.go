package tui

import (
	"os"
	"path/filepath"
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	defer resetThemeForTest()
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
	if cmd == nil {
		t.Fatal("enter preset returned nil command, want clear screen command")
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	defer resetThemeForTest()
	m := initialModel(nil)
	m.renderCache[renderCacheKey{Role: "assistant", Content: "cached", Width: 80}] = "old"
	updated, _ := m.openThemeEntry()
	m = updated.(model)
	m.themeEntry.Step = themeEntryText
	m.themeEntry.Background.SetValue("0a0b0c")
	m.themeEntry.Text.SetValue("#ABCDEF")

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("custom apply returned nil command, want clear screen command")
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	defer resetThemeForTest()
	m := initialModel(nil).applyThemeConfig(builtinThemes[2])
	updated, _ := m.openThemeEntry()
	m = updated.(model)

	updated, cmd := m.updateThemeEntry(tea.KeyPressMsg{Text: "d", Code: 'd'})
	if cmd == nil {
		t.Fatal("default shortcut returned nil command, want clear screen command")
	}
	got := updated.(model)
	if got.theme.ID != defaultTheme().ID || got.theme.Background != defaultTheme().Background || got.theme.Text != defaultTheme().Text {
		t.Fatalf("theme = %#v, want default %#v", got.theme, defaultTheme())
	}
}

func TestRenderThemeEntryShowsPresetsAndHexHint(t *testing.T) {
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
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
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	qubitDir := t.TempDir()
	rt, _ := newTestRuntime(t)
	rt.qubitDir = qubitDir

	defer resetThemeForTest()
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

func TestLightThemeMarkdownUsesReadableThemeText(t *testing.T) {
	applyTheme(builtinThemes[1])
	defer applyTheme(defaultTheme())

	style := noBackgroundMarkdownStyle()
	wantText := builtinThemes[1].Text
	if style.Document.Color == nil || *style.Document.Color != wantText {
		t.Fatalf("document markdown color = %v, want %q", style.Document.Color, wantText)
	}
	if style.Text.Color == nil || *style.Text.Color != wantText {
		t.Fatalf("inline markdown text color = %v, want %q", style.Text.Color, wantText)
	}
	if style.Paragraph.Color == nil || *style.Paragraph.Color != wantText {
		t.Fatalf("paragraph markdown color = %v, want %q", style.Paragraph.Color, wantText)
	}
	if style.CodeBlock.Color == nil || *style.CodeBlock.Color != wantText {
		t.Fatalf("code block markdown color = %v, want %q", style.CodeBlock.Color, wantText)
	}
}

func TestThemeConfigPersistsWithoutRuntime(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("QUBIT_CONFIG_DIR", configDir)

	defer resetThemeForTest()
	m := initialModel(nil).applyThemeConfig(builtinThemes[1])
	if m.err != "" {
		t.Fatalf("applyThemeConfig err = %q", m.err)
	}

	loaded := initialModel(nil)
	if loaded.theme.ID != "light" || loaded.theme.Background != "#f7f3ea" || loaded.theme.Text != "#24201a" {
		t.Fatalf("loaded theme = %#v, want persisted light preset", loaded.theme)
	}
}

func TestThemeInvalidGlobalConfigSurfacesLoadError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("QUBIT_CONFIG_DIR", configDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "theme.json"), []byte("{bad json\n"), 0600); err != nil {
		t.Fatalf("write invalid theme config: %v", err)
	}

	m := initialModel(nil)
	if m.theme.ID != defaultTheme().ID {
		t.Fatalf("theme = %#v, want default after invalid config", m.theme)
	}
	if !strings.Contains(m.status, "theme load failed") {
		t.Fatalf("status = %q, want theme load failure", m.status)
	}
	if !strings.Contains(m.err, "parse theme config") || !strings.Contains(m.err, filepath.Join(configDir, "theme.json")) {
		t.Fatalf("err = %q, want parse error with global path", m.err)
	}
}

func TestThemeLegacyConfigMigratesToGlobalWhenMissing(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("QUBIT_CONFIG_DIR", configDir)
	qubitDir := t.TempDir()
	legacyData := []byte(`{"ID":"dracula"}` + "\n")
	if err := os.WriteFile(filepath.Join(qubitDir, "theme.json"), legacyData, 0600); err != nil {
		t.Fatalf("write legacy theme config: %v", err)
	}

	loaded, err := loadThemeConfigWithResult(qubitDir)
	if err != nil {
		t.Fatalf("loadThemeConfigWithResult error = %v", err)
	}
	if !loaded.FromLegacy || loaded.Theme.ID != "dracula" {
		t.Fatalf("loaded = %#v, want migrated dracula from legacy", loaded)
	}
	globalPath := filepath.Join(configDir, "theme.json")
	globalData, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read migrated global theme config: %v", err)
	}
	if !strings.Contains(string(globalData), `"ID": "dracula"`) {
		t.Fatalf("global theme config = %s, want dracula preset", globalData)
	}
}

func TestThemeGlobalConfigWinsOverDifferentLegacyConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("QUBIT_CONFIG_DIR", configDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "theme.json"), []byte(`{"ID":"light"}`+"\n"), 0600); err != nil {
		t.Fatalf("write global theme config: %v", err)
	}
	qubitDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(qubitDir, "theme.json"), []byte(`{"ID":"neon"}`+"\n"), 0600); err != nil {
		t.Fatalf("write legacy theme config: %v", err)
	}

	loaded, err := loadThemeConfigWithResult(qubitDir)
	if err != nil {
		t.Fatalf("loadThemeConfigWithResult error = %v", err)
	}
	if loaded.FromLegacy {
		t.Fatalf("loaded.FromLegacy = true, want global config to win")
	}
	if loaded.Theme.ID != "light" {
		t.Fatalf("loaded theme = %#v, want global light", loaded.Theme)
	}
}

func TestThemePresetApplyReturnsClearScreenCommand(t *testing.T) {
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	defer resetThemeForTest()
	m := initialModel(nil)
	updated, cmd := m.applyThemePreset(1)
	if cmd == nil {
		t.Fatal("applyThemePreset cmd = nil, want clear screen command")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("applyThemePreset command returned nil, want clear screen message")
	}
	if got := updated.(model).theme.ID; got != "light" {
		t.Fatalf("theme ID = %q, want light", got)
	}
}

func TestThemeChromaColorsFollowCurrentTheme(t *testing.T) {
	applyTheme(builtinThemes[1])
	defer applyTheme(defaultTheme())
	style := noBackgroundMarkdownStyle()
	if style.CodeBlock.Chroma == nil {
		t.Fatal("style.CodeBlock.Chroma = nil")
	}
	if got := style.CodeBlock.Chroma.Keyword.Color; got == nil || *got != builtinThemes[1].Accent {
		t.Fatalf("keyword color = %v, want light accent %q", got, builtinThemes[1].Accent)
	}
	if got := style.CodeBlock.Chroma.LiteralString.Color; got == nil || *got != builtinThemes[1].Green {
		t.Fatalf("string color = %v, want light green %q", got, builtinThemes[1].Green)
	}
	if got := style.CodeBlock.Chroma.Comment.Color; got == nil || *got != builtinThemes[1].Muted {
		t.Fatalf("comment color = %v, want light muted %q", got, builtinThemes[1].Muted)
	}
	if style.CodeBlock.Chroma.Keyword.BackgroundColor != nil || style.CodeBlock.Chroma.Background.BackgroundColor != nil {
		t.Fatalf("chroma backgrounds were not cleared: keyword=%v background=%v", style.CodeBlock.Chroma.Keyword.BackgroundColor, style.CodeBlock.Chroma.Background.BackgroundColor)
	}
}

func resetThemeForTest() {
	if path, err := themeConfigPath(); err == nil {
		_ = os.Remove(path)
	}
	applyTheme(defaultTheme())
}
