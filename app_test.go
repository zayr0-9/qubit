package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestIsNewlineKey(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want bool
	}{
		{
			name: "plain enter sends, not newline",
			msg:  tea.KeyPressMsg{Code: tea.KeyEnter},
			want: false,
		},
		{
			name: "shift enter inserts newline",
			msg:  tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift},
			want: true,
		},
		{
			name: "alt enter inserts newline when terminal passes it through",
			msg:  tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt},
			want: true,
		},
		{
			name: "ctrl j inserts newline fallback",
			msg:  tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNewlineKey(tt.msg); got != tt.want {
				t.Fatalf("isNewlineKey(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestNormalizeInputNewlines(t *testing.T) {
	input := "a\r\nb\rc\n"
	want := "a\nb\nc\n"
	if got := normalizeInputNewlines(input); got != want {
		t.Fatalf("normalizeInputNewlines() = %q, want %q", got, want)
	}
}

func TestSessionActivatedRequestsMessages(t *testing.T) {
	m := model{session: "old", title: "Old", ready: true, messages: []chatMessage{{Role: "assistant", Content: "old message"}}}

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "session.activated", SessionID: "sess_1", SessionTitle: "Session one"})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("session activation returned nil command, want transcript load command")
	}
	if got.session != "sess_1" {
		t.Fatalf("session = %q, want sess_1", got.session)
	}
	if got.title != "Session one" {
		t.Fatalf("title = %q, want Session one", got.title)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading transcript")
	}
	if got.status != "loading transcript" {
		t.Fatalf("status = %q, want loading transcript", got.status)
	}
	if len(got.messages) != 1 || got.messages[0].Role != "assistant" || got.messages[0].Content == "old message" {
		t.Fatalf("messages = %#v, want loading placeholder", got.messages)
	}
}

func TestApplySessionMessagesReplacesTranscript(t *testing.T) {
	m := model{session: "sess_1", title: "Session one", busy: true, messages: []chatMessage{{Role: "assistant", Content: "loading"}}}
	messages := []chatMessage{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_1", SessionTitle: "Renamed", Messages: messages})

	if m.busy {
		t.Fatal("busy = true, want false")
	}
	if m.status != "ready" {
		t.Fatalf("status = %q, want ready", m.status)
	}
	if m.title != "Renamed" {
		t.Fatalf("title = %q, want Renamed", m.title)
	}
	if len(m.messages) != len(messages) {
		t.Fatalf("message count = %d, want %d", len(m.messages), len(messages))
	}
	for i := range messages {
		if m.messages[i] != messages[i] {
			t.Fatalf("message[%d] = %#v, want %#v", i, m.messages[i], messages[i])
		}
	}
}

func TestApplySessionMessagesEmptyShowsPlaceholder(t *testing.T) {
	m := model{session: "sess_1", busy: true}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_1"})

	if m.busy {
		t.Fatal("busy = true, want false")
	}
	if len(m.messages) != 1 || m.messages[0].Role != "assistant" || m.messages[0].Content == "" {
		t.Fatalf("messages = %#v, want non-empty assistant placeholder", m.messages)
	}
}

func TestApplySessionMessagesIgnoresStaleSession(t *testing.T) {
	m := model{session: "current", busy: true, messages: []chatMessage{{Role: "assistant", Content: "keep"}}}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "stale", Messages: []chatMessage{{Role: "user", Content: "stale"}}})

	if !m.busy {
		t.Fatal("busy = false, want unchanged true for stale transcript")
	}
	if len(m.messages) != 1 || m.messages[0].Content != "keep" {
		t.Fatalf("messages = %#v, want unchanged", m.messages)
	}
}

func TestAssistantEventStartsFakeStream(t *testing.T) {
	m := model{busy: true, messages: []chatMessage{{Role: "user", Content: "hello"}}}

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "assistant", SessionID: "sess_1", Content: "hello world"})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("assistant event returned nil command, want fake stream tick command")
	}
	if !got.streaming {
		t.Fatal("streaming = false, want true")
	}
	if got.streamingFullContent != "hello world" {
		t.Fatalf("streamingFullContent = %q, want hello world", got.streamingFullContent)
	}
	if got.streamingMessageIndex != 1 {
		t.Fatalf("streamingMessageIndex = %d, want 1", got.streamingMessageIndex)
	}
	if len(got.messages) != 2 || got.messages[1].Role != "assistant" || got.messages[1].Content != "" {
		t.Fatalf("messages = %#v, want empty assistant streaming placeholder", got.messages)
	}
	if got.session != "sess_1" {
		t.Fatalf("session = %q, want sess_1", got.session)
	}
}

