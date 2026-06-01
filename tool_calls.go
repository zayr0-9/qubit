package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type toolCallUI struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Step       int            `json:"step"`
	Status     string         `json:"status"`
	Args       map[string]any `json:"args,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	StartedAt  string         `json:"startedAt,omitempty"`
	FinishedAt string         `json:"finishedAt,omitempty"`
	DurationMs int            `json:"durationMs,omitempty"`
}

type toolGroup struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Step     int          `json:"step"`
	Calls    []toolCallUI `json:"calls"`
	Expanded bool         `json:"expanded,omitempty"`
}

type toolHitbox struct {
	GroupID string
	StartY  int
	EndY    int
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
	m.status = "thinking"
	m.refreshViewport()
}

func toolCallFromEvent(ev runtimeEvent) toolCallUI {
	return toolCallUI{
		ID:         ev.ToolCallID,
		Name:       ev.ToolName,
		Step:       ev.Step,
		Status:     ev.Status,
		Args:       ev.Args,
		Result:     ev.Result,
		StartedAt:  ev.StartedAt,
		FinishedAt: ev.FinishedAt,
		DurationMs: ev.DurationMs,
	}
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
	label := toolGroupLabel(group)
	icon := "▸"
	if group.Expanded {
		icon = "▾"
	}
	status := group.status()
	statusText := status
	if showDevToolDetails() {
		if duration := group.totalDurationMs(); duration > 0 {
			statusText = fmt.Sprintf("%s · %dms", statusText, duration)
		}
	}
	line := fmt.Sprintf("%s %s · %s", icon, label, statusText)
	switch status {
	case "completed":
		line = okSt.Render(line)
	case "failed", "denied", "unknown_tool":
		line = errSt.Render(line)
	default:
		line = mutedSt.Render(line)
	}
	if !group.Expanded {
		return line
	}
	return line + "\n" + m.renderToolGroupDetails(group, width)
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
	case "deleteFile":
		return fmt.Sprintf("Deleted %d %s", count, plural(count, "file", "files"))
	case "todoMd":
		return fmt.Sprintf("Updated %d todo %s", count, plural(count, "list", "lists"))
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
	if path := stringValue(call.Args, "path"); path != "" {
		return fmt.Sprintf("%s · %s", call.Status, oneLine(path, max(8, width-16)))
	}
	if paths, ok := call.Args["paths"].([]any); ok && len(paths) > 0 {
		return fmt.Sprintf("%s · %d files", call.Status, len(paths))
	}
	if pattern := stringValue(call.Args, "pattern"); pattern != "" {
		return fmt.Sprintf("%s · %q", call.Status, oneLine(pattern, max(8, width-16)))
	}
	if command := stringValue(call.Args, "command"); command != "" {
		return fmt.Sprintf("%s · %s", call.Status, oneLine(command, max(8, width-16)))
	}
	return call.Status
}

func toolCallDetailLines(call toolCallUI, width int) []string {
	var lines []string
	showDevDetails := showDevToolDetails()
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

func showDevToolDetails() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("QUBIT_DEV_TOOL_DETAILS")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
