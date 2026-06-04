package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

func TestInputSpinnerActiveDuringRunningStreamAfterForkTree(t *testing.T) {
	m := initialModel(nil)
	m.busy = true
	m.activeRunID = "run_1"
	m.mode = modeForkTree
	m.status = "fork tree"

	updated, cmd := m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)
	if cmd != nil {
		t.Fatal("esc returned command, want nil")
	}
	if got.mode != modeChat {
		t.Fatalf("mode = %v, want modeChat", got.mode)
	}
	if !got.inputSpinnerActive() {
		t.Fatal("inputSpinnerActive = false, want true while active run continues after returning from tree")
	}
	if prompt := plainText(got.inputPrompt()); prompt == plainText(idleInputPrompt()) {
		t.Fatalf("input prompt = %q, want streaming spinner prompt", prompt)
	}
}

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
	if !strings.Contains(viewport, "›1 hello") || !strings.Contains(viewport, "◆ hi") {
		t.Fatalf("viewport = %q, want numbered user and compact assistant icons", viewport)
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

func TestEscapeInIdleChatDoesNotQuit(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.status = "ready"

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("idle escape returned command, want nil")
	}
	if got.mode != modeChat {
		t.Fatalf("mode = %v, want chat", got.mode)
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
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
	rt.qubitDir = appRoot
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

func TestInputHistoryFiltersSlashCommandInputs(t *testing.T) {
	got := sanitizeInputHistory([]string{"first", "/help", "  /rename title  ", "second"})
	want := []string{"first", "second"}
	if len(got) != len(want) {
		t.Fatalf("sanitizeInputHistory = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sanitizeInputHistory = %#v, want %#v", got, want)
		}
	}
}

func TestSubmitSlashCommandDoesNotPersistInputHistory(t *testing.T) {
	appRoot := t.TempDir()
	rt, _ := newTestRuntime(t)
	rt.qubitDir = appRoot
	m := initialModel(rt)
	m.ready = true
	m.composer.SetValue("/help")

	updated, cmd := m.submitInput()
	got := updated.(model)
	if cmd != nil {
		t.Fatalf("submit /help command = %#v, want nil", cmd)
	}
	if len(got.inputHistory) != 0 {
		t.Fatalf("inputHistory = %#v, want empty", got.inputHistory)
	}
	loaded, err := loadInputHistory(appRoot)
	if err != nil {
		t.Fatalf("load input history: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("loaded input history = %#v, want empty", loaded)
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

	updated := m.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.viewport.YOffset() == 0 {
		t.Fatal("viewport did not move down on mouse wheel down")
	}
	if updated.autoScroll != updated.viewport.AtBottom() {
		t.Fatalf("autoScroll = %v, want AtBottom %v", updated.autoScroll, updated.viewport.AtBottom())
	}

	updated = updated.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelUp}).(model)
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

	if got.composer.Cursor() != len([]rune("hello")) {
		t.Fatalf("composer cursor = %d, want end", got.composer.Cursor())
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

func TestCtrlXCutsSelectedInput(t *testing.T) {
	m := initialModel(nil)
	m.composer.SetValue("hello")
	m.composer.SelectAll()

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	got := updated.(model)

	if cmd == nil {
		t.Fatal("ctrl+x returned nil command, want clipboard copy command")
	}
	if got.composer.HasSelection() {
		t.Fatal("selection = true, want false after cut")
	}
	if got.composer.Value() != "" {
		t.Fatalf("input value = %q, want empty after cut", got.composer.Value())
	}
	if got.status != "cut input" {
		t.Fatalf("status = %q, want cut input", got.status)
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

func TestForkCommandWithTitleRequestsForkAndLoadsForkTranscript(t *testing.T) {
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

func TestForkCommandEnterStartsInlineSelectorAndEnterForksHere(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_parent"
	m.autoNewSessionOnChat = false
	m.messages = []chatMessage{{Role: "user", Content: "question"}, {Role: "assistant", Content: "answer"}}
	m.composer.SetValue("/fork")

	updated, cmd := m.submitInput()
	got := updated.(model)
	if cmd != nil {
		t.Fatalf("submit /fork command = %T, want nil while selector is active", cmd)
	}
	if !got.forkSelector.Active {
		t.Fatal("fork selector inactive, want active")
	}
	if got.status != "fork point" {
		t.Fatalf("status = %q, want fork point", got.status)
	}
	if !strings.Contains(plainText(got.renderInputStatus()), "enter forks here") || !strings.Contains(plainText(got.renderInputStatus()), "plan") {
		t.Fatalf("input status = %q, want fork selector hint and mode", plainText(got.renderInputStatus()))
	}

	updated, cmd = got.updateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got = updated.(model)
	if !got.busy {
		t.Fatal("busy = false, want true while forking session")
	}
	if got.forkSelector.Active {
		t.Fatal("fork selector still active after fork request")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.fork", "sess_parent")
	if payload["messageIndex"] != float64(2) {
		t.Fatalf("session.fork messageIndex = %#v, want 2", payload["messageIndex"])
	}
}

func TestForkSelectorUpDownSelectsUserMessagesAndEnterStartsEdit(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.session = "sess_parent"
	m.inputHistory = []string{"history should not appear"}
	m.inputHistoryIndex = len(m.inputHistory)
	m.messages = []chatMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
	}
	m.layout()
	m.composer.SetValue("/fork")

	updated, _ := m.submitInput()
	got := updated.(model)

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = updated.(model)
	if got.composer.Value() != "/fork 3 second question" {
		t.Fatalf("composer after first up = %q, want /fork 3 second question", got.composer.Value())
	}

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = updated.(model)
	if got.composer.Value() != "/fork 1 first question" {
		t.Fatalf("composer after second up = %q, want /fork 1 first question", got.composer.Value())
	}

	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got = updated.(model)
	if got.composer.Value() != "/fork 3 second question" {
		t.Fatalf("composer after down = %q, want /fork 3 second question", got.composer.Value())
	}

	updated, cmd := got.updateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got = updated.(model)
	if cmd != nil {
		t.Fatalf("enter on selected fork point command = %T, want nil while editing", cmd)
	}
	if got.forkSelector.Active {
		t.Fatal("fork selector active after entering edit mode")
	}
	if !got.messageEdit.Active {
		t.Fatal("message edit inactive, want active")
	}
	if got.messageEdit.MessageIndex != 2 {
		t.Fatalf("edit message index = %d, want 2", got.messageEdit.MessageIndex)
	}
	if got.composer.Value() != "second question" {
		t.Fatalf("composer in edit mode = %q, want selected message", got.composer.Value())
	}
	if !strings.Contains(plainText(got.renderInputStatus()), "editing message") || !strings.Contains(plainText(got.renderInputStatus()), "plan") {
		t.Fatalf("input status = %q, want edit hint and mode", plainText(got.renderInputStatus()))
	}
}

func TestSubmitEditedMessagePreviewsTruncatedForkAndRequestsForkedReroll(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.width = 80
	m.height = 24
	m.session = "sess_parent"
	m.cwdBlockEnabled = false
	m.messages = []chatMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
	}
	m.messageEdit = messageEditState{Active: true, MessageIndex: 2, Original: "second question"}
	m.composer.SetValue("edited second question")
	m.layout()

	updated, cmd := m.submitInput()
	got := updated.(model)
	if !got.busy {
		t.Fatal("busy = false, want true while rerolling edited message")
	}
	if got.messageEdit.Active {
		t.Fatal("message edit still active after submit")
	}
	if len(got.messages) != 3 {
		t.Fatalf("message count = %d, want 3 after truncating from selected point", len(got.messages))
	}
	if got.messages[2].Role != "user" || got.messages[2].Content != "edited second question" {
		t.Fatalf("last message = %#v, want edited user message", got.messages[2])
	}

	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	assertPayload(t, payload, "chat", "sess_parent")
	if payload["input"] != "edited second question" {
		t.Fatalf("chat input = %#v, want edited second question", payload["input"])
	}
	if payload["replaceFromMessageIndex"] != float64(2) {
		t.Fatalf("replaceFromMessageIndex = %#v, want 2", payload["replaceFromMessageIndex"])
	}
	if payload["title"] != "Edit: " {
		t.Fatalf("title = %#v, want Edit: ", payload["title"])
	}
	if _, ok := payload["newSession"]; ok {
		t.Fatalf("newSession present in edit reroll payload: %#v", payload)
	}
	if payload["cwdBlockEnabled"] != false {
		t.Fatalf("cwdBlockEnabled = %#v, want false; payload=%#v", payload["cwdBlockEnabled"], payload)
	}
}

func TestForkSelectorNoUserMessagesStillForksHere(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_parent"
	m.messages = []chatMessage{{Role: "assistant", Content: "hello"}}
	m.composer.SetValue("/fork")

	updated, _ := m.submitInput()
	got := updated.(model)
	updated, _ = got.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = updated.(model)
	if got.composer.Value() != "/fork" {
		t.Fatalf("composer after up with no user messages = %q, want /fork", got.composer.Value())
	}

	updated, cmd := got.updateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got = updated.(model)
	if !got.busy {
		t.Fatal("busy = false, want true while forking here")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.fork", "sess_parent")
	if payload["messageIndex"] != float64(1) {
		t.Fatalf("session.fork messageIndex = %#v, want 1", payload["messageIndex"])
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
	if got.title != "" {
		t.Fatalf("title = %q, want empty title before first chat", got.title)
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
	m := model{busy: true, activeRunID: "run_runtime"}

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

func TestForeignRunEventsDoNotMirrorIntoIdleSession(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.session = "sess_t2"
	m.title = "Terminal 2"
	m.messages = []chatMessage{{Role: "assistant", Content: "keep t2"}}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_t1", SessionID: "sess_t1"})
	got := updated.(model)
	updated, _ = got.updateRuntime(runtimeEvent{Type: "assistant", RunID: "run_t1", SessionID: "sess_t1", Content: "t1 answer"})
	got = updated.(model)
	updated, _ = got.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_t1", SessionID: "sess_t1", Status: "completed"})
	got = updated.(model)

	if got.session != "sess_t2" {
		t.Fatalf("session = %q, want sess_t2", got.session)
	}
	if len(got.messages) != 1 || got.messages[0].Content != "keep t2" {
		t.Fatalf("messages = %#v, want original T2 transcript", got.messages)
	}
	if got.activeRunID != "" || got.busy || got.streaming {
		t.Fatalf("run state busy=%v streaming=%v activeRunID=%q, want idle", got.busy, got.streaming, got.activeRunID)
	}
}

func TestForeignRunEventsDoNotInterruptLocalRun(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.busy = true
	m.activeRunID = "run_t2"
	m.session = "sess_t2"
	m.messages = []chatMessage{{Role: "user", Content: "local"}}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "assistant", RunID: "run_t1", SessionID: "sess_t1", Content: "foreign"})
	got := updated.(model)
	updated, _ = got.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_t1", SessionID: "sess_t1", Status: "completed"})
	got = updated.(model)

	if got.session != "sess_t2" || got.activeRunID != "run_t2" || !got.busy {
		t.Fatalf("state session=%q activeRunID=%q busy=%v, want local run unchanged", got.session, got.activeRunID, got.busy)
	}
	if len(got.messages) != 1 || got.messages[0].Content != "local" {
		t.Fatalf("messages = %#v, want unchanged local transcript", got.messages)
	}
}

func TestSessionListDoesNotSwitchVisibleSession(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.session = "sess_t2"
	m.title = "Terminal 2"
	m.autoNewSessionOnChat = false

	m.applySessionList(runtimeEvent{Type: "session.list", SessionID: "sess_t1", SessionTitle: "Terminal 1", Sessions: []sessionInfo{
		{ID: "sess_t1", Title: "Terminal 1"},
		{ID: "sess_t2", Title: "Terminal 2"},
	}})

	if m.session != "sess_t2" {
		t.Fatalf("session = %q, want sess_t2", m.session)
	}
	if m.title != "Terminal 2" {
		t.Fatalf("title = %q, want Terminal 2", m.title)
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
	return stripANSI(s)
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

func TestRuntimeForkedEditRerollEventFlowActivatesFork(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.busy = true
	m.session = "sess_parent"
	m.title = "Parent"
	m.activeRunID = "run_edit"
	m.messages = []chatMessage{
		{Role: "user", Content: "first question"},
		{Role: "user", Content: "edited second question"},
	}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_edit", SessionID: "sess_fork"})
	got := updated.(model)
	if got.session != "sess_fork" {
		t.Fatalf("session after run_started = %q, want sess_fork", got.session)
	}

	updated, _ = got.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_edit", SessionID: "sess_fork", Status: "ready"})
	got = updated.(model)
	if got.session != "sess_fork" {
		t.Fatalf("session after run_finished = %q, want sess_fork", got.session)
	}
	if got.busy {
		t.Fatal("busy = true, want false after forked reroll finish")
	}
}

func TestActivatingEditedForkFromTreeLoadsOnlyForkLineageMessages(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.width = 100
	m.height = 30
	m.session = "sess_parent"
	m.title = "Parent"
	m.mode = modeForkTree
	m.messages = []chatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "Hi! What can I help you with?"},
		{Role: "user", Content: "how are you"},
		{Role: "assistant", Content: "I'm doing great, thanks for asking!"},
	}
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_parent", SessionID: "sess_parent", SessionTitle: "Parent", MessageRole: "user", MessageContent: "how are you", MessageCount: 4},
		{ID: "sess_edit", SessionID: "sess_edit", SessionTitle: "Edit: Parent", ParentSessionID: "sess_parent", ForkedFromMessageIndex: 2, MessageRole: "user", MessageContent: "how are you?", MessageCount: 4},
	}})
	for i, node := range m.forkTree.Nodes {
		if node.SessionID == "sess_edit" {
			m.forkTree.Selected = i
			break
		}
	}

	updated, cmd := m.activateSelectedForkTreeSession()
	got := updated.(model)
	if got.mode != modeChat {
		t.Fatalf("mode = %v, want chat", got.mode)
	}
	if got.session != "sess_edit" {
		t.Fatalf("session = %q, want sess_edit", got.session)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.activate", "sess_edit")

	updated, cmd = got.updateRuntime(runtimeEvent{Type: "session.activated", SessionID: "sess_edit", SessionTitle: "Edit: Parent"})
	got = updated.(model)
	transcriptPayload := runBatchSendCommand(t, cmd, stdin, "session.messages")
	assertPayload(t, transcriptPayload, "session.messages", "sess_edit")

	loadedForkLineage := []chatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "Hi! What can I help you with?"},
		{Role: "user", Content: "how are you?"},
		{Role: "assistant", Content: "I'm good!"},
	}
	updated, _ = got.updateRuntime(runtimeEvent{Type: "session.messages", SessionID: "sess_edit", SessionTitle: "Edit: Parent", Messages: loadedForkLineage})
	got = updated.(model)

	if len(got.messages) != len(loadedForkLineage) {
		t.Fatalf("loaded message count = %d, want %d", len(got.messages), len(loadedForkLineage))
	}
	for i, want := range loadedForkLineage {
		if got.messages[i] != want {
			t.Fatalf("message[%d] = %#v, want %#v", i, got.messages[i], want)
		}
	}
	for _, message := range got.messages {
		if message.Content == "how are you" || strings.Contains(message.Content, "doing great") {
			t.Fatalf("loaded edited fork merged original branch message: %#v in %#v", message, got.messages)
		}
	}
	viewport := plainText(got.viewport.View())
	if strings.Contains(viewport, "how are you\n") || strings.Contains(viewport, "doing great") {
		t.Fatalf("viewport merged original branch content: %q", viewport)
	}
	if !strings.Contains(viewport, "how are you?") || !strings.Contains(viewport, "I'm good!") {
		t.Fatalf("viewport = %q, want edited fork lineage", viewport)
	}
}

func TestSubmitInputIncludesSystemPromptMode(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.autoNewSessionOnChat = false
	m.session = "sess_1"
	m.permissionMode = permissionModeAlwaysAllow
	m.cwdBlockEnabled = false
	m.composer.SetValue("change files")

	_, cmd := m.submitInput()
	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	if payload["systemPromptMode"] != "edit" {
		t.Fatalf("systemPromptMode = %#v, want edit; payload=%#v", payload["systemPromptMode"], payload)
	}
	if payload["cwdBlockEnabled"] != false {
		t.Fatalf("cwdBlockEnabled = %#v, want false; payload=%#v", payload["cwdBlockEnabled"], payload)
	}
}

func TestFilteredSlashCommandsPrioritizesNameMatches(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.composer.SetValue("/provider")

	matches := m.filteredSlashCommands()
	if len(matches) < 2 {
		t.Fatalf("matches = %#v, want at least name and description matches", matches)
	}
	if matches[0].Name != "providers" {
		t.Fatalf("first match = %q, want providers; matches=%#v", matches[0].Name, matches)
	}
	seenDescriptionOnly := false
	for _, command := range matches {
		if command.Name == "models" {
			seenDescriptionOnly = true
			continue
		}
		if seenDescriptionOnly && strings.Contains(command.Name, "provider") {
			t.Fatalf("name match %q appeared after description-only match; matches=%#v", command.Name, matches)
		}
	}
}

func TestStreamingLifecycleAfterToolCalls(t *testing.T) {
	// Simulate the full event sequence: run_started → tool.call.start →
	// tool.call.finish → assistant → run_finished, verifying that streaming
	// starts correctly after the assistant event and that run_finished properly
	// completes the lifecycle, including draining remaining content.
	m := initialModel(nil)
	m.session = "sess_1"
	m.activeRunID = "run_tool"
	m.width = 100
	m.height = 30
	m.layout()

	// 1. run_started
	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_tool", SessionID: "sess_1"})
	m = updated.(model)
	if !m.busy {
		t.Fatal("busy = false after run_started, want true")
	}
	if m.activeRunID != "run_tool" {
		t.Fatalf("activeRunID = %q, want run_tool", m.activeRunID)
	}
	if m.status != "thinking" {
		t.Fatalf("status = %q, want thinking after run_started", m.status)
	}

	// 2. tool.call.start (editFile)
	m.applyToolCallStart(runtimeEvent{
		Type:       "tool.call.start",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "edit_1",
		ToolName:   "editFile",
		Status:     "running",
		Args:       map[string]any{"path": "test.txt", "operation": "replace"},
	})
	if m.status != "using tools" {
		t.Fatalf("status = %q after tool.call.start, want 'using tools'", m.status)
	}

	// 3. tool.call.finish (editFile)
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "edit_1",
		ToolName:   "editFile",
		Status:     "completed",
		Result:     map[string]any{"success": true, "replacements": float64(1)},
	})
	if m.status != "thinking" {
		t.Fatalf("status = %q after tool.call.finish, want 'thinking'", m.status)
	}

	// 4. assistant event — should start streaming
	updated, cmd := m.updateRuntime(runtimeEvent{
		Type:      "assistant",
		SessionID: "sess_1",
		RunID:     "run_tool",
		Content:   "I've edited the file for you.",
	})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("assistant event returned nil command, want fake stream tick")
	}
	if !m.streaming {
		t.Fatal("streaming = false after assistant event, want true")
	}
	if m.streamingFullContent != "I've edited the file for you." {
		t.Fatalf("streamingFullContent = %q, want assistant content", m.streamingFullContent)
	}
	if m.streamingFinished {
		t.Fatal("streamingFinished = true before run_finished, want false")
	}

	// 5. run_finished arrives while streaming — should set streamingFinished
	updated, _ = m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_tool", Status: "completed"})
	m = updated.(model)
	if !m.streaming {
		t.Fatal("streaming = false after run_finished during streaming, want true")
	}
	if !m.streamingFinished {
		t.Fatal("streamingFinished = false after run_finished, want true")
	}
	if !m.busy {
		t.Fatal("busy = false while streaming not yet drained, want true")
	}
	if m.streamingFinishStatus != "completed" {
		t.Fatalf("streamingFinishStatus = %q, want completed", m.streamingFinishStatus)
	}

	// 6. Drain fake stream ticks until streaming completes
	for i := 0; m.streaming && i < 100; i++ {
		updated, _ = m.updateFakeStreamTick()
		m = updated.(model)
	}
	if m.streaming {
		t.Fatal("streaming = true after draining all ticks, want false")
	}
	if m.busy {
		t.Fatal("busy = true after stream drained and run_finished, want false")
	}
	if m.activeRunID != "" {
		t.Fatalf("activeRunID = %q after completion, want empty", m.activeRunID)
	}
	if m.status != "completed" {
		t.Fatalf("status = %q after completion, want completed", m.status)
	}
	// Verify the assistant content was fully rendered
	foundAssistant := false
	for _, msg := range m.messages {
		if msg.Role == "assistant" && msg.Content == "I've edited the file for you." {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Fatalf("assistant message not found in messages after stream completion; messages=%#v", m.messages)
	}
}

func TestRunFinishedWithoutAssistantAfterToolCall(t *testing.T) {
	// Test the edge case: after tool.call.finish, if run_finished arrives
	// before any assistant event (no assistant response after tool use),
	// the model should transition cleanly to not-busy.
	m := initialModel(nil)
	m.session = "sess_1"
	m.activeRunID = "run_silent"
	m.width = 100
	m.height = 30
	m.layout()

	// 1. run_started
	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_silent", SessionID: "sess_1"})
	m = updated.(model)
	if !m.busy {
		t.Fatal("busy = false after run_started, want true")
	}

	// 2. tool.call.start
	m.applyToolCallStart(runtimeEvent{
		Type: "tool.call.start", SessionID: "sess_1", Step: 1,
		ToolCallID: "call_1", ToolName: "readFile", Status: "running",
	})

	// 3. tool.call.finish
	m.applyToolCallFinish(runtimeEvent{
		Type: "tool.call.finish", SessionID: "sess_1", Step: 1,
		ToolCallID: "call_1", ToolName: "readFile", Status: "completed",
	})

	// 4. run_finished without any intervening assistant event
	// This is the "edit file breaks streaming" scenario — the model
	// doesn't produce an assistant response after the tool call.
	updated, _ = m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_silent", Status: "completed"})
	m = updated.(model)

	// Since there's no streaming active, run_finished should directly clear busy
	if m.busy {
		t.Fatal("busy = true after run_finished with no streaming, want false")
	}
	if m.activeRunID != "" {
		t.Fatalf("activeRunID = %q after run_finished, want empty", m.activeRunID)
	}
	if m.status != "completed" {
		t.Fatalf("status = %q after run_finished, want completed", m.status)
	}
	// Streaming should not be stuck on
	if m.streaming {
		t.Fatal("streaming = true after run_finished with no assistant event, want false")
	}
}

func TestMultipleToolCallsBeforeAssistant(t *testing.T) {
	// Verify that multiple tool calls (e.g. editFile then readFile) followed
	// by an assistant event produce correct streaming behavior.
	m := initialModel(nil)
	m.session = "sess_1"
	m.activeRunID = "run_multi"
	m.width = 100
	m.height = 30
	m.layout()

	// run_started
	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_started", RunID: "run_multi", SessionID: "sess_1"})
	m = updated.(model)

	// First tool: editFile
	m.applyToolCallStart(runtimeEvent{
		Type: "tool.call.start", SessionID: "sess_1", Step: 1,
		ToolCallID: "edit_1", ToolName: "editFile", Status: "running",
	})
	m.applyToolCallFinish(runtimeEvent{
		Type: "tool.call.finish", SessionID: "sess_1", Step: 1,
		ToolCallID: "edit_1", ToolName: "editFile", Status: "completed",
	})

	// Second tool: readFile (same step — grouped)
	m.applyToolCallStart(runtimeEvent{
		Type: "tool.call.start", SessionID: "sess_1", Step: 1,
		ToolCallID: "read_1", ToolName: "readFile", Status: "running",
	})
	m.applyToolCallFinish(runtimeEvent{
		Type: "tool.call.finish", SessionID: "sess_1", Step: 1,
		ToolCallID: "read_1", ToolName: "readFile", Status: "completed",
	})

	// assistant event
	updated, cmd := m.updateRuntime(runtimeEvent{
		Type: "assistant", SessionID: "sess_1", RunID: "run_multi",
		Content: "Done with both tools.",
	})
	m = updated.(model)
	if !m.streaming {
		t.Fatal("streaming = false after assistant event, want true")
	}
	if cmd == nil {
		t.Fatal("assistant event returned nil command, want fake stream tick")
	}

	// run_finished while streaming
	updated, _ = m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_multi", Status: "completed"})
	m = updated.(model)
	if !m.streamingFinished {
		t.Fatal("streamingFinished = false after run_finished, want true")
	}

	// Drain fake stream ticks until streaming completes
	for i := 0; m.streaming && i < 100; i++ {
		updated, _ = m.updateFakeStreamTick()
		m = updated.(model)
	}
	if m.streaming {
		t.Fatal("streaming = true after draining all ticks, want false")
	}
	if m.busy {
		t.Fatal("busy = true after drain, want false")
	}
}

func TestRenderSlashCommandsUsesModalListWindow(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 12
	m.ready = true
	m.composer.SetValue("/")
	m.slashCursor = len(slashCommands) - 1

	rendered := plainText(m.renderSlashCommandModal(8))
	last := slashCommands[len(slashCommands)-1]
	if !strings.Contains(rendered, last.Usage) {
		t.Fatalf("rendered slash modal missing selected command %q:\n%s", last.Usage, rendered)
	}
	if strings.Contains(rendered, slashCommands[0].Usage) {
		t.Fatalf("rendered slash modal includes hidden first command:\n%s", rendered)
	}
	if !strings.Contains(rendered, "more above") {
		t.Fatalf("rendered slash modal missing list-window hint:\n%s", rendered)
	}
}

func TestSessionListKeepsTitleEmptyBeforeFirstChat(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.session = "sess_last"
	m.autoNewSessionOnChat = true

	m.applySessionList(runtimeEvent{
		Type:         "session.list",
		SessionID:    "sess_last",
		SessionTitle: "Last chat",
		Sessions:     []sessionInfo{{ID: "sess_last", Title: "Last chat"}},
	})

	if m.title != "" {
		t.Fatalf("title = %q, want empty title before first chat", m.title)
	}
}

func TestSessionPickerHidesForks(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.session = "sess_root_2"
	m.sessions = []sessionInfo{
		{ID: "sess_root_1", Title: "Root 1"},
		{ID: "sess_fork", Title: "Fork", ForkedFromSessionID: "sess_root_1"},
		{ID: "sess_root_2", Title: "Root 2"},
	}

	m.ensureSessionCursor()
	visible := m.sessionPickerSessions()
	if len(visible) != 2 {
		t.Fatalf("visible sessions = %d, want 2", len(visible))
	}
	for _, session := range visible {
		if session.ID == "sess_fork" {
			t.Fatalf("fork session should be hidden from picker: %#v", visible)
		}
	}
	if m.sessionCursor != 1 {
		t.Fatalf("sessionCursor = %d, want active root session index 1", m.sessionCursor)
	}

	rendered := m.renderSessionPicker(20)
	if strings.Contains(rendered, "Fork") || strings.Contains(rendered, "sess_fork") || strings.Contains(rendered, "↳") {
		t.Fatalf("rendered session picker includes fork:\n%s", rendered)
	}
}

func TestSessionPickerUsesAvailableTerminalWidth(t *testing.T) {
	m := initialModel(nil)
	m.width = 160
	m.session = "sess_long"
	title := "A very long planning session title that should use the available terminal width"
	m.sessions = []sessionInfo{{ID: "sess_long", Title: title, MessageCount: 12}}

	rendered := plainText(m.renderSessionPicker(20))

	if !strings.Contains(rendered, "available terminal width") {
		t.Fatalf("session picker did not use wide terminal title space:\n%s", rendered)
	}
	if strings.Contains(rendered, "A very long planning sessio…") {
		t.Fatalf("session picker appears truncated at old fixed width:\n%s", rendered)
	}
}

func TestSessionPickerRowTruncatesForNarrowWidth(t *testing.T) {
	row := renderSessionPickerRow(sessionInfo{ID: "sess", Title: "A very long session title", CreatedAt: "2026-06-02T13:45:00Z", MessageCount: 123}, false, 24, 20, 2)

	if !strings.Contains(row, "…") {
		t.Fatalf("row = %q, want truncated title or stats", row)
	}
	if got := len([]rune(row)); got > 24 {
		t.Fatalf("row width = %d, want <= 24; row=%q", got, row)
	}
}

func TestSessionPickerShowsMessageAndForkStats(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.session = "sess_root"
	m.sessions = []sessionInfo{
		{ID: "sess_root", Title: "Root session", CreatedAt: "2026-06-02T13:45:00Z", MessageCount: 12345},
		{ID: "sess_fork", Title: "Fork", ForkedFromSessionID: "sess_root"},
		{ID: "sess_grandfork", Title: "Grandfork", ForkedFromSessionID: "sess_fork"},
	}

	rendered := plainText(m.renderSessionPicker(20))

	if !regexp.MustCompile(`02-06 [0-9]{2}:45`).MatchString(rendered) || !strings.Contains(rendered, "12345 msgs · 2 forks") {
		t.Fatalf("rendered session picker missing date/message/fork stats:\n%s", rendered)
	}

	for _, line := range strings.Split(rendered, "\n") {
		line = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(line, "")
		if strings.Contains(line, "Root session") && strings.Contains(line, "msgs") && strings.Contains(line, "forks") && len([]rune(line)) > m.width {
			t.Fatalf("session row width = %d, want <= %d; row=%q", len([]rune(line)), m.width, line)
		}
	}
}

func TestSessionPickerActivateUsesVisibleNonForkSession(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.mode = modeSessionPicker
	m.sessions = []sessionInfo{
		{ID: "sess_root_1", Title: "Root 1"},
		{ID: "sess_fork", Title: "Fork", ForkedFromSessionID: "sess_root_1"},
		{ID: "sess_root_2", Title: "Root 2"},
	}
	m.sessionCursor = 1

	updated, cmd := m.updateSessionPicker(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)

	if got.session != "sess_root_2" {
		t.Fatalf("session = %q, want second visible root session", got.session)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.activate", "sess_root_2")
}

type recordingNotifier struct {
	payloads []notificationPayload
}

func (n *recordingNotifier) Notify(payload notificationPayload) error {
	n.payloads = append(n.payloads, payload)
	return nil
}

func runNotificationCommand(t *testing.T, cmd tea.Cmd) notificationResultMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("notification command is nil")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
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
				if result, ok := childMsg.(notificationResultMsg); ok {
					return result
				}
			case <-deadline:
				t.Fatal("timed out waiting for notification command")
			}
		}
		t.Fatal("batch did not include notification command")
	}
	result, ok := msg.(notificationResultMsg)
	if !ok {
		t.Fatalf("message = %#v, want notificationResultMsg", msg)
	}
	return result
}

