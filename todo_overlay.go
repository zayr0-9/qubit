package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const maxTodoOverlayRows = 8

type todoOverlayItem struct {
	Text      string
	Done      bool
	IsTask    bool
	Truncated bool
}

type todoOverlayListEntry struct {
	ID         string
	ModifiedAt string
}

type todoOverlaySnapshot struct {
	Action    string
	Name      string
	Message   string
	Err       string
	Items     []todoOverlayItem
	Lists     []todoOverlayListEntry
	Completed int
	Total     int
}

func (m model) renderTodoOverlay(maxHeight int) string {
	if m.mode != modeChat || m.permissionMode == permissionModeAsk || maxHeight <= 0 {
		return ""
	}
	call, ok := latestTodoToolCall(m.messages)
	if !ok {
		return ""
	}
	snapshot, ok := todoOverlaySnapshotFromCall(call)
	if !ok {
		return ""
	}
	return renderTodoOverlaySnapshot(snapshot, m.width, maxHeight)
}

func latestTodoToolCall(messages []chatMessage) (toolCallUI, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		group := messages[i].ToolGroup
		if messages[i].Role != "tool" || group == nil || group.Name != "todoMd" {
			continue
		}
		for j := len(group.Calls) - 1; j >= 0; j-- {
			call := group.Calls[j]
			if call.Name == "" {
				call.Name = group.Name
			}
			if call.Result != nil || call.Status != "running" {
				return call, true
			}
		}
	}
	return toolCallUI{}, false
}

func todoOverlaySnapshotFromCall(call toolCallUI) (todoOverlaySnapshot, bool) {
	result := call.Result
	data := unwrapTodoResultData(result)
	action := firstNonEmpty(stringValue(call.Args, "action"), stringValue(data, "action"))
	if action == "" && isArrayLikeTodoResult(result) {
		action = "list"
	}
	name := firstNonEmpty(stringValue(call.Args, "name"), stringValue(data, "id"), stringValue(data, "name"))

	snapshot := todoOverlaySnapshot{
		Action:  action,
		Name:    name,
		Message: firstNonEmpty(stringValue(data, "message"), stringValue(result, "message")),
		Err:     firstNonEmpty(stringValue(result, "error"), stringValue(data, "error")),
	}
	if ok, found := boolValue(result, "ok"); found && !ok && snapshot.Err == "" {
		snapshot.Err = fallback(stringValue(result, "error"), "todo tool failed")
	}

	if action == "list" {
		snapshot.Lists = todoOverlayListEntries(result, data)
		return snapshot, len(snapshot.Lists) > 0 || snapshot.Err != ""
	}

	content := firstNonEmpty(
		stringValue(data, "content"),
		stringValue(result, "content"),
		stringValue(data, "contentPreview"),
		stringValue(result, "contentPreview"),
	)
	if content == "" && action == "read" {
		if exists, found := boolValue(data, "exists"); found && !exists {
			snapshot.Err = "todo list not found"
		}
	}
	if content != "" {
		snapshot.Items, snapshot.Completed, snapshot.Total = parseTodoOverlayContent(content)
	}
	return snapshot, content != "" || snapshot.Message != "" || snapshot.Err != ""
}

func unwrapTodoResultData(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	if data, ok := result["data"].(map[string]any); ok {
		return data
	}
	return result
}

func isArrayLikeTodoResult(result map[string]any) bool {
	if result == nil {
		return false
	}
	_, ok := result["items"].([]any)
	if ok {
		return true
	}
	_, ok = result["data"].([]any)
	return ok
}

func todoOverlayListEntries(result map[string]any, data map[string]any) []todoOverlayListEntry {
	items := arrayValue(result, "data")
	if len(items) == 0 {
		items = arrayValue(result, "items")
	}
	if len(items) == 0 {
		items = arrayValue(data, "items")
	}
	entries := make([]todoOverlayListEntry, 0, len(items))
	for i := range items {
		item := mapFromArray(items, i)
		id := firstNonEmpty(stringValue(item, "id"), stringValue(item, "name"))
		if id == "" {
			continue
		}
		entries = append(entries, todoOverlayListEntry{ID: id, ModifiedAt: stringValue(item, "modifiedAt")})
	}
	return entries
}

func parseTodoOverlayContent(content string) ([]todoOverlayItem, int, int) {
	content = normalizeInputNewlines(content)
	items := []todoOverlayItem{}
	completed := 0
	total := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if text, done, ok := parseTodoCheckboxLine(trimmed); ok {
			items = append(items, todoOverlayItem{Text: text, Done: done, IsTask: true})
			total++
			if done {
				completed++
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if heading != "" {
				items = append(items, todoOverlayItem{Text: heading})
			}
		}
	}
	return items, completed, total
}

