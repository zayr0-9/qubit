export const MCP_TOOL_PREFIX = 'mcp__'

export function sanitizeMcpId(value: string, fallback = 'server'): string {
  const sanitized = String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 48)
  return sanitized || fallback
}

export function sanitizeMcpToolPart(value: string, fallback = 'tool'): string {
  const sanitized = String(value || '')
    .trim()
    .replace(/[^A-Za-z0-9_]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 64)
  return sanitized || fallback
}

export function mcpWrapperToolName(serverId: string, toolName: string): string {
  return `${MCP_TOOL_PREFIX}${sanitizeMcpId(serverId)}__${sanitizeMcpToolPart(toolName)}`
}

export function isMcpWrapperToolName(name: string): boolean {
  return String(name || '').startsWith(MCP_TOOL_PREFIX)
}

export function parseMcpWrapperToolName(name: string): { serverId: string; toolName: string } | null {
  if (!isMcpWrapperToolName(name)) return null
  const rest = name.slice(MCP_TOOL_PREFIX.length)
  const sep = rest.indexOf('__')
  if (sep < 0) return null
  return { serverId: rest.slice(0, sep), toolName: rest.slice(sep + 2) }
}