func TestFakeStreamCompletionNotifiesAfterRunFinished(t *testing.T) {
	n := &recordingNotifier{}
	m := model{
		notifier:              n,
		busy:                  true,
		activeRunID:           "run_1",
		title:                 "Demo chat",
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "ok",
		streamingFinished:     true,
		streamingFinishStatus: "completed",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, cmd := m.updateFakeStreamTick()
	got := updated.(model)
	runNotificationCommand(t, cmd)

	if got.streaming || got.busy || got.activeRunID != "" {
		t.Fatalf("run state = streaming:%v busy:%v activeRunID:%q, want complete", got.streaming, got.busy, got.activeRunID)
	}
	if len(n.payloads) != 1 {
		t.Fatalf("notifications = %#v, want one", n.payloads)
	}
	if n.payloads[0].Kind != notificationKindRunComplete || n.payloads[0].Title != "Qubit" {
		t.Fatalf("notification = %#v, want run-complete Qubit notification", n.payloads[0])
	}
	if !strings.Contains(n.payloads[0].Body, "Demo chat") {
		t.Fatalf("notification body = %q, want session title", n.payloads[0].Body)
	}
}

func TestRunFinishedDuringStreamDoesNotNotifyBeforeStreamDrains(t *testing.T) {
	n := &recordingNotifier{}
	m := model{
		notifier:              n,
		busy:                  true,
		activeRunID:           "run_1",
		streaming:             true,
		streamingMessageIndex: 0,
		streamingFullContent:  "hello",
		messages:              []chatMessage{{Role: "assistant", Content: ""}},
	}

	updated, _ := m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_1", Status: "completed"})
	got := updated.(model)

	if !got.streaming || !got.streamingFinished {
		t.Fatalf("streaming state = streaming:%v finished:%v, want run_finished recorded while stream continues", got.streaming, got.streamingFinished)
	}
	if len(n.payloads) != 0 {
		t.Fatalf("notifications = %#v, want none before stream drains", n.payloads)
	}
}

func TestRunFinishedWithoutStreamingNotifiesImmediately(t *testing.T) {
	rt, _ := newTestRuntime(t)
	n := &recordingNotifier{}
	m := model{
		notifier:    n,
		runtime:     rt,
		busy:        true,
		activeRunID: "run_1",
		title:       "Direct finish",
		messages:    []chatMessage{{Role: "user", Content: "hello"}},
	}

	updated, cmd := m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_1", Status: "completed"})
	got := updated.(model)
	runNotificationCommand(t, cmd)

	if got.busy || got.activeRunID != "" {
		t.Fatalf("run state = busy:%v activeRunID:%q, want complete", got.busy, got.activeRunID)
	}
	if len(n.payloads) != 1 || n.payloads[0].Kind != notificationKindRunComplete {
		t.Fatalf("notifications = %#v, want one run-complete notification", n.payloads)
	}
}

func TestRunFinishedStaleRunDoesNotNotify(t *testing.T) {
	n := &recordingNotifier{}
	m := model{notifier: n, busy: true, activeRunID: "run_current"}

	_, cmd := m.updateRuntime(runtimeEvent{Type: "run_finished", RunID: "run_old", Status: "completed"})

	if cmd == nil {
		t.Fatal("stale run_finished returned nil command, want waitRuntimeEvent command")
	}
	if len(n.payloads) != 0 {
		t.Fatalf("notifications = %#v, want none for stale run_finished", n.payloads)
	}
}

func TestSessionPickerUsesVisibleListWindow(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.sessionCursor = 10
	for i := 0; i < 20; i++ {
		m.sessions = append(m.sessions, sessionInfo{ID: fmt.Sprintf("sess_%02d", i), Title: fmt.Sprintf("Session %02d", i), MessageCount: i})
	}

	rendered := plainText(m.renderSessionPicker(10))

	if !strings.Contains(rendered, "Session 10") {
		t.Fatalf("rendered session picker does not include cursor row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "more above") || !strings.Contains(rendered, "more below") {
		t.Fatalf("rendered session picker missing scroll hints:\n%s", rendered)
	}
	if strings.Contains(rendered, "Session 00") || strings.Contains(rendered, "Session 19") {
		t.Fatalf("rendered session picker did not window long list:\n%s", rendered)
	}
}

func TestForkCommandWithMessageNumberStartsEdit(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.messages = []chatMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
	}
	m.layout()

	updated, cmd := m.handleSlashCommand("/fork 3")
	got := updated.(model)

	if cmd != nil {
		t.Fatalf("/fork 3 returned command = %T, want nil while editing", cmd)
	}
	if !got.messageEdit.Active {
		t.Fatal("message edit inactive, want active")
	}
	if got.messageEdit.MessageIndex != 2 {
		t.Fatalf("edit message index = %d, want 2", got.messageEdit.MessageIndex)
	}
	if got.composer.Value() != "second question" {
		t.Fatalf("composer value = %q, want second question", got.composer.Value())
	}
}
func TestConsecutiveToolGroupsWrapEveryFour(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 140
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "tools"}}
	tools := []string{"ripgrep", "readFile", "readFiles", "glob", "editFile", "bash", "powershell", "todoMd", "deleteFile"}
	for i, tool := range tools {
		m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: i + 1, ToolCallID: fmt.Sprintf("call_%d", i), ToolName: tool, Status: "completed"})
	}

	viewport := plainText(m.viewport.View())
	if strings.Contains(viewport, "+") && strings.Contains(viewport, "more") {
		t.Fatalf("viewport = %q, want wrapped tool groups without hidden overflow", viewport)
	}
	if strings.Count(viewport, "▸") != len(tools) {
		t.Fatalf("viewport = %q, want all tool groups visible", viewport)
	}
	cleanViewport := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(viewport, "")
	toolRows := regexp.MustCompile(`(?m)^▸`).FindAllStringIndex(cleanViewport, -1)
	if len(toolRows) != 3 {
		t.Fatalf("viewport = %q, want three tool rows after wrapping every four", viewport)
	}
}

