package main

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/mattn/go-runewidth"
)

type composerModel struct {
	value []rune

	cursor          int
	preferredColumn int

	selectionAnchor int
	selecting       bool

	width            int
	minHeight        int
	maxHeight        int
	maxContentHeight int
	charLimit        int
	scrollLine       int
	placeholder      string
	focused          bool
}

type composerLine struct {
	cells      []composerCell
	startIndex int
	endIndex   int
}

type composerCell struct {
	r     rune
	index int
	width int
}

type composerPasteMsg struct {
	text string
	err  error
}

func newComposer() composerModel {
	return composerModel{
		width:            20,
		minHeight:        1,
		maxHeight:        8,
		maxContentHeight: 80,
		charLimit:        4000,
		placeholder:      "message qubit...",
		focused:          true,
	}
}

func (c composerModel) Value() string {
	return string(c.value)
}

func (c *composerModel) SetValue(s string) {
	c.value = []rune(normalizeInputNewlines(s))
	if c.charLimit > 0 && len(c.value) > c.charLimit {
		c.value = c.value[:c.charLimit]
	}
	c.cursor = len(c.value)
	c.preferredColumn = -1
	c.ClearSelection()
	c.ensureCursorVisible()
}

func (c *composerModel) Reset() {
	c.value = nil
	c.cursor = 0
	c.preferredColumn = -1
	c.ClearSelection()
	c.scrollLine = 0
}

func (c *composerModel) SetWidth(width int) {
	c.width = max(1, width)
	c.ensureCursorVisible()
}

func (c composerModel) Height() int {
	lineCount := len(c.visualLines())
	if lineCount == 0 {
		lineCount = 1
	}
	height := max(c.minHeight, lineCount)
	if c.maxHeight > 0 {
		height = min(height, c.maxHeight)
	}
	return max(1, height)
}

func (c composerModel) HasSelection() bool {
	start, end := c.SelectionRange()
	return c.selecting && start != end
}

func (c composerModel) SelectionRange() (int, int) {
	if !c.selecting {
		return c.cursor, c.cursor
	}
	start := min(c.selectionAnchor, c.cursor)
	end := max(c.selectionAnchor, c.cursor)
	return clampInt(start, 0, len(c.value)), clampInt(end, 0, len(c.value))
}

func (c composerModel) SelectedText() string {
	if !c.HasSelection() {
		return ""
	}
	start, end := c.SelectionRange()
	return string(c.value[start:end])
}

func (c *composerModel) SelectAll() {
	if len(c.value) == 0 {
		c.ClearSelection()
		return
	}
	c.selectionAnchor = 0
	c.cursor = len(c.value)
	c.selecting = true
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) ClearSelection() {
	c.selecting = false
	c.selectionAnchor = c.cursor
}

func (c *composerModel) ReplaceSelection(s string) {
	if !c.HasSelection() {
		c.InsertString(s)
		return
	}
	start, end := c.SelectionRange()
	replacement := []rune(normalizeInputNewlines(s))
	if c.charLimit > 0 {
		available := c.charLimit - (len(c.value) - (end - start))
		if available < 0 {
			available = 0
		}
		if len(replacement) > available {
			replacement = replacement[:available]
		}
	}
	next := make([]rune, 0, len(c.value)-(end-start)+len(replacement))
	next = append(next, c.value[:start]...)
	next = append(next, replacement...)
	next = append(next, c.value[end:]...)
	c.value = next
	c.cursor = start + len(replacement)
	c.preferredColumn = -1
	c.ClearSelection()
	c.ensureCursorVisible()
}

