package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTreeCommandOpensForkTreeAndRequestsSessionTree(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_root"
	m.composer.SetValue("/tree")

	updated, cmd := m.submitInput()
	got := updated.(model)

	if got.mode != modeForkTree {
		t.Fatalf("mode = %v, want modeForkTree", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading fork tree")
	}
	if got.status != "loading fork tree" {
		t.Fatalf("status = %q, want loading fork tree", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.tree", "sess_root")
}

func TestCtrlSpaceOpensForkTreeDirectlyWithoutSlashPalette(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.session = "sess_root"
	m.composer.SetValue("/")

	updated, cmd := m.updateKey(tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl})
	got := updated.(model)

	if got.showSlashPalette() {
		t.Fatal("slash palette visible after ctrl+space shortcut")
	}
	if got.composer.Value() != "" {
		t.Fatalf("composer value = %q, want cleared", got.composer.Value())
	}
	if got.mode != modeForkTree {
		t.Fatalf("mode = %v, want modeForkTree", got.mode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading fork tree")
	}
	if got.status != "loading fork tree" {
		t.Fatalf("status = %q, want loading fork tree", got.status)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.tree", "sess_root")
}

func TestApplyForkTreeSelectsCurrentSessionAndRendersPreview(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.session = "sess_child"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()

	m.applyForkTree(runtimeEvent{Type: "session.tree", SessionID: "sess_child", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageRole: "user", MessageContent: "root question", MessageCount: 1},
		{ID: "sess_child", SessionID: "sess_child", SessionTitle: "Child", ParentSessionID: "sess_root", MessageRole: "assistant", MessageContent: "child **preview**", MessageCount: 2},
	}})

	if m.busy {
		t.Fatal("busy = true, want false after tree load")
	}
	if !m.forkTree.hasSelectedNode() {
		t.Fatal("selected node missing")
	}
	if got := m.forkTree.Nodes[m.forkTree.Selected].SessionID; got != "sess_child" {
		t.Fatalf("selected session = %q, want sess_child", got)
	}
	preview := m.renderForkTreeLineagePreview(m.forkTree.Nodes[m.forkTree.Selected], 60)
	if !strings.Contains(plainText(m.forkTree.Preview.View()), "Child") {
		t.Fatalf("preview title = %q, want selected child title", plainText(m.forkTree.Preview.View()))
	}
	plainPreview := plainText(preview)
	if !strings.Contains(plainPreview, "child") || !strings.Contains(plainPreview, "preview") {
		t.Fatalf("preview = %q, want selected child message", preview)
	}
}

func TestForkTreePreviewShowsScrollableChatLineage(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.session = "sess_child"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()

	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_child", SessionID: "sess_child", SessionTitle: "Child", MessageCount: 3, LineageMessages: []forkTreeLineageMessage{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
			{Role: "user", Content: "follow up"},
		}},
	}})

	node := m.forkTree.Nodes[m.forkTree.Selected]
	if node.SessionTitle != "Child" || node.MessageCount != 3 {
		t.Fatalf("selected node = %#v, want title Child and 3 msgs", node)
	}
	preview := plainText(m.renderForkTreeLineagePreview(node, 60))
	for _, want := range []string{"first question", "first answer", "follow up"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview = %q, want lineage message %q", preview, want)
		}
	}
}

