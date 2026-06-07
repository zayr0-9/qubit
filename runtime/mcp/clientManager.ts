import { PassThrough } from 'node:stream'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js'
import { UnauthorizedError } from '@modelcontextprotocol/sdk/client/auth.js'
import type { McpServerConfig, McpTestResult, McpToolInfo } from './types.js'
import type { McpSecretStore } from './secretStore.js'
import { QubitMcpOAuthProvider, type McpAuthUrlEmitter } from './oauth.js'
import { previewMcpValue, redactSecretText } from './redact.js'

interface ClientEntry {
  server: McpServerConfig
  client: Client
  transport: Transport
  tools?: McpToolInfo[]
}

export interface McpClientManagerOptions {
  secrets: McpSecretStore
  workspaceCwd: string
  authUrlEmitter?: McpAuthUrlEmitter
  oauthRedirectUrl?: string
}

export class McpClientManager {
  private servers = new Map<string, McpServerConfig>()
  private clients = new Map<string, ClientEntry>()
  private readonly secrets: McpSecretStore
  private readonly workspaceCwd: string
  private readonly authUrlEmitter?: McpAuthUrlEmitter
  private oauthRedirectUrl: string

  constructor(options: McpClientManagerOptions) {
    this.secrets = options.secrets
    this.workspaceCwd = options.workspaceCwd
    this.authUrlEmitter = options.authUrlEmitter
    this.oauthRedirectUrl = options.oauthRedirectUrl || 'http://127.0.0.1:0/mcp/oauth/callback'
  }

  setServers(servers: McpServerConfig[]): void {
    const nextIds = new Set(servers.map(server => server.id))
    for (const id of this.clients.keys()) {
      if (!nextIds.has(id)) void this.close(id)
    }
    this.servers = new Map(servers.map(server => [server.id, server]))
  }

  enabledServers(): McpServerConfig[] {
    return [...this.servers.values()].filter(server => server.enabled)
  }

  async listTools(serverId: string, options: { refresh?: boolean } = {}): Promise<McpToolInfo[]> {
    const entry = await this.ensureClient(serverId)
    if (!options.refresh && entry.tools) return entry.tools
    return await this.listToolsFromEntry(entry, true)
  }

  private async listToolsFromEntry(entry: ClientEntry, updateCache: boolean): Promise<McpToolInfo[]> {
    const result = await entry.client.listTools()
    const tools = (result.tools || []).map(tool => ({
      name: tool.name,
      title: tool.title,
      description: tool.description,
      inputSchema: tool.inputSchema,
      annotations: tool.annotations as Record<string, unknown> | undefined,
    }))
    if (updateCache) entry.tools = tools
    return tools
  }

  async callTool(serverId: string, toolName: string, args: Record<string, unknown>, signal?: AbortSignal): Promise<unknown> {
    signal?.throwIfAborted()
    const entry = await this.ensureClient(serverId)
    const result = await entry.client.callTool({ name: toolName, arguments: args }, undefined, { signal })
    signal?.throwIfAborted()
    return result
  }

  async test(serverId: string): Promise<McpTestResult> {
    const server = this.servers.get(serverId)
    if (!server) return { success: false, serverId, status: 'error', message: `Unknown MCP server: ${serverId}`, tools: [] }
    try {
      const tools = await this.listTools(serverId, { refresh: true })
      return { success: true, serverId, status: 'connected', message: `Connected to ${server.name}; found ${tools.length} tool(s).`, tools }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      const authRequired = error instanceof UnauthorizedError || /unauthori[sz]ed|auth/i.test(message)
      return { success: false, serverId, status: authRequired ? 'auth_required' : 'error', message: redactSecretText(message), tools: [] }
    }
  }

  toolCache(serverId: string): McpToolInfo[] {
    return this.clients.get(serverId)?.tools || []
  }

  setOAuthRedirectUrl(url: string): void {
    this.oauthRedirectUrl = url
  }

