package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	forkTreeNodeWidth = 24
	forkTreeColGap    = 10
	forkTreeRowGap    = 2
)

func newForkTreeState() forkTreeState {
	return forkTreeState{Loading: true, Selected: 0, Preview: viewport.New()}
}

func (m *model) applyForkTree(ev runtimeEvent) {
	state := m.forkTree
	state.Loading = false
	state.Nodes = cloneForkTreeNodes(ev.ForkTreeNodes)
	state.Selected = 0
	state.Preview = viewport.New()
	state.PreviewWidth = 0
	state.PreviewHeight = 0
	state.buildForkTreeLayout(m.session)
	m.forkTree = state
	m.busy = false
	m.status = "fork tree"
	m.err = ""
	m.updateForkTreePreview()
}

func cloneForkTreeNodes(nodes []forkTreeNode) []forkTreeNode {
	cloned := make([]forkTreeNode, len(nodes))
	copy(cloned, nodes)
	for i := range cloned {
		cloned[i].Parent = -1
		cloned[i].Children = nil
	}
	return cloned
}

func (s *forkTreeState) buildForkTreeLayout(currentSessionID string) {
	if len(s.Nodes) == 0 {
		s.Order = nil
		s.CanvasWidth = 0
		s.CanvasHeight = 0
		s.Selected = 0
		return
	}

	idToIndex := make(map[string]int, len(s.Nodes))
	for i := range s.Nodes {
		s.Nodes[i].Parent = -1
		s.Nodes[i].Children = nil
		idToIndex[s.Nodes[i].SessionID] = i
	}

	roots := make([]int, 0)
	for i := range s.Nodes {
		parentID := s.Nodes[i].ParentSessionID
		if parentID != "" {
			if parentIndex, ok := idToIndex[parentID]; ok && parentIndex != i {
				s.Nodes[i].Parent = parentIndex
				s.Nodes[parentIndex].Children = append(s.Nodes[parentIndex].Children, i)
				continue
			}
		}
		roots = append(roots, i)
	}

	lessNode := func(a, b int) bool {
		left := fallback(s.Nodes[a].ForkedAt, fallback(s.Nodes[a].CreatedAt, s.Nodes[a].UpdatedAt))
		right := fallback(s.Nodes[b].ForkedAt, fallback(s.Nodes[b].CreatedAt, s.Nodes[b].UpdatedAt))
		if left == right {
			return s.Nodes[a].SessionTitle < s.Nodes[b].SessionTitle
		}
		return left < right
	}
	sort.SliceStable(roots, func(i, j int) bool { return lessNode(roots[i], roots[j]) })
	for i := range s.Nodes {
		children := s.Nodes[i].Children
		sort.SliceStable(children, func(a, b int) bool { return lessNode(children[a], children[b]) })
		s.Nodes[i].Children = children
	}

	s.Order = nil
	row := 0
	maxDepth := 0
	var walk func(index int, depth int)
	walk = func(index int, depth int) {
		s.Nodes[index].X = depth * (forkTreeNodeWidth + forkTreeColGap)
		s.Nodes[index].Y = row * (3 + forkTreeRowGap)
		s.Order = append(s.Order, index)
		row++
		if depth > maxDepth {
			maxDepth = depth
		}
		for _, child := range s.Nodes[index].Children {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}

	s.CanvasWidth = max(forkTreeNodeWidth, maxDepth*(forkTreeNodeWidth+forkTreeColGap)+forkTreeNodeWidth)
	s.CanvasHeight = max(3, row*(3+forkTreeRowGap))

	for i, node := range s.Nodes {
		if node.SessionID == currentSessionID {
			s.Selected = i
			return
		}
	}
	if s.Selected < 0 || s.Selected >= len(s.Nodes) {
		s.Selected = 0
	}
}

func (m model) updateForkTree(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeChat
		m.status = "ready"
		return m, nil
	case "up", "k", "ctrl+p":
		m.moveForkTreeOrder(-1)
	case "down", "j", "ctrl+n":
		m.moveForkTreeOrder(1)
	case "left", "h":
		m.moveForkTreeParent()
	case "right", "l":
		m.moveForkTreeChild()
	case "pgup":
		m.forkTree.Preview.PageUp()
	case "pgdown":
		m.forkTree.Preview.PageDown()
	case "enter":
		return m.activateSelectedForkTreeSession()
	}
	return m, nil
}

