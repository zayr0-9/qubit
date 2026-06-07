import { isMcpWrapperToolName } from './names.js'
import { previewMcpValue } from './redact.js'
import type { McpToolMapping } from './types.js'

let mappings = new Map<string, McpToolMapping>()

export function setMcpToolMappings(next: Map<string, McpToolMapping>): void {
  mappings = next
}

export function mcpToolMapping(toolName: string): McpToolMapping | undefined {
  return mappings.get(toolName)
}

export function isMcpToolName(toolName: string): boolean {
  return isMcpWrapperToolName(toolName)
}

export function summarizeMcpArgs(toolName: string, args: unknown): Record<string, unknown> {
  const mapping = mappings.get(toolName)
  return {
    mcp: true,
    serverId: mapping?.serverId,
    serverName: mapping?.serverName,
    toolName: mapping?.toolName,
    displayName: mapping?.displayName,
    argsPreview: previewMcpValue(args, 1200),
  }
}

export function summarizeMcpResult(toolName: string, result: unknown): Record<string, unknown> {
  const mapping = mappings.get(toolName)
  const base = plainObject(result)
  const payload = plainObject(base.data !== undefined ? base.data : base.output !== undefined ? base.output : result)
  const nested = payload.result !== undefined ? payload.result : payload
  return {
    ok: Boolean(base.ok),
    ...(base.error ? { error: previewMcpValue(base.error, 1200) } : {}),
    mcp: true,
    serverId: mapping?.serverId || payload.serverId,
    serverName: mapping?.serverName || payload.serverName,
    toolName: mapping?.toolName || payload.toolName,
    displayName: mapping?.displayName,
    contentPreview: mcpContentPreview(nested),
    payloadPreview: previewMcpValue(nested, 2400),
  }
}

export function mcpContextChars(result: unknown): number {
  const base = plainObject(result)
  const payload = base.data !== undefined ? base.data : base.output !== undefined ? base.output : result
  const text = JSON.stringify(payload ?? '')
  return [...text].length
}

function mcpContentPreview(value: unknown): string {
  const source = plainObject(value)
  const content = Array.isArray(source.content) ? source.content : []
  const parts: string[] = []
  for (const item of content) {
    if (!item || typeof item !== 'object') continue
    const entry = item as Record<string, unknown>
    if (entry.type === 'text' && typeof entry.text === 'string') parts.push(entry.text)
    else if (entry.type === 'resource' && entry.resource && typeof entry.resource === 'object') {
      const resource = entry.resource as Record<string, unknown>
      if (typeof resource.text === 'string') parts.push(resource.text)
      else if (typeof resource.uri === 'string') parts.push(resource.uri)
    } else if (typeof entry.type === 'string') {
      parts.push(`[${entry.type}]`)
    }
  }
  if (parts.length > 0) return previewMcpValue(parts.join('\n'), 2400)
  return previewMcpValue(value, 2400)
}

function plainObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