func TestForkTreeExpandsMessageNodesIntoHorizontalLineage(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_root"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageNodes: []forkTreeMessageNode{
			{ID: "root-a", SessionID: "sess_root", Role: "user", Content: "A"},
			{ID: "root-b", ParentID: "root-a", SessionID: "sess_root", Role: "assistant", Content: "B"},
			{ID: "root-c", ParentID: "root-b", SessionID: "sess_root", Role: "user", Content: "C"},
			{ID: "root-d", ParentID: "root-c", SessionID: "sess_root", Role: "assistant", Content: "D"},
		}},
		{ID: "sess_fork", SessionID: "sess_fork", SessionTitle: "Fork", ParentSessionID: "sess_root", MessageNodes: []forkTreeMessageNode{
			{ID: "fork-b1", ParentID: "root-b", SessionID: "sess_fork", Role: "user", Content: "B1"},
			{ID: "fork-c1", ParentID: "fork-b1", SessionID: "sess_fork", Role: "assistant", Content: "C1"},
			{ID: "fork-d1", ParentID: "fork-c1", SessionID: "sess_fork", Role: "user", Content: "D1"},
		}},
	}})

	if len(m.forkTree.Nodes) != 7 {
		t.Fatalf("node count = %d, want 7", len(m.forkTree.Nodes))
	}
	byID := map[string]forkTreeNode{}
	for _, node := range m.forkTree.Nodes {
		byID[node.ID] = node
	}
	if byID["root-a"].Y != byID["root-b"].Y || byID["root-b"].Y != byID["root-c"].Y || byID["root-c"].Y != byID["root-d"].Y {
		t.Fatalf("root lineage not horizontal: A=%d B=%d C=%d D=%d", byID["root-a"].Y, byID["root-b"].Y, byID["root-c"].Y, byID["root-d"].Y)
	}
	if byID["fork-b1"].Y == byID["root-b"].Y {
		t.Fatalf("fork branch Y = root Y %d, want separate row", byID["fork-b1"].Y)
	}
	if byID["fork-b1"].X <= byID["root-b"].X {
		t.Fatalf("fork branch X = %d, root B X = %d; want branch to continue right", byID["fork-b1"].X, byID["root-b"].X)
	}
	if gap := byID["root-b"].X - byID["root-a"].X; gap > 8 {
		t.Fatalf("horizontal node gap = %d, want compact spacing", gap)
	}
	rendered := plainText(m.renderForkTreeCanvas(80, 12))
	if !strings.Contains(rendered, "■") {
		t.Fatalf("rendered tree = %q, want square symbolic nodes", rendered)
	}
	for _, hidden := range []string{"A", "B", "C", "D", "B1", "C1", "D1"} {
		if strings.Contains(rendered, hidden) {
			t.Fatalf("rendered tree = %q, want symbolic-only tree without %q", rendered, hidden)
		}
	}
	wantText := map[string]string{"root-a": "A", "root-b": "B", "root-c": "C", "root-d": "D", "fork-b1": "B1", "fork-c1": "C1", "fork-d1": "D1"}
	for id, want := range wantText {
		if got := forkTreeNodeText(byID[id]); got != want {
			t.Fatalf("node %s text = %q, want %q", id, got, want)
		}
	}
}

func TestForkTreeUpDownJumpsParallelBranches(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_root"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageNodes: []forkTreeMessageNode{
			{ID: "root-a", SessionID: "sess_root", Role: "user", Content: "A"},
			{ID: "root-b", ParentID: "root-a", SessionID: "sess_root", Role: "assistant", Content: "B"},
			{ID: "root-c", ParentID: "root-b", SessionID: "sess_root", Role: "user", Content: "C"},
		}},
		{ID: "sess_fork", SessionID: "sess_fork", SessionTitle: "Fork", ParentSessionID: "sess_root", MessageNodes: []forkTreeMessageNode{
			{ID: "fork-b1", ParentID: "root-b", SessionID: "sess_fork", Role: "user", Content: "B1"},
			{ID: "fork-c1", ParentID: "fork-b1", SessionID: "sess_fork", Role: "assistant", Content: "C1"},
		}},
	}})

	for i, node := range m.forkTree.Nodes {
		if node.ID == "root-c" {
			m.forkTree.Selected = i
			break
		}
	}
	updated, cmd := m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil")
	}
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].ID; got != "fork-b1" {
		t.Fatalf("down selected %q, want parallel fork-b1", got)
	}

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].ID; got != "root-c" {
		t.Fatalf("up selected %q, want parallel root-c", got)
	}

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.updateForkTree(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].ID; got != "fork-c1" {
		t.Fatalf("j selected %q, want order navigation to fork-c1", got)
	}
}

func TestForkTreeNavigationParentChildAndOrder(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_root"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageRole: "user", MessageContent: "root"},
		{ID: "sess_child", SessionID: "sess_child", SessionTitle: "Child", ParentSessionID: "sess_root", MessageRole: "assistant", MessageContent: "child"},
		{ID: "sess_grand", SessionID: "sess_grand", SessionTitle: "Grand", ParentSessionID: "sess_child", MessageRole: "user", MessageContent: "grand"},
	}})

	updated, cmd := m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd != nil {
		t.Fatal("right returned command, want nil")
	}
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].SessionID; got != "sess_child" {
		t.Fatalf("right selected %q, want sess_child", got)
	}

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].SessionID; got != "sess_root" {
		t.Fatalf("left selected %q, want sess_root", got)
	}

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].SessionID; got != "sess_child" {
		t.Fatalf("j selected %q, want sess_child", got)
	}
}

func TestRenderForkTreeModalHidesDevDetailsByDefault(t *testing.T) {
	t.Setenv("QUBIT_DEV", "")
	t.Setenv("QUBIT_DEV_FORK_TREE", "")
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_empty", SessionID: "sess_empty", SessionTitle: "Empty", ParentSessionID: "sess_parent", MessageCount: 0},
	}})

	rendered := plainText(m.renderForkTreeModal(20))
	if !strings.Contains(rendered, "fork tree") || !strings.Contains(rendered, "No text message preview") {
		t.Fatalf("rendered tree = %q, want title and preview fallback", rendered)
	}
	if strings.Contains(rendered, "sess_empty") || strings.Contains(rendered, "sess_parent") || strings.Contains(rendered, "fork of") {
		t.Fatalf("rendered tree = %q, want session/fork dev details hidden by default", rendered)
	}
}

