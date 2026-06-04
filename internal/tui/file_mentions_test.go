package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFileMentionPaletteFiltersAndAccepts(t *testing.T) {
	cwd := t.TempDir()
	writeTestFile(t, filepath.Join(cwd, "src", "app.go"))
	writeTestFile(t, filepath.Join(cwd, "docs", "api.md"))
	writeTestFile(t, filepath.Join(cwd, "node_modules", "ignored.js"))

	m := initialModel(&runtimeClient{launchCwd: cwd})
	m.ready = true
	m.width = 80
	m.height = 20
	m.layout()
	m.composer.SetValue("read @app")

	if !m.showFileMentionPalette() {
		t.Fatal("showFileMentionPalette = false, want true")
	}
	m.ensureFileMentionIndex()
	matches := m.filteredFileMentionEntries()
	if len(matches) == 0 || matches[0].Path != "src/app.go" {
		t.Fatalf("matches = %#v, want src/app.go first", matches)
	}
	for _, match := range matches {
		if strings.Contains(match.Path, "node_modules") {
			t.Fatalf("ignored file included in matches: %#v", match)
		}
	}

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	got := updated.(model)
	if got.composer.Value() != "read @app.go " {
		t.Fatalf("composer value = %q, want clean file name mention inserted", got.composer.Value())
	}
	if expanded := got.expandFileMentionsForSend("read @app.go"); expanded != "read @src/app.go" {
		t.Fatalf("expanded input = %q, want full relative path", expanded)
	}
}

func TestFileMentionNavigationAndRenderWindow(t *testing.T) {
	cwd := t.TempDir()
	for i := 0; i < 12; i++ {
		writeTestFile(t, filepath.Join(cwd, "file"+string(rune('a'+i))+".go"))
	}

	m := initialModel(&runtimeClient{launchCwd: cwd})
	m.ready = true
	m.width = 80
	m.height = 12
	m.layout()
	m.composer.SetValue("@")
	m.ensureFileMentionIndex()
	m.fileMention.Cursor = len(m.fileMention.Entries) - 1

	rendered := plainText(m.renderFileMentionModal(8))
	last := m.fileMention.Entries[len(m.fileMention.Entries)-1]
	if !strings.Contains(rendered, last.Path) {
		t.Fatalf("rendered file modal missing selected file %q:\n%s", last.Path, rendered)
	}
	if !strings.Contains(rendered, "more above") {
		t.Fatalf("rendered file modal missing list-window hint:\n%s", rendered)
	}

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got := updated.(model)
	if got.fileMention.Cursor != 0 {
		t.Fatalf("cursor after wrap = %d, want 0", got.fileMention.Cursor)
	}
}

func TestFileMentionQuotesPathsWithSpaces(t *testing.T) {
	cwd := t.TempDir()
	writeTestFile(t, filepath.Join(cwd, "docs", "my file.go"))

	m := initialModel(&runtimeClient{launchCwd: cwd})
	m.ready = true
	m.composer.SetValue("open @my")
	m.ensureFileMentionIndex()

	next, ok := m.acceptFileMentionSelection()
	if !ok {
		t.Fatal("acceptFileMentionSelection ok = false, want true")
	}
	if next.composer.Value() != "open @\"my file.go\" " {
		t.Fatalf("composer value = %q, want quoted file name", next.composer.Value())
	}
	if expanded := next.expandFileMentionsForSend("open @\"my file.go\""); expanded != "open @\"docs/my file.go\"" {
		t.Fatalf("expanded input = %q, want quoted full path", expanded)
	}
}

func TestShowFileMentionPaletteFalseForSlashBusyNotReady(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.composer.SetValue("/")
	if m.showFileMentionPalette() {
		t.Fatal("file mention palette visible for slash command")
	}
	m.composer.SetValue("@")
	m.ready = false
	if m.showFileMentionPalette() {
		t.Fatal("file mention palette visible when not ready")
	}
	m.ready = true
	m.busy = true
	if m.showFileMentionPalette() {
		t.Fatal("file mention palette visible when busy before streaming")
	}
	m.streaming = true
	if !m.showFileMentionPalette() {
		t.Fatal("file mention palette hidden while streaming")
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