func (m model) updateForkTreeMouseWheel(msg tea.MouseWheelMsg) model {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.forkTree.Preview.ScrollUp(max(1, m.forkTree.Preview.MouseWheelDelta))
	case tea.MouseWheelDown:
		m.forkTree.Preview.ScrollDown(max(1, m.forkTree.Preview.MouseWheelDelta))
	}
	return m
}

func (m *model) moveForkTreeOrder(delta int) {
	if len(m.forkTree.Order) == 0 || len(m.forkTree.Nodes) == 0 {
		return
	}
	currentPos := 0
	for i, index := range m.forkTree.Order {
		if index == m.forkTree.Selected {
			currentPos = i
			break
		}
	}
	currentPos = (currentPos + delta + len(m.forkTree.Order)) % len(m.forkTree.Order)
	m.forkTree.Selected = m.forkTree.Order[currentPos]
	m.updateForkTreePreview()
}

func (m *model) moveForkTreeParent() {
	if !m.forkTree.hasSelectedNode() {
		return
	}
	parent := m.forkTree.Nodes[m.forkTree.Selected].Parent
	if parent >= 0 {
		m.forkTree.Selected = parent
		m.updateForkTreePreview()
	}
}

func (m *model) moveForkTreeChild() {
	if !m.forkTree.hasSelectedNode() {
		return
	}
	children := m.forkTree.Nodes[m.forkTree.Selected].Children
	if len(children) > 0 {
		m.forkTree.Selected = children[0]
		m.updateForkTreePreview()
	}
}

func (s forkTreeState) hasSelectedNode() bool {
	return s.Selected >= 0 && s.Selected < len(s.Nodes)
}

func (m model) activateSelectedForkTreeSession() (tea.Model, tea.Cmd) {
	if !m.forkTree.hasSelectedNode() {
		return m, nil
	}
	node := m.forkTree.Nodes[m.forkTree.Selected]
	if node.SessionID == "" {
		return m, nil
	}
	m.mode = modeChat
	m.clearFakeStream()
	m.autoScroll = true
	m.busy = true
	m.session = node.SessionID
	m.title = node.SessionTitle
	m.autoNewSessionOnChat = false
	m.messages = nil
	m.status = "loading transcript"
	m.layout()
	m.refreshViewport()
	return m, sendRuntime(m.runtime, map[string]any{"type": "session.activate", "sessionId": node.SessionID})
}

func (m *model) updateForkTreePreview() {
	if m.forkTree.Preview.Width() <= 0 {
		m.forkTree.Preview.SetWidth(max(20, m.forkTree.PreviewWidth))
	}
	if m.forkTree.Preview.Height() <= 0 {
		m.forkTree.Preview.SetHeight(max(1, m.forkTree.PreviewHeight))
	}
	if !m.forkTree.hasSelectedNode() {
		m.forkTree.Preview.SetContent(mutedSt.Render("No fork tree nodes yet."))
		return
	}
	node := m.forkTree.Nodes[m.forkTree.Selected]
	width := max(20, m.forkTree.PreviewWidth)
	var content string
	if strings.TrimSpace(node.MessageContent) == "" {
		content = mutedSt.Render("No text message preview for this fork.")
	} else {
		rendered, err := renderMessageContentAtWidth(chatMessage{Role: node.MessageRole, Content: node.MessageContent}, width)
		if err != nil {
			content = wrap(node.MessageContent, width)
		} else {
			content = rendered
		}
	}
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(fallback(node.SessionTitle, "Untitled chat"))
	metaParts := []string{fallback(node.MessageRole, "text"), fmt.Sprintf("%d msgs", node.MessageCount)}
	if showForkTreeDevDetails() {
		metaParts = append(metaParts, short(node.SessionID, 14))
		if node.ParentSessionID != "" {
			metaParts = append(metaParts, "fork of "+short(node.ParentSessionID, 14))
		}
	}
	meta := mutedSt.Render(strings.Join(metaParts, " · "))
	m.forkTree.Preview.SetContent(header + "\n" + meta + "\n\n" + content)
}

