package main

import (
	"strings"
	"testing"
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
