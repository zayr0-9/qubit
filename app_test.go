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
