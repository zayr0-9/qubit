package tui

import (
	"strings"
	"testing"
)

func TestContextStatusForCodexModelUsesUsageAsContext(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.provider = "codex"
	m.activeProvider = "codex"
	m.model = "gpt-5.2-codex"
	m.maxContext = 400000
	m.lastCodexUsage = &codexUsage{InputTokens: 12345, CachedTokens: 12000, OutputTokens: 678}
	m.messages = []chatMessage{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "reasoning", Content: strings.Repeat("r", 40)},
		{Role: "tool", ToolGroup: &toolGroup{Name: "readFile", Step: 1, Calls: []toolCallUI{{ID: "read-1", Name: "readFile", Status: "completed", Args: map[string]any{"path": "agent.md"}, Result: map[string]any{"totalLines": float64(42)}}}}},
	}

	status := plainText(m.renderInputStatus())
	if !strings.Contains(status, "ctx 13k/400k cache 12k") || strings.Contains(status, "log in") {
		t.Fatalf("status = %q, want Codex input+output as context with cache mention", status)
	}
}

func TestContextStatusUsesLatestMessageCodexUsage(t *testing.T) {
	m := initialModel(nil)
	m.maxContext = 400000
	m.messages = []chatMessage{
		{Role: "assistant", Content: "older", CodexUsage: &codexUsage{InputTokens: 1000, OutputTokens: 200, Model: "gpt-5.2-codex"}},
		{Role: "user", Content: "next"},
		{Role: "assistant", Content: "newer", CodexUsage: &codexUsage{InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300, Model: "gpt-5.2-codex"}},
	}

	status := plainText(m.renderInputStatus())
	if !strings.Contains(status, "ctx 2.3k/400k cache 1.5k") || strings.Contains(status, "log in") {
		t.Fatalf("status = %q, want latest message Codex usage as context", status)
	}
}

func TestSessionMessagesRestoresLatestCodexUsage(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.provider = "codex"
	m.activeProvider = "codex"
	m.model = "gpt-5.2-codex"
	m.maxContext = 400000
	m.messages = []chatMessage{
		{Role: "user", Content: strings.Repeat("local estimate should not win", 200)},
	}
	m.lastCodexUsage = &codexUsage{InputTokens: 999, OutputTokens: 1}

	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_1", Messages: []chatMessage{
		{Role: "assistant", Content: "old", CodexUsage: &codexUsage{InputTokens: 1000, Model: "gpt-5.2-codex"}},
		{Role: "user", Content: strings.Repeat("large loaded transcript", 200)},
		{Role: "assistant", Content: "new", CodexUsage: &codexUsage{InputTokens: 3000, CachedTokens: 2500, OutputTokens: 400}},
	}})

	if m.lastCodexUsage == nil || m.lastCodexUsage.InputTokens != 3000 || m.lastCodexUsage.OutputTokens != 400 {
		t.Fatalf("lastCodexUsage = %#v, want latest persisted message usage", m.lastCodexUsage)
	}
	status := plainText(m.renderInputStatus())
	if !strings.Contains(status, "ctx 3.4k/400k cache 2.5k") || strings.Contains(status, "log in") {
		t.Fatalf("status = %q, want loaded session Codex usage as displayed context", status)
	}
}

func TestCodexUsageRuntimeEventUpdatesStatus(t *testing.T) {
	m := model{activeRunID: "run_1", maxContext: 400000}
	updated, _ := m.updateRuntime(runtimeEvent{Type: "codex.usage", RunID: "run_1", CodexUsage: &codexUsage{InputTokens: 5000, CachedTokens: 4000, OutputTokens: 600, Model: "gpt-5.2-codex"}})
	got := updated.(model)

	status := plainText(got.renderInputStatus())
	if !strings.Contains(status, "ctx 5.6k/400k cache 4k") || strings.Contains(status, "log in") {
		t.Fatalf("status = %q, want live Codex usage as context", status)
	}
}

func TestContextStatusUsesCodexUsageEvenWithoutProviderHint(t *testing.T) {
	m := initialModel(nil)
	m.provider = "openai"
	m.activeProvider = "openai"
	m.maxContext = 128000
	m.lastCodexUsage = &codexUsage{InputTokens: 12000, CachedTokens: 8000, OutputTokens: 500}

	status := plainText(m.renderInputStatus())
	if !strings.Contains(status, "ctx 12.5k/128k cache 8k") {
		t.Fatalf("status = %q, want Codex usage trusted when present", status)
	}
}

