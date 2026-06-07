package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) openMcpManager() (tea.Model, tea.Cmd) {
	m.mode = modeMcpManager
	m.busy = true
	m.status = "loading MCP servers"
	m.mcpCursor = 0
	return m, tea.Batch(
		sendRuntime(m.runtime, map[string]any{"type": "mcp.list"}),
		sendRuntime(m.runtime, map[string]any{"type": "mcp.catalog"}),
	)
}

func (m model) updateMcpManager(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeChat
		m.status = "ready"
		return m, nil
	case "up", "k", "ctrl+p":
		m.moveMcpCursor(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveMcpCursor(1)
		return m, nil
	case "a", "A":
		return m.openMcpAddMenuModal(), nil
	case "r", "R":
		m.busy = true
		m.status = "refreshing MCP servers"
		return m, tea.Batch(sendRuntime(m.runtime, map[string]any{"type": "mcp.list"}), sendRuntime(m.runtime, map[string]any{"type": "mcp.catalog"}))
	case "t", "T":
		selected, ok := m.selectedMcpServer()
		if !ok {
			m.status = "no MCP server selected"
			return m, nil
		}
		m.busy = true
		m.status = "testing MCP server"
		return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.test", "serverId": selected.ID})
	case "e", "E":
		selected, ok := m.selectedMcpServer()
		if !ok {
			return m, nil
		}
		m.busy = true
		m.status = "updating MCP server"
		return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.update", "serverId": selected.ID, "enabled": !selected.Enabled})
	case "o", "O":
		selected, ok := m.selectedMcpServer()
		if !ok {
			return m, nil
		}
		m.busy = true
		m.status = "starting MCP OAuth"
		return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.auth.start", "serverId": selected.ID})
	case "b", "B":
		selected, ok := m.selectedMcpServer()
		if !ok {
			return m, nil
		}
		return m.openMcpSecretEntry(selected.ID), nil
	case "d", "D", "delete":
		selected, ok := m.selectedMcpServer()
		if !ok {
			return m, nil
		}
		return m.openDeleteMcpConfirm(selected), nil
	case "enter":
		selected, ok := m.selectedMcpServer()
		if !ok {
			return m.openMcpCatalogModal(), nil
		}
		m.appendSystemDirect(mcpServerDetails(selected))
		m.status = "MCP details appended"
		return m, nil
	}
	return m, nil
}

func (m *model) moveMcpCursor(delta int) {
	if len(m.mcpServers) == 0 {
		m.mcpCursor = 0
		return
	}
	m.mcpCursor = moveListCursor(m.mcpCursor, len(m.mcpServers), delta)
}

func (m model) selectedMcpServer() (mcpServerInfo, bool) {
	if len(m.mcpServers) == 0 {
		return mcpServerInfo{}, false
	}
	cursor := clampInt(m.mcpCursor, 0, len(m.mcpServers)-1)
	return m.mcpServers[cursor], true
}

func (m model) openMcpAddMenuModal() model {
	m.previousMode = modeMcpManager
	m.mode = modeModal
	m.modal = &modalState{
		ID:          "mcp_add_menu",
		Kind:        modalKindCustom,
		Title:       "Add MCP server",
		Description: "Choose how to add an MCP server.",
		Options: []modalOption{
			{ID: "catalog", Label: "Starter catalog", Description: "Supabase, Notion, Linear, Cloudflare Docs, Sentry"},
			{ID: "remote", Label: "Remote URL", Description: "Add a custom Streamable HTTP MCP endpoint"},
			{ID: "stdio", Label: "Local stdio command", Description: "Add a custom local MCP server command"},
		},
		Actions: []modalAction{
			{ID: "select", Label: "Select", Style: "primary", Default: true},
			{ID: "cancel", Label: "Cancel"},
		},
		Payload: map[string]any{"action": "mcp.add.menu"},
	}
	m.status = "choose MCP add method"
	return m
}

func (m model) openMcpCatalogModal() model {
	options := make([]modalOption, 0, len(m.mcpCatalog))
	for _, entry := range m.mcpCatalog {
		description := strings.TrimSpace(entry.Description)
		if entry.Caveat != "" {
			description += " · " + entry.Caveat
		}
		options = append(options, modalOption{ID: entry.ID, Label: entry.Name, Description: description})
	}
	if len(options) == 0 {
		for _, fallback := range []string{"supabase", "notion", "linear", "cloudflare-docs", "sentry"} {
			options = append(options, modalOption{ID: fallback, Label: fallback, Description: "Starter MCP catalog entry"})
		}
	}
	m.previousMode = modeMcpManager
	m.mode = modeModal
	m.modal = &modalState{
		ID:           "mcp_catalog",
		Kind:         modalKindCustom,
		Title:        "Add MCP server",
		Description:  "Choose a starter hosted MCP server. Auth can be completed after adding.",
		Options:      options,
		OptionCursor: clampInt(m.mcpCatalogCursor, 0, max(0, len(options)-1)),
		Actions: []modalAction{
			{ID: "add", Label: "Add", Style: "primary", Default: true},
			{ID: "cancel", Label: "Cancel"},
		},
		Payload: map[string]any{"action": "mcp.catalog.add"},
	}
	m.status = "choose MCP starter"
	return m
}