func TestRenderForkTreeModalShowsDevDetailsWithFlag(t *testing.T) {
	t.Setenv("QUBIT_DEV_FORK_TREE", "1")
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_empty", SessionID: "sess_empty", SessionTitle: "Empty", ParentSessionID: "sess_parent", MessageRole: "user", MessageContent: "hello", MessageCount: 1},
	}})

	rendered := plainText(m.renderForkTreeModal(20))
	if !strings.Contains(rendered, "sess_empty") || !strings.Contains(rendered, "Empty") {
		t.Fatalf("rendered tree = %q, want session dev details with flag", rendered)
	}
}

func TestRenderForkTreeModalHidesMessageText(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageRole: "user", MessageContent: "root"},
		{ID: "sess_child", SessionID: "sess_child", SessionTitle: "Child", ParentSessionID: "sess_root", MessageRole: "assistant", MessageContent: "child"},
	}})

	canvas := newRuneCanvas(100, 10)
	drawForkTreeNode(canvas, m.forkTree.Nodes[0], true, false)
	drawForkTreeNode(canvas, m.forkTree.Nodes[1], false, false)
	rendered := canvas.render(0, 0, 100, 10)
	if strings.Contains(rendered, "root") || strings.Contains(rendered, "child") || strings.Contains(rendered, "user:") || strings.Contains(rendered, "assistant:") {
		t.Fatalf("rendered tree = %q, want symbolic-only tree without message text or role labels", rendered)
	}
	if got := forkTreeNodeText(m.forkTree.Nodes[0]); got != "root" {
		t.Fatalf("root node text = %q, want root for preview fallback", got)
	}
	if got := forkTreeNodeText(m.forkTree.Nodes[1]); got != "child" {
		t.Fatalf("child node text = %q, want child for preview fallback", got)
	}
}

func TestRenderForkTreeNodeIncludesAssistantText(t *testing.T) {
	node := forkTreeNode{MessageRole: "user", MessageContent: "root question", AssistantRole: "assistant", AssistantContent: "agent answer"}
	entries := forkTreeNodeEntries(node)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want user and assistant entries", entries)
	}
	if entries[0].text != "root question" || entries[1].text != "agent answer" || entries[1].role != "assistant" {
		t.Fatalf("entries = %#v, want user question and assistant answer", entries)
	}
}

func TestRenderForkTreeModalColorsSelectedNode(t *testing.T) {
	m := initialModel(nil)
	m.width = 100
	m.height = 30
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageRole: "user", MessageContent: "root"},
		{ID: "sess_child", SessionID: "sess_child", SessionTitle: "Child", ParentSessionID: "sess_root", MessageRole: "assistant", MessageContent: "child"},
	}})

	rendered := m.renderForkTreeModal(20)
	if !strings.Contains(rendered, "\x1b[") || !strings.Contains(rendered, "■") {
		t.Fatalf("rendered tree = %q, want ANSI colored selected node marker", rendered)
	}
}

func TestForkTreeLayoutTreatsZeroIndexForksAsRootSiblings(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_edit"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageRole: "user", MessageContent: "Hi there"},
		{ID: "sess_edit", SessionID: "sess_edit", SessionTitle: "Edit: Root", ParentSessionID: "sess_root", ForkedFromMessageIndex: 0, MessageRole: "user", MessageContent: "Hi"},
		{ID: "sess_later", SessionID: "sess_later", SessionTitle: "Later", ParentSessionID: "sess_root", ForkedFromMessageIndex: 2, MessageRole: "user", MessageContent: "Later fork"},
	}})

	rootIndex := -1
	editIndex := -1
	laterIndex := -1
	for i, node := range m.forkTree.Nodes {
		switch node.SessionID {
		case "sess_root":
			rootIndex = i
		case "sess_edit":
			editIndex = i
		case "sess_later":
			laterIndex = i
		}
	}
	if rootIndex < 0 || editIndex < 0 || laterIndex < 0 {
		t.Fatalf("missing expected nodes: root=%d edit=%d later=%d", rootIndex, editIndex, laterIndex)
	}
	if m.forkTree.Nodes[editIndex].Parent != -1 {
		t.Fatalf("zero-index edited fork parent = %d, want root-level sibling", m.forkTree.Nodes[editIndex].Parent)
	}
	if m.forkTree.Nodes[editIndex].X != m.forkTree.Nodes[rootIndex].X {
		t.Fatalf("zero-index edited fork X = %d, root X = %d; want same level", m.forkTree.Nodes[editIndex].X, m.forkTree.Nodes[rootIndex].X)
	}
	if m.forkTree.Nodes[laterIndex].Parent != rootIndex {
		t.Fatalf("nonzero fork parent = %d, want root index %d", m.forkTree.Nodes[laterIndex].Parent, rootIndex)
	}
	if m.forkTree.Nodes[laterIndex].X <= m.forkTree.Nodes[rootIndex].X {
		t.Fatalf("nonzero fork X = %d, root X = %d; want child level", m.forkTree.Nodes[laterIndex].X, m.forkTree.Nodes[rootIndex].X)
	}
}

