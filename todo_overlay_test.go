package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func todoOverlayTestModel() model {
	m := initialModel(nil)
	m.permissionMode = permissionModeAlwaysAllow
	return m
}

func TestTodoOverlayLatestTodoResultWins(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{
		{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "old", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "create"}, Result: map[string]any{"id": "old-list", "content": "- [ ] old task\n"}}}}},
		{Role: "assistant", Content: "working"},
		{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "new", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "edit", "name": "new-list"}, Result: map[string]any{"success": true, "content": "# Sprint\n- [x] inspect\n- [ ] implement\n"}}}}},
	}

	m.todoOverlayExpanded = true
	rendered := stripANSI(m.renderTodoOverlay(10))
	for _, want := range []string{"todo · new-list · 1/2 done", "Sprint", "inspect", "implement"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("todo overlay missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "old task") || strings.Contains(rendered, "old-list") {
		t.Fatalf("todo overlay used stale todo result:\n%s", rendered)
	}
}

func TestTodoOverlaySupportsWrappedToolResult(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "read", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "read", "name": "wrapped"}, Result: map[string]any{"ok": true, "data": map[string]any{"exists": true, "content": "- [X] done\n- [ ] next\n"}}}}}}}

	m.todoOverlayExpanded = true
	rendered := stripANSI(m.renderTodoOverlay(10))
	if !strings.Contains(rendered, "todo · wrapped · 1/2 done") || !strings.Contains(rendered, "done") || !strings.Contains(rendered, "next") {
		t.Fatalf("wrapped todo result overlay = %q, want parsed tasks", rendered)
	}
}

func TestTodoOverlayParsesSummarizedContentPreview(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "edit", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "edit", "name": "preview-list"}, Result: map[string]any{"ok": true, "success": true, "message": "Updated 1 line(s)", "contentPreview": "- [x] done\n- [ ] open\n"}}}}}}

	m.todoOverlayExpanded = true
	rendered := stripANSI(m.renderTodoOverlay(10))
	for _, want := range []string{"todo · preview-list · 1/2 done", "● done", "○ open"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("todo overlay missing summarized preview %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Updated 1 line") {
		t.Fatalf("todo overlay rendered edit message instead of list content:\n%s", rendered)
	}
}

func TestTodoOverlayRendersListResults(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "list", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "list"}, Result: map[string]any{"data": []any{
		map[string]any{"id": "alpha-list", "modifiedAt": "2026-06-03T10:00:00.000Z"},
		map[string]any{"id": "beta-list", "modifiedAt": "2026-06-03T09:00:00.000Z"},
	}}}}}}}

	m.todoOverlayExpanded = true
	rendered := stripANSI(m.renderTodoOverlay(10))
	if !strings.Contains(rendered, "todo · list") || !strings.Contains(rendered, "alpha-list") || !strings.Contains(rendered, "beta-list") {
		t.Fatalf("list todo overlay = %q, want list entries", rendered)
	}
}

func TestTodoOverlayThemeColorsBorderAndNoBackground(t *testing.T) {
	applyTheme(defaultTheme())
	defer applyTheme(defaultTheme())
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "create", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "create"}, Result: map[string]any{"id": "theme-list", "content": "- [x] colored\n- [ ] open\n"}}}}}}

	m.todoOverlayExpanded = true
	rendered := m.renderTodoOverlay(10)
	if !strings.Contains(rendered, "\x1b[38;2;232;161;93m") || !strings.Contains(rendered, "\x1b[38;2;155;226;143m") {
		t.Fatalf("todo overlay missing default accent/green foreground colors: %q", rendered)
	}
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("todo overlay emitted background ANSI sequence: %q", rendered)
	}
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "● colored") || !strings.Contains(plain, "○ open") {
		t.Fatalf("todo overlay missing aesthetic done/not-done icons: %q", plain)
	}
	if !strings.Contains(plain, "╭") || !strings.Contains(plain, "│") || !strings.Contains(plain, "╰") {
		t.Fatalf("todo overlay should render a bordered panel: %q", plain)
	}
}

func TestTodoOverlayReducesViewportHeightInLayout(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "create", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "create"}, Result: map[string]any{"id": "layout-list", "content": "- [ ] reserve space\n"}}}}}}

	without := m
	without.messages = nil
	without.layout()
	withOverlay := m
	withOverlay.layout()
	withOverlay.todoOverlayExpanded = true
	withOverlay.layout()

	if withOverlay.viewport.Height() >= without.viewport.Height() {
		t.Fatalf("viewport height with overlay = %d, without = %d; want overlay to reserve space", withOverlay.viewport.Height(), without.viewport.Height())
	}
	view := stripANSI(withOverlay.View().Content)
	if !strings.Contains(view, "todo · layout-list · 0/1 done") || !strings.Contains(view, "reserve space") {
		t.Fatalf("full view missing todo overlay:\n%s", view)
	}
}

