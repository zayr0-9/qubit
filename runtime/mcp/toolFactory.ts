import { defineTool, type AnyToolDefinition } from '@hyper-labs/hyper-router'
import type { AgentContext } from '@hyper-labs/hyper-router'
import type { McpClientManager } from './clientManager.js'
import { mcpWrapperToolName } from './names.js'
import type { McpServerConfig, McpToolInfo, McpToolMapping } from './types.js'

export function createMcpTools(servers: McpServerConfig[], toolMap: Map<string, McpToolInfo[]>, manager: McpClientManager): { tools: AnyToolDefinition[]; mappings: Map<string, McpToolMapping> } {
  const mappings = new Map<string, McpToolMapping>()
  const tools: AnyToolDefinition[] = []
  for (const server of servers) {
    if (!server.enabled) continue
    for (const tool of toolMap.get(server.id) || []) {
      const wrapperName = uniqueWrapperName(mcpWrapperToolName(server.id, tool.name), mappings)
      const mapping: McpToolMapping = {
        wrapperName,
        serverId: server.id,
        serverName: server.name,
        toolName: tool.name,
        displayName: `${server.name}: ${tool.title || tool.name}`,
        description: tool.description || `Call the ${tool.name} MCP tool on ${server.name}.`,
        inputSchema: tool.inputSchema || { type: 'object', properties: {}, additionalProperties: true },
        annotations: tool.annotations,
      }
      mappings.set(wrapperName, mapping)
      tools.push(defineTool({
        name: wrapperName,
        description: `MCP ${mapping.displayName}. ${mapping.description}`,
        inputSchema: mapping.inputSchema,
        permission: { mode: 'ask', reason: `MCP tool ${mapping.displayName} can access external services or mutate remote data.`, metadata: { mcp: true, serverId: server.id, toolName: tool.name } },
        async execute(args: Record<string, unknown>, context: AgentContext) {
          try {
            const result = await manager.callTool(server.id, tool.name, args || {}, context.signal)
            return { ok: true, data: { serverId: server.id, serverName: server.name, toolName: tool.name, result } }
          } catch (error) {
            return { ok: false, error: error instanceof Error ? error.message : String(error) }
          }
        },
      }))
    }
  }
  return { tools, mappings }
}

function uniqueWrapperName(name: string, mappings: Map<string, McpToolMapping>): string {
  if (!mappings.has(name)) return name
  for (let i = 2; i < 1000; i += 1) {
    const candidate = `${name}_${i}`
    if (!mappings.has(candidate)) return candidate
  }
  throw new Error(`Unable to create unique MCP tool name for ${name}`)
}
