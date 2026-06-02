package main

import (
	"strings"
	"testing"
)

func TestContextStatusForCodexModel(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.model = "gpt-5.2-codex"
	m.maxContext = 400000
	m.messages = []chatMessage{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "reasoning", Content: strings.Repeat("r", 40)},
		{Role: "tool", ToolGroup: &toolGroup{Name: "readFile", Step: 1, Calls: []toolCallUI{{ID: "read-1", Name: "readFile", Status: "completed", Args: map[string]any{"path": "agent.md"}, Result: map[string]any{"totalLines": float64(42)}}}}},
	}

	status := plainText(m.renderInputStatus())
	if !strings.Contains(status, "ctx ") || !strings.Contains(status, "/400k") {
		t.Fatalf("status = %q, want context usage next to mode/reasoning", status)
	}
}

func TestEstimateContextTokensIncludesReasoningAndToolGroups(t *testing.T) {
	messages := []chatMessage{
		{Role: "assistant", Content: "answer", ReasoningContent: strings.Repeat("r", 40)},
		{Role: "tool", ToolGroup: &toolGroup{Name: "bash", Step: 1, Calls: []toolCallUI{{ID: "bash-1", Name: "bash", Args: map[string]any{"command": "echo hello"}, Result: map[string]any{"stdoutPreview": "hello"}}}}},
	}

	withAll := estimateContextTokens(messages)
	withoutExtra := estimateContextTokens([]chatMessage{{Role: "assistant", Content: "answer"}})
	if withAll <= withoutExtra {
		t.Fatalf("with reasoning/tools = %d, without = %d; want reasoning/tool context included", withAll, withoutExtra)
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
