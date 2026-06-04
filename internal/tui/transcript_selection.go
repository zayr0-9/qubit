package tui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

var (
	ansiSequenceRE = regexp.MustCompile(`\x1b\[[0-9;:?]*[ -/]*[@-~]`)
	urlRE          = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
)

type transcriptSelectedLineRange struct {
	Line     int
	StartCol int
	EndCol   int
}

func (m model) updateMouseWheelRouted(msg tea.MouseWheelMsg) tea.Model {
	if updated, ok := m.updateTodoOverlayMouseWheel(msg); ok {
		return updated
	}
	if m.transcriptSelection.Active && m.mode == modeChat {
		return m.updateChatMouseWheel(msg)
	}
	switch m.mode {
	case modeForkTree:
		return m.updateForkTreeMouseWheel(msg)
	case modeSessionPicker:
		return m.updateSessionPickerMouseWheel(msg)
	case modeMdEditor:
		return m.updateMdEditorMouseWheel(msg)
	case modeKeyPicker:
		return m.updateKeyPickerMouseWheel(msg)
	case modeModal:
		return m.updateModalMouseWheel(msg)
	case modeThemeEntry:
		return m.updateThemeEntryMouseWheel(msg)
	default:
		return m.updateChatMouseWheel(msg)
	}
}

func (m model) updateChatMouseWheel(msg tea.MouseWheelMsg) tea.Model {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.autoScroll = false
		m.viewport.ScrollUp(max(1, m.viewport.MouseWheelDelta))
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(max(1, m.viewport.MouseWheelDelta))
		m.autoScroll = m.viewport.AtBottom()
	}
	return m
}

func (m model) updateSessionPickerMouseWheel(msg tea.MouseWheelMsg) model {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.moveSessionCursor(-1)
	case tea.MouseWheelDown:
		m.moveSessionCursor(1)
	}
	return m
}

func (m model) updateKeyPickerMouseWheel(msg tea.MouseWheelMsg) model {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.moveApiKeyCursor(-1)
	case tea.MouseWheelDown:
		m.moveApiKeyCursor(1)
	}
	return m
}

func (m model) updateModalMouseWheel(msg tea.MouseWheelMsg) model {
	if m.modal == nil {
		return m
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		if len(m.modal.Options) > 0 {
			m.moveModalOptionCursor(-1)
		} else {
			m.moveModalCursor(-1)
		}
	case tea.MouseWheelDown:
		if len(m.modal.Options) > 0 {
			m.moveModalOptionCursor(1)
		} else {
			m.moveModalCursor(1)
		}
	}
	return m
}

func (m model) updateThemeEntryMouseWheel(msg tea.MouseWheelMsg) model {
	if m.themeEntry == nil {
		return m
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.moveThemePreset(-1)
	case tea.MouseWheelDown:
		m.moveThemePreset(1)
	}
	return m
}

func (m model) updateMouseClick(msg tea.MouseClickMsg) tea.Model {
	mouse := msg.Mouse()
	if updated, ok := m.updateTodoOverlayMouseClick(mouse); ok {
		return updated
	}
	if m.mode != modeChat || mouse.Button != tea.MouseLeft {
		return m
	}
	point, ok := m.mouseToTranscriptPoint(mouse)
	if !ok {
		return m
	}
	m.transcriptSelection = transcriptSelectionState{
		Active:       true,
		Anchor:       point,
		Cursor:       point,
		MouseDownX:   mouse.X,
		MouseDownY:   mouse.Y,
		PendingClick: true,
	}
	m.status = "select transcript"
	m.repaintTranscriptSelection()
	return m
}

func (m model) updateMouseMotion(msg tea.MouseMotionMsg) tea.Model {
	if m.mode != modeChat || !m.transcriptSelection.Active {
		return m
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft && !m.transcriptSelection.Dragging {
		return m
	}
	point, ok := m.mouseToTranscriptPoint(mouse)
	if !ok {
		point = m.clampMouseToTranscriptPoint(mouse)
	}
	if absInt(mouse.X-m.transcriptSelection.MouseDownX) > 1 || absInt(mouse.Y-m.transcriptSelection.MouseDownY) > 0 {
		m.transcriptSelection.Dragging = true
		m.transcriptSelection.PendingClick = false
	}
	m.transcriptSelection.Cursor = point
	m = m.scrollTranscriptSelectionAtEdges(mouse)
	m.repaintTranscriptSelection()
	return m
}

func (m model) updateMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if updated, ok := m.updateTodoOverlayMouseRelease(mouse); ok {
		return updated, nil
	}
	if m.mode != modeChat || !m.transcriptSelection.Active {
		return m, nil
	}
	if !m.transcriptSelection.Dragging {
		if mouse.Mod&tea.ModCtrl != 0 {
			if url, ok := m.linkAtMouse(mouse); ok {
				m.transcriptSelection = transcriptSelectionState{}
				m.status = "opening link"
				m.repaintTranscriptSelection()
				return m, openBrowserCmd(url)
			}
		}
		m.transcriptSelection = transcriptSelectionState{}
		m.status = "ready"
		m.repaintTranscriptSelection()
		return m.toggleHitboxAtMouse(mouse), nil
	}
	if m.transcriptSelectedText() == "" {
		m.transcriptSelection = transcriptSelectionState{}
		m.status = "ready"
	} else {
		m.status = "transcript selection"
	}
	m.repaintTranscriptSelection()
	return m, nil
}

