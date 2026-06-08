package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *model) ensureChatList() {
	if m.chatList.Cache == nil {
		m.chatList.Cache = make(map[chatListItemKey]chatListRenderedItem)
	}
	width := max(20, m.viewport.Width())
	m.chatList.Width = width
	m.chatList.Height = max(1, m.viewport.Height())
	m.chatList.Items = m.buildChatListItems(width)
	m.syncLegacyViewportOffset()
}

func (m *model) buildChatListItems(width int) []chatListItem {
	items := make([]chatListItem, 0, len(m.messages))
	for i := 0; i < len(m.messages); i++ {
		message := m.messages[i]
		kind := chatItemMessage
		end := i
		switch message.Role {
		case "tool":
			kind = chatItemToolGroup
			for end+1 < len(m.messages) && m.messages[end+1].Role == "tool" {
				end++
			}
		case "reasoning":
			kind = chatItemReasoning
		case "view":
			kind = chatItemView
		}
		key := chatListItemKey{Segment: m.messageSegmentKey(i, width)}
		items = append(items, chatListItem{Kind: kind, StartIndex: i, EndIndex: end, Key: key})
		i = end
	}
	return items
}

func (m *model) renderChatListItem(item chatListItem) chatListRenderedItem {
	cacheable := true
	for i := item.StartIndex; i <= item.EndIndex; i++ {
		if (m.streaming && i == m.streamingMessageIndex) || m.runningToolSegment(i) {
			cacheable = false
			break
		}
	}
	if cacheable {
		if cached, ok := m.chatList.Cache[item.Key]; ok {
			return cached
		}
	}
	m.ensureSegmentCacheSize()
	segment, _ := m.renderMessageSegment(item.StartIndex, 0)
	lines := splitRenderedLines(segment.Text)
	if m.messages[item.StartIndex].Role == "user" {
		lines = renderUserMessageRows(lines, m.chatList.Width)
	}
	rendered := chatListRenderedItem{
		Key:       item.Key,
		Text:      segment.Text,
		Lines:     lines,
		Tools:     segment.Tools,
		Height:    len(lines),
		LastIndex: segment.LastIndex,
		Frozen:    cacheable,
	}
	if cacheable {
		m.chatList.Cache[item.Key] = rendered
	}
	return rendered
}

func (m *model) ensureChatListMetrics() chatListMetrics {
	m.ensureChatList()
	if chatListMetricsValid(m.chatList.Metrics, m.chatList.Width, m.chatList.Items) {
		m.chatList.TotalHeight = m.chatList.Metrics.TotalHeight
		return m.chatList.Metrics
	}
	metrics := chatListMetrics{
		Width:   m.chatList.Width,
		Keys:    make([]chatListItemKey, len(m.chatList.Items)),
		Starts:  make([]int, len(m.chatList.Items)),
		Heights: make([]int, len(m.chatList.Items)),
	}
	total := 0
	for i, item := range m.chatList.Items {
		rendered := m.renderChatListItem(item)
		metrics.Keys[i] = item.Key
		metrics.Starts[i] = total
		metrics.Heights[i] = rendered.Height
		total += rendered.Height
	}
	metrics.TotalHeight = total
	m.chatList.Metrics = metrics
	m.chatList.TotalHeight = total
	return metrics
}

func chatListMetricsValid(metrics chatListMetrics, width int, items []chatListItem) bool {
	if metrics.Width != width || len(metrics.Keys) != len(items) || len(metrics.Starts) != len(items) || len(metrics.Heights) != len(items) {
		return false
	}
	for i, item := range items {
		if metrics.Keys[i] != item.Key {
			return false
		}
	}
	return true
}

