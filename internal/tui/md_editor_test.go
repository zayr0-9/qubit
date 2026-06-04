package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMdEditorSlashCommandOpensListAndRequestsFiles(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.composer.SetValue("/md-editor")

	updated, cmd := m.submitInput()
	got := updated.(model)
	if got.mode != modeMdEditor || got.mdEditor.View != mdEditorList {
		t.Fatalf("mode/view = %v/%q, want md editor list", got.mode, got.mdEditor.View)
	}
	if !got.mdEditor.Loading {
		t.Fatalf("loading = %v, want true", got.mdEditor.Loading)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.list", "")
}

func TestMdEditorSlashCommandOpensDuringActiveRun(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.busy = true
	m.streaming = true
	m.activeRunID = "run_1"
	m.composer.SetValue("/md-editor")

	updated, cmd := m.submitInput()
	got := updated.(model)
	if got.mode != modeMdEditor || got.mdEditor.View != mdEditorList {
		t.Fatalf("mode/view = %v/%q, want md editor overlay while streaming", got.mode, got.mdEditor.View)
	}
	if !got.busy || !got.streaming || got.activeRunID != "run_1" {
		t.Fatalf("active run changed: busy=%v streaming=%v activeRunID=%q", got.busy, got.streaming, got.activeRunID)
	}
	if !got.mdEditor.Loading {
		t.Fatal("md editor loading = false, want true")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.list", "")
}

func TestMdEditorListRendersPlansAndUserDocsAndOpensSelection(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.width = 100
	m.height = 30
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdList(runtimeEvent{Type: "md.list", Files: []mdFileInfo{
		{Section: "user-docs", Name: "notes", Title: "Notes", Path: `D:\\repo\\.qubit\\user-docs\\notes.md`},
		{Section: "plans", Name: "launch", Title: "Launch Plan", Path: `D:\\repo\\.qubit\\plans\\launch.md`},
	}})

	rendered := plainText(m.renderMdEditor(20))
	if !strings.Contains(rendered, "notes.md") || !strings.Contains(rendered, "User notes") || !strings.Contains(rendered, "launch.md") || !strings.Contains(rendered, "Agent plans") {
		t.Fatalf("rendered list = %q, want separated user notes and agent plans sections", rendered)
	}
	if strings.Index(rendered, "User notes") > strings.Index(rendered, "Agent plans") {
		t.Fatalf("rendered list = %q, want user notes section before agent plans", rendered)
	}

	updated, cmd := m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if !got.mdEditor.Loading {
		t.Fatalf("loading = %v, want true", got.mdEditor.Loading)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.read", "")
	if payload["path"] != `D:\\repo\\.qubit\\user-docs\\notes.md` {
		t.Fatalf("path = %#v, want selected user doc", payload["path"])
	}
}

func TestMdEditorEditPlainEnterInsertsNewline(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\\repo\\.qubit\\plans\\launch.md`}, Content: "# Launch"})

	updated, _ := m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if m.mdEditor.View != mdEditorPreview {
		t.Fatalf("view = %q, want preview by default", m.mdEditor.View)
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)
	if m.mdEditor.View != mdEditorEdit {
		t.Fatalf("view = %q, want edit after ctrl+e", m.mdEditor.View)
	}
	updated, cmd := m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if cmd != nil {
		t.Fatalf("plain enter returned command, want nil")
	}
	if m.mdEditor.Editor.Value() != "# Launch\n" {
		t.Fatalf("editor value = %q, want plain enter to insert newline", m.mdEditor.Editor.Value())
	}
	if !m.mdEditor.Dirty {
		t.Fatal("dirty = false, want true after newline insert")
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)
	if m.mdEditor.View != mdEditorPreview {
		t.Fatalf("view = %q, want preview after second ctrl+e", m.mdEditor.View)
	}
}

func TestMdEditorPasteInsertsRawMarkdown(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\\repo\\.qubit\\plans\\launch.md`}, Content: "# Launch"})
	updated, _ := m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)

	m = m.updateMdEditorTeaPaste(tea.PasteMsg{Content: "\r\n- pasted\r\n```go\r\nfmt.Println(1)\r\n```"})
	want := "# Launch\n- pasted\n```go\nfmt.Println(1)\n```"
	if got := m.mdEditor.Editor.Value(); got != want {
		t.Fatalf("editor value = %q, want pasted markdown %q", got, want)
	}
	if !m.mdEditor.Dirty {
		t.Fatal("dirty = false, want true after paste")
	}
}

func TestMdEditorReadEditsRawMarkdownAndSaves(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.width = 100
	m.height = 30
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\repo\.qubit\plans\launch.md`}, Content: "# Launch\n\n- step"})

	if m.mdEditor.View != mdEditorPreview || m.mdEditor.Dirty {
		t.Fatalf("view/dirty = %q/%v, want preview/clean", m.mdEditor.View, m.mdEditor.Dirty)
	}
	rendered := plainText(m.renderMdEditor(20))
	if !strings.Contains(rendered, "Launch") || !strings.Contains(rendered, "step") {
		t.Fatalf("rendered preview = %q, want rendered markdown content", rendered)
	}

	updated, _ := m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)
	if m.mdEditor.View != mdEditorEdit {
		t.Fatalf("view = %q, want edit after ctrl+e", m.mdEditor.View)
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Text: "!", Code: '!'})
	m = updated.(model)
	if !m.mdEditor.Dirty || !strings.HasSuffix(m.mdEditor.Editor.Value(), "!") {
		t.Fatalf("dirty/value = %v/%q, want dirty appended raw text", m.mdEditor.Dirty, m.mdEditor.Editor.Value())
	}
	updated, cmd := m.updateMdEditor(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	got := updated.(model)
	if !got.mdEditor.Loading {
		t.Fatal("loading = false, want saving")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.save", "")
	if payload["content"] != "# Launch\n\n- step!" {
		t.Fatalf("content = %#v, want exact raw markdown", payload["content"])
	}

	got.applyMdSaved(runtimeEvent{Type: "md.saved", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\repo\.qubit\plans\launch.md`}, Content: "# Launch\n\n- step!"})
	if got.mdEditor.Dirty || got.mdEditor.OriginalContent != "# Launch\n\n- step!" {
		t.Fatalf("dirty/original = %v/%q, want saved clean state", got.mdEditor.Dirty, got.mdEditor.OriginalContent)
	}
	updated, _ = got.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	got = updated.(model)
	if got.mdEditor.View != mdEditorPreview {
		t.Fatalf("view = %q, want preview after second ctrl+e", got.mdEditor.View)
	}
}

