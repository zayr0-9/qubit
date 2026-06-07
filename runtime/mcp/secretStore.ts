import type { OAuthClientInformationMixed, OAuthTokens } from '@modelcontextprotocol/sdk/shared/auth.js'
import { maskSecret } from './redact.js'

export interface McpSecretStoreOptions {
  service: string
  keytar: unknown
}

interface KeytarLike {
  setPassword(service: string, account: string, password: string): Promise<void>
  getPassword(service: string, account: string): Promise<string | null>
  deletePassword(service: string, account: string): Promise<boolean>
}

export class McpSecretStore {
  private readonly service: string
  private readonly keytar: KeytarLike | null

  constructor(options: McpSecretStoreOptions) {
    this.service = options.service
    this.keytar = isKeytarLike(options.keytar) ? options.keytar : null
  }

  account(serverId: string, name: string): string {
    return `mcp:${serverId}:${name}`
  }

  async set(account: string, value: string): Promise<{ account: string; masked: string }> {
    this.ensureAvailable()
    await this.keytar!.setPassword(this.service, account, value)
    return { account, masked: maskSecret(value) }
  }

  async get(account?: string): Promise<string | undefined> {
    if (!account) return undefined
    this.ensureAvailable()
    return (await this.keytar!.getPassword(this.service, account)) || undefined
  }

  async delete(account?: string): Promise<void> {
    if (!account) return
    this.ensureAvailable()
    await this.keytar!.deletePassword(this.service, account)
  }

  async setJson<T>(account: string, value: T): Promise<{ account: string; masked: string }> {
    return await this.set(account, JSON.stringify(value))
  }

  async getJson<T>(account?: string): Promise<T | undefined> {
    const raw = await this.get(account)
    if (!raw) return undefined
    return JSON.parse(raw) as T
  }

  oauthTokensAccount(serverId: string): string {
    return this.account(serverId, 'oauth-tokens')
  }

  oauthClientAccount(serverId: string): string {
    return this.account(serverId, 'oauth-client')
  }

  codeVerifierAccount(serverId: string): string {
    return this.account(serverId, 'oauth-code-verifier')
  }

  discoveryAccount(serverId: string): string {
    return this.account(serverId, 'oauth-discovery')
  }

  async getTokens(serverId: string): Promise<OAuthTokens | undefined> {
    return await this.getJson<OAuthTokens>(this.oauthTokensAccount(serverId))
  }

  async saveTokens(serverId: string, tokens: OAuthTokens): Promise<void> {
    await this.setJson(this.oauthTokensAccount(serverId), tokens)
  }

  async getClientInformation(serverId: string): Promise<OAuthClientInformationMixed | undefined> {
    return await this.getJson<OAuthClientInformationMixed>(this.oauthClientAccount(serverId))
  }

  async saveClientInformation(serverId: string, info: OAuthClientInformationMixed): Promise<void> {
    await this.setJson(this.oauthClientAccount(serverId), info)
  }

  async deleteServerSecrets(serverId: string, accounts: string[] = []): Promise<void> {
    await Promise.all([
      this.delete(this.oauthTokensAccount(serverId)),
      this.delete(this.oauthClientAccount(serverId)),
      this.delete(this.codeVerifierAccount(serverId)),
      this.delete(this.discoveryAccount(serverId)),
      ...accounts.map(account => this.delete(account)),
    ])
  }

  private ensureAvailable(): void {
    if (this.keytar) return
    throw new Error('Secure OS keychain storage is unavailable for MCP secrets.')
  }
}

function isKeytarLike(value: unknown): value is KeytarLike {
  return Boolean(value) && typeof value === 'object'
    && typeof (value as KeytarLike).setPassword === 'function'
    && typeof (value as KeytarLike).getPassword === 'function'
    && typeof (value as KeytarLike).deletePassword === 'function'
}
