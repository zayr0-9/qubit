package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestComposerSelectAllAndReplace(t *testing.T) {
	c := newComposer()
	c.SetValue("hello")
	c.SelectAll()

	if !c.HasSelection() {
		t.Fatal("HasSelection() = false, want true")
	}
	if got := c.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText() = %q, want hello", got)
	}

	c.InsertString("x")
	if got := c.Value(); got != "x" {
		t.Fatalf("Value() = %q, want x", got)
	}
	if c.HasSelection() {
		t.Fatal("HasSelection() = true, want false after replacement")
	}
}

func TestComposerShiftSelection(t *testing.T) {
	c := newComposer()
	c.SetValue("abc")
	c.MoveLeft(true)
	c.MoveLeft(true)

	if got := c.SelectedText(); got != "bc" {
		t.Fatalf("SelectedText() = %q, want bc", got)
	}
	c.MoveLeft(false)
	if c.HasSelection() {
		t.Fatal("selection should clear on plain movement")
	}
}

func TestComposerWordSelection(t *testing.T) {
	c := newComposer()
	c.SetValue("hello world")
	c.MoveWordLeft(true)

	if got := c.SelectedText(); got != "world" {
		t.Fatalf("SelectedText() = %q, want world", got)
	}
}

func TestComposerViewDoesNotHighlightPrompt(t *testing.T) {
	c := newComposer()
	c.SetValue("hello")
	c.SelectAll()
	view := c.View("› ", 0)

	if strings.HasPrefix(view, inputSelectSt.Render("›")) || strings.HasPrefix(view, inputSelectSt.Render("› ")) {
		t.Fatalf("prompt appears highlighted in view: %q", view)
	}
	if !strings.Contains(view, inputSelectSt.Render("h")) {
		t.Fatalf("selected text was not highlighted inline: %q", view)
	}
}

func TestComposerHeightRespectsMax(t *testing.T) {
	c := newComposer()
	c.maxHeight = 3
	c.SetWidth(5)
	c.SetValue("one two three four five six seven")
	c.SelectAll()

	if got := c.Height(); got != 3 {
		t.Fatalf("Height() = %d, want 3", got)
	}
	if lines := strings.Count(c.View("› ", 0), "\n") + 1; lines != 3 {
		t.Fatalf("view lines = %d, want 3", lines)
	}
}

func TestComposerPromptOnlyAppearsOnFirstVisibleLine(t *testing.T) {
	c := newComposer()
	c.SetWidth(4)
	c.SetValue("abcdefgh")

	view := c.View("› ", 0)
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("view line count = %d, want at least 2", len(lines))
	}
	if !strings.HasPrefix(lines[0], "› ") {
		t.Fatalf("first line = %q, want prompt prefix", lines[0])
	}
	for i, line := range lines[1:] {
		if strings.HasPrefix(line, "› ") {
			t.Fatalf("line %d has duplicate prompt: %q", i+2, line)
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("line %d = %q, want prompt-width indentation", i+2, line)
		}
	}
}

func TestComposerCtrlArrowKeyMovesByWordFromModifiers(t *testing.T) {
	c := newComposer()
	c.SetValue("hello world")
	c.MoveToBegin(false)

	handled, cmd := c.UpdateKey(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl})
	if !handled || cmd != nil {
		t.Fatalf("UpdateKey handled/cmd = %v/%v, want true/nil", handled, cmd)
	}
	if c.cursor != len([]rune("hello ")) {
		t.Fatalf("cursor after ctrl+right = %d, want after first word", c.cursor)
	}
	if c.HasSelection() {
		t.Fatal("ctrl+right selected text, want plain word movement")
	}

	handled, cmd = c.UpdateKey(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl})
	if !handled || cmd != nil {
		t.Fatalf("UpdateKey handled/cmd = %v/%v, want true/nil", handled, cmd)
	}
	if c.cursor != 0 {
		t.Fatalf("cursor after ctrl+left = %d, want beginning", c.cursor)
	}
}

func TestComposerCtrlShiftArrowKeySelectsByWordFromModifiers(t *testing.T) {
	c := newComposer()
	c.SetValue("hello world")

	handled, cmd := c.UpdateKey(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl | tea.ModShift})
	if !handled || cmd != nil {
		t.Fatalf("UpdateKey handled/cmd = %v/%v, want true/nil", handled, cmd)
	}
	if got := c.SelectedText(); got != "world" {
		t.Fatalf("SelectedText() = %q, want world", got)
	}
}

