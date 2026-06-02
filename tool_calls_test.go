package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestToolCallEventsGroupSameTool(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read files"}}

	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 1, ToolCallID: "call_a", ToolName: "readFile", Status: "running", Args: map[string]any{"path": "agent.md"}})
	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 1, ToolCallID: "call_b", ToolName: "readFile", Status: "running", Args: map[string]any{"path": "agent_tools.md"}})

	if len(m.messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(m.messages))
	}
	group := m.messages[1].ToolGroup
	if group == nil {
		t.Fatal("tool group missing")
	}
	if len(group.Calls) != 2 {
		t.Fatalf("tool call count = %d, want 2", len(group.Calls))
	}
	for i := 0; m.toolCallRevealing && i < 20; i++ {
		updatedModel, _ := m.updateToolCallRevealTick()
		m = updatedModel.(model)
	}
	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "Read 2 files") {
		t.Fatalf("viewport = %q, want grouped read label", viewport)
	}
}

func TestToolCallFinishUpdatesDetails(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "search"}}

	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 1, ToolCallID: "rg_1", ToolName: "ripgrep", Status: "running", Args: map[string]any{"pattern": "tool.call", "searchPath": "D:\\qubit"}})
	m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: 1, ToolCallID: "rg_1", ToolName: "ripgrep", Status: "completed", Result: map[string]any{"matchCount": float64(7)}, DurationMs: 12})

	group := m.messages[1].ToolGroup
	if got := group.status(); got != "completed" {
		t.Fatalf("status = %q, want completed", got)
	}
	if !strings.Contains(plainText(m.viewport.View()), "7 matches") {
		t.Fatalf("viewport = %q, want match count", m.viewport.View())
	}
}

func TestToolCallFinishWithoutStartCreatesGroup(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "unknown"}}

	m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: 1, ToolCallID: "bad_1", ToolName: "missingTool", Status: "unknown_tool", Result: map[string]any{"error": "Unknown tool"}})

	if len(m.messages) != 2 || m.messages[1].ToolGroup == nil {
		t.Fatalf("messages = %#v, want tool group", m.messages)
	}
	if got := m.messages[1].ToolGroup.status(); got != "failed" {
		t.Fatalf("group status = %q, want failed", got)
	}
}

func TestToolGroupExpandedRenderingAndMouseToggle(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "completed",
		Args:       map[string]any{"path": "agent.md"},
		Result:     map[string]any{"totalLines": float64(42), "contentPreview": "# Qubit Agent Guide"},
	})

	if len(m.toolHitboxes) == 0 {
		t.Fatal("tool hitbox missing")
	}
	updated := m.updateMouseClick(tea.MouseClickMsg{X: 2, Y: m.chatTopY + m.toolHitboxes[0].StartY - m.viewport.YOffset(), Button: tea.MouseLeft}).(model)
	if !updated.messages[1].ToolGroup.Expanded {
		t.Fatal("tool group not expanded after click")
	}
	viewport := plainText(updated.viewport.View())
	if !strings.Contains(viewport, "# Qubit Agent Guide") || !strings.Contains(viewport, "lines: 42") {
		t.Fatalf("viewport = %q, want expanded details", viewport)
	}
}

func TestToolGroupHitboxAlignsWithRenderedRow(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "completed",
		Args:       map[string]any{"path": "agent.md"},
		Result:     map[string]any{"totalLines": float64(42), "contentPreview": "# Qubit Agent Guide"},
	})

	if len(m.toolHitboxes) == 0 {
		t.Fatal("tool hitbox missing")
	}
	visibleToolLine := -1
	for i, line := range strings.Split(plainText(m.viewport.View()), "\n") {
		if strings.Contains(line, "Read 1 file") {
			visibleToolLine = i + m.viewport.YOffset()
			break
		}
	}
	if visibleToolLine < 0 {
		t.Fatalf("viewport = %q, want rendered tool row", plainText(m.viewport.View()))
	}
	if got := m.toolHitboxes[0].StartY; got != visibleToolLine {
		t.Fatalf("hitbox startY = %d, rendered row = %d", got, visibleToolLine)
	}

	updated := m.updateMouseClick(tea.MouseClickMsg{X: 2, Y: m.chatTopY + visibleToolLine - m.viewport.YOffset(), Button: tea.MouseLeft}).(model)
	if !updated.messages[1].ToolGroup.Expanded {
		t.Fatal("click on visible tool row did not expand group")
	}
}

