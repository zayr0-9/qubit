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
	plain := transcriptRenderLines(segment.Text)
	if m.messages[item.StartIndex].Role == "user" {
		lines = renderUserMessageRows(lines, m.chatList.Width)
		plain = transcriptRenderLines(strings.Join(lines, "\n"))
	}
	rendered := chatListRenderedItem{
		Key:       item.Key,
		Text:      segment.Text,
		Lines:     lines,
		Plain:     plain,
		Links:     segment.Links,
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

func (m *model) remeasureChatList() {
	m.ensureChatList()
	total := 0
	for _, item := range m.chatList.Items {
		rendered := m.renderChatListItem(item)
		total += rendered.Height
	}
	m.chatList.TotalHeight = total
}

func (m *model) renderChatListVisible() chatListVisibleRender {
	m.ensureChatList()
	height := max(1, m.chatList.Height)
	renderedItems := make([]chatListRenderedItem, len(m.chatList.Items))
	itemStarts := make([]int, len(m.chatList.Items))
	total := 0
	for i, item := range m.chatList.Items {
		itemStarts[i] = total
		rendered := m.renderChatListItem(item)
		renderedItems[i] = rendered
		total += rendered.Height
	}
	m.chatList.TotalHeight = total
	if m.autoScroll {
		m.chatList.YOffset = max(0, total-height)
	} else {
		m.chatList.YOffset = clampInt(m.chatList.YOffset, 0, max(0, total-height))
	}
	start := m.chatList.YOffset
	end := start + height
	var outLines []string
	var plain []transcriptRenderLine
	var links []linkHitbox
	var tools []toolHitbox
	var rows []chatVisibleRow
	for itemIndex, rendered := range renderedItems {
		itemStart := itemStarts[itemIndex]
		itemEnd := itemStart + rendered.Height
		if itemEnd <= start {
			continue
		}
		if itemStart >= end {
			break
		}
		localStart := max(0, start-itemStart)
		localEnd := min(rendered.Height, end-itemStart)
		for localY := localStart; localY < localEnd; localY++ {
			absoluteY := itemStart + localY
			screenY := absoluteY - start
			line := ""
			if localY < len(rendered.Lines) {
				line = rendered.Lines[localY]
			}
			plainLine := transcriptRenderLine{}
			if localY < len(rendered.Plain) {
				plainLine = rendered.Plain[localY]
			}
			outLines = append(outLines, line)
			plain = append(plain, plainLine)
			rows = append(rows, chatVisibleRow{ScreenY: screenY, AbsoluteY: absoluteY, ItemIndex: itemIndex, LocalY: localY, Text: line, Plain: plainLine})
		}
		for _, box := range rendered.Links {
			absLine := itemStart + box.Line
			if absLine >= start && absLine < end {
				box.Line = absLine
				links = append(links, box)
			}
		}
		for _, box := range rendered.Tools {
			box.StartY += itemStart
			box.EndY += itemStart
			if box.EndY >= start && box.StartY < end {
				tools = append(tools, box)
			}
		}
	}
	visible := chatListVisibleRender{Content: strings.Join(outLines, "\n"), Lines: plain, ToolHitboxes: tools, LinkHitboxes: links, VisibleRows: rows, TotalHeight: total}
	m.chatList.Visible = visible
	m.transcriptContent = visible.Content
	m.transcriptLines = visible.Lines
	m.linkHitboxes = visible.LinkHitboxes
	m.toolHitboxes = visible.ToolHitboxes
	return visible
}

func (m *model) renderChatListView() string {
	visible := m.renderChatListVisible()
	if len(m.transcriptSelectedRanges()) == 0 {
		return visible.Content
	}
	return m.applyTranscriptSelectionToVisible(visible.Content)
}

func (m *model) chatListLineAtAbsoluteY(y int) (transcriptRenderLine, bool) {
	m.ensureChatList()
	if y < 0 {
		return transcriptRenderLine{}, false
	}
	current := 0
	for _, item := range m.chatList.Items {
		rendered := m.renderChatListItem(item)
		if y < current+rendered.Height {
			local := y - current
			if local >= 0 && local < len(rendered.Plain) {
				return rendered.Plain[local], true
			}
			return transcriptRenderLine{}, false
		}
		current += rendered.Height
	}
	return transcriptRenderLine{}, false
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
