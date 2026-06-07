import type { OAuthClientInformationMixed, OAuthClientMetadata, OAuthTokens } from '@modelcontextprotocol/sdk/shared/auth.js'
import type { OAuthClientProvider, OAuthDiscoveryState } from '@modelcontextprotocol/sdk/client/auth.js'
import type { McpServerConfig } from './types.js'
import type { McpSecretStore } from './secretStore.js'

export type McpAuthUrlEmitter = (event: { serverId: string; serverName: string; authUrl: string }) => void | Promise<void>

export class QubitMcpOAuthProvider implements OAuthClientProvider {
  readonly clientMetadataUrl?: string | undefined
  private pendingAuthorizationUrl = ''

  constructor(
    private readonly server: McpServerConfig,
    private readonly redirectUrlValue: string,
    private readonly secrets: McpSecretStore,
    private readonly emitAuthUrl?: McpAuthUrlEmitter
  ) {}

  get redirectUrl(): string {
    return this.redirectUrlValue
  }

  get clientMetadata(): OAuthClientMetadata {
    return {
      client_name: 'Qubit',
      redirect_uris: [this.redirectUrlValue],
      grant_types: ['authorization_code', 'refresh_token'],
      response_types: ['code'],
      token_endpoint_auth_method: 'none',
      scope: this.server.auth.scope,
    }
  }

  async state(): Promise<string> {
    return `qubit-${this.server.id}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
  }

  async clientInformation(): Promise<OAuthClientInformationMixed | undefined> {
    const clientInformation = await this.secrets.getClientInformation(this.server.id)
    if (clientInformation && !clientInformationAllowsRedirectUri(clientInformation, this.redirectUrlValue)) {
      await Promise.all([
        this.secrets.delete(this.secrets.oauthClientAccount(this.server.id)),
        this.secrets.delete(this.secrets.oauthTokensAccount(this.server.id)),
        this.secrets.delete(this.secrets.codeVerifierAccount(this.server.id)),
      ])
      return undefined
    }
    return clientInformation
  }

  async saveClientInformation(clientInformation: OAuthClientInformationMixed): Promise<void> {
    await this.secrets.saveClientInformation(this.server.id, clientInformation)
  }

  async tokens(): Promise<OAuthTokens | undefined> {
    return await this.secrets.getTokens(this.server.id)
  }

  async saveTokens(tokens: OAuthTokens): Promise<void> {
    await this.secrets.saveTokens(this.server.id, tokens)
  }

  async redirectToAuthorization(authorizationUrl: URL): Promise<void> {
    this.pendingAuthorizationUrl = authorizationUrl.toString()
    await this.emitAuthUrl?.({ serverId: this.server.id, serverName: this.server.name, authUrl: this.pendingAuthorizationUrl })
  }

  async saveCodeVerifier(codeVerifier: string): Promise<void> {
    await this.secrets.set(this.secrets.codeVerifierAccount(this.server.id), codeVerifier)
  }

  async codeVerifier(): Promise<string> {
    const verifier = await this.secrets.get(this.secrets.codeVerifierAccount(this.server.id))
    if (!verifier) throw new Error(`Missing MCP OAuth code verifier for ${this.server.name}`)
    return verifier
  }

  async saveDiscoveryState(state: OAuthDiscoveryState): Promise<void> {
    await this.secrets.setJson(this.secrets.discoveryAccount(this.server.id), state)
  }

  async discoveryState(): Promise<OAuthDiscoveryState | undefined> {
    return await this.secrets.getJson<OAuthDiscoveryState>(this.secrets.discoveryAccount(this.server.id))
  }

  async invalidateCredentials(scope: 'all' | 'client' | 'tokens' | 'verifier' | 'discovery'): Promise<void> {
    if (scope === 'all' || scope === 'client') await this.secrets.delete(this.secrets.oauthClientAccount(this.server.id))
    if (scope === 'all' || scope === 'tokens') await this.secrets.delete(this.secrets.oauthTokensAccount(this.server.id))
    if (scope === 'all' || scope === 'verifier') await this.secrets.delete(this.secrets.codeVerifierAccount(this.server.id))
    if (scope === 'all' || scope === 'discovery') await this.secrets.delete(this.secrets.discoveryAccount(this.server.id))
  }

  authorizationUrl(): string {
    return this.pendingAuthorizationUrl
  }
}

function clientInformationAllowsRedirectUri(clientInformation: OAuthClientInformationMixed, redirectUri: string): boolean {
  if (!('redirect_uris' in clientInformation) || !Array.isArray(clientInformation.redirect_uris)) return true
  return clientInformation.redirect_uris.some(uri => String(uri) === redirectUri)
}