func parseTodoCheckboxLine(line string) (string, bool, bool) {
	if len(line) < 6 {
		return "", false, false
	}
	marker := line[0]
	if marker != '-' && marker != '*' && marker != '+' {
		return "", false, false
	}
	rest := strings.TrimSpace(line[1:])
	if len(rest) < 4 || rest[0] != '[' || rest[2] != ']' {
		return "", false, false
	}
	state := rest[1]
	if state != ' ' && state != 'x' && state != 'X' {
		return "", false, false
	}
	text := strings.TrimSpace(rest[3:])
	if text == "" {
		text = "(untitled task)"
	}
	return text, state == 'x' || state == 'X', true
}

func renderTodoOverlaySnapshot(snapshot todoOverlaySnapshot, width int, maxHeight int) string {
	panelWidth := min(max(44, width-8), 96)
	contentWidth := max(20, panelWidth-6)
	rowBudget := min(maxTodoOverlayRows, max(1, maxHeight-2))

	lines := []string{renderTodoOverlayHeader(snapshot, contentWidth)}
	if snapshot.Err != "" {
		lines = append(lines, errSt.Render("! "+oneLine(snapshot.Err, contentWidth-2)))
	} else if snapshot.Action == "list" {
		lines = append(lines, renderTodoOverlayListRows(snapshot.Lists, contentWidth, rowBudget-1)...)
	} else if len(snapshot.Items) > 0 {
		lines = append(lines, renderTodoOverlayItemRows(snapshot.Items, contentWidth, rowBudget-1)...)
	} else if snapshot.Message != "" {
		lines = append(lines, mutedSt.Render(oneLine(snapshot.Message, contentWidth)))
	} else {
		lines = append(lines, mutedSt.Render("No todo items in latest result"))
	}

	if len(lines) > rowBudget {
		lines = lines[:rowBudget]
	}
	body := strings.Join(lines, "\n")
	return inputStyle.Width(width).Render(renderAccentBorderedPanel(body, panelWidth))
}

func renderTodoOverlayHeader(snapshot todoOverlaySnapshot, width int) string {
	title := "todo"
	if snapshot.Name != "" {
		title += " · " + snapshot.Name
	}
	if snapshot.Total > 0 {
		title += fmt.Sprintf(" · %d/%d done", snapshot.Completed, snapshot.Total)
	} else if snapshot.Action != "" {
		title += " · " + snapshot.Action
	}
	icon := lipgloss.NewStyle().Foreground(accent).Render("✦")
	if snapshot.Total > 0 && snapshot.Completed == snapshot.Total {
		icon = okSt.Render("✓")
	}
	if snapshot.Err != "" {
		icon = errSt.Render("!")
	}
	return icon + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(oneLine(title, max(8, width-2)))
}

func renderTodoOverlayItemRows(items []todoOverlayItem, width int, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	rows := make([]string, 0, min(len(items), maxRows))
	for i, item := range items {
		if len(rows) >= maxRows {
			rows[len(rows)-1] = mutedSt.Render(fmt.Sprintf("… %d more", len(items)-i+1))
			break
		}
		if item.IsTask {
			marker := mutedSt.Render("○")
			label := oneLine(item.Text, max(8, width-3))
			if item.Done {
				marker = okSt.Render("●")
				label = mutedSt.Render(label)
			}
			rows = append(rows, marker+" "+label)
			continue
		}
		rows = append(rows, mutedSt.Render("# "+oneLine(item.Text, max(8, width-2))))
	}
	return rows
}

func renderTodoOverlayListRows(entries []todoOverlayListEntry, width int, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	if len(entries) == 0 {
		return []string{mutedSt.Render("no todo lists")}
	}
	rows := make([]string, 0, min(len(entries), maxRows))
	for i, entry := range entries {
		if len(rows) >= maxRows {
			rows[len(rows)-1] = mutedSt.Render(fmt.Sprintf("… %d more", len(entries)-i+1))
			break
		}
		meta := ""
		if entry.ModifiedAt != "" {
			meta = mutedSt.Render(" · " + oneLine(entry.ModifiedAt, 19))
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(cyan).Render(oneLine(entry.ID, max(8, width-lipgloss.Width(stripANSI(meta))-2)))+meta)
	}
	return rows
}
