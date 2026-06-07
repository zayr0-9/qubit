package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) renderMcpManager(height int) string {
	var b strings.Builder
	b.WriteString(accentSt().Bold(true).Render("MCP servers"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("a add · enter details · t test · o OAuth · b bearer token · e enable/disable · d delete · r refresh · esc close"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("Add supports starter catalog, custom remote URLs, and local stdio commands."))
	b.WriteString("\n\n")
	if len(m.mcpServers) == 0 {
		b.WriteString(mutedSt.Render("No MCP servers configured. Press a to add from catalog, remote URL, or stdio command."))
		return b.String()
	}
	window := visibleListWindow(len(m.mcpServers), clampInt(m.mcpCursor, 0, len(m.mcpServers)-1), max(1, height-5))
	for i := window.Start; i < window.End; i++ {
		server := m.mcpServers[i]
		selected := i == m.mcpCursor
		marker := "  "
		if selected {
			marker = accentSt().Render("› ")
		}
		state := "off"
		if server.Enabled {
			state = "on"
		}
		status := fallback(server.Status, "unknown")
		line := fmt.Sprintf("%s%s %s · %s · %s · %d tools", marker, server.Name, mutedSt.Render(state), mutedSt.Render(server.Transport), mutedSt.Render(status), server.ToolCount)
		b.WriteString(line)
		if server.AuthType != "" && server.AuthType != "none" {
			b.WriteString(mutedSt.Render(" · auth " + server.AuthType))
		}
		b.WriteString("\n")
		if selected {
			detail := firstNonEmpty(server.StatusMessage, server.Caveat, server.URL, server.Command)
			if detail != "" {
				b.WriteString(mutedSt.Render("  " + oneLine(detail, max(20, m.width-4))))
				b.WriteString("\n")
			}
		}
	}
	return lipgloss.NewStyle().Width(max(1, m.width-2)).Render(b.String())
}

func (m model) renderMcpSecretEntry(height int) string {
	if m.mcpSecretEntry == nil {
		return mutedSt.Render("MCP secret entry unavailable")
	}
	masked := strings.Repeat("•", len([]rune(m.mcpSecretEntry.Secret.Value())))
	if masked == "" {
		masked = mutedSt.Render("paste or type bearer token")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		accentSt().Bold(true).Render("MCP bearer token"),
		mutedSt.Render("Secret input is masked and will be stored in the OS keychain."),
		"",
		masked,
	)
}

func (m model) renderMcpAddEntry(height int) string {
	if m.mcpAddEntry == nil {
		return mutedSt.Render("MCP add entry unavailable")
	}
	var b strings.Builder
	b.WriteString(accentSt().Bold(true).Render("Add MCP server"))
	b.WriteString("\n")
	if m.mcpAddEntry.Kind == mcpAddRemote {
		b.WriteString(mutedSt.Render("Custom remote Streamable HTTP MCP endpoint."))
	} else {
		b.WriteString(mutedSt.Render("Custom local stdio MCP command."))
	}
	b.WriteString("\n\n")
	fields := []struct {
		step  mcpAddEntryStep
		label string
		value string
	}{
		{mcpAddName, "Name", m.mcpAddEntry.Name.Value()},
	}
	if m.mcpAddEntry.Kind == mcpAddRemote {
		fields = append(fields, struct {
			step  mcpAddEntryStep
			label string
			value string
		}{mcpAddURL, "URL", m.mcpAddEntry.URL.Value()})
	} else {
		fields = append(fields,
			struct {
				step  mcpAddEntryStep
				label string
				value string
			}{mcpAddCommand, "Command", m.mcpAddEntry.Command.Value()},
			struct {
				step  mcpAddEntryStep
				label string
				value string
			}{mcpAddArgs, "Args", m.mcpAddEntry.Args.Value()},
		)
	}
	for _, field := range fields {
		marker := "  "
		label := mutedSt.Render(field.label + ":")
		if field.step == m.mcpAddEntry.Step {
			marker = accentSt().Render("› ")
			label = accentSt().Render(field.label + ":")
		}
		value := field.value
		if value == "" {
			value = mutedSt.Render("empty")
		}
		b.WriteString(marker + label + " " + value + "\n")
	}
	if m.mcpAddEntry.Err != "" {
		b.WriteString(errSt.Render(m.mcpAddEntry.Err))
		b.WriteString("\n")
	}
	return b.String()
}