func TestApplySessionMessagesCanLoadPersistedToolGroups(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()

	loaded := []chatMessage{
		{Role: "user", Content: "read it"},
		{Role: "tool", ToolGroup: &toolGroup{ID: "stored-tool-1-readFile-0", Name: "readFile", Step: 1, Calls: []toolCallUI{{ID: "readFile-1-0", Name: "readFile", Step: 1, Status: "completed", Args: map[string]any{"path": "agent.md"}, Result: map[string]any{"totalLines": float64(42), "contentPreview": "# Qubit Agent Guide"}}}}},
		{Role: "assistant", Content: "done"},
	}
	m.applySessionMessages(runtimeEvent{Type: "session.messages", SessionID: "sess_1", Messages: loaded})

	if len(m.messages) != 3 || m.messages[1].ToolGroup == nil {
		t.Fatalf("messages = %#v, want persisted tool group", m.messages)
	}
	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "Read 1 file") || !strings.Contains(viewport, "done") {
		t.Fatalf("viewport = %q, want loaded tool group and assistant", viewport)
	}
}

func TestExpandedToolGroupHitboxOnlyCoversHeader(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "completed",
		Args:       map[string]any{"path": "agent.md"},
		Result:     map[string]any{"totalLines": float64(42), "contentPreview": "# Qubit Agent Guide"},
	})
	m.messages[1].ToolGroup.Expanded = true
	m.refreshViewport()

	if len(m.toolHitboxes) != 1 {
		t.Fatalf("hitboxes = %d, want 1", len(m.toolHitboxes))
	}
	if m.toolHitboxes[0].StartY != m.toolHitboxes[0].EndY {
		t.Fatalf("hitbox = %#v, want single header row", m.toolHitboxes[0])
	}

	detailClickY := m.chatTopY + m.toolHitboxes[0].StartY - m.viewport.YOffset() + 1
	updated := m.updateMouseClick(tea.MouseClickMsg{X: 4, Y: detailClickY, Button: tea.MouseLeft}).(model)
	if !updated.messages[1].ToolGroup.Expanded {
		t.Fatal("detail-row click collapsed group; want only header row clickable")
	}
}

func TestToolGroupHidesDevDetailsByDefault(t *testing.T) {
	t.Setenv("QUBIT_DEV_TOOL_DETAILS", "")
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "completed",
		Args:       map[string]any{"path": "agent.md"},
		Result:     map[string]any{"totalLines": float64(42), "sizeBytes": float64(1234), "contentPreview": "# Qubit Agent Guide"},
		DurationMs: 9,
	})
	m.messages[1].ToolGroup.Expanded = true
	m.refreshViewport()

	viewport := plainText(m.viewport.View())
	if strings.Contains(viewport, "duration:") || strings.Contains(viewport, "size:") || strings.Contains(viewport, "9ms") {
		t.Fatalf("viewport = %q, want duration/size hidden without dev flag", viewport)
	}
}

func TestToolGroupShowsDevDetailsWithFlag(t *testing.T) {
	t.Setenv("QUBIT_DEV_TOOL_DETAILS", "1")
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "completed",
		Args:       map[string]any{"path": "agent.md"},
		Result:     map[string]any{"totalLines": float64(42), "sizeBytes": float64(1234), "contentPreview": "# Qubit Agent Guide"},
		DurationMs: 9,
	})
	m.messages[1].ToolGroup.Expanded = true
	m.refreshViewport()

	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "duration: 9ms") || !strings.Contains(viewport, "size: 1234 bytes") || !strings.Contains(viewport, "· 9ms") {
		t.Fatalf("viewport = %q, want duration/size with dev flag", viewport)
	}
	if strings.Contains(viewport, "completed · 9ms") {
		t.Fatalf("viewport = %q, want status conveyed by color rather than completed text", viewport)
	}
}