  async startOAuth(serverId: string, redirectUrl: string, authorizationCode: Promise<string>, emitAuthUrl?: McpAuthUrlEmitter): Promise<McpTestResult> {
    const server = this.servers.get(serverId)
    if (!server) return { success: false, serverId, status: 'error', message: `Unknown MCP server: ${serverId}`, tools: [] }
    await this.close(serverId)
    const provider = new QubitMcpOAuthProvider(server, redirectUrl, this.secrets, emitAuthUrl || this.authUrlEmitter)
    const connectWithProvider = async (): Promise<ClientEntry> => {
      const client = new Client({ name: 'qubit', version: '0.1.0' }, { capabilities: {} })
      const transport = new StreamableHTTPClientTransport(new URL(server.url || ''), { authProvider: provider })
      await client.connect(transport)
      const entry = { server, client, transport }
      this.clients.set(serverId, entry)
      return entry
    }
    try {
      try {
        const entry = await connectWithProvider()
        const tools = await this.listToolsFromEntry(entry, true)
        return { success: true, serverId, status: 'connected', message: `Connected to ${server.name}; found ${tools.length} tool(s).`, tools }
      } catch (error) {
        if (!(error instanceof UnauthorizedError)) throw error
      }
      const code = await authorizationCode
      const authTransport = new StreamableHTTPClientTransport(new URL(server.url || ''), { authProvider: provider })
      await authTransport.finishAuth(code)
      await authTransport.close()
      const entry = await connectWithProvider()
      const tools = await this.listToolsFromEntry(entry, true)
      return { success: true, serverId, status: 'connected', message: `Authorized ${server.name}; found ${tools.length} tool(s).`, tools }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      const authRequired = error instanceof UnauthorizedError || /unauthori[sz]ed|auth/i.test(message)
      return { success: false, serverId, status: authRequired ? 'auth_required' : 'error', message: redactSecretText(message), tools: [] }
    }
  }

  async close(serverId: string): Promise<void> {
    const entry = this.clients.get(serverId)
    if (!entry) return
    this.clients.delete(serverId)
    await entry.transport.close?.()
  }

  async closeAll(): Promise<void> {
    await Promise.all([...this.clients.keys()].map(id => this.close(id)))
  }

  private async ensureClient(serverId: string): Promise<ClientEntry> {
    const server = this.servers.get(serverId)
    if (!server) throw new Error(`Unknown MCP server: ${serverId}`)
    if (!server.enabled) throw new Error(`MCP server is disabled: ${server.name}`)
    const existing = this.clients.get(serverId)
    if (existing) return existing
    const client = new Client({ name: 'qubit', version: '0.1.0' }, { capabilities: {} })
    const transport = await this.createTransport(server)
    await client.connect(transport)
    const entry = { server, client, transport }
    this.clients.set(serverId, entry)
    return entry
  }

  private async createTransport(server: McpServerConfig): Promise<Transport> {
    if (server.transport === 'stdio') {
      const stderr = new PassThrough()
      stderr.on('data', chunk => {
        const text = redactSecretText(String(chunk || '')).trim()
        if (text) console.error(`[mcp:${server.id}] ${previewMcpValue(text, 800)}`)
      })
      return new StdioClientTransport({
        command: server.command || '',
        args: server.args || [],
        cwd: server.cwd || this.workspaceCwd,
        env: await this.resolveEnv(server),
        stderr,
      })
    }

    const requestInit: RequestInit = {}
    if (server.auth.type === 'bearer') {
      const token = await this.resolveBearer(server)
      if (!token) throw new Error(`MCP bearer token is not configured for ${server.name}`)
      requestInit.headers = { Authorization: `Bearer ${token}` }
    }
    if (server.auth.type === 'env') {
      const token = server.auth.envVar ? process.env[server.auth.envVar] : undefined
      if (!token) throw new Error(`MCP auth environment variable is not set: ${server.auth.envVar || 'unknown'}`)
      requestInit.headers = { Authorization: `Bearer ${token}` }
    }

    const authProvider = server.auth.type === 'oauth'
      ? new QubitMcpOAuthProvider(server, this.oauthRedirectUrl, this.secrets, this.authUrlEmitter)
      : undefined
    return new StreamableHTTPClientTransport(new URL(server.url || ''), { requestInit, authProvider })
  }

  private async resolveBearer(server: McpServerConfig): Promise<string | undefined> {
    if (server.auth.source === 'env') return server.auth.envVar ? process.env[server.auth.envVar] : undefined
    return await this.secrets.get(server.auth.account)
  }

  private async resolveEnv(server: McpServerConfig): Promise<Record<string, string>> {
    const env: Record<string, string> = {}
    for (const [key, ref] of Object.entries(server.env || {})) {
      if (ref.source === 'literal') env[key] = ref.value || ''
      else if (ref.source === 'env') env[key] = ref.envVar ? process.env[ref.envVar] || '' : ''
      else if (ref.source === 'keychain') env[key] = await this.secrets.get(ref.account) || ''
    }
    return env
  }
}