func TestForkTreeUpDownJumpsBetweenForksFromDescendants(t *testing.T) {
	m := initialModel(nil)
	m.session = "sess_root"
	m.mode = modeForkTree
	m.forkTree = newForkTreeState()
	m.applyForkTree(runtimeEvent{Type: "session.tree", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_root", SessionID: "sess_root", SessionTitle: "Root", MessageNodes: []forkTreeMessageNode{
			{ID: "root-a", SessionID: "sess_root", Role: "user", Content: "A"},
			{ID: "root-b", ParentID: "root-a", SessionID: "sess_root", Role: "assistant", Content: "B"},
			{ID: "root-c", ParentID: "root-b", SessionID: "sess_root", Role: "user", Content: "C"},
			{ID: "root-d", ParentID: "root-c", SessionID: "sess_root", Role: "assistant", Content: "D"},
		}},
		{ID: "sess_fork", SessionID: "sess_fork", SessionTitle: "Fork", ParentSessionID: "sess_root", MessageNodes: []forkTreeMessageNode{
			{ID: "fork-b1", ParentID: "root-b", SessionID: "sess_fork", Role: "user", Content: "B1"},
			{ID: "fork-c1", ParentID: "fork-b1", SessionID: "sess_fork", Role: "assistant", Content: "C1"},
		}},
	}})

	for i, node := range m.forkTree.Nodes {
		if node.ID == "root-d" {
			m.forkTree.Selected = i
			break
		}
	}
	updated, cmd := m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("down returned command, want nil")
	}
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].ID; got != "fork-c1" {
		t.Fatalf("down selected %q, want nearest descendant on fork row", got)
	}

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].ID; got != "root-d" {
		t.Fatalf("up selected %q, want nearest node on original branch row", got)
	}
}

func TestSessionPickerTOpensForkTreeForSelectedSession(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := initialModel(rt)
	m.ready = true
	m.mode = modeSessionPicker
	m.session = "sess_current"
	m.sessionCursor = 1
	m.sessions = []sessionInfo{
		{ID: "sess_one", Title: "One"},
		{ID: "sess_two", Title: "Two"},
	}

	updated, cmd := m.updateSessionPicker(tea.KeyPressMsg{Text: "t", Code: 't'})
	got := updated.(model)

	if got.mode != modeForkTree {
		t.Fatalf("mode = %v, want modeForkTree", got.mode)
	}
	if got.previousMode != modeSessionPicker {
		t.Fatalf("previousMode = %v, want modeSessionPicker", got.previousMode)
	}
	if !got.busy {
		t.Fatal("busy = false, want true while loading fork tree")
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "session.tree", "sess_two")

	got.applyForkTree(runtimeEvent{Type: "session.tree", SessionID: "sess_two", ForkTreeNodes: []forkTreeNode{
		{ID: "sess_one", SessionID: "sess_one", SessionTitle: "One", MessageRole: "user", MessageContent: "one"},
		{ID: "sess_two", SessionID: "sess_two", SessionTitle: "Two", MessageRole: "user", MessageContent: "two"},
	}})
	if selected := got.forkTree.Nodes[got.forkTree.Selected].SessionID; selected != "sess_two" {
		t.Fatalf("selected tree session = %q, want sess_two", selected)
	}
}

func TestForkTreeEscapeReturnsToSessionPickerWhenOpenedFromSessions(t *testing.T) {
	m := initialModel(nil)
	m.mode = modeForkTree
	m.previousMode = modeSessionPicker
	m.status = "fork tree"

	updated, cmd := m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("esc returned command, want nil")
	}
	if got.mode != modeSessionPicker {
		t.Fatalf("mode = %v, want modeSessionPicker", got.mode)
	}
	if got.previousMode != modeChat {
		t.Fatalf("previousMode = %v, want reset to modeChat", got.previousMode)
	}
	if got.status != "ready" {
		t.Fatalf("status = %q, want ready", got.status)
	}
}