func TestToolCallStartAnimatesLabelReveal(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "read"}}

	m.applyToolCallStart(runtimeEvent{
		Type:       "tool.call.start",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "read_1",
		ToolName:   "readFile",
		Status:     "running",
		Args:       map[string]any{"path": "agent.md"},
	})

	if !m.toolCallRevealing {
		t.Fatal("toolCallRevealing = false, want true")
	}
	initialViewport := plainText(m.viewport.View())
	if strings.Contains(initialViewport, "Read 1 file") {
		t.Fatalf("viewport = %q, want partial tool label before reveal ticks", initialViewport)
	}

	updatedModel, cmd := m.updateToolCallRevealTick()
	m = updatedModel.(model)
	if cmd == nil {
		t.Fatal("first reveal tick returned nil command, want next tick")
	}
	if !m.toolCallRevealing {
		t.Fatal("toolCallRevealing = false after partial tick, want true")
	}
	if m.toolCallRevealVisibleRunes != 4 {
		t.Fatalf("visible runes = %d, want one-rune reveal step for short label", m.toolCallRevealVisibleRunes)
	}
	if strings.Contains(plainText(m.viewport.View()), "Read 1 file") {
		t.Fatalf("viewport = %q, want short label to remain partial after one tick", plainText(m.viewport.View()))
	}

	for i := 0; m.toolCallRevealing && i < 20; i++ {
		updatedModel, _ = m.updateToolCallRevealTick()
		m = updatedModel.(model)
	}
	if m.toolCallRevealing {
		t.Fatal("toolCallRevealing = true after draining ticks, want false")
	}
	if !strings.Contains(plainText(m.viewport.View()), "Read 1 file") {
		t.Fatalf("viewport = %q, want full tool label after reveal", plainText(m.viewport.View()))
	}
}

func TestToolCallFinishCompletesActiveReveal(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "search"}}

	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 1, ToolCallID: "rg_1", ToolName: "ripgrep", Status: "running", Args: map[string]any{"pattern": "tool.call"}})
	m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: 1, ToolCallID: "rg_1", ToolName: "ripgrep", Status: "completed", Result: map[string]any{"matchCount": float64(7)}})

	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "Searched 1 time") || !strings.Contains(viewport, "7 matches") {
		t.Fatalf("viewport = %q, want completed full tool label", viewport)
	}
}

func TestEditToolGroupRendersInlineDiff(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "edit"}}
	m.applyToolCallFinish(runtimeEvent{
		Type:       "tool.call.finish",
		SessionID:  "sess_1",
		Step:       1,
		ToolCallID: "edit_1",
		ToolName:   "editFile",
		Status:     "completed",
		Args: map[string]any{
			"path":               "main.go",
			"operation":          "replace_first",
			"searchPreview":      "old line",
			"replacementPreview": "new line",
		},
		Result: map[string]any{"lineInfo": map[string]any{"oldStartLine": float64(12), "newStartLine": float64(12)}},
	})

	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "Edited 1 file") || !strings.Contains(viewport, "-12") || !strings.Contains(viewport, "+12") || !strings.Contains(viewport, "old line") || !strings.Contains(viewport, "new line") {
		t.Fatalf("viewport = %q, want inline edit diff with line numbers", viewport)
	}
}

func TestMultiCallSyntheticEditEventsRenderAsInlineDiff(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_1"
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "user", Content: "multi edit"}}

	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 2, ToolCallID: "multi-edit-1", ToolName: "editFile", Status: "running", Args: map[string]any{"path": "a.go", "operation": "replace_first", "searchPreview": "old a", "replacementPreview": "new a"}})
	m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: 2, ToolCallID: "multi-edit-1", ToolName: "editFile", Status: "completed", Args: map[string]any{"path": "a.go", "operation": "replace_first", "searchPreview": "old a", "replacementPreview": "new a"}, Result: map[string]any{"lineInfo": map[string]any{"oldStartLine": float64(3), "newStartLine": float64(3)}}})
	m.applyToolCallStart(runtimeEvent{Type: "tool.call.start", SessionID: "sess_1", Step: 3, ToolCallID: "multi-edit-2", ToolName: "editFile", Status: "running", Args: map[string]any{"path": "b.go", "operation": "replace_first", "searchPreview": "old b", "replacementPreview": "new b"}})
	m.applyToolCallFinish(runtimeEvent{Type: "tool.call.finish", SessionID: "sess_1", Step: 3, ToolCallID: "multi-edit-2", ToolName: "editFile", Status: "completed", Args: map[string]any{"path": "b.go", "operation": "replace_first", "searchPreview": "old b", "replacementPreview": "new b"}, Result: map[string]any{"lineInfo": map[string]any{"oldStartLine": float64(7), "newStartLine": float64(7)}}})

	viewport := plainText(m.viewport.View())
	if !strings.Contains(viewport, "old a") || !strings.Contains(viewport, "new a") || !strings.Contains(viewport, "old b") || !strings.Contains(viewport, "new b") {
		t.Fatalf("viewport = %q, want synthetic nested edit events rendered as inline diffs", viewport)
	}
}