func TestAutoScrollStaysOffDuringNewContentUntilUserReachesBottom(t *testing.T) {
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
	m.viewport.GotoBottom()
	m.autoScroll = true

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = updated.(model)
	if m.autoScroll {
		t.Fatal("autoScroll = true after PgUp, want false")
	}
	yOffset := m.viewport.YOffset()
	m.messages[0].Content += "\nline 11\nline 12"
	m.refreshViewport()
	if m.autoScroll {
		t.Fatal("autoScroll restarted before user reached bottom")
	}
	if got := m.viewport.YOffset(); got != yOffset {
		t.Fatalf("YOffset after new content = %d, want preserved %d", got, yOffset)
	}

	for i := 0; i < 20 && !m.viewport.AtBottom(); i++ {
		m = m.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	}
	if !m.viewport.AtBottom() {
		t.Fatal("viewport did not reach bottom")
	}
	if !m.autoScroll {
		t.Fatal("autoScroll = false at bottom, want true")
	}
}

func TestPlanDisplayEventAddsUiOnlyMarkdownMessage(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	updated, _ := m.updateRuntime(runtimeEvent{Type: "plan.view", Name: "launch", Path: "./.qubit/plans/launch.md", Content: "# Launch\n\n- step"})
	got := updated.(model)

	if len(got.messages) < 2 {
		t.Fatalf("messages = %#v, want plan display appended", got.messages)
	}
	last := got.messages[len(got.messages)-1]
	if last.Role != "view" || last.ViewType != "plan" || last.Content == "" {
		t.Fatalf("last message = %#v, want plan view message", last)
	}
	viewport := plainText(got.viewport.View())
	if !strings.Contains(viewport, "Plan: launch") || !strings.Contains(viewport, "Launch") || !strings.Contains(viewport, "step") {
		t.Fatalf("viewport = %q, want rendered plan markdown", viewport)
	}
	if !strings.Contains(viewport, "╭") || !strings.Contains(viewport, "│") || !strings.Contains(viewport, "╰") {
		t.Fatalf("viewport = %q, want bordered plan view", viewport)
	}
}