func TestTodoOverlayHiddenInPlanMode(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.permissionMode = permissionModeAsk
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "create", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "create"}, Result: map[string]any{"id": "hidden-list", "content": "- [ ] hidden\n"}}}}}}

	if rendered := m.renderTodoOverlay(10); rendered != "" {
		t.Fatalf("renderTodoOverlay in plan mode = %q, want hidden", rendered)
	}
}

func TestTodoOverlayCollapsedByDefault(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.layout()
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "collapsed", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "read", "name": "collapsed-list"}, Result: map[string]any{"content": "- [x] done\n- [ ] open\n"}}}}}}
	m.layout()

	rendered := stripANSI(m.renderTodoOverlay(10))
	if !strings.Contains(rendered, "✦ todo · collapsed-list · 1/2 done") {
		t.Fatalf("collapsed todo overlay = %q, want header", rendered)
	}
	if strings.Contains(rendered, "● done") || strings.Contains(rendered, "○ open") || strings.Contains(rendered, "╭") {
		t.Fatalf("collapsed todo overlay should hide rows and border: %q", rendered)
	}
}

func TestTodoOverlayClickTogglesExpanded(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "toggle", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "read", "name": "toggle-list"}, Result: map[string]any{"content": "- [x] done\n- [ ] open\n"}}}}}}
	m.layout()
	bounds := m.todoOverlayBounds()
	if !bounds.Visible {
		t.Fatal("todo overlay bounds not visible")
	}

	clicked := m.updateMouseClick(tea.MouseClickMsg{X: bounds.StartX + 1, Y: bounds.HeaderStartY, Button: tea.MouseLeft}).(model)
	updatedModel, cmd := clicked.updateMouseRelease(tea.MouseReleaseMsg{X: bounds.StartX + 1, Y: bounds.HeaderStartY, Button: tea.MouseLeft})
	if cmd != nil {
		t.Fatalf("todo toggle command = %v, want nil", cmd)
	}
	updated := updatedModel.(model)
	if !updated.todoOverlayExpanded {
		t.Fatal("todo overlay did not expand after header click")
	}
	rendered := stripANSI(updated.renderTodoOverlay(10))
	if !strings.Contains(rendered, "● done") || !strings.Contains(rendered, "○ open") || !strings.Contains(rendered, "╭") {
		t.Fatalf("expanded todo overlay missing bordered rows: %q", rendered)
	}
}

func TestTodoOverlayMouseWheelScrollsExpandedList(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "scroll", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "read", "name": "scroll-list"}, Result: map[string]any{"content": "- [ ] one\n- [ ] two\n- [ ] three\n- [ ] four\n- [ ] five\n- [ ] six\n- [ ] seven\n- [ ] eight\n"}}}}}}
	m.layout()
	m.todoOverlayExpanded = true
	m.layout()
	bounds := m.todoOverlayBounds()
	if !bounds.Visible {
		t.Fatal("todo overlay bounds not visible")
	}

	updated := m.updateMouseWheelRouted(tea.MouseWheelMsg{X: bounds.StartX + 1, Y: bounds.StartY + 2, Button: tea.MouseWheelDown}).(model)
	if updated.todoOverlayScroll <= 0 {
		t.Fatalf("todoOverlayScroll = %d, want > 0", updated.todoOverlayScroll)
	}
	rendered := stripANSI(updated.renderTodoOverlay(6))
	if !strings.Contains(rendered, "↓") && !strings.Contains(rendered, "↑") {
		t.Fatalf("scrolled todo overlay missing scroll hint: %q", rendered)
	}
}

func TestTodoOverlayExpandStartsAtBottomForScrollableList(t *testing.T) {
	m := todoOverlayTestModel()
	m.width = 100
	m.height = 30
	m.messages = []chatMessage{{Role: "tool", ToolGroup: &toolGroup{Name: "todoMd", Calls: []toolCallUI{{ID: "bottom", Name: "todoMd", Status: "completed", Args: map[string]any{"action": "read", "name": "bottom-list"}, Result: map[string]any{"content": "- [ ] one\n- [ ] two\n- [ ] three\n- [ ] four\n- [ ] five\n- [ ] six\n- [ ] seven\n- [ ] eight\n"}}}}}}
	m.layout()
	bounds := m.todoOverlayBounds()
	clicked := m.updateMouseClick(tea.MouseClickMsg{X: bounds.StartX + 1, Y: bounds.HeaderStartY, Button: tea.MouseLeft}).(model)
	updatedModel, _ := clicked.updateMouseRelease(tea.MouseReleaseMsg{X: bounds.StartX + 1, Y: bounds.HeaderStartY, Button: tea.MouseLeft})
	updated := updatedModel.(model)

	if !updated.todoOverlayExpanded {
		t.Fatal("todo overlay did not expand")
	}
	if updated.todoOverlayScroll <= 0 {
		t.Fatalf("todoOverlayScroll = %d, want bottom offset", updated.todoOverlayScroll)
	}
	rendered := stripANSI(updated.renderTodoOverlay(10))
	if !strings.Contains(rendered, "eight") || strings.Contains(rendered, "○ one") {
		t.Fatalf("expanded todo overlay should show bottom rows, got: %q", rendered)
	}
}