func (m model) renderForkTreeModal(height int) string {
	if height <= 0 {
		return ""
	}
	if m.forkTree.Loading {
		return mutedSt.Render("loading fork tree...")
	}
	if len(m.forkTree.Nodes) == 0 {
		return mutedSt.Render("no sessions yet")
	}

	previewWidth := clampInt(m.width/3, 24, max(24, m.width-36))
	if m.width < 90 {
		previewWidth = min(max(20, m.width/2-2), max(20, m.width-24))
	}
	treeWidth := max(20, m.width-previewWidth-4)
	paneHeight := max(1, height-2)
	m.forkTree.PreviewWidth = max(20, previewWidth-2)
	m.forkTree.PreviewHeight = paneHeight
	m.forkTree.Preview.SetWidth(m.forkTree.PreviewWidth)
	m.forkTree.Preview.SetHeight(paneHeight)
	m.updateForkTreePreview()

	previewTitle := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("selected message")
	preview := previewTitle + "\n" + m.forkTree.Preview.View()
	preview = renderFixedHeight(preview, height)

	treeTitle := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("fork tree") + "  " + mutedSt.Render("current lineage · text messages only")
	tree := treeTitle + "\n" + m.renderForkTreeCanvas(treeWidth, paneHeight)
	tree = renderFixedHeight(tree, height)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Background(surface).Foreground(text).Padding(0, 1).Width(previewWidth).Render(preview),
		"  ",
		lipgloss.NewStyle().Foreground(text).Width(treeWidth).Render(tree),
	)
}

func (m model) renderForkTreeCanvas(width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	canvas := newRuneCanvas(max(width, m.forkTree.CanvasWidth+2), max(height, m.forkTree.CanvasHeight+2))
	selectedAncestors := m.forkTree.selectedAncestorEdges()
	for i := range m.forkTree.Nodes {
		node := m.forkTree.Nodes[i]
		if node.Parent >= 0 && node.Parent < len(m.forkTree.Nodes) {
			parent := m.forkTree.Nodes[node.Parent]
			edgeSelected := i == m.forkTree.Selected || selectedAncestors[i]
			drawForkTreeEdge(canvas, parent.X+forkTreeNodeWidth, parent.Y+1, node.X, node.Y+1, edgeSelected)
		}
	}
	showDev := showForkTreeDevDetails()
	for i := range m.forkTree.Nodes {
		drawForkTreeNode(canvas, m.forkTree.Nodes[i], i == m.forkTree.Selected, showDev)
	}

	selected := m.forkTree.Nodes[m.forkTree.Selected]
	offsetX := clampInt(selected.X+forkTreeNodeWidth/2-width/2, 0, max(0, canvas.width-width))
	offsetY := clampInt(selected.Y+1-height/2, 0, max(0, canvas.height()-height))
	return canvas.render(offsetX, offsetY, width, height)
}

func (s forkTreeState) selectedAncestorEdges() map[int]bool {
	ancestors := map[int]bool{}
	if !s.hasSelectedNode() {
		return ancestors
	}
	for index := s.Selected; index >= 0 && index < len(s.Nodes); {
		parent := s.Nodes[index].Parent
		if parent < 0 || parent >= len(s.Nodes) {
			break
		}
		ancestors[index] = true
		index = parent
	}
	return ancestors
}