func (c *composerModel) InsertString(s string) {
	if c.HasSelection() {
		c.ReplaceSelection(s)
		return
	}
	runes := []rune(normalizeInputNewlines(s))
	if len(runes) == 0 {
		return
	}
	if c.charLimit > 0 {
		available := c.charLimit - len(c.value)
		if available <= 0 {
			return
		}
		if len(runes) > available {
			runes = runes[:available]
		}
	}
	c.cursor = clampInt(c.cursor, 0, len(c.value))
	next := make([]rune, 0, len(c.value)+len(runes))
	next = append(next, c.value[:c.cursor]...)
	next = append(next, runes...)
	next = append(next, c.value[c.cursor:]...)
	c.value = next
	c.cursor += len(runes)
	c.preferredColumn = -1
	c.ClearSelection()
	c.ensureCursorVisible()
}

func (c *composerModel) DeleteBackward() {
	if c.HasSelection() {
		c.ReplaceSelection("")
		return
	}
	if c.cursor <= 0 {
		return
	}
	c.cursor = clampInt(c.cursor, 0, len(c.value))
	c.value = append(c.value[:c.cursor-1], c.value[c.cursor:]...)
	c.cursor--
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) DeleteForward() {
	if c.HasSelection() {
		c.ReplaceSelection("")
		return
	}
	c.cursor = clampInt(c.cursor, 0, len(c.value))
	if c.cursor >= len(c.value) {
		return
	}
	c.value = append(c.value[:c.cursor], c.value[c.cursor+1:]...)
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) beginOrUpdateSelection(selecting bool) {
	if selecting {
		if !c.selecting {
			c.selectionAnchor = c.cursor
			c.selecting = true
		}
		return
	}
	c.ClearSelection()
}

