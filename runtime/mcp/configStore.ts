import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { writeJsonAtomic } from '../jsonStore.js'
import { catalogEntryById } from './catalog.js'
import { sanitizeMcpId } from './names.js'
import type { McpAuthConfig, McpConfigFile, McpEnvValueRef, McpServerConfig } from './types.js'

export const MCP_CONFIG_VERSION = 1

export class McpConfigStore {
  readonly path: string

  constructor(configDir: string) {
    this.path = join(configDir, 'mcp.json')
  }

  async load(): Promise<McpConfigFile> {
    let parsed: unknown
    try {
      parsed = JSON.parse(await readFile(this.path, 'utf8'))
    } catch {
      parsed = null
    }
    const normalized = normalizeMcpConfig(parsed)
    await this.save(normalized)
    return normalized
  }

  async save(config: McpConfigFile): Promise<void> {
    await writeJsonAtomic(this.path, normalizeMcpConfig(config), { mode: 0o600 })
  }
}

export function normalizeMcpConfig(raw: unknown): McpConfigFile {
  const source = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
  const servers = Array.isArray(source.servers) ? source.servers : []
  const seen = new Set<string>()
  const normalizedServers: McpServerConfig[] = []
  for (const item of servers) {
    const server = normalizeMcpServer(item)
    if (!server || seen.has(server.id)) continue
    seen.add(server.id)
    normalizedServers.push(server)
  }
  return { version: MCP_CONFIG_VERSION, servers: normalizedServers }
}

export function normalizeMcpServer(raw: unknown): McpServerConfig | null {
  if (!raw || typeof raw !== 'object') return null
  const source = raw as Record<string, unknown>
  const catalogId = typeof source.catalogId === 'string' ? sanitizeMcpId(source.catalogId, '') : ''
  const catalog = catalogId ? catalogEntryById(catalogId) : undefined
  const id = sanitizeMcpId(String(source.id || catalog?.id || source.name || 'mcp'))
  const now = new Date().toISOString()
  const transport = source.transport === 'stdio' ? 'stdio' : 'streamable_http'
  const name = String(source.name || catalog?.name || id).trim() || id
  const auth = normalizeAuthConfig(source.auth, catalog?.defaultAuthType)
  const server: McpServerConfig = {
    id,
    name,
    enabled: source.enabled !== false,
    transport,
    auth,
    createdAt: typeof source.createdAt === 'string' ? source.createdAt : now,
    updatedAt: typeof source.updatedAt === 'string' ? source.updatedAt : now,
    ...(catalogId ? { catalogId } : {}),
    ...(typeof source.status === 'string' ? { status: normalizeStatus(source.status) } : {}),
    ...(typeof source.statusMessage === 'string' ? { statusMessage: source.statusMessage } : {}),
    ...(Number.isFinite(Number(source.toolCount)) ? { toolCount: Number(source.toolCount) } : {}),
    ...(Array.isArray(source.tools) ? { tools: source.tools
      .filter(tool => tool && typeof tool === 'object' && typeof (tool as Record<string, unknown>).name === 'string')
      .map(tool => {
        const item = tool as Record<string, unknown>
        return {
          name: String(item.name),
          ...(typeof item.title === 'string' ? { title: item.title } : {}),
          ...(typeof item.description === 'string' ? { description: item.description } : {}),
          ...(item.inputSchema !== undefined ? { inputSchema: item.inputSchema } : {}),
          ...(item.annotations && typeof item.annotations === 'object' && !Array.isArray(item.annotations) ? { annotations: item.annotations as Record<string, unknown> } : {}),
        }
      }) } : {}),
  }
  if (transport === 'stdio') {
    server.command = typeof source.command === 'string' ? source.command.trim() : ''
    server.args = Array.isArray(source.args) ? source.args.map(String) : []
    if (typeof source.cwd === 'string' && source.cwd.trim()) server.cwd = source.cwd.trim()
    server.env = normalizeEnv(source.env)
    if (!server.command) return null
  } else {
    const url = String(source.url || catalog?.url || '').trim()
    if (!url) return null
    server.url = url
  }
  return server
}

function normalizeAuthConfig(raw: unknown, fallback: McpAuthConfig['type'] = 'none'): McpAuthConfig {
  const source = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
  const rawType = String(source.type || fallback || 'none')
  const type: McpAuthConfig['type'] = rawType === 'oauth' || rawType === 'bearer' || rawType === 'env' ? rawType : 'none'
  return {
    type,
    ...(source.source === 'keychain' || source.source === 'env' || source.source === 'none' ? { source: source.source } : {}),
    ...(typeof source.envVar === 'string' ? { envVar: source.envVar } : {}),
    ...(typeof source.account === 'string' ? { account: source.account } : {}),
    ...(typeof source.masked === 'string' ? { masked: source.masked } : {}),
    ...(typeof source.status === 'string' ? { status: source.status } : {}),
    ...(typeof source.accountLabel === 'string' ? { accountLabel: source.accountLabel } : {}),
    ...(typeof source.scope === 'string' ? { scope: source.scope } : {}),
  }
}

function normalizeEnv(raw: unknown): Record<string, McpEnvValueRef> | undefined {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined
  const out: Record<string, McpEnvValueRef> = {}
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue
    if (typeof value === 'string') {
      out[key] = { source: 'literal', value }
      continue
    }
    if (!value || typeof value !== 'object' || Array.isArray(value)) continue
    const item = value as Record<string, unknown>
    const source = item.source === 'env' || item.source === 'keychain' ? item.source : 'literal'
    out[key] = {
      source,
      ...(typeof item.value === 'string' && source === 'literal' ? { value: item.value } : {}),
      ...(typeof item.envVar === 'string' && source === 'env' ? { envVar: item.envVar } : {}),
      ...(typeof item.account === 'string' && source === 'keychain' ? { account: item.account } : {}),
      ...(typeof item.masked === 'string' ? { masked: item.masked } : {}),
    }
  }
  return Object.keys(out).length > 0 ? out : undefined
}

function normalizeStatus(value: string): McpServerConfig['status'] {
  return value === 'connected' || value === 'disconnected' || value === 'auth_required' || value === 'error' ? value : 'unknown'
}