func (m model) linkAtMouse(mouse tea.Mouse) (string, bool) {
	if mouse.Button != tea.MouseLeft || mouse.Y < m.chatTopY || mouse.Y >= m.chatTopY+m.viewport.Height() {
		return "", false
	}
	line := m.viewport.YOffset() + mouse.Y - m.chatTopY
	for _, hitbox := range m.linkHitboxes {
		if hitbox.Line == line && mouse.X >= hitbox.StartX && mouse.X <= hitbox.EndX {
			return hitbox.URL, true
		}
	}
	return "", false
}

func (m model) toggleHitboxAtMouse(mouse tea.Mouse) model {
	if mouse.Button != tea.MouseLeft || mouse.Y < m.chatTopY || mouse.Y >= m.chatTopY+m.viewport.Height() {
		return m
	}
	contentY := m.viewport.YOffset() + mouse.Y - m.chatTopY
	for _, hitbox := range m.toolHitboxes {
		if contentY >= hitbox.StartY && contentY <= hitbox.EndY && mouse.X >= hitbox.StartX && mouse.X <= hitbox.EndX {
			m.autoScroll = false
			if hitbox.Kind == "reasoning" {
				m.toggleReasoningBlock(hitbox.MessageIndex)
			} else {
				m.toggleToolGroup(hitbox.GroupID)
			}
			return m
		}
	}
	return m
}

func (m model) mouseToTranscriptPoint(mouse tea.Mouse) (transcriptSelectionPoint, bool) {
	if mouse.Y < m.chatTopY || mouse.Y >= m.chatTopY+m.viewport.Height() {
		return transcriptSelectionPoint{}, false
	}
	line := m.viewport.YOffset() + mouse.Y - m.chatTopY
	if line < 0 || line >= len(m.transcriptLines) {
		return transcriptSelectionPoint{}, false
	}
	plain := m.transcriptLines[line].Text
	col := clampInt(mouse.X, 0, runewidth.StringWidth(plain))
	return transcriptSelectionPoint{Line: line, Col: col}, true
}

func (m model) clampMouseToTranscriptPoint(mouse tea.Mouse) transcriptSelectionPoint {
	if len(m.transcriptLines) == 0 {
		return transcriptSelectionPoint{}
	}
	line := m.viewport.YOffset() + mouse.Y - m.chatTopY
	line = clampInt(line, 0, len(m.transcriptLines)-1)
	plain := m.transcriptLines[line].Text
	col := clampInt(mouse.X, 0, runewidth.StringWidth(plain))
	return transcriptSelectionPoint{Line: line, Col: col}
}

func (m model) scrollTranscriptSelectionAtEdges(mouse tea.Mouse) model {
	if m.viewport.Height() <= 0 {
		return m
	}
	if mouse.Y <= m.chatTopY {
		m.autoScroll = false
		m.viewport.ScrollUp(max(1, m.viewport.MouseWheelDelta))
		m.transcriptSelection.Cursor = m.clampMouseToTranscriptPoint(tea.Mouse{X: mouse.X, Y: m.chatTopY})
	} else if mouse.Y >= m.chatTopY+m.viewport.Height()-1 {
		m.viewport.ScrollDown(max(1, m.viewport.MouseWheelDelta))
		m.autoScroll = m.viewport.AtBottom()
		m.transcriptSelection.Cursor = m.clampMouseToTranscriptPoint(tea.Mouse{X: mouse.X, Y: m.chatTopY + m.viewport.Height() - 1})
	}
	return m
}

func (m model) transcriptSelectedText() string {
	ranges := m.transcriptSelectedRanges()
	if len(ranges) == 0 {
		return ""
	}
	selected := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.Line < 0 || r.Line >= len(m.transcriptLines) {
			continue
		}
		line := m.transcriptLines[r.Line].Text
		part := strings.TrimRight(sliceStringByCells(line, r.StartCol, r.EndCol), " ")
		selected = append(selected, part)
	}
	return strings.TrimRight(strings.Join(selected, "\n"), "\n")
}