func drawForkTreeEdge(canvas *runeCanvas, x1 int, y1 int, x2 int, y2 int, selected bool) {
	style := mutedSt
	if selected {
		style = lipgloss.NewStyle().Foreground(accent).Bold(true)
	}
	midX := x1 + max(2, (x2-x1)/2)
	canvas.drawHorizontal(x1, midX, y1, '─', style)
	canvas.set(midX, y1, '┐', style)
	canvas.drawVertical(midX, y1, y2, '│', style)
	canvas.set(midX, y2, '└', style)
	canvas.drawHorizontal(midX, x2-1, y2, '─', style)
}

func drawForkTreeNode(canvas *runeCanvas, node forkTreeNode, selected bool, showDev bool) {
	prefix := "█"
	style := lipgloss.NewStyle().Foreground(cyan)
	textStyle := lipgloss.NewStyle().Foreground(text)
	if selected {
		prefix = "▓"
		style = lipgloss.NewStyle().Foreground(accent).Bold(true)
		textStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	}
	lines := []string{
		oneLine(fallback(node.MessageRole, "text")+": "+node.MessageContent, forkTreeNodeWidth-3),
	}
	if showDev {
		lines = append(lines,
			oneLine(fallback(node.SessionTitle, "Untitled chat"), forkTreeNodeWidth-3),
			oneLine(short(node.SessionID, 18), forkTreeNodeWidth-3),
		)
	}
	for row, line := range lines {
		y := node.Y + row
		canvas.writeStyledString(node.X, y, prefix, style)
		canvas.writeStyledString(node.X+2, y, line, textStyle)
	}
}

type runeCanvas struct {
	width int
	rows  [][]string
}

func newRuneCanvas(width int, height int) *runeCanvas {
	rows := make([][]string, height)
	for y := range rows {
		rows[y] = make([]string, width)
		for x := range rows[y] {
			rows[y][x] = " "
		}
	}
	return &runeCanvas{width: width, rows: rows}
}

func (c *runeCanvas) height() int {
	return len(c.rows)
}

func (c *runeCanvas) set(x int, y int, r rune, style lipgloss.Style) {
	c.writeStyledString(x, y, string(r), style)
}

func (c *runeCanvas) writeStyledString(x int, y int, s string, style lipgloss.Style) {
	if y < 0 || y >= len(c.rows) {
		return
	}
	for i, r := range []rune(s) {
		cellX := x + i
		if cellX < 0 || cellX >= c.width {
			continue
		}
		cell := string(r)
		if cell != " " {
			cell = style.Render(cell)
		}
		c.rows[y][cellX] = cell
	}
}

func (c *runeCanvas) drawHorizontal(x1 int, x2 int, y int, r rune, style lipgloss.Style) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	for x := x1; x <= x2; x++ {
		c.set(x, y, r, style)
	}
}

func (c *runeCanvas) drawVertical(x int, y1 int, y2 int, r rune, style lipgloss.Style) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	for y := y1; y <= y2; y++ {
		c.set(x, y, r, style)
	}
}

func (c *runeCanvas) render(offsetX int, offsetY int, width int, height int) string {
	lines := make([]string, 0, height)
	for y := 0; y < height; y++ {
		sourceY := offsetY + y
		if sourceY < 0 || sourceY >= c.height() {
			lines = append(lines, "")
			continue
		}
		endX := min(c.width, offsetX+width)
		line := c.rows[sourceY][clampInt(offsetX, 0, c.width):endX]
		lines = append(lines, strings.TrimRight(strings.Join(line, ""), " "))
	}
	return strings.Join(lines, "\n")
}

func showForkTreeDevDetails() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("QUBIT_DEV_FORK_TREE")))
	if value == "1" || value == "true" || value == "yes" || value == "on" {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(os.Getenv("QUBIT_DEV")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