func TestMdEditorCreateAndRenameFlow(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdList(runtimeEvent{Type: "md.list"})

	updated, cmd := m.updateMdEditor(tea.KeyPressMsg{Text: "n", Code: 'n'})
	m = updated.(model)
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.create", "")

	m.applyMdCreated(runtimeEvent{Type: "md.created", File: &mdFileInfo{Section: "user-docs", Name: "note-1", Path: `D:\repo\.qubit\user-docs\note-1.md`}, Content: ""})
	if m.mdEditor.View != mdEditorPreview || m.mdEditor.Current == nil || m.mdEditor.Current.Section != "user-docs" {
		t.Fatalf("created state = view %q current %#v, want opened user doc in preview", m.mdEditor.View, m.mdEditor.Current)
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)
	if m.mdEditor.View != mdEditorEdit {
		t.Fatalf("view = %q, want edit for rename", m.mdEditor.View)
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(model)
	if m.mdEditor.View != mdEditorRename || m.mdEditor.Rename.Value() != "note-1" {
		t.Fatalf("rename view/value = %q/%q", m.mdEditor.View, m.mdEditor.Rename.Value())
	}
	m.mdEditor.Rename.SetValue("project notes")
	updated, cmd = m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	payload = runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "md.rename", "")
	if payload["name"] != "project notes" {
		t.Fatalf("rename name = %#v", payload["name"])
	}

	m.applyMdRenamed(runtimeEvent{Type: "md.renamed", Path: `D:\repo\.qubit\user-docs\note-1.md`, File: &mdFileInfo{Section: "user-docs", Name: "project-notes", Path: `D:\repo\.qubit\user-docs\project-notes.md`}})
	if m.mdEditor.View != mdEditorEdit || m.mdEditor.Current == nil || m.mdEditor.Current.Name != "project-notes" {
		t.Fatalf("renamed state = view %q current %#v", m.mdEditor.View, m.mdEditor.Current)
	}
}

func TestMdEditorDirtyEscRequiresDiscardConfirmation(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\repo\.qubit\plans\launch.md`}, Content: "# Launch"})
	updated, _ := m.updateMdEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	m = updated.(model)
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Text: "!", Code: '!'})
	m = updated.(model)

	updated, cmd := m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("dirty esc returned command, want nil")
	}
	if m.mdEditor.View != mdEditorDiscardConfirm {
		t.Fatalf("view = %q, want discard confirm", m.mdEditor.View)
	}
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(model)
	if m.mdEditor.View != mdEditorEdit || !m.mdEditor.Dirty {
		t.Fatalf("after cancel view/dirty = %q/%v, want edit/dirty", m.mdEditor.View, m.mdEditor.Dirty)
	}

	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(model)
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(model)
	updated, _ = m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if m.mdEditor.View != mdEditorList || m.mdEditor.Dirty || m.mdEditor.Editor.Value() != "# Launch" {
		t.Fatalf("after discard view/dirty/value = %q/%v/%q", m.mdEditor.View, m.mdEditor.Dirty, m.mdEditor.Editor.Value())
	}
}

func TestMdEditorUpFromBottomVisibleLineDoesNotScrollPrematurely(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 14
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "long", Path: `D:\repo\.qubit\plans\long.md`}, Content: "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight"})
	m.layoutMdEditorComposer()
	m.mdEditor.Editor.MoveToEnd(false)
	m.layoutMdEditorComposer()
	bottomScroll := m.mdEditor.Editor.ScrollLine()

	updated, _ := m.updateMdEditor(tea.KeyPressMsg{Code: tea.KeyUp})
	got := updated.(model)
	if got.mdEditor.Editor.ScrollLine() != bottomScroll {
		t.Fatalf("scrollLine after one up = %d, want %d; cursor should move within visible box before scrolling", got.mdEditor.Editor.ScrollLine(), bottomScroll)
	}
}

func TestMdEditorRenamePasteInsertsIntoRenameInput(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeMdEditor
	m.mdEditor = newMdEditorState()
	m.applyMdRead(runtimeEvent{Type: "md.read", File: &mdFileInfo{Section: "plans", Name: "launch", Path: `D:\\repo\\.qubit\\plans\\launch.md`}, Content: "# Launch"})
	m = m.openMdEditorRename()
	m.mdEditor.Rename.Reset()

	m = m.updateMdEditorTeaPaste(tea.PasteMsg{Content: "renamed doc\r\n"})

	if m.mdEditor.View != mdEditorRename {
		t.Fatalf("view = %q, want rename", m.mdEditor.View)
	}
	if got := m.mdEditor.Rename.Value(); got != "renamed doc" {
		t.Fatalf("rename value = %q, want pasted rename", got)
	}
	if got := m.mdEditor.Editor.Value(); got != "# Launch" {
		t.Fatalf("editor body changed to %q, want unchanged", got)
	}
}
