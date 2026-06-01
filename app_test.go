package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestRefreshViewportRendersRoleIconsInline(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.messages = []chatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	m.layout()
	m.refreshViewport()

	viewport := plainText(m.viewport.View())
	if strings.Contains(viewport, "You") || strings.Contains(viewport, "Qubit") {
		t.Fatalf("viewport = %q, want role names removed", viewport)
	}
	if !strings.Contains(viewport, "◆   hello") || !strings.Contains(viewport, "◆   hi") {
		t.Fatalf("viewport = %q, want inline user and assistant icons", viewport)
	}
}

func TestSessionPickerEnterClearsOldMessagesWhileLoading(t *testing.T) {
	m := initialModel(nil)
	m.sessions = []sessionInfo{{ID: "sess_1", Title: "Empty old session"}}
	m.messages = []chatMessage{{Role: "assistant", Content: "current conversation should disappear"}}
	m.ready = true

	updated, cmd := m.updateSessionPicker(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("session picker enter returned nil command, want activate command")
	}
	if got.session != "sess_1" {
		t.Fatalf("session = %q, want sess_1", got.session)
	}
	if got.title != "Empty old session" {
		t.Fatalf("title = %q, want Empty old session", got.title)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading transcript")
	}
	if got.status != "loading transcript" {
		t.Fatalf("status = %q, want loading transcript", got.status)
	}
	if len(got.messages) != 0 {
		t.Fatalf("messages = %#v, want old visible chat cleared while loading", got.messages)
	}
	if strings.Contains(got.viewport.View(), "current conversation should disappear") {
		t.Fatalf("viewport still contains previous conversation: %q", got.viewport.View())
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
		activeRunID:           "run_1",
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
	if got.activeRunID != "" {
		t.Fatalf("activeRunID = %q, want cleared", got.activeRunID)
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

func TestEscapeAbortsFakeStreamPreservingVisibleContent(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{
		runtime:               rt,
		busy:                  true,
		activeRunID:           "run_1",
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "hello world",
		streamingVisibleRunes: 5,
		streamingFinished:     true,
		streamingFinishStatus: "completed",
		lastRunStartedSession: "sess_1",
		messages:              []chatMessage{{Role: "assistant", Content: "hello"}},
	}

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	payload := runSendCommand(t, cmd, stdin)
	if payload["type"] != "chat.cancel" || payload["runId"] != "run_1" {
		t.Fatalf("payload = %#v, want chat.cancel for run_1", payload)
	}
	if got.streaming {
		t.Fatal("streaming = true, want false")
	}
	if got.busy {
		t.Fatal("busy = true, want false")
	}
	if got.status != "aborted" {
		t.Fatalf("status = %q, want aborted", got.status)
	}
	if got.activeRunID != "" {
		t.Fatalf("activeRunID = %q, want cleared", got.activeRunID)
	}
	if got.lastRunStartedSession != "" {
		t.Fatalf("lastRunStartedSession = %q, want cleared", got.lastRunStartedSession)
	}
	if got.streamingFullContent != "" || got.streamingVisibleRunes != 0 || got.streamingFinished || got.streamingFinishStatus != "" {
		t.Fatalf("streaming state not cleared: %#v", got)
	}
	if len(got.messages) != 1 || got.messages[0].Content != "hello" {
		t.Fatalf("messages = %#v, want partial assistant content preserved", got.messages)
	}
}

func TestEscapeClearsSelectionBeforeAbortingFakeStream(t *testing.T) {
	m := model{
		busy:                  true,
		activeRunID:           "run_1",
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "hello world",
		messages:              []chatMessage{{Role: "assistant", Content: "hel"}},
	}
	m.composer.SetValue("draft")
	m.composer.SelectAll()

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("escape with selection returned command, want nil")
	}
	if got.composer.HasSelection() {
		t.Fatal("composer still has selection, want cleared")
	}
	if !got.streaming {
		t.Fatal("streaming = false, want selection clear to leave stream active")
	}
	if !got.busy {
		t.Fatal("busy = false, want selection clear to leave run busy")
	}
	if got.activeRunID != "run_1" {
		t.Fatalf("activeRunID = %q, want run_1", got.activeRunID)
	}
	if got.messages[0].Content != "hel" {
		t.Fatalf("content = %q, want hel", got.messages[0].Content)
	}
}

func TestEscapeWhileThinkingSendsCancel(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{runtime: rt, busy: true, activeRunID: "run_thinking", status: "thinking"}

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	payload := runSendCommand(t, cmd, stdin)
	if payload["type"] != "chat.cancel" || payload["runId"] != "run_thinking" {
		t.Fatalf("payload = %#v, want chat.cancel for run_thinking", payload)
	}
	if got.busy {
		t.Fatal("busy = true, want false")
	}
	if got.activeRunID != "" {
		t.Fatalf("activeRunID = %q, want cleared", got.activeRunID)
	}
	if got.status != "aborted" {
		t.Fatalf("status = %q, want aborted", got.status)
	}
}

func TestFakeStreamTickAfterAbortKeepsPartialContent(t *testing.T) {
	m := model{
		busy:     false,
		status:   "aborted",
		messages: []chatMessage{{Role: "assistant", Content: "hello"}},
	}

	updated, cmd := m.updateFakeStreamTick()
	got := updated.(model)

	if cmd != nil {
		t.Fatal("fake stream tick after abort returned command, want nil")
	}
	if got.messages[0].Content != "hello" {
		t.Fatalf("content = %q, want partial content preserved", got.messages[0].Content)
	}
	if got.status != "aborted" {
		t.Fatalf("status = %q, want aborted", got.status)
	}
}

func TestApplySessionMessagesRelayoutsViewport(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.layout()
	m.viewport.SetHeight(3)
	messages := []chatMessage{{Role: "assistant", Content: strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")}}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", Messages: messages})

	if got := m.viewport.Height(); got <= 3 {
		t.Fatalf("viewport height = %d, want layout recalculated above stale height 3", got)
	}
	if !m.viewport.AtBottom() {
		t.Fatalf("viewport not at bottom after loaded transcript: y=%d total=%d height=%d", m.viewport.YOffset(), m.viewport.TotalLineCount(), m.viewport.Height())
	}
}

func TestApplySessionMessagesClearsFakeStream(t *testing.T) {
	m := model{
		session:               "sess_1",
		busy:                  true,
		activeRunID:           "run_1",
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
	if m.activeRunID != "" {
		t.Fatalf("activeRunID = %q, want cleared", m.activeRunID)
	}
	if len(m.messages) != 1 || m.messages[0] != messages[0] {
		t.Fatalf("messages = %#v, want transcript replacement", m.messages)
	}
}

func TestFakeStreamTickClearsInvalidIndex(t *testing.T) {
	m := model{
		busy:                  true,
		activeRunID:           "run_1",
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
	if got.activeRunID != "" {
		t.Fatalf("activeRunID = %q, want cleared", got.activeRunID)
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

func TestInputHistoryCyclesFromEmptyComposer(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.inputHistory = []string{"first", "second"}
	m.inputHistoryIndex = len(m.inputHistory)
	m.layout()

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got := updated.(model)
	if got.composer.Value() != "second" {
		t.Fatalf("composer value after first up = %q, want second", got.composer.Value())
	}

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = updated.(model)
	if got.composer.Value() != "first" {
		t.Fatalf("composer value after second up = %q, want first", got.composer.Value())
	}

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got = updated.(model)
	if got.composer.Value() != "second" {
		t.Fatalf("composer value after down = %q, want second", got.composer.Value())
	}

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got = updated.(model)
	if got.composer.Value() != "" {
		t.Fatalf("composer value after down past newest = %q, want empty", got.composer.Value())
	}
	if got.inputHistoryActive {
		t.Fatal("inputHistoryActive = true after returning to empty composer, want false")
	}
}

func TestInputHistoryDoesNotStartWhenComposerHasText(t *testing.T) {
	m := initialModel(nil)
	m.inputHistory = []string{"old"}
	m.inputHistoryIndex = len(m.inputHistory)
	m.composer.SetValue("draft")

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got := updated.(model)
	if got.composer.Value() != "draft" {
		t.Fatalf("composer value = %q, want draft", got.composer.Value())
	}
	if got.inputHistoryActive {
		t.Fatal("inputHistoryActive = true, want false")
	}
}

func TestSubmitInputPersistsInputHistory(t *testing.T) {
	appRoot := t.TempDir()
	rt, stdin := newTestRuntime(t)
	rt.appRoot = appRoot
	m := initialModel(rt)
	m.ready = true
	m.autoNewSessionOnChat = false
	m.session = "sess_1"
	m.composer.SetValue("remember me")

	updated, cmd := m.submitInput()
	got := updated.(model)
	runBatchSendCommand(t, cmd, stdin, "chat")

	if len(got.inputHistory) != 1 || got.inputHistory[0] != "remember me" {
		t.Fatalf("inputHistory = %#v, want [remember me]", got.inputHistory)
	}
	loaded, err := loadInputHistory(appRoot)
	if err != nil {
		t.Fatalf("load input history: %v", err)
	}
	if len(loaded) != 1 || loaded[0] != "remember me" {
		t.Fatalf("loaded input history = %#v, want [remember me]", loaded)
	}
}

func TestPageDownLeavesAutoScrollAtBottom(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.layout()
	m.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10")
	m.viewport.GotoTop()

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	got := updated.(model)

	if got.viewport.YOffset() == 0 {
		t.Fatal("viewport did not move down on PgDown")
	}
	if got.viewport.AtBottom() != got.autoScroll {
		t.Fatalf("autoScroll = %v, want AtBottom %v", got.autoScroll, got.viewport.AtBottom())
	}
}

func TestMouseWheelScrollsViewport(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.layout()
	m.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10")
	m.viewport.GotoTop()

	updated := m.updateMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.viewport.YOffset() == 0 {
		t.Fatal("viewport did not move down on mouse wheel down")
	}
	if updated.autoScroll != updated.viewport.AtBottom() {
		t.Fatalf("autoScroll = %v, want AtBottom %v", updated.autoScroll, updated.viewport.AtBottom())
	}

	updated = updated.updateMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp}).(model)
	if updated.autoScroll {
		t.Fatal("autoScroll = true after mouse wheel up, want false")
	}
}

func TestRefreshViewportPreservesOffsetWhenAutoScrollDisabled(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.layout()
	m.messages = []chatMessage{{Role: "assistant", Content: strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")}}
	m.refreshViewport()
	m.viewport.SetYOffset(2)
	m.autoScroll = false

	m.refreshViewport()

	if got := m.viewport.YOffset(); got != 2 {
		t.Fatalf("YOffset = %d, want preserved offset 2", got)
	}
}

func TestLayoutPreservesOffsetWhenAutoScrollDisabled(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.layout()
	m.messages = []chatMessage{{Role: "assistant", Content: strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")}}
	m.refreshViewport()
	m.viewport.SetYOffset(2)
	m.autoScroll = false

	m.layout()

	if got := m.viewport.YOffset(); got != 2 {
		t.Fatalf("YOffset = %d, want preserved offset 2", got)
	}
}

func TestLayoutKeepsBottomWhenAutoScrollEnabled(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.messages = []chatMessage{{Role: "assistant", Content: strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "line 9", "line 10",
	}, "\n")}}
	m.autoScroll = true
	m.layout()

	if !m.viewport.AtBottom() {
		t.Fatalf("viewport not at bottom after layout with autoScroll enabled: y=%d total=%d height=%d", m.viewport.YOffset(), m.viewport.TotalLineCount(), m.viewport.Height())
	}
}

func TestRenderMessageContentUsesCache(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.layout()
	message := chatMessage{Role: "assistant", Content: "**hello**"}

	first := m.renderMessageContent(message, true)
	if len(m.renderCache) != 2 {
		t.Fatalf("render cache entries = %d, want 2 including initial ready message and test message", len(m.renderCache))
	}
	second := m.renderMessageContent(message, true)
	if first != second {
		t.Fatalf("cached render changed: first %q second %q", first, second)
	}
	if len(m.renderCache) != 2 {
		t.Fatalf("render cache entries after cached render = %d, want 2", len(m.renderCache))
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

func TestSlashSessionsOpensPickerAndRequestsList(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.sessions = []sessionInfo{{ID: "sess_1", Title: "Existing"}}

	updated, cmd := m.handleSlashCommand("/sessions")
	got := updated.(model)

	if got.mode != modeSessionPicker {
		t.Fatalf("mode = %v, want session picker", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading sessions")
	}
	if got.status != "loading sessions" {
		t.Fatalf("status = %q, want loading sessions", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.list", "")
}

func TestCreateNewSessionCommandAndCreatedEvent(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.composer.SetValue("/new Project chat")

	updated, cmd := m.submitInput()
	got := updated.(model)

	if !got.busy {
		t.Fatal("busy = false, want true while creating session")
	}
	if got.status != "creating session" {
		t.Fatalf("status = %q, want creating session", got.status)
	}
	if got.composer.Value() != "" {
		t.Fatalf("composer value = %q, want cleared after submit", got.composer.Value())
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.new", "")
	if gotTitle := payload["title"]; gotTitle != "Project chat" {
		t.Fatalf("session.new title = %#v, want Project chat", gotTitle)
	}

	updated, _ = got.updateRuntime(runtimeEvent{Type: "session.created", SessionID: "sess_new", SessionTitle: "Project chat"})
	got = updated.(model)
	if got.busy {
		t.Fatal("busy = true, want false after session.created")
	}
	if got.session != "sess_new" {
		t.Fatalf("session = %q, want sess_new", got.session)
	}
	if got.title != "Project chat" {
		t.Fatalf("title = %q, want Project chat", got.title)
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}
	if len(got.messages) != 1 || !strings.Contains(got.messages[0].Content, "Started new session") {
		t.Fatalf("messages = %#v, want new session confirmation", got.messages)
	}
}

func TestForkCommandRequestsForkAndLoadsForkTranscript(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_parent"
	m.title = "Parent"
	m.autoNewSessionOnChat = false
	m.messages = []chatMessage{{Role: "user", Content: "question"}, {Role: "assistant", Content: "answer"}}
	m.composer.SetValue("/fork Experiment")

	updated, cmd := m.submitInput()
	got := updated.(model)

	if !got.busy {
		t.Fatal("busy = false, want true while forking session")
	}
	if got.status != "forking session" {
		t.Fatalf("status = %q, want forking session", got.status)
	}
	if got.autoNewSessionOnChat {
		t.Fatal("autoNewSessionOnChat = true, want false after fork command")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.fork", "sess_parent")
	if payload["title"] != "Experiment" {
		t.Fatalf("session.fork title = %#v, want Experiment", payload["title"])
	}
	if payload["messageIndex"] != float64(2) {
		t.Fatalf("session.fork messageIndex = %#v, want 2", payload["messageIndex"])
	}

	updated, cmd = got.updateRuntime(runtimeEvent{Type: "session.forked", SessionID: "sess_child", SessionTitle: "Experiment"})
	got = updated.(model)
	if got.session != "sess_child" {
		t.Fatalf("session = %q, want sess_child", got.session)
	}
	if got.title != "Experiment" {
		t.Fatalf("title = %q, want Experiment", got.title)
	}
	if got.status != "loading transcript" {
		t.Fatalf("status = %q, want loading transcript", got.status)
	}
	transcriptPayload := runBatchSendCommand(t, cmd, stdin, "session.messages")
	assertPayload(t, transcriptPayload, "session.messages", "sess_child")

	loaded := []chatMessage{{Role: "user", Content: "question"}, {Role: "assistant", Content: "answer"}}
	got.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_child", SessionTitle: "Experiment", Messages: loaded})
	if got.busy {
		t.Fatal("busy = true, want false after fork transcript load")
	}
	if len(got.messages) != len(loaded) {
		t.Fatalf("message count = %d, want %d", len(got.messages), len(loaded))
	}
}

func TestSessionPickerLoadsSelectedSessionTranscript(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.width = 80
	m.height = 24
	m.layout()
	m.mode = modeSessionPicker
	m.session = "current"
	m.title = "Current"
	m.messages = []chatMessage{{Role: "assistant", Content: "current conversation should disappear"}}
	m.sessions = []sessionInfo{
		{ID: "current", Title: "Current"},
		{ID: "sess_old", Title: "Older session", MessageCount: 2},
	}
	m.sessionCursor = 1
	m.refreshViewport()

	updated, cmd := m.updateSessionPicker(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)

	if got.mode != modeChat {
		t.Fatalf("mode = %v, want chat", got.mode)
	}
	if got.session != "sess_old" {
		t.Fatalf("session = %q, want sess_old", got.session)
	}
	if got.title != "Older session" {
		t.Fatalf("title = %q, want Older session", got.title)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while switching/loading")
	}
	if got.status != "loading transcript" {
		t.Fatalf("status = %q, want loading transcript", got.status)
	}
	if len(got.messages) != 0 {
		t.Fatalf("messages = %#v, want cleared while transcript loads", got.messages)
	}
	if strings.Contains(got.viewport.View(), "current conversation should disappear") {
		t.Fatalf("viewport still contains previous conversation: %q", got.viewport.View())
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.activate", "sess_old")

	updated, cmd = got.updateRuntime(runtimeEvent{Type: "session.activated", SessionID: "sess_old", SessionTitle: "Older session"})
	got = updated.(model)
	transcriptPayload := runBatchSendCommand(t, cmd, stdin, "session.messages")
	assertPayload(t, transcriptPayload, "session.messages", "sess_old")

	loaded := []chatMessage{{Role: "user", Content: "old question"}, {Role: "assistant", Content: "old answer"}}
	got.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_old", SessionTitle: "Older session", Messages: loaded})

	if got.busy {
		t.Fatal("busy = true, want false after transcript load")
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}
	if len(got.messages) != len(loaded) {
		t.Fatalf("message count = %d, want %d", len(got.messages), len(loaded))
	}
	if got.messages[0] != loaded[0] || got.messages[1] != loaded[1] {
		t.Fatalf("messages = %#v, want loaded transcript %#v", got.messages, loaded)
	}
	viewport := got.viewport.View()
	if !strings.Contains(plainText(viewport), "old question") || !strings.Contains(plainText(viewport), "old answer") {
		t.Fatalf("viewport = %q, want loaded transcript content", viewport)
	}
	if strings.Contains(plainText(viewport), "current conversation should disappear") {
		t.Fatalf("viewport still contains previous conversation after load: %q", viewport)
	}
}

func TestReadyRequestsSessionList(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "ready", SessionID: "sess_1", SessionTitle: "First", Provider: "stub", Model: "test-model"})
	got := updated.(model)

	if !got.ready {
		t.Fatal("ready = false, want true")
	}
	if got.session != "sess_1" {
		t.Fatalf("session = %q, want sess_1", got.session)
	}
	if got.title != "First" {
		t.Fatalf("title = %q, want First", got.title)
	}
	if !got.autoNewSessionOnChat {
		t.Fatal("autoNewSessionOnChat = false, want true after fresh ready event")
	}
	payload := runBatchSendCommand(t, cmd, stdin, "session.list")
	assertPayload(t, payload, "session.list", "")
}

func TestFirstChatAfterReadyRequestsNewSession(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_last"
	m.title = "Last chat"
	m.autoNewSessionOnChat = true
	m.composer.SetValue("hello new work")

	updated, cmd := m.submitInput()
	got := updated.(model)

	if got.autoNewSessionOnChat {
		t.Fatal("autoNewSessionOnChat = true after submit, want consumed")
	}
	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	assertPayload(t, payload, "chat", "")
	if payload["sessionId"] != nil {
		t.Fatalf("sessionId = %#v, want omitted for auto-new first chat", payload["sessionId"])
	}
	if payload["newSession"] != true {
		t.Fatalf("newSession = %#v, want true", payload["newSession"])
	}
	if payload["title"] != "hello new work" {
		t.Fatalf("title = %#v, want derived title", payload["title"])
	}
	runID, ok := payload["runId"].(string)
	if !ok || !strings.HasPrefix(runID, "run_") {
		t.Fatalf("runId = %#v, want generated run_ id", payload["runId"])
	}
	if got.activeRunID != runID {
		t.Fatalf("activeRunID = %q, want payload runId %q", got.activeRunID, runID)
	}
}

func TestRunLifecycleTracksActiveRunID(t *testing.T) {
	m := model{busy: true, activeRunID: "run_local"}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_runtime", SessionID: "sess_1"})
	got := updated.(model)
	if got.activeRunID != "run_runtime" {
		t.Fatalf("activeRunID after run_started = %q, want run_runtime", got.activeRunID)
	}

	updated, _ = got.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_runtime", Status: "completed"})
	got = updated.(model)
	if got.activeRunID != "" {
		t.Fatalf("activeRunID after run_finished = %q, want cleared", got.activeRunID)
	}
	if got.busy {
		t.Fatal("busy = true, want false after run_finished")
	}
}

func TestSelectedSessionChatUsesExistingSession(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_selected"
	m.autoNewSessionOnChat = false
	m.composer.SetValue("continue here")

	_, cmd := m.submitInput()
	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	assertPayload(t, payload, "chat", "sess_selected")
	if payload["newSession"] != nil {
		t.Fatalf("newSession = %#v, want omitted for explicit session chat", payload["newSession"])
	}
}

func TestStaleStartupSessionListDoesNotOverrideNewRunSession(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.busy = true
	m.session = "sess_last"
	m.lastRunStartedSession = "sess_new"

	m.applySessionList(runtimeEvent{
		Type:         "session.list",
		SessionID:    "sess_last",
		SessionTitle: "Last chat",
		Sessions: []sessionInfo{
			{ID: "sess_last", Title: "Last chat"},
			{ID: "sess_new", Title: "New chat"},
		},
	})

	if m.session != "sess_last" {
		t.Fatalf("precondition changed unexpectedly, session = %q", m.session)
	}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", SessionID: "sess_new"})
	got := updated.(model)
	got.applySessionList(runtimeEvent{
		Type:         "session.list",
		SessionID:    "sess_last",
		SessionTitle: "Last chat",
		Sessions: []sessionInfo{
			{ID: "sess_last", Title: "Last chat"},
			{ID: "sess_new", Title: "New chat"},
		},
	})

	if got.session != "sess_new" {
		t.Fatalf("session = %q, want stale session.list not to override active new run session", got.session)
	}
}

func newTestRuntime(t *testing.T) (*runtimeClient, *recordingWriteCloser) {
	t.Helper()
	stdin := &recordingWriteCloser{}
	return &runtimeClient{stdin: stdin, events: make(chan runtimeEvent, 8), errs: make(chan error, 1)}, stdin
}

type recordingWriteCloser struct {
	strings.Builder
}

func (w *recordingWriteCloser) Close() error { return nil }

func (w *recordingWriteCloser) lastPayload(t *testing.T) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(w.String()), "\n")
	if len(lines) == 0 || lines[len(lines)-1] == "" {
		t.Fatal("runtime stdin received no payloads")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &payload); err != nil {
		t.Fatalf("decode runtime payload %q: %v", lines[len(lines)-1], err)
	}
	return payload
}

func runSendCommand(t *testing.T, cmd tea.Cmd, stdin *recordingWriteCloser) map[string]any {
	t.Helper()
	if cmd == nil {
		t.Fatal("command is nil")
	}
	msg := cmd()
	assertSendDone(t, msg)
	return stdin.lastPayload(t)
}

func runBatchSendCommand(t *testing.T, cmd tea.Cmd, stdin *recordingWriteCloser, wantType string) map[string]any {
	t.Helper()
	if cmd == nil {
		t.Fatal("command is nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		assertSendDone(t, msg)
		payload := stdin.lastPayload(t)
		if payload["type"] != wantType {
			t.Fatalf("payload type = %#v, want %q; payload=%#v", payload["type"], wantType, payload)
		}
		return payload
	}

	results := make(chan tea.Msg, len(batch))
	for _, child := range batch {
		if child == nil {
			continue
		}
		go func(child tea.Cmd) {
			results <- child()
		}(child)
	}

	deadline := time.After(250 * time.Millisecond)
	for range batch {
		select {
		case childMsg := <-results:
			if _, ok := childMsg.(sendDoneMsg); !ok {
				continue
			}
			assertSendDone(t, childMsg)
			payload := stdin.lastPayload(t)
			if payload["type"] == wantType {
				return payload
			}
		case <-deadline:
			t.Fatalf("timed out waiting for runtime payload type %q in batch", wantType)
		}
	}
	t.Fatalf("batch did not include runtime payload type %q", wantType)
	return nil
}

func assertSendDone(t *testing.T, msg tea.Msg) {
	t.Helper()
	done, ok := msg.(sendDoneMsg)
	if !ok {
		t.Fatalf("message = %T, want sendDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("sendDoneMsg error = %v", done.err)
	}
}

func plainText(s string) string {
	replacer := strings.NewReplacer(
		"\x1b[0m", "",
		"\x1b[m", "",
		"\x1b[1;38;2;242;166;90m", "",
		"\x1b[1;38;2;139;211;221m", "",
		"\x1b[1;38;2;255;107;107m", "",
		"\x1b[38;5;252m", "",
	)
	return replacer.Replace(s)
}

func assertPayload(t *testing.T, payload map[string]any, wantType string, wantSessionID string) {
	t.Helper()
	if payload["type"] != wantType {
		t.Fatalf("payload type = %#v, want %q; payload=%#v", payload["type"], wantType, payload)
	}
	if wantSessionID != "" && payload["sessionId"] != wantSessionID {
		t.Fatalf("payload sessionId = %#v, want %q; payload=%#v", payload["sessionId"], wantSessionID, payload)
	}
	if payload["id"] == "" {
		t.Fatalf("payload missing generated id: %#v", payload)
	}
}