func TestFakeStreamTickAdvancesContent(t *testing.T) {
	m := model{
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "abcdefghi",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, cmd := m.updateFakeStreamTick()
	got := updated.(model)

	if !got.streaming {
		t.Fatal("streaming = false, want true after partial tick")
	}
	if cmd == nil {
		t.Fatal("fake stream tick returned nil command, want next tick")
	}
	if got.messages[0].Content != "abc" {
		t.Fatalf("content = %q, want abc", got.messages[0].Content)
	}
	if got.streamingVisibleRunes != 3 {
		t.Fatalf("streamingVisibleRunes = %d, want 3", got.streamingVisibleRunes)
	}
}

func TestFakeStreamTickCompletesAfterRunFinished(t *testing.T) {
	m := model{
		busy:                  true,
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "ok",
		streamingFinished:     true,
		streamingFinishStatus: "completed",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, cmd := m.updateFakeStreamTick()
	got := updated.(model)

	if cmd != nil {
		t.Fatal("fake stream completion returned command, want nil")
	}
	if got.streaming {
		t.Fatal("streaming = true, want false")
	}
	if got.busy {
		t.Fatal("busy = true, want false after streamed run finished")
	}
	if got.status != "completed" {
		t.Fatalf("status = %q, want completed", got.status)
	}
	if got.messages[0].Content != "ok" {
		t.Fatalf("content = %q, want ok", got.messages[0].Content)
	}
}

func TestRunFinishedWaitsForFakeStream(t *testing.T) {
	m := model{
		busy:                  true,
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "hello",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_finished", Status: "done"})
	got := updated.(model)

	if !got.streaming {
		t.Fatal("streaming = false, want true")
	}
	if !got.streamingFinished {
		t.Fatal("streamingFinished = false, want true")
	}
	if !got.busy {
		t.Fatal("busy = false, want true until fake stream completes")
	}
	if got.streamingFinishStatus != "done" {
		t.Fatalf("streamingFinishStatus = %q, want done", got.streamingFinishStatus)
	}
}

func TestApplySessionMessagesClearsFakeStream(t *testing.T) {
	m := model{
		session:               "sess_1",
		busy:                  true,
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "old",
		messages:              []chatMessage{{Role: "assistant", Content: "o"}},
	}
	messages := []chatMessage{{Role: "user", Content: "new"}}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_1", Messages: messages})

	if m.streaming {
		t.Fatal("streaming = true, want false")
	}
	if m.busy {
		t.Fatal("busy = true, want false")
	}
	if len(m.messages) != 1 || m.messages[0] != messages[0] {
		t.Fatalf("messages = %#v, want transcript replacement", m.messages)
	}
}

func TestFakeStreamTickClearsInvalidIndex(t *testing.T) {
	m := model{
		busy:                  true,
		streaming:             true,
		streamingMessageIndex: 3,
		streamingFullContent:  "hello",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, cmd := m.updateFakeStreamTick()
	got := updated.(model)

	if cmd != nil {
		t.Fatal("invalid fake stream tick returned command, want nil")
	}
	if got.streaming {
		t.Fatal("streaming = true, want false")
	}
	if got.busy {
		t.Fatal("busy = true, want false")
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}
}

func TestChatArrowKeysEditInputInsteadOfScrollingViewport(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.layout()
	m.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5")
	m.viewport.GotoBottom()
	startOffset := m.viewport.YOffset()

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got := updated.(model)

	if got.viewport.YOffset() != startOffset {
		t.Fatalf("viewport YOffset changed from %d to %d; arrow keys should stay in composer", startOffset, got.viewport.YOffset())
	}
}

func TestCtrlEndMovesComposerToEnd(t *testing.T) {
	m := initialModel(nil)
	m.composer.SetValue("hello")
	m.composer.MoveToBegin(false)

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	got := updated.(model)

	if got.composer.cursor != len([]rune("hello")) {
		t.Fatalf("composer cursor = %d, want end", got.composer.cursor)
	}
}

func TestCtrlASelectsAllInput(t *testing.T) {
	m := initialModel(nil)
	m.composer.SetValue("hello")

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	got := updated.(model)

	if !got.composer.HasSelection() {
		t.Fatal("composer selection = false, want true")
	}
	if got.composer.SelectedText() != "hello" {
		t.Fatalf("selected text = %q, want hello", got.composer.SelectedText())
	}
}

func TestTypingReplacesSelectedInput(t *testing.T) {
	m := initialModel(nil)
	m.composer.SetValue("hello")
	m.composer.SelectAll()

	updated, _ := m.updateKey(tea.KeyPressMsg{Text: "x", Code: 'x'})
	got := updated.(model)

	if got.composer.HasSelection() {
		t.Fatal("selection = true, want false after replacement")
	}
	if got.composer.Value() != "x" {
		t.Fatalf("input value = %q, want x", got.composer.Value())
	}
}