func TestGeneratedImageEventAddsViewMessage(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	updated, _ := m.updateRuntime(runtimeEvent{Type: "generated.image", Path: `D:\qubit\.qubit\generated\generated-run-01.png`, MimeType: "image/png", SizeBytes: 12})
	got := updated.(model)

	if len(got.messages) < 2 {
		t.Fatalf("messages = %#v, want generated image view appended", got.messages)
	}
	last := got.messages[len(got.messages)-1]
	if last.Role != "view" || last.ViewType != "image" || last.Path == "" {
		t.Fatalf("last message = %#v, want generated image view message", last)
	}
	viewport := plainText(got.viewport.View())
	if !strings.Contains(viewport, "Generated image") || !strings.Contains(viewport, `generated-run-01.png`) || !strings.Contains(viewport, "image/png") {
		t.Fatalf("viewport = %q, want generated image path and metadata", viewport)
	}
}

func TestSessionPickerSortsByMostRecentActivity(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_old"
	m.sessions = []sessionInfo{
		{ID: "sess_new_created", Title: "New created", CreatedAt: "2026-01-03T00:00:00Z", UpdatedAt: "2026-01-03T00:00:00Z"},
		{ID: "sess_recent_activity", Title: "Recent activity", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-04T00:00:00Z"},
		{ID: "sess_fallback", Title: "Fallback created", CreatedAt: "2026-01-02T00:00:00Z"},
		{ID: "sess_fork", Title: "Fork", CreatedAt: "2026-01-05T00:00:00Z", UpdatedAt: "2026-01-05T00:00:00Z", ForkedFromSessionID: "sess_recent_activity"},
	}

	visible := m.sessionPickerSessions()

	if len(visible) != 3 {
		t.Fatalf("visible sessions = %d, want 3", len(visible))
	}
	want := []string{"sess_recent_activity", "sess_new_created", "sess_fallback"}
	for i, id := range want {
		if visible[i].ID != id {
			t.Fatalf("visible[%d] = %q, want %q; visible=%#v", i, visible[i].ID, id, visible)
		}
	}
}

func TestSubmitExistingSessionUpdatesLocalSessionRecency(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.autoNewSessionOnChat = false
	m.session = "sess_old"
	m.sessions = []sessionInfo{
		{ID: "sess_newer", Title: "Newer", CreatedAt: "2026-01-03T00:00:00Z", UpdatedAt: "2026-01-03T00:00:00Z"},
		{ID: "sess_old", Title: "Older", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
	}
	m.composer.SetValue("new activity")

	updated, cmd := m.submitInput()
	got := updated.(model)
	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	assertPayload(t, payload, "chat", "sess_old")

	visible := got.sessionPickerSessions()
	if len(visible) < 1 || visible[0].ID != "sess_old" {
		t.Fatalf("visible sessions after submit = %#v, want sess_old first", visible)
	}
}

func TestApplySessionListKeepsLocalNewerActivity(t *testing.T) {
	m := initialModel(nil)
	m.sessions = []sessionInfo{
		{ID: "sess_active", Title: "Active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-05T00:00:00Z"},
		{ID: "sess_other", Title: "Other", CreatedAt: "2026-01-04T00:00:00Z", UpdatedAt: "2026-01-04T00:00:00Z"},
	}

	m.applySessionList(runtimeEvent{Type: "session.list", SessionID: "sess_active", Sessions: []sessionInfo{
		{ID: "sess_other", Title: "Other", CreatedAt: "2026-01-04T00:00:00Z", UpdatedAt: "2026-01-04T00:00:00Z"},
		{ID: "sess_active", Title: "Active", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
	}})

	visible := m.sessionPickerSessions()
	if len(visible) < 1 || visible[0].ID != "sess_active" {
		t.Fatalf("visible sessions after stale list = %#v, want sess_active first", visible)
	}
}

func TestApplySessionMessagesPreservesPersistedPlanDisplayMessages(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	m.applySessionMessages(runtimeEvent{Type: "session.messages", Messages: []chatMessage{
		{Role: "tool", ToolGroup: &toolGroup{ID: "stored-tool-1-planMd-0", Name: "planMd", Step: 1, Calls: []toolCallUI{{ID: "plan-1", Name: "planMd", Step: 1, Status: "completed", Args: map[string]any{"action": "display", "name": "distribution-plan"}, Result: map[string]any{"displayed": true, "name": "distribution-plan", "path": "./.qubit/plans/distribution-plan.md"}}}}},
		{Role: "view", ViewType: "plan", Title: "Plan: distribution-plan", Path: "./.qubit/plans/distribution-plan.md", Content: "# Distribution Plan\n\n- package"},
	}})

	if len(m.messages) != 2 {
		t.Fatalf("messages = %#v, want tool row plus plan view", m.messages)
	}
	last := m.messages[1]
	if last.Role != "view" || last.ViewType != "plan" || !strings.Contains(last.Content, "Distribution Plan") {
		t.Fatalf("last message = %#v, want persisted plan display", last)
	}
	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "Plan: distribution-plan") || !strings.Contains(viewport, "Distribution") || !strings.Contains(viewport, "package") {
		t.Fatalf("viewport = %q, want persisted plan rendered", viewport)
	}
	if !strings.Contains(viewport, "╭") || !strings.Contains(viewport, "│") || !strings.Contains(viewport, "╰") {
		t.Fatalf("viewport = %q, want bordered persisted plan view", viewport)
	}
}

func TestSlashHelpWhileStreamingQueuesLocalStatus(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.busy = true
	m.activeRunID = "run_1"
	m.streaming = true
	m.streamingMessageIndex = 0
	m.streamingFullContent = "assistant response"
	m.messages = []chatMessage{{Role: "assistant", Content: "assistant"}}

	updated, cmd := m.handleSlashCommand("/help")
	got := updated.(model)

	if cmd != nil {
		t.Fatalf("/help while streaming cmd = %T, want nil", cmd)
	}
	if len(got.queuedMessages) != 1 {
		t.Fatalf("queuedMessages = %#v, want one queued status", got.queuedMessages)
	}
	if got.queuedMessages[0].SendToModel || got.queuedMessages[0].Kind != queuedMessageStatus {
		t.Fatalf("queued status = %#v, want display-only status", got.queuedMessages[0])
	}
	if len(got.messages) != 1 {
		t.Fatalf("messages changed while streaming: %#v", got.messages)
	}
}

func TestQueuedStatusFlushesAfterStreamCompletion(t *testing.T) {
	m := initialModel(nil)
	m.busy = true
	m.activeRunID = "run_1"
	m.streaming = true
	m.streamingMessageIndex = 0
	m.streamingFullContent = "ok"
	m.streamingFinished = true
	m.streamingFinishStatus = "completed"
	m.messages = []chatMessage{{Role: "assistant", Content: ""}}
	m.queueStatus("Mode: edit")

	updated, _ := m.updateFakeStreamTick()
	got := updated.(model)
	if got.busy || got.streaming || got.activeRunID != "" {
		t.Fatalf("run state busy=%v streaming=%v activeRunID=%q, want idle", got.busy, got.streaming, got.activeRunID)
	}
	if len(got.queuedMessages) != 0 {
		t.Fatalf("queuedMessages = %#v, want flushed", got.queuedMessages)
	}
	if len(got.messages) != 2 || !got.messages[1].LocalOnly || got.messages[1].Role != "status" || got.messages[1].Content != "Mode: edit" {
		t.Fatalf("messages = %#v, want local-only status appended", got.messages)
	}
}

func TestBusyUserInputQueuesAndStartsAfterRunFinishes(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.busy = true
	m.activeRunID = "run_1"
	m.streaming = true
	m.streamingMessageIndex = 0
	m.streamingFullContent = "ok"
	m.streamingFinished = true
	m.streamingFinishStatus = "completed"
	m.autoNewSessionOnChat = false
	m.session = "sess_1"
	m.messages = []chatMessage{{Role: "assistant", Content: ""}}
	m.composer.SetValue("next question")

	updated, cmd := m.submitInput()
	got := updated.(model)
	if cmd != nil {
		t.Fatalf("submit busy user input cmd = %T, want nil until current run finishes", cmd)
	}
	if len(got.queuedMessages) != 1 || got.queuedMessages[0].Kind != queuedMessageUser || !got.queuedMessages[0].SendToModel {
		t.Fatalf("queuedMessages = %#v, want queued user message", got.queuedMessages)
	}

	updated, cmd = got.updateFakeStreamTick()
	got = updated.(model)
	payload := runBatchSendCommand(t, cmd, stdin, "chat")
	if payload["input"] != "next question" {
		t.Fatalf("chat input = %#v, want queued next question", payload["input"])
	}
	if got.activeRunID == "" || !got.busy {
		t.Fatalf("queued run state busy=%v activeRunID=%q, want new active run", got.busy, got.activeRunID)
	}
	if len(got.queuedMessages) != 0 {
		t.Fatalf("queuedMessages = %#v, want consumed", got.queuedMessages)
	}
}

func TestUnsafeSlashCommandDuringStreamDoesNotInterruptRun(t *testing.T) {
	rt, _ := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.busy = true
	m.activeRunID = "run_1"
	m.streaming = true
	m.messages = []chatMessage{{Role: "assistant", Content: "partial"}}

	updated, cmd := m.handleSlashCommand("/new Later")
	got := updated.(model)

	if cmd != nil {
		t.Fatalf("unsafe slash command during stream cmd = %T, want nil", cmd)
	}
	if !got.busy || !got.streaming || got.activeRunID != "run_1" {
		t.Fatalf("run interrupted: busy=%v streaming=%v activeRunID=%q", got.busy, got.streaming, got.activeRunID)
	}
	if len(got.queuedMessages) != 1 || got.queuedMessages[0].SendToModel {
		t.Fatalf("queuedMessages = %#v, want local status explaining block", got.queuedMessages)
	}
}

func TestSessionPickerSearchFiltersTitles(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.mode = modeSessionPicker
	m.width = 100
	m.sessions = []sessionInfo{
		{ID: "sess_alpha", Title: "Alpha planning"},
		{ID: "sess_beta", Title: "Beta release notes"},
		{ID: "sess_gamma", Title: "Gamma planning"},
	}

	updated, _ := m.updateSessionPicker(tea.KeyPressMsg{Text: "s"})
	got := updated.(model)
	if !got.sessionSearchMode {
		t.Fatal("session search mode = false, want true")
	}

	updated, _ = got.updateSessionPicker(tea.KeyPressMsg{Text: "plan"})
	got = updated.(model)
	visible := got.sessionPickerSessions()
	if got.sessionSearchQuery != "plan" {
		t.Fatalf("sessionSearchQuery = %q, want plan", got.sessionSearchQuery)
	}
	if len(visible) != 2 || visible[0].ID != "sess_alpha" || visible[1].ID != "sess_gamma" {
		t.Fatalf("visible sessions = %#v, want alpha and gamma", visible)
	}

	rendered := got.renderSessionPicker(20)
	if !strings.Contains(rendered, "plan") || strings.Contains(rendered, "Beta release notes") {
		t.Fatalf("rendered search picker mismatch:\n%s", rendered)
	}
}

func TestSessionPickerSearchEscapeClearsSearch(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeSessionPicker
	m.sessionSearchMode = true
	m.sessionSearchQuery = "beta"
	m.sessions = []sessionInfo{{ID: "sess_alpha", Title: "Alpha"}, {ID: "sess_beta", Title: "Beta"}}

	updated, _ := m.updateSessionPicker(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	if got.sessionSearchMode || got.sessionSearchQuery != "" {
		t.Fatalf("search mode/query = %v/%q, want cleared", got.sessionSearchMode, got.sessionSearchQuery)
	}
	if visible := got.sessionPickerSessions(); len(visible) != 2 {
		t.Fatalf("visible sessions after escape = %d, want 2", len(visible))
	}
	if got.mode != modeSessionPicker {
		t.Fatalf("mode = %v, want session picker", got.mode)
	}
}

func TestTranscriptMouseDragSelectsTextAndEscClears(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 10
	m.layout()
	m.messages = []chatMessage{{Role: "assistant", Content: "line 1\nline 2\nline 3\nline 4"}}
	m.refreshViewport()
	m.viewport.GotoTop()

	clicked := m.updateMouseClick(tea.MouseClickMsg{X: 0, Y: m.chatTopY, Button: tea.MouseLeft}).(model)
	if !clicked.transcriptSelection.Active {
		t.Fatal("transcript selection not active after mouse click")
	}
	dragged := clicked.updateMouseMotion(tea.MouseMotionMsg{X: 6, Y: clicked.chatTopY + 2, Button: tea.MouseLeft}).(model)
	releasedModel, releaseCmd := dragged.updateMouseRelease(tea.MouseReleaseMsg{X: 6, Y: dragged.chatTopY + 2, Button: tea.MouseLeft})
	if releaseCmd != nil {
		t.Fatalf("release command = %v, want nil", releaseCmd)
	}
	released := releasedModel.(model)

	if !released.transcriptSelection.Active {
		t.Fatal("transcript selection cleared after drag release")
	}
	if got, want := released.transcriptSelectedText(), "◆ line 1\n  line 2\n  line 3\n  line"; got != want {
		t.Fatalf("selected text = %q, want %q", got, want)
	}

	updated, cmd := released.updateKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc command = %#v, want nil", cmd)
	}
	cleared := updated.(model)
	if cleared.transcriptSelection.Active {
		t.Fatal("transcript selection still active after esc")
	}
}

func TestCtrlCCopiesTranscriptSelectionBeforeQuit(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 10
	m.layout()
	content := "copy me\nnot me"
	m.viewport.SetContent(content)
	m.transcriptLines = transcriptRenderLines(content)
	m.transcriptSelection = transcriptSelectionState{
		Active: true,
		Anchor: transcriptSelectionPoint{Line: 0, Col: 0},
		Cursor: transcriptSelectionPoint{Line: 0, Col: 7},
	}

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	got := updated.(model)
	if cmd == nil {
		t.Fatal("ctrl+c returned nil command, want clipboard copy command")
	}
	if got.status != "copied transcript" {
		t.Fatalf("status = %q, want copied transcript", got.status)
	}
}

func TestMouseWheelRoutesToVisibleLists(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 10
	m.layout()
	m.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5\nline 6")
	m.viewport.GotoTop()
	m.mode = modeSessionPicker
	m.sessions = []sessionInfo{{ID: "one", Title: "one"}, {ID: "two", Title: "two"}}

	updated := m.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.sessionCursor != 1 {
		t.Fatalf("sessionCursor = %d, want 1", updated.sessionCursor)
	}
	if updated.viewport.YOffset() != 0 {
		t.Fatalf("viewport scrolled in session picker: y=%d", updated.viewport.YOffset())
	}

	updated.mode = modeKeyPicker
	updated.apiKeys = []apiKeyInfo{{Alias: "one"}, {Alias: "two"}}
	updated.apiKeyCursor = 0
	updated = updated.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.apiKeyCursor != 1 {
		t.Fatalf("apiKeyCursor = %d, want 1", updated.apiKeyCursor)
	}

	updated.mode = modeModal
	updated.modal = &modalState{Options: []modalOption{{ID: "one"}, {ID: "two"}}, OptionCursor: 0}
	updated = updated.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.modal.OptionCursor != 1 {
		t.Fatalf("modal option cursor = %d, want 1", updated.modal.OptionCursor)
	}
}

func TestMouseWheelDuringTranscriptSelectionScrollsChat(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 8
	m.layout()
	content := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"
	m.viewport.SetContent(content)
	m.transcriptLines = transcriptRenderLines(content)
	m.viewport.GotoTop()
	m.mode = modeChat
	m.transcriptSelection = transcriptSelectionState{Active: true, Anchor: transcriptSelectionPoint{Line: 0, Col: 0}, Cursor: transcriptSelectionPoint{Line: 0, Col: 1}}

	updated := m.updateMouseWheelRouted(tea.MouseWheelMsg{Button: tea.MouseWheelDown}).(model)
	if updated.viewport.YOffset() == 0 {
		t.Fatal("viewport did not scroll while transcript selection was active")
	}
	if !updated.transcriptSelection.Active {
		t.Fatal("transcript selection cleared by wheel scroll")
	}
}

func TestTranscriptLinkHitboxesExtractURLs(t *testing.T) {
	lines := transcriptRenderLines("prefix https://example.com/path, suffix\nwide αβ https://example.org/q?x=1!")
	boxes := transcriptLinkHitboxes(lines)
	if len(boxes) != 2 {
		t.Fatalf("link hitboxes = %d, want 2: %#v", len(boxes), boxes)
	}
	if boxes[0].URL != "https://example.com/path" || boxes[0].Line != 0 {
		t.Fatalf("first hitbox = %#v", boxes[0])
	}
	wantStart := runewidth.StringWidth("prefix ")
	wantEnd := wantStart + runewidth.StringWidth("https://example.com/path") - 1
	if boxes[0].StartX != wantStart || boxes[0].EndX != wantEnd {
		t.Fatalf("first hitbox x = %d..%d, want %d..%d", boxes[0].StartX, boxes[0].EndX, wantStart, wantEnd)
	}
	if boxes[1].URL != "https://example.org/q?x=1" {
		t.Fatalf("second URL = %q, want punctuation trimmed", boxes[1].URL)
	}
}

func TestCtrlClickTranscriptLinkReturnsOpenCommand(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 100
	m.height = 10
	m.layout()
	content := "see https://example.com/docs now"
	m.viewport.SetContent(content)
	m.transcriptContent = content
	m.transcriptLines = transcriptRenderLines(content)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
	m.viewport.GotoTop()

	if len(m.linkHitboxes) != 1 {
		t.Fatalf("link hitboxes = %d, want 1", len(m.linkHitboxes))
	}
	x := m.linkHitboxes[0].StartX
	clicked := m.updateMouseClick(tea.MouseClickMsg{X: x, Y: m.chatTopY, Button: tea.MouseLeft, Mod: tea.ModCtrl}).(model)
	updatedModel, cmd := clicked.updateMouseRelease(tea.MouseReleaseMsg{X: x, Y: clicked.chatTopY, Button: tea.MouseLeft, Mod: tea.ModCtrl})
	updated := updatedModel.(model)
	if cmd == nil {
		t.Fatal("ctrl+click link command = nil, want open browser command")
	}
	if updated.status != "opening link" {
		t.Fatalf("status = %q, want opening link", updated.status)
	}
	if updated.transcriptSelection.Active {
		t.Fatal("transcript selection remains active after opening link")
	}
}

func TestCtrlDragTranscriptLinkDoesNotOpen(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 100
	m.height = 10
	m.layout()
	content := "see https://example.com/docs now"
	m.viewport.SetContent(content)
	m.transcriptContent = content
	m.transcriptLines = transcriptRenderLines(content)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
	m.viewport.GotoTop()

	x := m.linkHitboxes[0].StartX
	clicked := m.updateMouseClick(tea.MouseClickMsg{X: x, Y: m.chatTopY, Button: tea.MouseLeft, Mod: tea.ModCtrl}).(model)
	dragged := clicked.updateMouseMotion(tea.MouseMotionMsg{X: x + 4, Y: clicked.chatTopY + 1, Button: tea.MouseLeft, Mod: tea.ModCtrl}).(model)
	updatedModel, cmd := dragged.updateMouseRelease(tea.MouseReleaseMsg{X: x + 4, Y: dragged.chatTopY + 1, Button: tea.MouseLeft, Mod: tea.ModCtrl})
	updated := updatedModel.(model)
	if cmd != nil {
		t.Fatalf("ctrl+drag release command = %v, want nil", cmd)
	}
	if !updated.transcriptSelection.Active {
		t.Fatal("transcript selection not active after ctrl+drag")
	}
}

func TestPlainClickTranscriptLinkDoesNotOpen(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 100
	m.height = 10
	m.layout()
	content := "see https://example.com/docs now"
	m.viewport.SetContent(content)
	m.transcriptContent = content
	m.transcriptLines = transcriptRenderLines(content)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
	m.viewport.GotoTop()

	x := m.linkHitboxes[0].StartX
	clicked := m.updateMouseClick(tea.MouseClickMsg{X: x, Y: m.chatTopY, Button: tea.MouseLeft}).(model)
	updatedModel, cmd := clicked.updateMouseRelease(tea.MouseReleaseMsg{X: x, Y: clicked.chatTopY, Button: tea.MouseLeft})
	updated := updatedModel.(model)
	if cmd != nil {
		t.Fatalf("plain click command = %v, want nil", cmd)
	}
	if updated.status != "ready" {
		t.Fatalf("status = %q, want ready", updated.status)
	}
}

func TestPlanViewAndTodoOverlayUseMatchingBorder(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	plan := m.renderPlanViewMessage(chatMessage{Role: "view", ViewType: "plan", Title: "Plan: launch", Content: "# Launch\n\n- step"}, "Plan: launch", 80)
	todo := renderTodoOverlaySnapshot(todoOverlaySnapshot{Name: "launch", Items: []todoOverlayItem{{Text: "step", IsTask: true}}, Total: 1}, 100, 10, true, 0)
	plainPlan := stripANSI(plan)
	plainTodo := stripANSI(todo)
	for _, border := range []string{"╭", "│", "╰"} {
		if !strings.Contains(plainPlan, border) || !strings.Contains(plainTodo, border) {
			t.Fatalf("plan/todo borders mismatch: plan=%q todo=%q", plainPlan, plainTodo)
		}
	}
}

func TestForkAndHistoryComposerStateColors(t *testing.T) {
	m := initialModel(nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.session = "sess_parent"
	m.inputHistory = []string{"history item"}
	m.inputHistoryIndex = len(m.inputHistory)
	m.messages = []chatMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
	}
	m.layout()

	updated, _ := m.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	history := updated.(model)
	if !history.inputHistoryActive || history.composer.Value() != "history item" {
		t.Fatalf("history state/input = %v/%q, want active history item", history.inputHistoryActive, history.composer.Value())
	}
	if got := history.composerTextStyle().Render("history item"); !strings.Contains(history.renderInput(), got) {
		t.Fatalf("history input missing history color: %q want %q", history.renderInput(), got)
	}
	if !strings.Contains(stripANSI(history.renderInputStatus()), "history") || strings.Contains(stripANSI(history.renderInputStatus()), "editing message") {
		t.Fatalf("history status = %q, want history hint distinct from editing", history.renderInputStatus())
	}
	if history.composerTextStyle().Render("x") == messageEditInputSt.Render("x") || history.composerTextStyle().Render("x") == forkSelectInputSt.Render("x") {
		t.Fatal("history composer style should be distinct from edit and fork selector styles")
	}

	updated, _ = history.updateKey(tea.KeyPressMsg{Text: "x", Code: 'x'})
	typed := updated.(model)
	if typed.inputHistoryActive {
		t.Fatal("inputHistoryActive = true after typing, want cleared")
	}
	if strings.Contains(stripANSI(typed.renderInputStatus()), "history") {
		t.Fatalf("typed status still shows history: %q", typed.renderInputStatus())
	}

	m.composer.SetValue("/fork")
	updated, _ = m.submitInput()
	forking := updated.(model)
	updated, _ = forking.updateKey(tea.KeyPressMsg{Code: tea.KeyUp})
	forking = updated.(model)
	if !forking.forkSelector.Active || forking.forkSelector.Cursor < 0 {
		t.Fatalf("fork selector state = %#v, want selected past message", forking.forkSelector)
	}
	if got := forkSelectInputSt.Render(forking.composer.Value()); !strings.Contains(forking.renderInput(), got) {
		t.Fatalf("fork selector input missing fork color: %q want %q", forking.renderInput(), got)
	}
	if !strings.Contains(stripANSI(forking.renderInputStatus()), "selected past message") {
		t.Fatalf("fork selector status = %q, want selected past message hint", forking.renderInputStatus())
	}
	if forking.composerTextStyle().Render("x") == messageEditInputSt.Render("x") || forking.composerTextStyle().Render("x") == inputHistorySt.Render("x") {
		t.Fatal("fork selector composer style should be distinct from edit and history styles")
	}

	updated, _ = forking.updateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	editing := updated.(model)
	if !editing.messageEdit.Active {
		t.Fatal("message edit inactive, want active after selecting fork point")
	}
	if got := messageEditInputSt.Render(editing.composer.Value()); !strings.Contains(editing.renderInput(), got) {
		t.Fatalf("edit input missing edit color: %q want %q", editing.renderInput(), got)
	}
	if !strings.Contains(stripANSI(editing.renderInputStatus()), "editing message") {
		t.Fatalf("edit status = %q, want editing message hint", editing.renderInputStatus())
	}
	if editing.composerTextStyle().Render("x") == forkSelectInputSt.Render("x") || editing.composerTextStyle().Render("x") == inputHistorySt.Render("x") {
		t.Fatal("edit composer style should be distinct from fork selector and history styles")
	}
}

func TestSlashFavouriteSessionRequestsRuntime(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_active"

	updated, cmd := m.handleSlashCommand("/favourite-session")
	got := updated.(model)

	if !got.busy {
		t.Fatal("busy = false, want true while favouriting session")
	}
	if got.status != "favouriting session" {
		t.Fatalf("status = %q, want favouriting session", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.favourite", "sess_active")
}

func TestSlashFavouriteSessionWithoutActiveSessionDoesNotSend(t *testing.T) {
	m := initialModel(nil)
	m.ready = true

	updated, cmd := m.handleSlashCommand("/favourite-session")
	got := updated.(model)

	if cmd != nil {
		t.Fatal("cmd is non-nil, want no runtime request without active session")
	}
	if got.busy {
		t.Fatal("busy = true, want false without active session")
	}
	if len(got.messages) == 0 || !strings.Contains(got.messages[len(got.messages)-1].Content, "No active session") {
		t.Fatalf("messages = %#v, want no active session notice", got.messages)
	}
}

func TestSessionFavouritedEventUpdatesLocalSession(t *testing.T) {
	m := initialModel(nil)
	m.busy = true
	m.session = "sess_active"
	m.title = "Active"
	m.sessions = []sessionInfo{{ID: "sess_active", Title: "Active"}}

	m.applySessionFavourited(runtimeEvent{
		Type:      "session.favourited",
		SessionID: "sess_active",
		Session:   &sessionInfo{ID: "sess_active", Title: "Active", FavouritedAt: "2026-06-04T06:00:00Z"},
	})

	if m.busy {
		t.Fatal("busy = true, want false after favourite event")
	}
	if m.status != "session favourited" {
		t.Fatalf("status = %q, want session favourited", m.status)
	}
	if len(m.sessions) != 1 || m.sessions[0].FavouritedAt != "2026-06-04T06:00:00Z" {
		t.Fatalf("sessions = %#v, want favourited session metadata", m.sessions)
	}
}

func TestSessionPickerFavouritesFirstWithoutDuplicates(t *testing.T) {
	m := initialModel(nil)
	m.sessions = []sessionInfo{
		{ID: "sess_recent", Title: "Recent", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-05T00:00:00Z"},
		{ID: "sess_fav_old", Title: "Favourite old", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z", FavouritedAt: "2026-01-06T00:00:00Z"},
		{ID: "sess_fav_new", Title: "Favourite new", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-04T00:00:00Z", FavouritedAt: "2026-01-03T00:00:00Z"},
		{ID: "sess_old", Title: "Old", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "sess_fork", Title: "Fork", CreatedAt: "2026-01-07T00:00:00Z", UpdatedAt: "2026-01-07T00:00:00Z", ForkedFromSessionID: "sess_recent", FavouritedAt: "2026-01-07T00:00:00Z"},
	}

	visible := m.sessionPickerSessions()
	want := []string{"sess_fav_new", "sess_fav_old", "sess_recent", "sess_old"}
	if len(visible) != len(want) {
		t.Fatalf("visible sessions = %#v, want %d sessions", visible, len(want))
	}
	seen := map[string]bool{}
	for i, id := range want {
		if visible[i].ID != id {
			t.Fatalf("visible[%d] = %q, want %q; visible=%#v", i, visible[i].ID, id, visible)
		}
		if seen[visible[i].ID] {
			t.Fatalf("duplicate session in visible list: %#v", visible)
		}
		seen[visible[i].ID] = true
	}
}

func TestSessionPickerSearchKeepsFavouritesFirst(t *testing.T) {
	m := initialModel(nil)
	m.sessionSearchQuery = "plan"
	m.sessions = []sessionInfo{
		{ID: "sess_recent", Title: "Plan recent", UpdatedAt: "2026-01-05T00:00:00Z"},
		{ID: "sess_fav", Title: "Plan favourite", UpdatedAt: "2026-01-01T00:00:00Z", FavouritedAt: "2026-01-02T00:00:00Z"},
		{ID: "sess_other", Title: "Other", UpdatedAt: "2026-01-06T00:00:00Z"},
	}

	visible := m.sessionPickerSessions()
	if len(visible) != 2 || visible[0].ID != "sess_fav" || visible[1].ID != "sess_recent" {
		t.Fatalf("visible = %#v, want favourite search match before recent match", visible)
	}
}

func TestChatPasteMsgInsertsIntoComposer(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.layout()

	updated, cmd := m.Update(tea.PasteMsg{Content: "hello\r\nworld"})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("paste returned command, want nil")
	}
	if got.composer.Value() != "hello\nworld" {
		t.Fatalf("composer value = %q, want normalized multiline paste", got.composer.Value())
	}
	if got.composer.Height() < 2 {
		t.Fatalf("composer height = %d, want multiline paste to grow input", got.composer.Height())
	}
}

func TestChatCtrlVPasteMsgInsertsIntoComposer(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.layout()

	updated, cmd := m.Update(composerPasteMsg{Text: "from clipboard"})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("composer paste returned command, want nil")
	}
	if got.composer.Value() != "from clipboard" {
		t.Fatalf("composer value = %q, want clipboard text", got.composer.Value())
	}
}

func TestChatPasteReplacesSelectedComposerText(t *testing.T) {
	m := initialModel(nil)
	m.width = 80
	m.height = 24
	m.composer.SetValue("replace me")
	m.composer.SelectAll()

	updated, _ := m.Update(tea.PasteMsg{Content: "pasted"})
	got := updated.(model)

	if got.composer.HasSelection() {
		t.Fatal("composer selection still active after paste")
	}
	if got.composer.Value() != "pasted" {
		t.Fatalf("composer value = %q, want pasted", got.composer.Value())
	}
}

func TestHiddenSessionsIgnoredBySessionPicker(t *testing.T) {
	m := initialModel(nil)
	m.sessions = []sessionInfo{
		{ID: "visible", Title: "Visible", CreatedAt: "2026-01-02T00:00:00Z"},
		{ID: "hidden", Title: "Hidden", CreatedAt: "2026-01-03T00:00:00Z", Hidden: true, Kind: "subagent"},
	}
	sessions := m.sessionPickerSessions()
	if len(sessions) != 1 || sessions[0].ID != "visible" {
		t.Fatalf("sessions = %#v, want only visible session", sessions)
	}
}
