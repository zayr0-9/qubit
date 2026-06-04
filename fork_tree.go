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
	forkTreeNodeWidth  = 2
	forkTreeColGap     = 3
	forkTreeRowGap     = 2
	forkTreeNodeHeight = 1
)

func newForkTreeState() forkTreeState {
	return forkTreeState{Loading: true, Selected: 0, Preview: viewport.New()}
}

func (m *model) applyForkTree(ev runtimeEvent) {
	state := m.forkTree
	focalSessionID := fallback(ev.SessionID, fallback(state.FocalSessionID, m.session))
	state.Loading = false
	state.FocalSessionID = focalSessionID
	state.Nodes = expandForkTreeMessageNodes(cloneForkTreeNodes(ev.ForkTreeNodes))
	state.Selected = 0
	state.Preview = viewport.New()
	state.PreviewWidth = 0
	state.PreviewHeight = 0
	state.buildForkTreeLayout(focalSessionID)
	m.forkTree = state
	m.busy = false
	m.status = "fork tree"
	m.err = ""
	m.updateForkTreePreview(false)
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

func expandForkTreeMessageNodes(sessionNodes []forkTreeNode) []forkTreeNode {
	expanded := make([]forkTreeNode, 0, len(sessionNodes))
	for _, sessionNode := range sessionNodes {
		if len(sessionNode.MessageNodes) == 0 {
			expanded = append(expanded, sessionNode)
			continue
		}
		for i, messageNode := range sessionNode.MessageNodes {
			content := strings.TrimSpace(messageNode.Content)
			if content == "" {
				continue
			}
			node := sessionNode
			node.ID = fallback(messageNode.ID, fmt.Sprintf("%s:%d", sessionNode.SessionID, i))
			node.ParentNodeID = messageNode.ParentID
			node.ParentSessionID = ""
			node.MessageRole = fallback(messageNode.Role, node.MessageRole)
			node.MessageContent = content
			node.AssistantRole = ""
			node.AssistantContent = ""
			node.MessageNodes = nil
			node.ForkedFromMessageIndex = messageNode.MessageIndex
			node.Parent = -1
			node.Children = nil
			expanded = append(expanded, node)
		}
	}
	return expanded
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
	sessionIDToIndex := make(map[string]int, len(s.Nodes))
	for i := range s.Nodes {
		s.Nodes[i].Parent = -1
		s.Nodes[i].Children = nil
		if s.Nodes[i].ID != "" {
			idToIndex[s.Nodes[i].ID] = i
		}
		if _, exists := sessionIDToIndex[s.Nodes[i].SessionID]; !exists {
			sessionIDToIndex[s.Nodes[i].SessionID] = i
		}
	}

	roots := make([]int, 0)
	for i := range s.Nodes {
		parentID := s.Nodes[i].ParentNodeID
		if parentID == "" {
			parentID = s.Nodes[i].ParentSessionID
		}
		if parentID != "" && !isVirtualRootFork(s.Nodes[i]) {
			parentIndex, ok := idToIndex[parentID]
			if !ok {
				parentIndex, ok = sessionIDToIndex[parentID]
			}
			if ok && parentIndex != i {
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
	maxDepth := 0
	maxRow := 0
	nextRow := 0
	var walk func(index int, depth int, row int)
	walk = func(index int, depth int, row int) {
		s.Nodes[index].X = depth * (forkTreeNodeWidth + forkTreeColGap)
		s.Nodes[index].Y = row * (forkTreeNodeHeight + forkTreeRowGap)
		s.Order = append(s.Order, index)
		if depth > maxDepth {
			maxDepth = depth
		}
		if row > maxRow {
			maxRow = row
		}
		children := s.Nodes[index].Children
		sameSessionChildren := make([]int, 0, len(children))
		forkChildren := make([]int, 0, len(children))
		for _, child := range children {
			if s.Nodes[child].SessionID == s.Nodes[index].SessionID {
				sameSessionChildren = append(sameSessionChildren, child)
			} else {
				forkChildren = append(forkChildren, child)
			}
		}
		for i, child := range sameSessionChildren {
			childRow := row
			if i > 0 {
				nextRow++
				childRow = nextRow
			}
			walk(child, depth+1, childRow)
		}
		for _, child := range forkChildren {
			nextRow++
			walk(child, depth+1, nextRow)
		}
	}
	for _, root := range roots {
		rootRow := nextRow
		walk(root, 0, rootRow)
		nextRow = max(nextRow, maxRow) + 1
	}

	s.CanvasWidth = max(forkTreeNodeWidth, maxDepth*(forkTreeNodeWidth+forkTreeColGap)+forkTreeNodeWidth)
	s.CanvasHeight = max(forkTreeNodeHeight, (maxRow+1)*(forkTreeNodeHeight+forkTreeRowGap))

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

func isVirtualRootFork(node forkTreeNode) bool {
	return node.ParentSessionID != "" && node.ForkedFromMessageIndex == 0 && strings.HasPrefix(strings.TrimSpace(node.SessionTitle), "Edit:")
}

func (m model) updateForkTree(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.previousMode == modeSessionPicker {
			m.setSessionCursorForSession(m.currentForkTreeSessionID())
			m.mode = modeSessionPicker
		} else {
			m.mode = modeChat
		}
		m.previousMode = modeChat
		m.status = "ready"
		return m, nil
	case "up":
		m.moveForkTreeParallel(-1)
	case "down":
		m.moveForkTreeParallel(1)
	case "k", "ctrl+p":
		m.moveForkTreeOrder(-1)
	case "j", "ctrl+n":
		m.moveForkTreeOrder(1)
	case "left", "h":
		m.moveForkTreeParent()
	case "right", "l":
		m.moveForkTreeChild()
	case "pgup":
		if m.previousMode == modeSessionPicker {
			return m.pageForkTreeSession(-1)
		}
		m.forkTree.Preview.PageUp()
	case "pgdown":
		if m.previousMode == modeSessionPicker {
			return m.pageForkTreeSession(1)
		}
		m.forkTree.Preview.PageDown()
	case "enter":
		return m.activateSelectedForkTreeSession()
	}
	return m, nil
}

func (m model) currentForkTreeSessionID() string {
	if m.forkTree.FocalSessionID != "" {
		return m.forkTree.FocalSessionID
	}
	if m.forkTree.hasSelectedNode() {
		return m.forkTree.Nodes[m.forkTree.Selected].SessionID
	}
	return ""
}

func (m model) pageForkTreeSession(delta int) (tea.Model, tea.Cmd) {
	sessions := m.sessionPickerSessions()
	if len(sessions) == 0 {
		return m, nil
	}
	currentID := m.currentForkTreeSessionID()
	index := m.sessionCursor
	for i, session := range sessions {
		if session.ID == currentID {
			index = i
			break
		}
	}
	if index < 0 || index >= len(sessions) {
		index = 0
	}
	index = (index + delta + len(sessions)) % len(sessions)
	m.sessionCursor = index
	return m.openForkTreeForSession(sessions[index].ID)
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
	m.updateForkTreePreview(true)
}

func (m *model) moveForkTreeParallel(delta int) {
	if !m.forkTree.hasSelectedNode() || len(m.forkTree.Nodes) == 0 {
		return
	}
	selected := m.forkTree.Nodes[m.forkTree.Selected]
	candidates := make([]int, 0)
	for i, node := range m.forkTree.Nodes {
		if i == m.forkTree.Selected || node.Y == selected.Y {
			continue
		}
		if delta < 0 && node.Y < selected.Y {
			candidates = append(candidates, i)
		}
		if delta > 0 && node.Y > selected.Y {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := m.forkTree.Nodes[candidates[i]]
		right := m.forkTree.Nodes[candidates[j]]
		leftRowDistance := absInt(left.Y - selected.Y)
		rightRowDistance := absInt(right.Y - selected.Y)
		if leftRowDistance != rightRowDistance {
			return leftRowDistance < rightRowDistance
		}
		leftXDistance := absInt(left.X - selected.X)
		rightXDistance := absInt(right.X - selected.X)
		if leftXDistance != rightXDistance {
			return leftXDistance < rightXDistance
		}
		return left.X < right.X
	})
	m.forkTree.Selected = candidates[0]
	m.updateForkTreePreview(true)
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *model) moveForkTreeParent() {
	if !m.forkTree.hasSelectedNode() {
		return
	}
	parent := m.forkTree.Nodes[m.forkTree.Selected].Parent
	if parent >= 0 {
		m.forkTree.Selected = parent
		m.updateForkTreePreview(true)
	}
}

func (m *model) moveForkTreeChild() {
	if !m.forkTree.hasSelectedNode() {
		return
	}
	children := m.forkTree.Nodes[m.forkTree.Selected].Children
	if len(children) > 0 {
		m.forkTree.Selected = children[0]
		m.updateForkTreePreview(true)
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

func (m *model) updateForkTreePreview(focusSelectedMessage bool) {
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
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(fallback(node.SessionTitle, "Untitled chat"))
	metaParts := []string{fmt.Sprintf("%d msgs", node.MessageCount)}
	if showForkTreeDevDetails() {
		metaParts = append(metaParts, short(node.SessionID, 14))
		if node.ParentSessionID != "" {
			metaParts = append(metaParts, "fork of "+short(node.ParentSessionID, 14))
		}
	}
	meta := mutedSt.Render(strings.Join(metaParts, " · "))
	content, selectedMessageOffset := m.renderForkTreeLineagePreviewWithOffset(node, width)
	m.forkTree.Preview.SetContent(header + "\n" + meta + "\n\n" + content)
	if focusSelectedMessage {
		m.forkTree.Preview.SetYOffset(selectedMessageOffset + 3)
	}
}

func (m *model) renderForkTreeLineagePreview(node forkTreeNode, width int) string {
	content, _ := m.renderForkTreeLineagePreviewWithOffset(node, width)
	return content
}

func (m *model) renderForkTreeLineagePreviewWithOffset(node forkTreeNode, width int) (string, int) {
	if len(node.TranscriptMessages) > 0 {
		return m.renderForkTreeTranscriptPreviewWithOffset(node, width)
	}

	messages := forkTreeLineageMessages(node)
	if len(messages) == 0 {
		return mutedSt.Render("No text message preview for this fork."), 0
	}
	selectedMessageOffset := 0
	parts := make([]string, 0, len(messages))
	for i, message := range messages {
		rendered, err := renderMessageContentAtWidth(message, width)
		if err != nil {
			rendered = wrap(message.Content, width)
		}
		if i == selectedForkTreeLineageMessageIndex(node, len(messages)) {
			selectedMessageOffset = forkTreePreviewLineCount(parts)
		}
		parts = append(parts, renderMessageWithIcon(message, rendered, 0))
	}
	return strings.Join(parts, "\n\n"), selectedMessageOffset
}

func (m *model) renderForkTreeTranscriptPreviewWithOffset(node forkTreeNode, width int) (string, int) {
	messages := node.TranscriptMessages
	selectedIndex := selectedForkTreeTranscriptMessageIndex(node, messages)
	selectedMessageOffset := 0
	parts := make([]string, 0, len(messages))
	contentLine := 0
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		if i > 0 {
			separator := messageSeparator(messages[i-1], message)
			parts = append(parts, separator)
			contentLine += separatorBlankLineCount(separator)
		}
		if i == selectedIndex {
			selectedMessageOffset = contentLine
		}
		if message.Role == "view" {
			rendered := m.renderViewMessage(message)
			parts = append(parts, rendered)
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "tool" {
			groups := []*toolGroup{}
			for i < len(messages) && messages[i].Role == "tool" {
				if messages[i].ToolGroup != nil {
					groups = append(groups, messages[i].ToolGroup)
				}
				i++
			}
			i--
			rendered := m.renderCollapsedToolGroups(groups, max(20, width))
			parts = append(parts, rendered)
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "reasoning" {
			rendered := m.renderReasoningBlock(message, max(20, width))
			parts = append(parts, rendered)
			contentLine += renderedLineCount(rendered)
			continue
		}
		rendered := renderMessageWithIcon(message, m.renderForkTreeMessageContent(message, width), messageDisplayNumber(messages, i))
		parts = append(parts, rendered)
		contentLine += renderedLineCount(rendered)
	}
	return strings.Join(parts, ""), selectedMessageOffset
}

func (m *model) renderForkTreeMessageContent(message chatMessage, width int) string {
	rendered, err := renderMessageContentAtWidth(message, max(20, width))
	if err != nil {
		return wrap(message.Content, max(20, width))
	}
	return rendered
}

func selectedForkTreeTranscriptMessageIndex(node forkTreeNode, messages []chatMessage) int {
	if len(messages) == 0 {
		return 0
	}
	target := node.ForkedFromMessageIndex
	if target < 0 {
		target = 0
	}
	textIndex := 0
	lastTextMessage := -1
	for i, message := range messages {
		if !forkTreePreviewTextMessage(message) {
			continue
		}
		lastTextMessage = i
		if textIndex >= target {
			return i
		}
		textIndex++
	}
	if lastTextMessage >= 0 {
		return lastTextMessage
	}
	return min(target, len(messages)-1)
}

func forkTreePreviewTextMessage(message chatMessage) bool {
	role := normalizedForkTreeRole(message.Role)
	return (role == "user" || role == "assistant") && strings.TrimSpace(message.Content) != ""
}

func forkTreePreviewLineCount(parts []string) int {
	if len(parts) == 0 {
		return 0
	}
	return strings.Count(strings.Join(parts, "\n\n"), "\n") + 2
}

func selectedForkTreeLineageMessageIndex(node forkTreeNode, messageCount int) int {
	if messageCount <= 0 {
		return 0
	}
	index := node.ForkedFromMessageIndex
	if index < 0 || index >= messageCount {
		index = messageCount - 1
	}
	return index
}

func forkTreeLineageMessages(node forkTreeNode) []chatMessage {
	if len(node.LineageMessages) > 0 {
		messages := make([]chatMessage, 0, len(node.LineageMessages))
		for _, message := range node.LineageMessages {
			content := strings.TrimSpace(message.Content)
			if content == "" {
				continue
			}
			messages = append(messages, chatMessage{Role: fallback(normalizedForkTreeRole(message.Role), "assistant"), Content: content})
		}
		return messages
	}
	messages := make([]chatMessage, 0, 2)
	if content := strings.TrimSpace(node.MessageContent); content != "" {
		messages = append(messages, chatMessage{Role: fallback(normalizedForkTreeRole(node.MessageRole), "user"), Content: content})
	}
	if content := strings.TrimSpace(node.AssistantContent); content != "" && content != strings.TrimSpace(node.MessageContent) {
		messages = append(messages, chatMessage{Role: fallback(normalizedForkTreeRole(node.AssistantRole), "assistant"), Content: content})
	}
	return messages
}
func normalizedForkTreeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "user"
	case "assistant", "agent":
		return "assistant"
	default:
		return ""
	}
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
	m.updateForkTreePreview(false)

	previewTitle := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("chat lineage")
	preview := previewTitle + "\n" + m.forkTree.Preview.View()
	preview = renderFixedHeight(preview, height)

	treeTitle := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("fork tree") + "  " + mutedSt.Render("current lineage · full transcript")
	tree := treeTitle + "\n" + m.renderForkTreeCanvas(treeWidth, paneHeight)
	tree = renderFixedHeight(tree, height)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Foreground(text).Padding(0, 1).Width(previewWidth).Render(preview),
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
			drawForkTreeEdge(canvas, parent.X+forkTreeNodeWidth, parent.Y, node.X, node.Y, forkTreeEdgeStyle(edgeSelected, m.inputCursorPulse))
		}
	}
	showDev := showForkTreeDevDetails()
	for i := range m.forkTree.Nodes {
		drawForkTreeNode(canvas, m.forkTree.Nodes[i], i == m.forkTree.Selected, showDev)
	}

	selected := m.forkTree.Nodes[m.forkTree.Selected]
	offsetX := clampInt(selected.X+forkTreeNodeWidth/2-width/2, 0, max(0, canvas.width-width))
	offsetY := clampInt(selected.Y-height/2, 0, max(0, canvas.height()-height))
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

func forkTreeEdgeStyle(selected bool, pulseFrame int) lipgloss.Style {
	if !selected {
		return mutedSt
	}
	if len(forkTreeBranchStyles) == 0 {
		return selectSt
	}
	return forkTreeBranchStyles[pulseFrame%len(forkTreeBranchStyles)].Bold(true)
}

func drawForkTreeEdge(canvas *runeCanvas, x1 int, y1 int, x2 int, y2 int, style lipgloss.Style) {
	midX := x1 + max(2, (x2-x1)/2)
	canvas.drawHorizontal(x1, midX, y1, '─', style)
	if y1 == y2 {
		canvas.drawHorizontal(midX, x2-1, y2, '─', style)
		return
	}
	cornerDown := '┐'
	cornerOut := '└'
	if y2 < y1 {
		cornerDown = '┘'
		cornerOut = '┌'
	}
	canvas.set(midX, y1, cornerDown, style)
	canvas.drawVertical(midX, y1+sign(y2-y1), y2-sign(y2-y1), '│', style)
	canvas.set(midX, y2, cornerOut, style)
	canvas.drawHorizontal(midX, x2-1, y2, '─', style)
}

func drawForkTreeNode(canvas *runeCanvas, node forkTreeNode, selected bool, showDev bool) {
	markerRole := node.MessageRole
	entries := forkTreeNodeEntries(node)
	if len(entries) > 0 {
		markerRole = entries[0].role
	}
	canvas.writeStyledString(node.X, node.Y, "■", forkTreeNodeMarkerStyle(normalizedForkTreeRole(markerRole), selected))
	if showDev {
		devStyle := mutedSt
		if selected {
			devStyle = forkTreeSelectedSt
		}
		canvas.writeStyledString(node.X+2, node.Y, short(node.SessionID, 10), devStyle)
	}
}

type forkTreeNodeEntry struct {
	role string
	text string
}

func forkTreeNodeEntries(node forkTreeNode) []forkTreeNodeEntry {
	entries := make([]forkTreeNodeEntry, 0, 2)
	if text := strings.TrimSpace(node.MessageContent); text != "" {
		entries = append(entries, forkTreeNodeEntry{role: node.MessageRole, text: text})
	}
	if text := strings.TrimSpace(node.AssistantContent); text != "" && text != strings.TrimSpace(node.MessageContent) {
		entries = append(entries, forkTreeNodeEntry{role: fallback(node.AssistantRole, "assistant"), text: text})
	}
	if len(entries) == 0 {
		entries = append(entries, forkTreeNodeEntry{role: node.MessageRole, text: "No text preview"})
	}
	return entries
}

func forkTreeNodeText(node forkTreeNode) string {
	entries := forkTreeNodeEntries(node)
	if len(entries) == 0 {
		return "No text preview"
	}
	return entries[0].text
}

func forkTreeNodeExtraRows(node forkTreeNode) int {
	return 0
}

func forkTreeNodeMarkerStyle(role string, selected bool) lipgloss.Style {
	if selected {
		return forkTreeSelectedSt
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "agent":
		return lipgloss.NewStyle().Foreground(cyan)
	case "user":
		return lipgloss.NewStyle().Foreground(accent)
	default:
		return mutedSt
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
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