func chatListFirstVisibleItem(starts []int, heights []int, start int) int {
	low, high := 0, len(starts)
	for low < high {
		mid := low + (high-low)/2
		if starts[mid]+heights[mid] <= start {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low
}

func (m *model) remeasureChatList() {
	m.ensureChatListMetrics()
}

func (m *model) renderChatListVisible() chatListVisibleRender {
	metrics := m.ensureChatListMetrics()
	height := max(1, m.chatList.Height)
	total := metrics.TotalHeight
	if m.autoScroll {
		m.chatList.YOffset = max(0, total-height)
	} else {
		m.chatList.YOffset = clampInt(m.chatList.YOffset, 0, max(0, total-height))
	}
	start := m.chatList.YOffset
	end := start + height
	var outLines []string
	var plain []transcriptRenderLine
	var tools []toolHitbox
	var rows []chatVisibleRow
	for itemIndex := chatListFirstVisibleItem(metrics.Starts, metrics.Heights, start); itemIndex < len(m.chatList.Items); itemIndex++ {
		itemStart := metrics.Starts[itemIndex]
		itemHeight := metrics.Heights[itemIndex]
		if itemStart >= end {
			break
		}
		rendered := m.renderChatListItem(m.chatList.Items[itemIndex])
		localStart := max(0, start-itemStart)
		localEnd := min(itemHeight, end-itemStart)
		for localY := localStart; localY < localEnd; localY++ {
			absoluteY := itemStart + localY
			screenY := absoluteY - start
			line := ""
			if localY < len(rendered.Lines) {
				line = rendered.Lines[localY]
			}
			plainLine := renderedPlainLine(rendered, localY)
			outLines = append(outLines, line)
			plain = append(plain, plainLine)
			rows = append(rows, chatVisibleRow{ScreenY: screenY, AbsoluteY: absoluteY, ItemIndex: itemIndex, LocalY: localY, Text: line, Plain: plainLine})
		}
		for _, box := range rendered.Tools {
			box.StartY += itemStart
			box.EndY += itemStart
			if box.EndY >= start && box.StartY < end {
				tools = append(tools, box)
			}
		}
	}
	links := visibleLinkHitboxes(plain, start)
	visible := chatListVisibleRender{Content: strings.Join(outLines, "\n"), Lines: plain, ToolHitboxes: tools, LinkHitboxes: links, VisibleRows: rows, TotalHeight: total}
	m.chatList.Visible = visible
	m.transcriptContent = visible.Content
	m.transcriptLines = visible.Lines
	m.linkHitboxes = visible.LinkHitboxes
	m.toolHitboxes = visible.ToolHitboxes
	return visible
}

func renderedPlainLine(rendered chatListRenderedItem, localY int) transcriptRenderLine {
	if localY < 0 || localY >= len(rendered.Lines) {
		return transcriptRenderLine{}
	}
	text := stripANSI(rendered.Lines[localY])
	return transcriptRenderLine{Text: text, Selectable: strings.TrimSpace(text) != ""}
}

func visibleLinkHitboxes(lines []transcriptRenderLine, absoluteStart int) []linkHitbox {
	boxes := transcriptLinkHitboxes(lines)
	for i := range boxes {
		boxes[i].Line += absoluteStart
	}
	return boxes
}

func (m *model) renderChatListView() string {
	visible := m.renderChatListVisible()
	if len(m.transcriptSelectedRanges()) == 0 {
		return visible.Content
	}
	return m.applyTranscriptSelectionToVisible(visible.Content)
}

func (m *model) chatListLineAtAbsoluteY(y int) (transcriptRenderLine, bool) {
	if y < 0 {
		return transcriptRenderLine{}, false
	}
	metrics := m.ensureChatListMetrics()
	itemIndex := chatListFirstVisibleItem(metrics.Starts, metrics.Heights, y)
	if itemIndex < 0 || itemIndex >= len(m.chatList.Items) {
		return transcriptRenderLine{}, false
	}
	local := y - metrics.Starts[itemIndex]
	if local < 0 || local >= metrics.Heights[itemIndex] {
		return transcriptRenderLine{}, false
	}
	rendered := m.renderChatListItem(m.chatList.Items[itemIndex])
	if local >= len(rendered.Lines) {
		return transcriptRenderLine{}, false
	}
	return renderedPlainLine(rendered, local), true
}

func (m *model) chatListPlainLines(start, end int) []transcriptRenderLine {
	if end < start {
		return nil
	}
	lines := make([]transcriptRenderLine, 0, end-start+1)
	for y := start; y <= end; y++ {
		line, ok := m.chatListLineAtAbsoluteY(y)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func renderUserMessageRows(lines []string, width int) []string {
	if len(lines) == 0 {
		return lines
	}
	styled := make([]string, len(lines))
	targetWidth := max(1, width-chatStyle.GetHorizontalPadding())
	for i, line := range lines {
		if strings.TrimSpace(stripANSI(line)) == "" {
			styled[i] = line
			continue
		}
		padding := strings.Repeat(" ", max(0, targetWidth-lipgloss.Width(line)))
		styled[i] = applyRowBackground(line+padding, colorToHex(userMessageBg))
	}
	return styled
}

func applyRowBackground(line string, bgHex string) string {
	rgb, err := parseHexRGB(bgHex)
	if err != nil {
		return line
	}
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2])
	return bgSeq + reapplyBackgroundAfterANSIReset(line, bgSeq) + "\x1b[0m"
}

func reapplyBackgroundAfterANSIReset(s string, bgSeq string) string {
	var b strings.Builder
	b.Grow(len(s) + strings.Count(s, "\x1b[")*len(bgSeq))
	for i := 0; i < len(s); {
		if s[i] != '\x1b' || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := i + 2
		for end < len(s) && s[end] != 'm' {
			end++
		}
		if end >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		sequence := s[i+2 : end]
		b.WriteString(s[i : end+1])
		if sgrResetsBackground(sequence) {
			b.WriteString(bgSeq)
		}
		i = end + 1
	}
	return b.String()
}

func sgrResetsBackground(sequence string) bool {
	if sequence == "" {
		return true
	}
	for _, part := range strings.Split(sequence, ";") {
		if part == "0" || part == "49" {
			return true
		}
	}
	return false
}
