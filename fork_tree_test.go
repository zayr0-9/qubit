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
	assertPayload(t, payload, "session.tree", "")
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
	preview := plainText(m.forkTree.Preview.View())
	if !strings.Contains(preview, "Child") {
		t.Fatalf("preview = %q, want selected child title", preview)
	}
	if !strings.Contains(plainText(m.forkTree.Preview.View()), "Child") {
		t.Fatalf("preview did not include selected child title")
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

	updated, _ = m.updateForkTree(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(model)
	if got := m.forkTree.Nodes[m.forkTree.Selected].SessionID; got != "sess_child" {
		t.Fatalf("down selected %q, want sess_child", got)
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
	if !strings.Contains(rendered, "\x1b[") || !strings.Contains(rendered, "▓") {
		t.Fatalf("rendered tree = %q, want ANSI colored selected node marker", rendered)
	}
}