func TestEstimateContextTokensIncludesReasoningAndToolGroups(t *testing.T) {
	messages := []chatMessage{
		{Role: "assistant", Content: "answer", ReasoningContent: strings.Repeat("r", 40)},
		{Role: "tool", ToolGroup: &toolGroup{Name: "bash", Step: 1, Calls: []toolCallUI{{ID: "bash-1", Name: "bash", Args: map[string]any{"command": "echo hello"}, Result: map[string]any{"stdoutPreview": "hello"}, ContextChars: 5}}}},
	}

	withAll := estimateContextTokens(messages)
	withoutExtra := estimateContextTokens([]chatMessage{{Role: "assistant", Content: "answer"}})
	if withAll <= withoutExtra {
		t.Fatalf("with reasoning/tools = %d, without = %d; want reasoning/tool context included", withAll, withoutExtra)
	}
}

func TestEstimateContextTokensUsesToolContextCharsOverUIPreview(t *testing.T) {
	messages := []chatMessage{
		{Role: "tool", ToolGroup: &toolGroup{Name: "readFile", Step: 1, Calls: []toolCallUI{{ID: "read-1", Name: "readFile", Result: map[string]any{"contentPreview": "short preview"}, ContextChars: 16000}}}},
	}

	used := estimateContextTokens(messages)
	if used < 4000 {
		t.Fatalf("used tokens = %d, want full tool context char count rather than preview length", used)
	}
}

func TestAssistantEventSurfacesReasoningBlock(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()

	m.applyAssistantEvent(runtimeEvent{Type: "assistant", Content: "final answer", ReasoningContent: "thinking out loud"})

	if len(m.messages) < 2 {
		t.Fatalf("messages = %#v, want reasoning and assistant messages", m.messages)
	}
	reasoning := m.messages[len(m.messages)-2]
	assistant := m.messages[len(m.messages)-1]
	if reasoning.Role != "reasoning" || reasoning.Content != "thinking out loud" {
		t.Fatalf("reasoning message = %#v, want surfaced reasoning block", reasoning)
	}
	if assistant.Role != "assistant" || assistant.ReasoningContent != "thinking out loud" {
		t.Fatalf("assistant message = %#v, want reasoning retained for context", assistant)
	}
}

func TestReasoningDeltaStreamsIntoSingleReasoningBlock(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", Content: "step one"})
	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", Content: " step two"})

	if got := m.messages[len(m.messages)-1]; got.Role != "reasoning" || got.Content != "step one step two" {
		t.Fatalf("last message = %#v, want streamed reasoning block", got)
	}
}

func TestReasoningBlockTitleUsesBoldHeading(t *testing.T) {
	title := reasoningBlockTitle("**Investigating project details**\n\nReading files and checking tests.")
	if title != "Investigating project details" {
		t.Fatalf("title = %q, want bold heading", title)
	}
}

func TestAssistantEventDoesNotDuplicateStreamedReasoningBlock(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", Content: "thinking"})
	m.applyAssistantEvent(runtimeEvent{Type: "assistant", RunID: "run_1", Content: "final answer", ReasoningContent: "thinking"})

	reasoningBlocks := 0
	for _, message := range m.messages {
		if message.Role == "reasoning" {
			reasoningBlocks++
		}
	}
	if reasoningBlocks != 1 {
		t.Fatalf("reasoning blocks = %d, messages = %#v; want one streamed reasoning block", reasoningBlocks, m.messages)
	}
}

func TestAssistantEventReplacesMalformedStreamedReasoningDraft(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", Content: "I see that the is asking me to inspect code."})
	m.applyAssistantEvent(runtimeEvent{Type: "assistant", RunID: "run_1", Content: "final answer", ReasoningContent: "I see that the user is asking me to inspect code."})

	reasoningBlocks := 0
	for _, message := range m.messages {
		if message.Role == "reasoning" {
			reasoningBlocks++
			if message.Content != "I see that the user is asking me to inspect code." {
				t.Fatalf("reasoning content = %q, want final corrected reasoning", message.Content)
			}
		}
	}
	if reasoningBlocks != 1 {
		t.Fatalf("reasoning blocks = %d, messages = %#v; want one corrected reasoning block", reasoningBlocks, m.messages)
	}
}

func TestAssistantEventDoesNotAppendAggregateWhenFinalCorrectsToolInterleavedReasoning(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"
	m.session = "sess_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "before tol"})
	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", RunID: "run_1", SessionID: "sess_1", Step: 1, ToolCallID: "call_1", ToolName: "readFiles", Status: "running"})
	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "after tool"})
	m.applyAssistantEvent(runtimeEvent{Type: "assistant", RunID: "run_1", SessionID: "sess_1", Content: "final", ReasoningContent: "before tool\n\nafter tool"})

	reasoningBlocks := 0
	for _, message := range m.messages {
		if message.Role == "reasoning" {
			reasoningBlocks++
		}
	}
	if reasoningBlocks != 2 {
		t.Fatalf("reasoning blocks = %d, messages = %#v; want two corrected streamed reasoning blocks and no aggregate duplicate", reasoningBlocks, m.messages)
	}
	for _, message := range m.messages {
		if message.Role == "reasoning" {
			if message.Content != "before tool" {
				t.Fatalf("first reasoning message = %#v, want corrected first reasoning", message)
			}
			break
		}
	}
}

