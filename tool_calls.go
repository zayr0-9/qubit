package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type toolCallUI struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Step         int            `json:"step"`
	Status       string         `json:"status"`
	Args         map[string]any `json:"args,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
	ContextChars int            `json:"contextChars,omitempty"`
	StartedAt    string         `json:"startedAt,omitempty"`
	FinishedAt   string         `json:"finishedAt,omitempty"`
	DurationMs   int            `json:"durationMs,omitempty"`
}

type toolGroup struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Step     int          `json:"step"`
	Calls    []toolCallUI `json:"calls"`
	Expanded bool         `json:"expanded,omitempty"`
}

type toolHitbox struct {
	Kind         string
	GroupID      string
	MessageIndex int
	StartY       int
	EndY         int
	StartX       int
	EndX         int
}

func (m *model) applyToolCallStart(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
	group := m.ensureToolGroup(ev.ToolName, ev.Step)
	call := toolCallFromEvent(ev)
	if call.Status == "" {
		call.Status = "running"
	}
	upsertToolCall(group, call)
	m.startToolCallReveal(group)
	m.status = "using tools"
	m.refreshViewport()
}

func (m *model) applyToolCallFinish(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
	group := m.ensureToolGroup(ev.ToolName, ev.Step)
	call := toolCallFromEvent(ev)
	upsertToolCall(group, call)
	m.completeToolCallReveal(group)
	m.status = "thinking"
	m.refreshViewport()
}

func toolCallFromEvent(ev runtimeEvent) toolCallUI {
	return toolCallUI{
		ID:           ev.ToolCallID,
		Name:         ev.ToolName,
		Step:         ev.Step,
		Status:       ev.Status,
		Args:         ev.Args,
		Result:       ev.Result,
		ContextChars: ev.ContextChars,
		StartedAt:    ev.StartedAt,
		FinishedAt:   ev.FinishedAt,
		DurationMs:   ev.DurationMs,
	}
}

func (m *model) startToolCallReveal(group *toolGroup) {
	messageIndex := m.toolGroupMessageIndex(group)
	if messageIndex < 0 {
		return
	}
	labelRunes := len([]rune(toolGroupLabel(group)))
	m.toolCallRevealing = true
	m.toolCallRevealMessageIndex = messageIndex
	m.toolCallRevealVisibleRunes = min(3, max(1, labelRunes))
}

func (m *model) completeToolCallReveal(group *toolGroup) {
	if group == nil || !m.toolCallRevealing {
		return
	}
	messageIndex := m.toolGroupMessageIndex(group)
	if messageIndex != m.toolCallRevealMessageIndex {
		return
	}
	m.toolCallRevealVisibleRunes = len([]rune(toolGroupLabel(group)))
}

func (m model) updateToolCallRevealTick() (tea.Model, tea.Cmd) {
	if !m.toolCallRevealing {
		return m, nil
	}
	if m.toolCallRevealMessageIndex < 0 || m.toolCallRevealMessageIndex >= len(m.messages) {
		m.clearToolCallReveal()
		return m, nil
	}
	group := m.messages[m.toolCallRevealMessageIndex].ToolGroup
	if group == nil {
		m.clearToolCallReveal()
		return m, nil
	}
	labelRunes := len([]rune(toolGroupLabel(group)))
	if m.toolCallRevealVisibleRunes < labelRunes {
		m.toolCallRevealVisibleRunes = min(labelRunes, m.toolCallRevealVisibleRunes+toolCallRevealChunkSize(labelRunes, m.toolCallRevealVisibleRunes))
		m.refreshViewport()
		return m, toolCallRevealTick()
	}
	m.clearToolCallReveal()
	m.refreshViewport()
	return m, nil
}

func (m *model) clearToolCallReveal() {
	m.toolCallRevealing = false
	m.toolCallRevealMessageIndex = 0
	m.toolCallRevealVisibleRunes = 0
}

func (m *model) toolGroupMessageIndex(group *toolGroup) int {
	if group == nil {
		return -1
	}
	for i := range m.messages {
		if m.messages[i].ToolGroup == group {
			return i
		}
		if m.messages[i].ToolGroup != nil && m.messages[i].ToolGroup.ID == group.ID {
			return i
		}
	}
	return -1
}

const (
	toolCallRevealTickInterval = 25 * time.Millisecond
	toolCallRevealDuration     = 300 * time.Millisecond
)

func toolCallRevealTick() tea.Cmd {
	return tea.Tick(toolCallRevealTickInterval, func(time.Time) tea.Msg {
		return toolCallRevealTickMsg{}
	})
}

func toolCallRevealChunkSize(totalRunes int, visibleRunes int) int {
	remaining := totalRunes - visibleRunes
	if remaining <= 0 {
		return 0
	}
	steps := max(1, int(toolCallRevealDuration/toolCallRevealTickInterval))
	return max(1, min(remaining, (totalRunes+steps-1)/steps))
}

func (m *model) ensureToolGroup(toolName string, step int) *toolGroup {
	if toolName == "" {
		toolName = "tool"
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		message := &m.messages[i]
		if message.Role != "tool" || message.ToolGroup == nil {
			if message.Role == "assistant" || message.Role == "user" || message.Role == "error" {
				break
			}
			continue
		}
		if message.ToolGroup.Name == toolName && message.ToolGroup.Step == step {
			return message.ToolGroup
		}
	}

	group := &toolGroup{ID: fmt.Sprintf("tool-%d-%s-%d", step, toolName, len(m.messages)), Name: toolName, Step: step}
	m.messages = append(m.messages, chatMessage{Role: "tool", ToolGroup: group})
	return group
}

func upsertToolCall(group *toolGroup, call toolCallUI) {
	if call.ID == "" {
		call.ID = fmt.Sprintf("%s-%d-%d", call.Name, call.Step, len(group.Calls))
	}
	for i := range group.Calls {
		if group.Calls[i].ID != call.ID {
			continue
		}
		if call.Name == "" {
			call.Name = group.Calls[i].Name
		}
		if call.Args == nil {
			call.Args = group.Calls[i].Args
		}
		if call.Result == nil {
			call.Result = group.Calls[i].Result
		}
		if call.ContextChars == 0 {
			call.ContextChars = group.Calls[i].ContextChars
		}
		if call.StartedAt == "" {
			call.StartedAt = group.Calls[i].StartedAt
		}
		group.Calls[i] = call
		return
	}
	group.Calls = append(group.Calls, call)
}

func (g *toolGroup) status() string {
	if g == nil || len(g.Calls) == 0 {
		return "running"
	}
	hasRunning := false
	hasProblem := false
	for _, call := range g.Calls {
		switch call.Status {
		case "running", "":
			hasRunning = true
		case "completed":
		default:
			hasProblem = true
		}
	}
	if hasRunning {
		return "running"
	}
	if hasProblem {
		return "failed"
	}
	return "completed"
}

func (g *toolGroup) totalDurationMs() int {
	total := 0
	for _, call := range g.Calls {
		total += call.DurationMs
	}
	return total
}

func (m *model) renderToolGroup(group *toolGroup, width int) string {
	if group == nil {
		return mutedSt.Render("tool activity")
	}
	line := m.renderToolGroupInline(group)
	if isEditToolGroup(group) {
		diff := m.renderEditToolGroupDiff(group, width)
		if diff != "" {
			line += "\n" + diff
		}
	}
	if !group.Expanded {
		return line
	}
	return line + "\n" + m.renderToolGroupDetails(group, width)
}

func (m *model) renderToolGroupInline(group *toolGroup) string {
	if group == nil {
		return mutedSt.Render("tool activity")
	}
	icon := "▸"
	if group.Expanded {
		icon = "▾"
	}
	status := group.status()
	icon = m.toolStatusStyle(status).Render(icon)
	labelStyle := m.toolKindStyle(group.Name)
	if status == "failed" || status == "denied" || status == "unknown_tool" {
		labelStyle = errSt
	}
	label := labelStyle.Render(m.visibleToolGroupLabel(group))
	line := fmt.Sprintf("%s %s", icon, label)
	if showDevToolDetails() {
		if duration := group.totalDurationMs(); duration > 0 {
			line += mutedSt.Render(fmt.Sprintf(" · %dms", duration))
		}
	}
	return line
}

func (m *model) toolStatusStyle(status string) lipgloss.Style {
	switch status {
	case "completed":
		return okSt
	case "failed", "denied", "unknown_tool":
		return errSt
	default:
		if len(toolStatusPulseStyles) == 0 {
			return mutedSt
		}
		return toolStatusPulseStyles[m.inputCursorPulse%len(toolStatusPulseStyles)]
	}
}

func (m *model) toolKindStyle(toolName string) lipgloss.Style {
	switch toolName {
	case "readFile", "readFileContinuation", "readFiles":
		return toolReadSt
	case "glob", "ripgrep":
		return toolSearchSt
	case "createFile", "editFile", "multiEdit", "deleteFile":
		return toolWriteSt
	case "planMd":
		return toolWriteSt
	case "bash", "powershell":
		return toolShellSt
	default:
		return toolOtherSt
	}
}

func (m *model) visibleToolGroupLabel(group *toolGroup) string {
	label := toolGroupLabel(group)
	if !m.toolCallRevealing || m.toolGroupMessageIndex(group) != m.toolCallRevealMessageIndex {
		return label
	}
	runes := []rune(label)
	visible := min(len(runes), max(1, m.toolCallRevealVisibleRunes))
	return string(runes[:visible])
}

func toolGroupLabel(group *toolGroup) string {
	count := len(group.Calls)
	if count <= 0 {
		count = 1
	}
	switch group.Name {
	case "readFile", "readFileContinuation", "readFiles":
		files := toolGroupFileCount(group)
		if files <= 0 {
			files = count
		}
		return fmt.Sprintf("Read %d %s", files, plural(files, "file", "files"))
	case "glob":
		return fmt.Sprintf("Found files%s", toolGroupMatchSuffix(group))
	case "ripgrep":
		return fmt.Sprintf("Searched %d %s%s", count, plural(count, "time", "times"), toolGroupMatchSuffix(group))
	case "bash":
		return "Ran Bash" + toolGroupCommandSuffix(group)
	case "powershell":
		return "Ran PowerShell" + toolGroupCommandSuffix(group)
	case "createFile":
		return fmt.Sprintf("Created %d %s", count, plural(count, "file", "files"))
	case "editFile", "multiEdit":
		return fmt.Sprintf("Edited %d %s", count, plural(count, "file", "files"))
	case "multiCall":
		nested := toolGroupNestedCallCount(group)
		if nested <= 0 {
			nested = count
		}
		return fmt.Sprintf("Ran %d tool %s", nested, plural(nested, "call", "calls"))
	case "deleteFile":
		return fmt.Sprintf("Deleted %d %s", count, plural(count, "file", "files"))
	case "todoMd":
		return fmt.Sprintf("Updated %d todo %s", count, plural(count, "list", "lists"))
	case "planMd":
		action := "Updated"
		if len(group.Calls) == 1 {
			switch stringValue(group.Calls[0].Args, "action") {
			case "list":
				action = "Listed"
			case "read":
				action = "Read"
			case "display", "view":
				action = "Displayed"
			case "create":
				action = "Created"
			}
		}
		return fmt.Sprintf("%s %d plan %s", action, count, plural(count, "file", "files"))
	default:
		return fmt.Sprintf("Used %s %d %s", group.Name, count, plural(count, "time", "times"))
	}
}

func toolGroupFileCount(group *toolGroup) int {
	files := 0
	for _, call := range group.Calls {
		if paths, ok := call.Args["paths"].([]any); ok {
			files += len(paths)
			continue
		}
		if resultFiles, ok := call.Result["files"].([]any); ok {
			files += len(resultFiles)
			continue
		}
		if stringValue(call.Args, "path") != "" {
			files++
		}
	}
	return files
}

func toolGroupNestedCallCount(group *toolGroup) int {
	total := 0
	for _, call := range group.Calls {
		if calls, ok := call.Args["calls"].([]any); ok {
			total += len(calls)
			continue
		}
		if results, ok := call.Result["results"].([]any); ok {
			total += len(results)
		}
	}
	return total
}

func toolGroupMatchSuffix(group *toolGroup) string {
	total := 0
	seen := false
	for _, call := range group.Calls {
		if n, ok := numberValue(call.Result, "matchCount"); ok {
			total += n
			seen = true
		}
	}
	if !seen {
		return ""
	}
	return fmt.Sprintf(" · %d %s", total, plural(total, "match", "matches"))
}

func toolGroupCommandSuffix(group *toolGroup) string {
	if len(group.Calls) != 1 {
		return ""
	}
	cmd := stringValue(group.Calls[0].Args, "command")
	if cmd == "" {
		return ""
	}
	return ": " + oneLine(cmd, 42)
}

func (m *model) renderToolGroupDetails(group *toolGroup, width int) string {
	detailWidth := max(20, width-4)
	var b strings.Builder
	calls := append([]toolCallUI(nil), group.Calls...)
	if group != nil && group.Name == "multiCall" {
		if nested := multiCallNestedToolCalls(group); len(nested) > 0 {
			calls = nested
		}
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].ID < calls[j].ID })
	for i, call := range calls {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(mutedSt.Render(fmt.Sprintf("  %d. %s", i+1, toolCallTitle(call, detailWidth))))
		for _, line := range toolCallDetailLines(call, detailWidth) {
			b.WriteString("\n")
			b.WriteString(mutedSt.Render("     " + oneLine(line, detailWidth)))
		}
	}
	return b.String()
}

func toolCallTitle(call toolCallUI, width int) string {
	status := call.Status
	if status == "" {
		status = "completed"
	}
	prefix := status
	if call.Name != "" {
		prefix = fmt.Sprintf("%s · %s", status, call.Name)
	}
	if path := stringValue(call.Args, "path"); path != "" {
		return fmt.Sprintf("%s · %s", prefix, oneLine(path, max(8, width-16)))
	}
	if paths, ok := call.Args["paths"].([]any); ok && len(paths) > 0 {
		return fmt.Sprintf("%s · %d files", prefix, len(paths))
	}
	if pattern := stringValue(call.Args, "pattern"); pattern != "" {
		return fmt.Sprintf("%s · %q", prefix, oneLine(pattern, max(8, width-16)))
	}
	if command := stringValue(call.Args, "command"); command != "" {
		return fmt.Sprintf("%s · %s", prefix, oneLine(command, max(8, width-16)))
	}
	return prefix
}

func toolCallDetailLines(call toolCallUI, width int) []string {
	var lines []string
	showDevDetails := showDevToolDetails()
	if call.Name != "" {
		lines = append(lines, fmt.Sprintf("tool: %s", call.Name))
	}
	if showDevDetails && call.DurationMs > 0 {
		lines = append(lines, fmt.Sprintf("duration: %dms", call.DurationMs))
	}
	appendString := func(label, key string, source map[string]any) {
		if value := stringValue(source, key); value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", label, value))
		}
	}
	appendString("cwd", "cwd", call.Args)
	appendString("description", "description", call.Args)
	appendString("message", "message", call.Result)
	if n, ok := numberValue(call.Result, "totalLines"); ok {
		lines = append(lines, fmt.Sprintf("lines: %d", n))
	}
	if showDevDetails {
		if n, ok := numberValue(call.Result, "sizeBytes"); ok {
			lines = append(lines, fmt.Sprintf("size: %d bytes", n))
		}
	}
	if n, ok := numberValue(call.Result, "matchCount"); ok {
		lines = append(lines, fmt.Sprintf("matches: %d", n))
	}
	if n, ok := numberValue(call.Result, "exitCode"); ok {
		lines = append(lines, fmt.Sprintf("exit: %d", n))
	}
	appendPreview := func(label, key string) {
		if preview := stringValue(call.Result, key); preview != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", label, strings.ReplaceAll(oneLine(preview, width), "\t", "  ")))
		}
	}
	appendPreview("preview", "contentPreview")
	appendPreview("stdout", "stdoutPreview")
	appendPreview("stderr", "stderrPreview")
	appendPreview("error", "error")
	if len(lines) == 0 {
		lines = append(lines, "no details")
	}
	return lines
}

func multiCallNestedToolCalls(group *toolGroup) []toolCallUI {
	if group == nil {
		return nil
	}
	var nested []toolCallUI
	for _, call := range group.Calls {
		results := arrayValue(call.Result, "results")
		argsCalls := arrayValue(call.Args, "calls")
		count := max(len(results), len(argsCalls))
		for i := 0; i < count; i++ {
			result := mapFromArray(results, i)
			argCall := mapFromArray(argsCalls, i)
			toolName := firstNonEmpty(stringValue(result, "tool"), stringValue(argCall, "tool"))
			args := objectValue(argCall, "args")
			resultSummary := objectValue(result, "result")
			if toolName == "" && args == nil && resultSummary == nil {
				continue
			}
			status := "failed"
			if ok, found := boolValue(result, "ok"); found && ok {
				status = "completed"
			}
			index := i
			if n, found := numberValue(result, "index"); found {
				index = n
			}
			if toolName == "" {
				toolName = "tool"
			}
			nested = append(nested, toolCallUI{
				ID:     fmt.Sprintf("%s-nested-%d-%s", call.ID, index, toolName),
				Name:   toolName,
				Step:   call.Step,
				Status: status,
				Args:   args,
				Result: resultSummary,
			})
		}
	}
	return nested
}

func plural(n int, one string, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func stringValue(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	value, ok := source[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func numberValue(source map[string]any, key string) (int, bool) {
	if source == nil {
		return 0, false
	}
	value, ok := source[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}

func boolValue(source map[string]any, key string) (bool, bool) {
	if source == nil {
		return false, false
	}
	value, ok := source[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

const toolGroupsPerLine = 4

func (m *model) renderCollapsedToolGroups(groups []*toolGroup, width int) string {
	if len(groups) == 0 {
		return ""
	}
	if len(groups) == 1 {
		return m.renderToolGroup(groups[0], width)
	}
	rows := make([]string, 0, (len(groups)+toolGroupsPerLine-1)/toolGroupsPerLine)
	for start := 0; start < len(groups); start += toolGroupsPerLine {
		end := min(len(groups), start+toolGroupsPerLine)
		rows = append(rows, m.renderToolGroupRow(groups[start:end], width))
	}
	return strings.Join(rows, "\n")
}

func (m *model) renderToolGroupRow(groups []*toolGroup, width int) string {
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, m.renderToolGroupInline(group))
	}
	line := strings.Join(parts, mutedSt.Render("  ·  "))
	for _, group := range groups {
		if isEditToolGroup(group) {
			if diff := m.renderEditToolGroupDiff(group, width); diff != "" {
				line += "\n" + diff
			}
		}
		line += m.expandedToolGroupDetails(group, width)
	}
	return line
}

func (m *model) expandedToolGroupDetails(group *toolGroup, width int) string {
	if group == nil || !group.Expanded {
		return ""
	}
	return "\n" + m.renderToolGroupDetails(group, width)
}

func (m *model) appendToolGroupHitboxes(groups []*toolGroup, startLine int, width int) {
	if len(groups) == 0 {
		return
	}
	lineY := startLine
	separatorWidth := lipgloss.Width(mutedSt.Render("  ·  "))
	for start := 0; start < len(groups); start += toolGroupsPerLine {
		end := min(len(groups), start+toolGroupsPerLine)
		rowGroups := groups[start:end]
		x := 0
		for i, group := range rowGroups {
			if group == nil {
				continue
			}
			segment := m.renderToolGroupInline(group)
			segmentWidth := lipgloss.Width(segment)
			m.toolHitboxes = append(m.toolHitboxes, toolHitbox{Kind: "tool", GroupID: group.ID, StartY: lineY, EndY: lineY, StartX: x, EndX: x + max(0, segmentWidth-1)})
			x += segmentWidth
			if i < len(rowGroups)-1 {
				x += separatorWidth
			}
		}
		lineY += renderedLineCount(m.renderToolGroupRow(rowGroups, width))
	}
}

func (m *model) toggleReasoningBlock(index int) bool {
	if index < 0 || index >= len(m.messages) || m.messages[index].Role != "reasoning" {
		return false
	}
	m.messages[index].Expanded = !m.messages[index].Expanded
	m.refreshViewport()
	return true
}

func (m *model) toggleToolGroup(groupID string) bool {
	for i := range m.messages {
		group := m.messages[i].ToolGroup
		if group == nil || group.ID != groupID {
			continue
		}
		group.Expanded = !group.Expanded
		m.refreshViewport()
		return true
	}
	return false
}

func (m *model) hasRunningToolGroup() bool {
	for i := range m.messages {
		group := m.messages[i].ToolGroup
		if group != nil && group.status() == "running" {
			return true
		}
	}
	return false
}

func showDevToolDetails() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("QUBIT_DEV_TOOL_DETAILS")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func isEditToolGroup(group *toolGroup) bool {
	return group != nil && (group.Name == "editFile" || group.Name == "multiEdit")
}

const maxInlineEditDiffLines = 12

func (m *model) renderEditToolGroupDiff(group *toolGroup, width int) string {
	if group == nil {
		return ""
	}
	detailWidth := max(20, width-4)
	var blocks []string
	shown := 0
	for _, call := range group.Calls {
		if group.Name == "multiEdit" {
			for _, edit := range multiEditDiffItems(call) {
				block := renderEditDiffBlock(edit, detailWidth)
				if block != "" {
					blocks = append(blocks, block)
					shown++
				}
				if shown >= 3 {
					break
				}
			}
		} else {
			block := renderEditDiffBlock(editDiffItemFromCall(call), detailWidth)
			if block != "" {
				blocks = append(blocks, block)
				shown++
			}
		}
		if shown >= 3 {
			break
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	if shown < editDiffCandidateCount(group) {
		blocks = append(blocks, mutedSt.Render("  … more edits"))
	}
	return strings.Join(blocks, "\n")
}

type editDiffItem struct {
	Path        string
	Operation   string
	Search      string
	Replacement string
	Content     string
	LineInfo    map[string]any
}

func editDiffItemFromCall(call toolCallUI) editDiffItem {
	return editDiffItem{
		Path:        stringValue(call.Args, "path"),
		Operation:   stringValue(call.Args, "operation"),
		Search:      stringValue(call.Args, "searchPreview"),
		Replacement: stringValue(call.Args, "replacementPreview"),
		Content:     stringValue(call.Args, "contentPreview"),
		LineInfo:    objectValue(call.Result, "lineInfo"),
	}
}

func multiEditDiffItems(call toolCallUI) []editDiffItem {
	argEdits := arrayValue(call.Args, "edits")
	resultItems := arrayValue(call.Result, "results")
	count := max(len(argEdits), len(resultItems))
	items := make([]editDiffItem, 0, count)
	for i := 0; i < count; i++ {
		args := mapFromArray(argEdits, i)
		result := mapFromArray(resultItems, i)
		items = append(items, editDiffItem{
			Path:        firstNonEmpty(stringValue(result, "path"), stringValue(args, "path")),
			Operation:   firstNonEmpty(stringValue(result, "operation"), stringValue(args, "operation")),
			Search:      firstNonEmpty(stringValue(result, "searchPreview"), stringValue(args, "searchPreview")),
			Replacement: firstNonEmpty(stringValue(result, "replacementPreview"), stringValue(args, "replacementPreview")),
			Content:     firstNonEmpty(stringValue(result, "contentPreview"), stringValue(args, "contentPreview")),
			LineInfo:    objectValue(result, "lineInfo"),
		})
	}
	return items
}

func renderEditDiffBlock(item editDiffItem, width int) string {
	operation := item.Operation
	if operation == "" {
		operation = "replace"
	}
	removed := item.Search
	added := item.Replacement
	if operation == "append" {
		removed = ""
		added = firstNonEmpty(item.Content, item.Replacement)
	}
	if strings.TrimSpace(removed) == "" && strings.TrimSpace(added) == "" {
		return ""
	}

	var b strings.Builder
	if item.Path != "" {
		b.WriteString(mutedSt.Render("  " + oneLine(item.Path, max(12, width-2))))
		b.WriteString("\n")
	}
	lineWidth := max(8, width-10)
	oldLine := numberValueOrDefault(item.LineInfo, "oldStartLine", 0)
	newLine := numberValueOrDefault(item.LineInfo, "newStartLine", oldLine)
	written := 0
	for _, line := range splitDiffLines(removed) {
		if written >= maxInlineEditDiffLines {
			b.WriteString(mutedSt.Render("  … diff truncated"))
			return b.String()
		}
		b.WriteString(renderDiffLine('-', oldLine, line, lineWidth))
		b.WriteString("\n")
		if oldLine > 0 {
			oldLine++
		}
		written++
	}
	addedLines := splitDiffLines(added)
	for i, line := range addedLines {
		if written >= maxInlineEditDiffLines {
			b.WriteString(mutedSt.Render("  … diff truncated"))
			return b.String()
		}
		b.WriteString(renderDiffLine('+', newLine, line, lineWidth))
		if i < len(addedLines)-1 {
			b.WriteString("\n")
		}
		if newLine > 0 {
			newLine++
		}
		written++
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func renderDiffLine(sign rune, lineNumber int, content string, width int) string {
	number := ""
	if lineNumber > 0 {
		number = fmt.Sprintf("%d", lineNumber)
	}
	line := fmt.Sprintf("  %c%-4s  %s", sign, number, oneLine(content, width))
	if sign == '-' {
		return diffRemovedSt.Render(line)
	}
	return diffAddedSt.Render(line)
}

func editDiffCandidateCount(group *toolGroup) int {
	if group == nil {
		return 0
	}
	if group.Name != "multiEdit" {
		return len(group.Calls)
	}
	total := 0
	for _, call := range group.Calls {
		total += max(len(arrayValue(call.Args, "edits")), len(arrayValue(call.Result, "results")))
	}
	return total
}

func objectValue(source map[string]any, key string) map[string]any {
	if source == nil {
		return nil
	}
	if value, ok := source[key].(map[string]any); ok {
		return value
	}
	return nil
}

func arrayValue(source map[string]any, key string) []any {
	if source == nil {
		return nil
	}
	if value, ok := source[key].([]any); ok {
		return value
	}
	return nil
}

func mapFromArray(items []any, index int) map[string]any {
	if index < 0 || index >= len(items) {
		return nil
	}
	if value, ok := items[index].(map[string]any); ok {
		return value
	}
	return nil
}

func numberValueOrDefault(source map[string]any, key string, fallbackValue int) int {
	if n, ok := numberValue(source, key); ok {
		return n
	}
	return fallbackValue
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
