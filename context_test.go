package main

import (
	"strings"
	"testing"
)

func TestContextStatusForCodexModelIncludesLatestUsageLog(t *testing.T) {
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
	if !strings.Contains(status, "ctx ") || !strings.Contains(status, "/400k") || !strings.Contains(status, "log in 12.3k/cache 12k/out 678") {
		t.Fatalf("status = %q, want context usage and latest Codex usage log next to mode/reasoning", status)
	}
}

func TestContextStatusHidesCodexUsageForOtherProviders(t *testing.T) {
	m := initialModel(nil)
	m.provider = "openai"
	m.activeProvider = "openai"
	m.maxContext = 128000
	m.lastCodexUsage = &codexUsage{InputTokens: 12000, OutputTokens: 500}

	status := plainText(m.renderInputStatus())
	if strings.Contains(status, "log in") {
		t.Fatalf("status = %q, want Codex usage hidden for non-Codex providers", status)
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