func TestReasoningDeltaAfterToolStartsNewReasoningBlock(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"
	m.session = "sess_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "before tool"})
	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", RunID: "run_1", SessionID: "sess_1", Step: 1, ToolCallID: "call_1", ToolName: "readFiles", Status: "running"})
	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "after tool"})

	got := m.messages[len(m.messages)-3:]
	if got[0].Role != "reasoning" || got[0].Content != "before tool" || got[1].Role != "tool" || got[2].Role != "reasoning" || got[2].Content != "after tool" {
		t.Fatalf("tail messages = %#v, want reasoning/tool/reasoning interleaving", got)
	}
}

func TestAssistantEventDoesNotDuplicateMultipleStreamedReasoningBlocks(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.layout()
	m.activeRunID = "run_1"
	m.session = "sess_1"

	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "before tool"})
	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", RunID: "run_1", SessionID: "sess_1", Step: 1, ToolCallID: "call_1", ToolName: "readFiles", Status: "running"})
	m.applyReasoningDeltaEvent(runtimeEvent{Type: "reasoning.delta", RunID: "run_1", SessionID: "sess_1", Content: "after tool"})
	m.applyAssistantEvent(runtimeEvent{Type: "assistant", RunID: "run_1", SessionID: "sess_1", Content: "final", ReasoningContent: "before tool\n\nafter tool"})

	reasoningBlocks := 0
	for _, message := range m.messages {
		if message.Role == "reasoning" {
			reasoningBlocks++
		}
	}
	if reasoningBlocks != 2 {
		t.Fatalf("reasoning blocks = %d, messages = %#v; want two streamed reasoning blocks and no aggregate duplicate", reasoningBlocks, m.messages)
	}
}

func TestUserMessageDisplayNumberUsesVisibleMessageIndex(t *testing.T) {
	messages := []chatMessage{
		{Role: "user", Content: "first"},
		{Role: "reasoning", Content: "think"},
		{Role: "tool", ToolGroup: &toolGroup{Name: "readFile"}},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "second"},
	}

	if got := messageDisplayNumber(messages, 4); got != 5 {
		t.Fatalf("message display number = %d, want visible message index 5", got)
	}
}

func TestRenderInputStatusShowsCwdBetweenReasoningAndContext(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: `D:\qubit`})
	m.width = 120
	m.maxContext = 400000
	m.reasoningLevel = reasoningLevelHigh
	m.messages = nil

	status := plainText(m.renderInputStatus())
	want := `high · D:\qubit · 0 forks · ctx 0/400k`
	if !strings.Contains(status, want) {
		t.Fatalf("status = %q, want cwd and fork count before ctx as %q", status, want)
	}
}

func TestRenderInputStatusShowsCurrentSessionForksBetweenCwdAndContext(t *testing.T) {
	m := initialModel(&runtimeClient{launchCwd: `/home/me/project`})
	m.width = 120
	m.maxContext = 400000
	m.messages = nil
	m.session = "sess_root"
	m.sessions = []sessionInfo{
		{ID: "sess_root", Title: "Root"},
		{ID: "sess_fork_one", Title: "Fork one", ForkedFromSessionID: "sess_root"},
		{ID: "sess_fork_two", Title: "Fork two", ForkedFromSessionID: "sess_root"},
		{ID: "sess_grandfork", Title: "Grandfork", ForkedFromSessionID: "sess_fork_one"},
	}

	status := plainText(m.renderInputStatus())
	want := `medium · /home/me/project · 3 forks · ctx 0/400k`
	if !strings.Contains(status, want) {
		t.Fatalf("status = %q, want cwd, current fork count, then ctx as %q", status, want)
	}
}

func TestRenderHeaderShowsRuntimeConnectionStatus(t *testing.T) {
	t.Setenv("QUBIT_CONFIG_DIR", t.TempDir())
	m := initialModel(&runtimeClient{})
	m.width = 120
	m.provider = "stub"
	m.model = "test-model"
	m.title = "Default chat"

	m.runtimeConnected = true
	connected := plainText(m.renderHeader())
	if !strings.Contains(connected, "qubit  connected  Default chat") {
		t.Fatalf("connected header = %q, want connected status after app title", connected)
	}

	m.runtimeConnected = false
	disconnected := plainText(m.renderHeader())
	if !strings.Contains(disconnected, "qubit  disconnected  Default chat") {
		t.Fatalf("disconnected header = %q, want disconnected status after app title", disconnected)
	}
}
