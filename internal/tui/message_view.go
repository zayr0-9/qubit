package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *model) refreshViewport() {
	previousYOffset := m.viewport.YOffset()
	m.ensureSegmentCacheSize()
	m.toolHitboxes = nil
	var b strings.Builder
	contentLine := 0
	for i := 0; i < len(m.messages); i++ {
		segment, nextIndex := m.renderMessageSegment(i, contentLine)
		b.WriteString(segment.Text)
		for _, hitbox := range segment.Tools {
			hitbox.StartY += contentLine
			hitbox.EndY += contentLine
			m.toolHitboxes = append(m.toolHitboxes, hitbox)
		}
		contentLine += appendedLineCountDelta(segment.Text, contentLine == 0)
		i = nextIndex
	}
	content := b.String()
	m.transcriptContent = content
	m.rebuildTranscriptMetadata()
	m.repaintTranscriptSelection()
	m.restoreViewportPosition(previousYOffset)
}
func (m *model) ensureSegmentCacheSize() {
	if len(m.messageRenderSegments) == len(m.messages) {
		return
	}
	next := make([]messageRenderSegment, len(m.messages))
	copy(next, m.messageRenderSegments)
	m.messageRenderSegments = next
}
func (m *model) renderMessageSegment(index int, contentLine int) (messageRenderSegment, int) {
	message := m.messages[index]
	width := max(20, m.viewport.Width())
	key := m.messageSegmentKey(index, width)
	cacheable := !(m.streaming && index == m.streamingMessageIndex) && !m.runningToolSegment(index)
	if cacheable && index < len(m.messageRenderSegments) && m.messageRenderSegments[index].Key == key {
		return m.messageRenderSegments[index], m.messageRenderSegments[index].LastIndex
	}

	var rendered string
	localToolHitboxes := []toolHitbox(nil)
	lastIndex := index
	switch message.Role {
	case "view":
		rendered = m.renderViewMessage(message)
	case "tool":
		groups := []*toolGroup{}
		for lastIndex < len(m.messages) && m.messages[lastIndex].Role == "tool" {
			if m.messages[lastIndex].ToolGroup != nil {
				groups = append(groups, m.messages[lastIndex].ToolGroup)
			}
			lastIndex++
		}
		lastIndex--
		rendered = m.renderCollapsedToolGroups(groups, width)
		localToolHitboxes = m.toolGroupHitboxes(groups, width)
	case "reasoning":
		rendered = m.renderReasoningBlock(message, width)
		headingWidth := lipgloss.Width(m.renderReasoningBlockHeading(message))
		localToolHitboxes = append(localToolHitboxes, toolHitbox{Kind: "reasoning", MessageIndex: index, StartY: 0, EndY: 0, StartX: 0, EndX: max(0, headingWidth-1)})
	default:
		renderedContent := m.renderMessageContent(message, cacheable)
		rendered = renderMessageWithIcon(message, renderedContent, messageDisplayNumber(m.messages, index))
		if message.Role == "user" && index == len(m.messages)-1 {
			rendered += "\n"
		}
	}
	if index > 0 {
		separator := messageSeparator(m.messages[index-1], message)
		rendered = separator + rendered
		sepLines := separatorBlankLineCount(separator)
		for i := range localToolHitboxes {
			localToolHitboxes[i].StartY += sepLines
			localToolHitboxes[i].EndY += sepLines
		}
	}
	lines := transcriptRenderLines(rendered)
	segment := messageRenderSegment{
		Key:       key,
		Text:      rendered,
		Lines:     lines,
		Links:     transcriptLinkHitboxes(lines),
		Tools:     localToolHitboxes,
		LineCount: renderedLineCount(rendered),
		LastIndex: lastIndex,
	}
	if cacheable && index < len(m.messageRenderSegments) {
		m.messageRenderSegments[index] = segment
	}
	return segment, lastIndex
}
func (m *model) messageSegmentKey(index int, width int) messageRenderSegmentKey {
	message := m.messages[index]
	key := messageRenderSegmentKey{
		Index:            index,
		Role:             message.Role,
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
		ViewType:         message.ViewType,
		Title:            message.Title,
		Path:             message.Path,
		MessageKind:      message.MessageKind,
		Expanded:         message.Expanded,
		IsLastMessage:    index == len(m.messages)-1,
		Width:            width,
		DisplayNumber:    messageDisplayNumber(m.messages, index),
	}
	if index > 0 {
		prev := m.messages[index-1]
		key.PreviousRole = prev.Role
		key.PreviousContent = prev.Content
		key.PreviousReasoningContent = prev.ReasoningContent
	}
	if message.ToolGroup != nil {
		key.ToolGroupID = message.ToolGroup.ID
		key.ToolGroupLabel = toolGroupLabel(message.ToolGroup)
		key.ToolGroupExpanded = message.ToolGroup.Expanded
	}
	if m.toolCallRevealing && index == m.toolCallRevealMessageIndex {
		key.ToolCallRevealVisible = m.toolCallRevealVisibleRunes
	}
	if message.Role == "tool" {
		for i := index; i < len(m.messages) && m.messages[i].Role == "tool"; i++ {
			if group := m.messages[i].ToolGroup; group != nil {
				key.Content += "|" + group.ID + ":" + toolGroupLabel(group) + ":" + fmt.Sprint(group.Expanded)
			}
		}
	}
	return key
}
func (m *model) runningToolSegment(index int) bool {
	if index < 0 || index >= len(m.messages) || m.messages[index].Role != "tool" {
		return false
	}
	for i := index; i < len(m.messages) && m.messages[i].Role == "tool"; i++ {
		group := m.messages[i].ToolGroup
		if group == nil {
			continue
		}
		for _, call := range group.Calls {
			if call.Status == "running" {
				return true
			}
		}
	}
	return false
}
func (m *model) rebuildTranscriptMetadata() {
	// Selection and link hit-testing must be derived from the exact string sent to
	// the viewport. Segment-local strings often begin with separators; splitting
	// and concatenating per-segment line metadata adds phantom blank rows at those
	// boundaries, which makes mouse Y coordinates drift as the transcript grows.
	m.transcriptLines = transcriptRenderLines(m.transcriptContent)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
}
func (m *model) restoreViewportPosition(yOffset int) {
	if m.autoScroll {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(clampInt(yOffset, 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
}
func messageSeparator(prev chatMessage, next chatMessage) string {
	if prev.Role == "user" {
		return "\n\n"
	}
	if prev.Role == "tool" || next.Role == "tool" {
		return "\n"
	}
	if prev.Role == next.Role {
		return "\n"
	}
	return "\n\n"
}
func separatorBlankLineCount(separator string) int {
	newlines := strings.Count(separator, "\n")
	if newlines == 0 {
		return 0
	}
	return newlines - 1
}
func (m *model) renderViewMessage(message chatMessage) string {
	title := fallback(message.Title, "View")
	isPlan := message.ViewType == "plan"
	if isPlan && !strings.HasPrefix(title, "Plan:") {
		title = "Plan: " + title
	}

	width := max(20, m.viewport.Width()-2)
	if isPlan {
		return m.renderPlanViewMessage(message, title, width)
	}

	header := aiIcon.Render("◇") + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title)
	if message.Path != "" {
		header += mutedSt.Render(" · " + oneLine(message.Path, max(12, m.viewport.Width()-lipgloss.Width(title)-8)))
	}
	content, err := m.renderMessageContentAtWidth(chatMessage{Role: "assistant", Content: message.Content}, width)
	if err != nil {
		content = wrap(message.Content, width)
	}
	if strings.TrimSpace(content) == "" {
		return header
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return header + "\n" + strings.Join(lines, "\n")
}
func renderAccentBorderedPanel(body string, width int) string {
	return lipgloss.NewStyle().
		Foreground(text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 2).
		Width(width).
		Render(body)
}
func renderMessageWithIcon(message chatMessage, content string, number int) string {
	icon := aiIcon.Render("◆")
	if message.LocalOnly || message.Role == "status" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "reasoning" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "user" {
		if number > 0 {
			icon = userIcon.Render(fmt.Sprintf("›%d", number))
		} else {
			icon = userIcon.Render("›")
		}
	} else if message.Role == "error" {
		icon = errorIcon.Render("!")
	}

	if content == "" {
		return icon
	}

	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return icon
	}
	indent := strings.Repeat(" ", max(2, lipgloss.Width(icon)+1))
	lines[0] = icon + " " + strings.TrimLeft(lines[0], " \t")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
func messageDisplayNumber(messages []chatMessage, index int) int {
	if index < 0 || index >= len(messages) || messages[index].Role != "user" {
		return 0
	}
	return index + 1
}
func (m *model) renderMessageContent(message chatMessage, cacheable bool) string {
	width := max(20, m.viewport.Width())
	if m.streaming && !cacheable {
		return m.renderStreamingMessageContent(message, width)
	}
	key := renderCacheKey{Role: message.Role, Content: message.Content, ReasoningContent: message.ReasoningContent, Width: width}
	if cacheable && m.renderCache != nil {
		if cached, ok := m.renderCache[key]; ok {
			return cached
		}
	}

	rendered, err := m.renderMessageContentAtWidth(message, width)
	if err != nil {
		rendered = wrap(message.Content, width)
	}
	if cacheable && m.renderCache != nil {
		m.renderCache[key] = rendered
	}
	return rendered
}
func (m *model) renderStreamingMessageContent(message chatMessage, width int) string {
	if m.streamingMarkdownCache.Width == width && m.streamingMarkdownCache.Content == message.Content {
		return m.streamingMarkdownCache.Rendered
	}
	if !shouldMarkdownRenderStreaming(message.Content) {
		rendered := wrap(message.Content, width)
		m.streamingMarkdownCache = streamingMarkdownCache{Content: message.Content, Width: width, Rendered: rendered}
		return rendered
	}
	rendered, err := m.renderMessageContentAtWidth(message, width)
	if err != nil {
		rendered = wrap(message.Content, width)
	}
	m.streamingMarkdownCache = streamingMarkdownCache{Content: message.Content, Width: width, Rendered: rendered}
	return rendered
}
func shouldMarkdownRenderStreaming(content string) bool {
	if strings.HasSuffix(content, "\n") || strings.HasSuffix(content, " ") || strings.HasSuffix(content, "\t") {
		return true
	}
	for _, marker := range []string{"```", "**", "__", "*", "_", "#", "- ", "1. ", "http://", "https://"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func (m *model) clearRenderCaches() {
	m.renderCache = make(map[renderCacheKey]string)
	m.markdownRenderers = make(markdownRendererCache)
	m.messageRenderSegments = nil
	m.streamingMarkdownCache = streamingMarkdownCache{}
	m.streamingTranscriptCache = streamingTranscriptCache{}
}

func (m *model) refreshViewportForStreaming() {
	if !m.streaming || m.streamingMessageIndex < 0 || m.streamingMessageIndex >= len(m.messages) {
		m.refreshViewport()
		return
	}
	previousYOffset := m.viewport.YOffset()
	width := max(20, m.viewport.Width())
	prefix := m.streamingPrefix(width)
	segment, _ := m.renderMessageSegment(m.streamingMessageIndex, prefix.PrefixLineCount)
	content := prefix.Prefix + segment.Text
	m.transcriptContent = content
	m.rebuildTranscriptMetadata()
	m.toolHitboxes = append([]toolHitbox{}, prefix.PrefixTools...)
	m.repaintTranscriptSelection()
	m.restoreViewportPosition(previousYOffset)
}

func (m *model) streamingPrefix(width int) streamingTranscriptCache {
	if m.streamingTranscriptCache.Width == width && m.streamingTranscriptCache.MessageIndex == m.streamingMessageIndex {
		return m.streamingTranscriptCache
	}
	m.ensureSegmentCacheSize()
	var b strings.Builder
	var lines []transcriptRenderLine
	var links []linkHitbox
	var tools []toolHitbox
	lineOffset := 0
	for i := 0; i < m.streamingMessageIndex; i++ {
		segment, nextIndex := m.renderMessageSegment(i, lineOffset)
		b.WriteString(segment.Text)
		lines = append(lines, segment.Lines...)
		for _, hitbox := range segment.Links {
			hitbox.Line += lineOffset
			links = append(links, hitbox)
		}
		for _, hitbox := range segment.Tools {
			hitbox.StartY += lineOffset
			hitbox.EndY += lineOffset
			tools = append(tools, hitbox)
		}
		lineOffset += appendedLineCountDelta(segment.Text, lineOffset == 0)
		i = nextIndex
	}
	cache := streamingTranscriptCache{MessageIndex: m.streamingMessageIndex, Width: width, Prefix: b.String(), PrefixLines: lines, PrefixLinks: links, PrefixTools: tools, PrefixLineCount: lineOffset}
	m.streamingTranscriptCache = cache
	return cache
}

func appendedLineCountDelta(s string, emptyPrefix bool) int {
	if s == "" {
		return 0
	}
	if emptyPrefix {
		return renderedLineCount(s)
	}
	return strings.Count(s, "\n")
}