func TestComposerUndoRedo(t *testing.T) {
	c := newComposer()
	c.InsertString("hello")
	c.InsertString(" world")

	if !c.Undo() {
		t.Fatal("Undo() = false, want true")
	}
	if got := c.Value(); got != "hello" {
		t.Fatalf("Value() after undo = %q, want hello", got)
	}
	if !c.Redo() {
		t.Fatal("Redo() = false, want true")
	}
	if got := c.Value(); got != "hello world" {
		t.Fatalf("Value() after redo = %q, want hello world", got)
	}
}

func TestComposerCtrlZCtrlShiftZKeysUndoRedo(t *testing.T) {
	c := newComposer()
	c.InsertString("one")
	c.InsertString(" two")

	handled, cmd := c.UpdateKey(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if !handled || cmd != nil {
		t.Fatalf("ctrl+z handled/cmd = %v/%v, want true/nil", handled, cmd)
	}
	if got := c.Value(); got != "one" {
		t.Fatalf("Value() after ctrl+z = %q, want one", got)
	}

	handled, cmd = c.UpdateKey(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl | tea.ModShift})
	if !handled || cmd != nil {
		t.Fatalf("ctrl+shift+z handled/cmd = %v/%v, want true/nil", handled, cmd)
	}
	if got := c.Value(); got != "one two" {
		t.Fatalf("Value() after ctrl+shift+z = %q, want one two", got)
	}
}

func TestComposerUndoRestoresDeletedWord(t *testing.T) {
	c := newComposer()
	c.SetValue("hello world")
	c.DeleteWordBackward()

	if got := c.Value(); got != "hello " {
		t.Fatalf("Value() after delete word = %q, want hello", got)
	}
	if !c.Undo() {
		t.Fatal("Undo() = false, want true")
	}
	if got := c.Value(); got != "hello world" {
		t.Fatalf("Value() after undo = %q, want hello world", got)
	}
	if c.cursor != len([]rune("hello world")) {
		t.Fatalf("cursor after undo = %d, want end", c.cursor)
	}
}

func TestComposerCharLimitAllowsLargePrompts(t *testing.T) {
	c := newComposer()
	input := strings.Repeat("x", composerCharLimit+1)

	c.SetValue(input)
	if got := len([]rune(c.Value())); got != composerCharLimit {
		t.Fatalf("SetValue length = %d, want %d", got, composerCharLimit)
	}

	c.Reset()
	c.InsertString(input)
	if got := len([]rune(c.Value())); got != composerCharLimit {
		t.Fatalf("InsertString length = %d, want %d", got, composerCharLimit)
	}
}

func TestComposerStyledTextDoesNotStylePrompt(t *testing.T) {
	c := newComposer()
	c.focused = false
	c.SetValue("hello")

	view := c.ViewStyled("› ", 0, forkSelectInputSt)

	if strings.HasPrefix(view, forkSelectInputSt.Render("›")) || strings.HasPrefix(view, forkSelectInputSt.Render("› ")) {
		t.Fatalf("prompt appears styled as input text: %q", view)
	}
	if !strings.HasPrefix(view, "› "+forkSelectInputSt.Render("hello")) {
		t.Fatalf("normal input text was not styled after raw prompt: %q", view)
	}
}

func TestComposerStyledTextDoesNotOverrideSelectionOrPlaceholder(t *testing.T) {
	c := newComposer()
	c.SetValue("hello")
	c.SelectAll()

	selectedView := c.ViewStyled("› ", 0, messageEditInputSt)
	if !strings.Contains(selectedView, inputSelectSt.Render("h")) {
		t.Fatalf("selection styling was not preserved: %q", selectedView)
	}
	if strings.Contains(selectedView, messageEditInputSt.Render("h")) {
		t.Fatalf("normal text style overrode selected text: %q", selectedView)
	}

	c.Reset()
	placeholderView := c.ViewStyled("› ", 0, messageEditInputSt)
	if !strings.Contains(placeholderView, mutedSt.Render(c.placeholder)) {
		t.Fatalf("placeholder did not keep muted style: %q", placeholderView)
	}
}
