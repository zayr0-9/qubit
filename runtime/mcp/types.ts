export type McpTransportType = 'streamable_http' | 'stdio'
export type McpAuthType = 'none' | 'oauth' | 'bearer' | 'env'
export type McpServerStatus = 'connected' | 'disconnected' | 'auth_required' | 'error' | 'unknown'

export interface McpAuthConfig {
  type: McpAuthType
  source?: 'keychain' | 'env' | 'none'
  envVar?: string
  account?: string
  masked?: string
  status?: string
  accountLabel?: string
  scope?: string
}

export interface McpEnvValueRef {
  source: 'literal' | 'env' | 'keychain'
  value?: string
  envVar?: string
  account?: string
  masked?: string
}

export interface McpServerConfig {
  id: string
  name: string
  enabled: boolean
  transport: McpTransportType
  url?: string
  command?: string
  args?: string[]
  cwd?: string
  env?: Record<string, McpEnvValueRef>
  auth: McpAuthConfig
  catalogId?: string
  createdAt: string
  updatedAt: string
  toolCount?: number
  tools?: McpToolInfo[]
  status?: McpServerStatus
  statusMessage?: string
}

export interface McpConfigFile {
  version: 1
  servers: McpServerConfig[]
}

export interface McpCatalogEntry {
  id: string
  name: string
  description: string
  transport: McpTransportType
  url: string
  authTypes: McpAuthType[]
  defaultAuthType: McpAuthType
  docsUrl: string
  repoUrl?: string
  toolsSummary: string
  caveat: string
  safety: 'read_only' | 'mixed' | 'write_capable'
}

export interface McpToolInfo {
  name: string
  title?: string
  description?: string
  inputSchema?: unknown
  annotations?: Record<string, unknown>
}

export interface McpServerListItem {
  id: string
  name: string
  enabled: boolean
  transport: McpTransportType
  url?: string
  command?: string
  authType: McpAuthType
  authStatus?: string
  status: McpServerStatus
  statusMessage?: string
  toolCount: number
  catalogId?: string
  caveat?: string
}

export interface McpToolMapping {
  wrapperName: string
  serverId: string
  serverName: string
  toolName: string
  displayName: string
  description: string
  inputSchema?: unknown
  annotations?: Record<string, unknown>
}

export interface McpCallResult {
  serverId: string
  serverName: string
  toolName: string
  contentPreview: string
  content?: unknown
  structuredContent?: unknown
  isError?: boolean
}

export interface McpTestResult {
  success: boolean
  serverId: string
  status: McpServerStatus
  message: string
  tools: McpToolInfo[]
}
