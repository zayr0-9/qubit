package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	return renderTodoOverlaySnapshot(snapshot, m.width, maxHeight, m.todoOverlayExpanded, m.todoOverlayScroll)
}

func (m model) todoOverlaySnapshot() (todoOverlaySnapshot, string, bool) {
	call, ok := latestTodoToolCall(m.messages)
	if !ok {
		return todoOverlaySnapshot{}, "", false
	}
	snapshot, ok := todoOverlaySnapshotFromCall(call)
	if !ok {
		return todoOverlaySnapshot{}, "", false
	}
	return snapshot, todoOverlayKey(call, snapshot), true
}

func todoOverlayKey(call toolCallUI, snapshot todoOverlaySnapshot) string {
	return strings.Join([]string{call.ID, snapshot.Name, snapshot.Action, fmt.Sprint(snapshot.Completed), fmt.Sprint(snapshot.Total), snapshot.Message, snapshot.Err}, "|")
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
func renderTodoOverlaySnapshot(snapshot todoOverlaySnapshot, width int, maxHeight int, expanded bool, scroll int) string {
	panelWidth := min(max(44, width-8), 96)
	contentWidth := max(20, panelWidth-6)
	header := renderTodoOverlayHeader(snapshot, contentWidth)
	if !expanded {
		return inputStyle.Width(width).Render(header)
	}

	rowBudget := min(maxTodoOverlayRows, max(1, maxHeight-2))
	bodyRows := renderTodoOverlayBodyRows(snapshot, contentWidth)
	capacity := max(0, rowBudget-1)
	bodyRows, scroll = sliceTodoOverlayRows(bodyRows, capacity, scroll)

	lines := append([]string{header}, bodyRows...)
	body := strings.Join(lines, "\n")
	return inputStyle.Width(width).Render(renderAccentBorderedPanel(body, panelWidth))
}

func renderTodoOverlayBodyRows(snapshot todoOverlaySnapshot, width int) []string {
	if snapshot.Err != "" {
		return []string{errSt.Render("! " + oneLine(snapshot.Err, width-2))}
	}
	if snapshot.Action == "list" {
		return renderTodoOverlayListRows(snapshot.Lists, width, 0)
	}
	if len(snapshot.Items) > 0 {
		return renderTodoOverlayItemRows(snapshot.Items, width, 0)
	}
	if snapshot.Message != "" {
		return []string{mutedSt.Render(oneLine(snapshot.Message, width))}
	}
	return []string{mutedSt.Render("No todo items in latest result")}
}

func sliceTodoOverlayRows(rows []string, capacity int, scroll int) ([]string, int) {
	if capacity <= 0 || len(rows) <= capacity {
		return rows[:min(len(rows), max(0, capacity))], 0
	}
	maxScroll := max(0, len(rows)-capacity)
	scroll = clampInt(scroll, 0, maxScroll)
	visible := append([]string{}, rows[scroll:min(len(rows), scroll+capacity)]...)
	if scroll > 0 {
		visible[0] = mutedSt.Render(fmt.Sprintf("↑ %d more", scroll))
	}
	if scroll+capacity < len(rows) {
		visible[len(visible)-1] = mutedSt.Render(fmt.Sprintf("↓ %d more", len(rows)-scroll-capacity))
	}
	return visible, scroll
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
	if snapshot.Err != "" {
		icon = errSt.Render("!")
	}
	return icon + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(oneLine(title, max(8, width-2)))
}

func renderTodoOverlayItemRows(items []todoOverlayItem, width int, maxRows int) []string {
	limit := len(items)
	if maxRows > 0 {
		limit = min(limit, maxRows)
	}
	rows := make([]string, 0, limit)
	for i, item := range items {
		if maxRows > 0 && len(rows) >= maxRows {
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
	if len(entries) == 0 {
		return []string{mutedSt.Render("no todo lists")}
	}
	limit := len(entries)
	if maxRows > 0 {
		limit = min(limit, maxRows)
	}
	rows := make([]string, 0, limit)
	for i, entry := range entries {
		if maxRows > 0 && len(rows) >= maxRows {
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

func (m model) todoOverlayBounds() todoOverlayBounds {
	if m.width <= 0 || m.height <= 0 {
		return todoOverlayBounds{}
	}
	queuedStatus := m.renderQueuedStatus()
	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	preOverlayBottomHeight := lipgloss.Height(queuedStatus) + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	maxHeight := max(0, min(maxTodoOverlayRows+2, m.height-preOverlayBottomHeight-4))
	overlay := m.renderTodoOverlay(maxHeight)
	height := lipgloss.Height(overlay)
	if height <= 0 {
		return todoOverlayBounds{}
	}
	mainHeight := max(0, m.height-preOverlayBottomHeight-height)
	startY := mainHeight + lipgloss.Height(queuedStatus)
	plainLines := strings.Split(stripANSI(overlay), "\n")
	endX := 0
	for _, line := range plainLines {
		endX = max(endX, lipgloss.Width(line)-1)
	}
	if endX < 0 {
		endX = 0
	}
	headerY := startY
	if m.todoOverlayExpanded {
		headerY = min(startY+1, startY+height-1)
	}
	return todoOverlayBounds{Visible: true, HeaderStartY: headerY, HeaderEndY: headerY, StartY: startY, EndY: startY + height - 1, StartX: 0, EndX: endX}
}

func (b todoOverlayBounds) contains(mouse tea.Mouse) bool {
	return b.Visible && mouse.Y >= b.StartY && mouse.Y <= b.EndY && mouse.X >= b.StartX && mouse.X <= b.EndX
}

func (b todoOverlayBounds) containsHeader(mouse tea.Mouse) bool {
	return b.Visible && mouse.Y >= b.HeaderStartY && mouse.Y <= b.HeaderEndY && mouse.X >= b.StartX && mouse.X <= b.EndX
}

func (m *model) syncTodoOverlayState() {
	_, key, ok := m.todoOverlaySnapshot()
	if !ok {
		m.todoOverlayKey = ""
		m.todoOverlayScroll = 0
		m.todoOverlayMouseDownHeader = false
		return
	}
	if key != m.todoOverlayKey {
		m.todoOverlayKey = key
		m.todoOverlayExpanded = false
		m.todoOverlayScroll = 0
		m.todoOverlayMouseDownHeader = false
	}
	m.clampTodoOverlayScroll()
}

func (m *model) clampTodoOverlayScroll() {
	snapshot, _, ok := m.todoOverlaySnapshot()
	if !ok {
		m.todoOverlayScroll = 0
		return
	}
	capacity := m.todoOverlayRowCapacity()
	maxScroll := max(0, len(renderTodoOverlayBodyRows(snapshot, max(20, min(max(44, m.width-8), 96)-6)))-capacity)
	m.todoOverlayScroll = clampInt(m.todoOverlayScroll, 0, maxScroll)
}

func (m model) todoOverlayRowCapacity() int {
	queuedStatus := m.renderQueuedStatus()
	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	preOverlayBottomHeight := lipgloss.Height(queuedStatus) + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	maxHeight := max(0, min(maxTodoOverlayRows+2, m.height-preOverlayBottomHeight-4))
	rowBudget := min(maxTodoOverlayRows, max(1, maxHeight-2))
	return max(0, rowBudget-1)
}

func (m model) updateTodoOverlayMouseWheel(msg tea.MouseWheelMsg) (model, bool) {
	mouse := msg.Mouse()
	bounds := m.todoOverlayBounds()
	if !m.todoOverlayExpanded || !bounds.contains(mouse) {
		return m, false
	}
	delta := max(1, m.viewport.MouseWheelDelta)
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.todoOverlayScroll -= delta
	case tea.MouseWheelDown:
		m.todoOverlayScroll += delta
	default:
		return m, false
	}
	m.clampTodoOverlayScroll()
	return m, true
}

func (m model) updateTodoOverlayMouseClick(mouse tea.Mouse) (model, bool) {
	if mouse.Button != tea.MouseLeft || m.mode != modeChat {
		return m, false
	}
	if !m.todoOverlayBounds().containsHeader(mouse) {
		m.todoOverlayMouseDownHeader = false
		return m, false
	}
	m.todoOverlayMouseDownHeader = true
	return m, true
}

func (m model) updateTodoOverlayMouseRelease(mouse tea.Mouse) (model, bool) {
	if mouse.Button != tea.MouseLeft || m.mode != modeChat || !m.todoOverlayMouseDownHeader {
		return m, false
	}
	m.todoOverlayMouseDownHeader = false
	if !m.todoOverlayBounds().containsHeader(mouse) {
		return m, true
	}
	m.todoOverlayExpanded = !m.todoOverlayExpanded
	if m.todoOverlayExpanded {
		m.scrollTodoOverlayToBottom()
	} else {
		m.clampTodoOverlayScroll()
	}
	m.status = "ready"
	return m, true
}

func (m *model) scrollTodoOverlayToBottom() {
	snapshot, _, ok := m.todoOverlaySnapshot()
	if !ok {
		m.todoOverlayScroll = 0
		return
	}
	capacity := m.todoOverlayRowCapacity()
	contentWidth := max(20, min(max(44, m.width-8), 96)-6)
	m.todoOverlayScroll = max(0, len(renderTodoOverlayBodyRows(snapshot, contentWidth))-capacity)
}