func (c *composerModel) MoveLeft(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	if c.cursor > 0 {
		c.cursor--
	}
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveRight(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	if c.cursor < len(c.value) {
		c.cursor++
	}
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveWordLeft(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	i := c.cursor
	for i > 0 && unicode.IsSpace(c.value[i-1]) {
		i--
	}
	if i > 0 && isWordRune(c.value[i-1]) {
		for i > 0 && isWordRune(c.value[i-1]) {
			i--
		}
	} else {
		for i > 0 && !unicode.IsSpace(c.value[i-1]) && !isWordRune(c.value[i-1]) {
			i--
		}
	}
	c.cursor = i
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveWordRight(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	i := c.cursor
	if i < len(c.value) && isWordRune(c.value[i]) {
		for i < len(c.value) && isWordRune(c.value[i]) {
			i++
		}
	} else if i < len(c.value) && !unicode.IsSpace(c.value[i]) {
		for i < len(c.value) && !unicode.IsSpace(c.value[i]) && !isWordRune(c.value[i]) {
			i++
		}
	}
	for i < len(c.value) && unicode.IsSpace(c.value[i]) {
		i++
	}
	c.cursor = i
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveLineUp(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	pos := c.visualPositionForIndex(c.cursor)
	if c.preferredColumn < 0 {
		c.preferredColumn = pos.col
	}
	if pos.line <= 0 {
		c.cursor = 0
	} else {
		c.cursor = c.indexForVisualPosition(pos.line-1, c.preferredColumn)
	}
	c.ensureCursorVisible()
}

func (c *composerModel) MoveLineDown(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	lines := c.visualLines()
	pos := c.visualPositionForIndex(c.cursor)
	if c.preferredColumn < 0 {
		c.preferredColumn = pos.col
	}
	if pos.line >= len(lines)-1 {
		c.cursor = len(c.value)
	} else {
		c.cursor = c.indexForVisualPosition(pos.line+1, c.preferredColumn)
	}
	c.ensureCursorVisible()
}

func (c *composerModel) MoveLineStart(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	pos := c.visualPositionForIndex(c.cursor)
	lines := c.visualLines()
	if pos.line >= 0 && pos.line < len(lines) {
		c.cursor = lines[pos.line].startIndex
	}
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveLineEnd(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	pos := c.visualPositionForIndex(c.cursor)
	lines := c.visualLines()
	if pos.line >= 0 && pos.line < len(lines) {
		c.cursor = lines[pos.line].endIndex
	}
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveToBegin(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	c.cursor = 0
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) MoveToEnd(selecting bool) {
	c.beginOrUpdateSelection(selecting)
	c.cursor = len(c.value)
	c.preferredColumn = -1
	c.ensureCursorVisible()
}

func (c *composerModel) UpdateKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "ctrl+a":
		c.SelectAll()
		return true, nil
	case "ctrl+v":
		return true, pasteClipboardCmd()
	case "backspace", "ctrl+h":
		c.DeleteBackward()
		return true, nil
	case "delete":
		c.DeleteForward()
		return true, nil
	case "left", "ctrl+b":
		c.MoveLeft(false)
		return true, nil
	case "shift+left":
		c.MoveLeft(true)
		return true, nil
	case "right", "ctrl+f":
		c.MoveRight(false)
		return true, nil
	case "shift+right":
		c.MoveRight(true)
		return true, nil
	case "up", "ctrl+p":
		c.MoveLineUp(false)
		return true, nil
	case "shift+up":
		c.MoveLineUp(true)
		return true, nil
	case "down", "ctrl+n":
		c.MoveLineDown(false)
		return true, nil
	case "shift+down":
		c.MoveLineDown(true)
		return true, nil
	case "home":
		c.MoveLineStart(false)
		return true, nil
	case "shift+home":
		c.MoveLineStart(true)
		return true, nil
	case "end":
		c.MoveLineEnd(false)
		return true, nil
	case "shift+end":
		c.MoveLineEnd(true)
		return true, nil
	case "ctrl+home", "alt+<":
		c.MoveToBegin(false)
		return true, nil
	case "ctrl+shift+home":
		c.MoveToBegin(true)
		return true, nil
	case "ctrl+end", "alt+>":
		c.MoveToEnd(false)
		return true, nil
	case "ctrl+shift+end":
		c.MoveToEnd(true)
		return true, nil
	case "ctrl+left", "alt+left", "alt+b":
		c.MoveWordLeft(false)
		return true, nil
	case "ctrl+shift+left", "shift+alt+left", "alt+shift+left":
		c.MoveWordLeft(true)
		return true, nil
	case "ctrl+right", "alt+right", "alt+f":
		c.MoveWordRight(false)
		return true, nil
	case "ctrl+shift+right", "shift+alt+right", "alt+shift+right":
		c.MoveWordRight(true)
		return true, nil
	case "ctrl+u":
		c.selecting = true
		c.selectionAnchor = 0
		c.ReplaceSelection("")
		return true, nil
	case "ctrl+k":
		c.selecting = true
		c.selectionAnchor = len(c.value)
		c.ReplaceSelection("")
		return true, nil
	}

	if msg.Text != "" {
		c.InsertString(msg.Text)
		return true, nil
	}
	return false, nil
}

func (c *composerModel) InsertNewline() {
	c.InsertString("\n")
}

func (c composerModel) View(prompt string, pulseFrame int) string {
	lines := c.visualLines()
	height := c.Height()
	start := clampInt(c.scrollLine, 0, max(0, len(lines)-1))
	end := min(len(lines), start+height)
	visible := lines[start:end]
	if len(visible) == 0 {
		visible = []composerLine{{startIndex: 0, endIndex: 0}}
	}

	promptWidth := lipgloss.Width(prompt)
	continuationPrompt := strings.Repeat(" ", promptWidth)

	var out []string
	for i, line := range visible {
		linePrompt := continuationPrompt
		if i == 0 {
			linePrompt = prompt
		}
		out = append(out, linePrompt+c.renderLine(line, pulseFrame))
	}
	for len(out) < height {
		linePrompt := continuationPrompt
		if len(out) == 0 {
			linePrompt = prompt
		}
		out = append(out, linePrompt)
	}
	return strings.Join(out, "\n")
}

func (c composerModel) renderLine(line composerLine, pulseFrame int) string {
	if len(c.value) == 0 && line.startIndex == 0 {
		if c.focused && c.cursor == 0 {
			return renderComposerCursor(" ", pulseFrame) + mutedSt.Render(c.placeholder)
		}
		return mutedSt.Render(c.placeholder)
	}

	start, end := c.SelectionRange()
	var b strings.Builder
	cursorRendered := false
	for _, cell := range line.cells {
		selected := c.HasSelection() && cell.index >= start && cell.index < end
		cursorHere := c.focused && !c.HasSelection() && c.cursor == cell.index
		text := string(cell.r)
		switch {
		case cursorHere:
			b.WriteString(renderComposerCursor(text, pulseFrame))
			cursorRendered = true
		case selected:
			b.WriteString(inputSelectSt.Render(text))
		default:
			b.WriteString(text)
		}
	}
	if c.focused && !c.HasSelection() && !cursorRendered && c.cursor == line.endIndex {
		b.WriteString(renderComposerCursor(" ", pulseFrame))
	}
	return b.String()
}

func renderComposerCursor(s string, pulseFrame int) string {
	return composerCursorStyles[pulseFrame%len(composerCursorStyles)].Render(s)
}

func (c composerModel) visualLines() []composerLine {
	width := max(1, c.width)
	if len(c.value) == 0 {
		return []composerLine{{startIndex: 0, endIndex: 0}}
	}

	lines := make([]composerLine, 0)
	current := composerLine{startIndex: 0}
	currentWidth := 0
	for i, r := range c.value {
		if r == '\n' {
			current.endIndex = i
			lines = append(lines, current)
			current = composerLine{startIndex: i + 1}
			currentWidth = 0
			continue
		}
		rw := max(1, runewidth.RuneWidth(r))
		if currentWidth+rw > width && len(current.cells) > 0 {
			current.endIndex = i
			lines = append(lines, current)
			current = composerLine{startIndex: i}
			currentWidth = 0
		}
		current.cells = append(current.cells, composerCell{r: r, index: i, width: rw})
		currentWidth += rw
	}
	current.endIndex = len(c.value)
	lines = append(lines, current)
	return lines
}

type composerVisualPosition struct {
	line int
	col  int
}

func (c composerModel) visualPositionForIndex(index int) composerVisualPosition {
	index = clampInt(index, 0, len(c.value))
	lines := c.visualLines()
	for lineIndex, line := range lines {
		if index < line.startIndex || index > line.endIndex {
			continue
		}
		col := 0
		for _, cell := range line.cells {
			if cell.index >= index {
				return composerVisualPosition{line: lineIndex, col: col}
			}
			col += cell.width
		}
		return composerVisualPosition{line: lineIndex, col: col}
	}
	last := len(lines) - 1
	return composerVisualPosition{line: last, col: lineDisplayWidth(lines[last])}
}

func (c composerModel) indexForVisualPosition(lineIndex int, col int) int {
	lines := c.visualLines()
	if len(lines) == 0 {
		return 0
	}
	lineIndex = clampInt(lineIndex, 0, len(lines)-1)
	line := lines[lineIndex]
	currentCol := 0
	for _, cell := range line.cells {
		if currentCol+cell.width > col {
			return cell.index
		}
		currentCol += cell.width
	}
	return line.endIndex
}

func (c *composerModel) ensureCursorVisible() {
	lines := c.visualLines()
	if len(lines) == 0 {
		c.scrollLine = 0
		return
	}
	pos := c.visualPositionForIndex(c.cursor)
	height := c.Height()
	if pos.line < c.scrollLine {
		c.scrollLine = pos.line
	}
	if pos.line >= c.scrollLine+height {
		c.scrollLine = pos.line - height + 1
	}
	c.scrollLine = clampInt(c.scrollLine, 0, max(0, len(lines)-height))
}

func lineDisplayWidth(line composerLine) int {
	width := 0
	for _, cell := range line.cells {
		width += cell.width
	}
	return width
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func pasteClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		return composerPasteMsg{text: text, err: err}
	}
}

func copyClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(text); err != nil {
			return runtimeErrMsg{err: fmt.Errorf("copy selection: %w", err)}
		}
		return nil
	}
}

func clampInt(value int, low int, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