func (m model) transcriptSelectedRanges() []transcriptSelectedLineRange {
	if !m.transcriptSelection.Active || len(m.transcriptLines) == 0 {
		return nil
	}
	start := m.transcriptSelection.Anchor
	end := m.transcriptSelection.Cursor
	if compareTranscriptPoints(end, start) < 0 {
		start, end = end, start
	}
	start.Line = clampInt(start.Line, 0, len(m.transcriptLines)-1)
	end.Line = clampInt(end.Line, 0, len(m.transcriptLines)-1)
	if start.Line == end.Line && start.Col == end.Col {
		return nil
	}
	ranges := make([]transcriptSelectedLineRange, 0, end.Line-start.Line+1)
	for line := start.Line; line <= end.Line; line++ {
		lineWidth := runewidth.StringWidth(m.transcriptLines[line].Text)
		startCol := 0
		endCol := lineWidth
		if line == start.Line {
			startCol = clampInt(start.Col, 0, lineWidth)
		}
		if line == end.Line {
			endCol = clampInt(end.Col, 0, lineWidth)
		}
		if endCol < startCol {
			startCol, endCol = endCol, startCol
		}
		if startCol == endCol {
			continue
		}
		ranges = append(ranges, transcriptSelectedLineRange{Line: line, StartCol: startCol, EndCol: endCol})
	}
	return ranges
}

func compareTranscriptPoints(a transcriptSelectionPoint, b transcriptSelectionPoint) int {
	if a.Line < b.Line {
		return -1
	}
	if a.Line > b.Line {
		return 1
	}
	if a.Col < b.Col {
		return -1
	}
	if a.Col > b.Col {
		return 1
	}
	return 0
}

func (m *model) repaintTranscriptSelection() {
	content := m.transcriptContent
	if content == "" {
		content = m.viewport.View()
	}
	m.viewport.SetContent(applyTranscriptSelection(content, m.transcriptSelectedRanges()))
}

func applyTranscriptSelection(content string, ranges []transcriptSelectedLineRange) string {
	if len(ranges) == 0 || content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for _, r := range ranges {
		if r.Line < 0 || r.Line >= len(lines) {
			continue
		}
		lines[r.Line] = highlightLineCells(lines[r.Line], r.StartCol, r.EndCol)
	}
	return strings.Join(lines, "\n")
}

func highlightLineCells(line string, startCol int, endCol int) string {
	if startCol == endCol {
		return line
	}
	plain := stripANSI(line)
	width := runewidth.StringWidth(plain)
	startCol = clampInt(startCol, 0, width)
	endCol = clampInt(endCol, startCol, width)
	prefix := sliceStringByCells(plain, 0, startCol)
	selected := sliceStringByCells(plain, startCol, endCol)
	suffix := sliceStringByCells(plain, endCol, width)
	if selected == "" {
		return line
	}
	return prefix + transcriptSelectionStyle().Render(selected) + suffix
}

func transcriptSelectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(accent).Foreground(bg)
}

func transcriptRenderLines(content string) []transcriptRenderLine {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	out := make([]transcriptRenderLine, len(lines))
	for i, line := range lines {
		out[i] = transcriptRenderLine{Text: stripANSI(line), Selectable: strings.TrimSpace(stripANSI(line)) != ""}
	}
	return out
}

func transcriptLinkHitboxes(lines []transcriptRenderLine) []linkHitbox {
	var boxes []linkHitbox
	for lineIndex, line := range lines {
		matches := urlRE.FindAllStringIndex(line.Text, -1)
		for _, match := range matches {
			raw := line.Text[match[0]:match[1]]
			url := strings.TrimRight(raw, ".,;:!?")
			if url == "" {
				continue
			}
			startX := runewidth.StringWidth(line.Text[:match[0]])
			endX := startX + runewidth.StringWidth(url) - 1
			boxes = append(boxes, linkHitbox{URL: url, Line: lineIndex, StartX: startX, EndX: endX})
		}
	}
	return boxes
}

func stripANSI(s string) string {
	return ansiSequenceRE.ReplaceAllString(s, "")
}

func sliceStringByCells(s string, startCell int, endCell int) string {
	if endCell <= startCell {
		return ""
	}
	var b strings.Builder
	cell := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			w = 1
		}
		next := cell + w
		if next > startCell && cell < endCell {
			b.WriteRune(r)
		}
		cell = next
		if cell >= endCell {
			break
		}
	}
	return b.String()
}