func (m model) openDeleteMcpConfirm(selected mcpServerInfo) model {
	m.previousMode = modeMcpManager
	m.mode = modeModal
	m.modal = &modalState{
		ID:          "mcp_delete",
		Kind:        modalKindConfirm,
		Title:       "Delete MCP server?",
		Description: fmt.Sprintf("Remove %s from Qubit MCP config? Stored MCP secrets for this server will also be deleted.", selected.Name),
		Fields: []modalField{
			{Label: "Server", Value: selected.Name},
			{Label: "Transport", Value: selected.Transport},
		},
		Actions: []modalAction{
			{ID: "delete", Label: "Delete", Style: "danger"},
			{ID: "cancel", Label: "Cancel", Default: true},
		},
		Cursor: 1,
		Payload: map[string]any{
			"action":   "mcp.delete",
			"serverId": selected.ID,
		},
	}
	m.status = "confirm MCP delete"
	return m
}

func (m model) openMcpAddEntry(kind mcpAddEntryKind) model {
	name := newKeyEntryComposer("", "server name")
	url := newKeyEntryComposer("", "https://example.com/mcp")
	command := newKeyEntryComposer("", "npx")
	args := newKeyEntryComposer("", "-y @scope/server")
	name.SetCharLimit(80)
	url.SetCharLimit(2048)
	command.SetCharLimit(256)
	args.SetCharLimit(2048)
	m.mode = modeMcpAddEntry
	m.mcpAddEntry = &mcpAddEntryState{Kind: kind, Step: mcpAddName, Name: name, URL: url, Command: command, Args: args}
	if kind == mcpAddRemote {
		m.status = "enter MCP server name"
	} else {
		m.status = "enter local MCP server name"
	}
	return m
}

func (m model) updateMcpAddEntry(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mcpAddEntry == nil {
		m.mode = modeMcpManager
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mcpAddEntry = nil
		m.mode = modeMcpManager
		m.status = "MCP add cancelled"
		return m, nil
	case "left", "shift+tab":
		m.moveMcpAddStep(-1)
		return m, nil
	case "right", "tab":
		m.moveMcpAddStep(1)
		return m, nil
	case "enter":
		return m.advanceMcpAddEntry()
	}
	composer := m.activeMcpAddComposer()
	if composer == nil {
		return m, nil
	}
	handled, cmd := composer.UpdateKey(msg)
	if handled {
		m.mcpAddEntry.Err = ""
		m.layout()
		return m, cmd
	}
	return m, nil
}

func (m *model) moveMcpAddStep(delta int) {
	if m.mcpAddEntry == nil {
		return
	}
	steps := m.mcpAddSteps()
	idx := 0
	for i, step := range steps {
		if step == m.mcpAddEntry.Step {
			idx = i
			break
		}
	}
	idx = moveListCursor(idx, len(steps), delta)
	m.mcpAddEntry.Step = steps[idx]
	m.status = m.mcpAddStepStatus()
}

func (m model) mcpAddSteps() []mcpAddEntryStep {
	if m.mcpAddEntry != nil && m.mcpAddEntry.Kind == mcpAddStdio {
		return []mcpAddEntryStep{mcpAddName, mcpAddCommand, mcpAddArgs}
	}
	return []mcpAddEntryStep{mcpAddName, mcpAddURL}
}

func (m model) activeMcpAddComposer() *composerModel {
	if m.mcpAddEntry == nil {
		return nil
	}
	switch m.mcpAddEntry.Step {
	case mcpAddName:
		return &m.mcpAddEntry.Name
	case mcpAddURL:
		return &m.mcpAddEntry.URL
	case mcpAddCommand:
		return &m.mcpAddEntry.Command
	case mcpAddArgs:
		return &m.mcpAddEntry.Args
	default:
		return nil
	}
}

func (m model) advanceMcpAddEntry() (tea.Model, tea.Cmd) {
	if m.mcpAddEntry == nil {
		return m, nil
	}
	name := strings.TrimSpace(m.mcpAddEntry.Name.Value())
	if m.mcpAddEntry.Step == mcpAddName {
		if name == "" {
			m.status = "MCP server name is required"
			return m, nil
		}
		m.moveMcpAddStep(1)
		return m, nil
	}
	if m.mcpAddEntry.Kind == mcpAddRemote {
		url := strings.TrimSpace(m.mcpAddEntry.URL.Value())
		if url == "" {
			m.status = "MCP server URL is required"
			return m, nil
		}
		m.mcpAddEntry = nil
		m.mode = modeMcpManager
		m.busy = true
		m.status = "adding remote MCP server"
		return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.add", "transport": "streamable_http", "name": name, "url": url, "authType": "oauth"})
	}
	if m.mcpAddEntry.Step == mcpAddCommand {
		if strings.TrimSpace(m.mcpAddEntry.Command.Value()) == "" {
			m.status = "MCP stdio command is required"
			return m, nil
		}
		m.moveMcpAddStep(1)
		return m, nil
	}
	command := strings.TrimSpace(m.mcpAddEntry.Command.Value())
	args := shellSplit(strings.TrimSpace(m.mcpAddEntry.Args.Value()))
	m.mcpAddEntry = nil
	m.mode = modeMcpManager
	m.busy = true
	m.status = "adding stdio MCP server"
	return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.add", "transport": "stdio", "name": name, "command": command, "args": args})
}

