package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSlashMcpOpensManagerAndRequestsData(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeChat, runtime: rt}
	updated, cmd := m.handleSlashCommand("/mcp")
	got := updated.(model)
	if got.mode != modeMcpManager || !got.busy {
		t.Fatalf("mode/busy = %v/%v, want MCP manager busy", got.mode, got.busy)
	}
	payload := runBatchSendCommand(t, cmd, stdin, "mcp.list")
	assertPayload(t, payload, "mcp.list", "")
}

func TestMcpCatalogAddModalSendsAdd(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeMcpManager, runtime: rt, mcpCatalog: []mcpCatalogEntry{{ID: "supabase", Name: "Supabase", Description: "db"}}}
	m = m.openMcpCatalogModal()
	updated, cmd := m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeMcpManager || !got.busy {
		t.Fatalf("mode/busy = %v/%v, want manager busy", got.mode, got.busy)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "mcp.add", "")
	if payload["catalogId"] != "supabase" {
		t.Fatalf("catalogId = %#v, want supabase", payload["catalogId"])
	}
}

func TestMcpSecretEntryMasksAndSendsSecret(t *testing.T) {
	rt, stdin := newTestRuntime(t)
	m := model{mode: modeMcpManager, runtime: rt}
	m = m.openMcpSecretEntry("linear")
	m.mcpSecretEntry.Secret.InsertString("lin-secret-123456")
	view := m.renderMcpSecretEntry(10)
	if strings.Contains(view, "lin-secret-123456") {
		t.Fatalf("secret leaked in view: %q", view)
	}
	updated, cmd := m.updateMcpSecretEntry(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(model)
	if got.mode != modeMcpManager || !got.busy || got.mcpSecretEntry != nil {
		t.Fatalf("entry not closed after save: mode=%v busy=%v entry=%#v", got.mode, got.busy, got.mcpSecretEntry)
	}
	payload := runSendCommand(t, cmd, stdin)
	assertPayload(t, payload, "mcp.secret.set", "")
	if payload["serverId"] != "linear" || payload["value"] != "lin-secret-123456" {
		t.Fatalf("payload = %#v, want server secret", payload)
	}
}

func TestApplyMcpEventsUpdatesState(t *testing.T) {
	m := model{mode: modeMcpManager, busy: true}
	m.applyMcpEvent(runtimeEvent{Type: "mcp.list", McpServers: []mcpServerInfo{{ID: "sentry", Name: "Sentry", Enabled: true, ToolCount: 3}}, McpCatalog: []mcpCatalogEntry{{ID: "sentry", Name: "Sentry"}}, Status: "loaded"})
	if m.busy || m.status != "loaded" || len(m.mcpServers) != 1 || len(m.mcpCatalog) != 1 {
		t.Fatalf("mcp state not applied: %#v", m)
	}
}