func (m model) mcpAddStepStatus() string {
	if m.mcpAddEntry == nil {
		return "add MCP"
	}
	switch m.mcpAddEntry.Step {
	case mcpAddName:
		return "enter MCP server name"
	case mcpAddURL:
		return "enter MCP server URL"
	case mcpAddCommand:
		return "enter MCP stdio command"
	case mcpAddArgs:
		return "enter MCP stdio args"
	default:
		return "add MCP"
	}
}

func (m model) updateMcpAddEntryPaste(msg composerPasteMsg) model {
	return m.insertMcpAddPaste(msg.Text)
}

func (m model) updateMcpAddEntryTeaPaste(msg tea.PasteMsg) model {
	return m.insertMcpAddPaste(msg.Content)
}

func (m model) insertMcpAddPaste(text string) model {
	if m.mode != modeMcpAddEntry || m.mcpAddEntry == nil {
		return m
	}
	if composer := m.activeMcpAddComposer(); composer != nil {
		composer.InsertString(strings.TrimSpace(text))
		m.layout()
	}
	return m
}

func shellSplit(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return strings.Fields(input)
}

func (m model) openMcpSecretEntry(serverID string) model {
	secret := newKeyEntryComposer("", "MCP bearer token")
	secret.SetCharLimit(8192)
	m.mode = modeMcpSecretEntry
	m.mcpSecretEntry = &mcpSecretEntryState{ServerID: serverID, Name: "bearer-token", Secret: secret}
	m.status = "enter MCP bearer token"
	return m
}

func (m model) updateMcpSecretEntry(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mcpSecretEntry == nil {
		m.mode = modeMcpManager
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mcpSecretEntry = nil
		m.mode = modeMcpManager
		m.status = "MCP secret entry cancelled"
		return m, nil
	case "enter":
		secret := strings.TrimSpace(m.mcpSecretEntry.Secret.Value())
		if secret == "" {
			m.status = "MCP bearer token is required"
			return m, nil
		}
		serverID := m.mcpSecretEntry.ServerID
		m.mcpSecretEntry.Secret.Reset()
		m.mcpSecretEntry = nil
		m.mode = modeMcpManager
		m.busy = true
		m.status = "saving MCP secret"
		return m, sendRuntime(m.runtime, map[string]any{"type": "mcp.secret.set", "serverId": serverID, "name": "bearer-token", "value": secret})
	}
	handled, cmd := m.mcpSecretEntry.Secret.UpdateKey(msg)
	if handled {
		m.layout()
		return m, cmd
	}
	return m, nil
}

func (m model) updateMcpSecretEntryPaste(msg composerPasteMsg) model {
	return m.insertMcpSecretPaste(msg.Text)
}

func (m model) updateMcpSecretEntryTeaPaste(msg tea.PasteMsg) model {
	return m.insertMcpSecretPaste(msg.Content)
}

func (m model) insertMcpSecretPaste(text string) model {
	if m.mode != modeMcpSecretEntry || m.mcpSecretEntry == nil {
		return m
	}
	m.mcpSecretEntry.Secret.InsertString(strings.TrimSpace(text))
	m.layout()
	return m
}

func (m *model) applyMcpEvent(ev runtimeEvent) {
	if len(ev.McpServers) > 0 {
		m.mcpServers = ev.McpServers
		if len(m.mcpServers) == 0 {
			m.mcpCursor = 0
		} else {
			m.mcpCursor = clampInt(m.mcpCursor, 0, len(m.mcpServers)-1)
		}
	}
	if len(ev.McpCatalog) > 0 {
		m.mcpCatalog = ev.McpCatalog
	}
	if ev.Status != "" {
		m.status = ev.Status
	}
	m.busy = false
}

func (m *model) applyMcpAuthStarted(ev runtimeEvent) {
	m.busy = false
	m.status = fallback(ev.Status, "MCP authorization started")
	if ev.AuthURL != "" {
		m.appendSystemDirect(fmt.Sprintf("Open this URL to authorize %s MCP:\n%s", fallback(ev.Name, "server"), ev.AuthURL))
	}
}

func mcpServerDetails(server mcpServerInfo) string {
	return fmt.Sprintf("MCP server: %s\nID: %s\nTransport: %s\nAuth: %s (%s)\nStatus: %s\nTools: %d\n%s", server.Name, server.ID, server.Transport, fallback(server.AuthType, "none"), fallback(server.AuthStatus, "unknown"), fallback(server.Status, "unknown"), server.ToolCount, server.Caveat)
}
