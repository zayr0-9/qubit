import test from 'node:test'
import assert from 'node:assert/strict'
import { QubitMcpOAuthProvider } from './oauth.js'
import { McpSecretStore } from './secretStore.js'
import type { McpServerConfig } from './types.js'

const server: McpServerConfig = {
  id: 'supabase',
  name: 'Supabase',
  enabled: true,
  transport: 'streamable_http',
  url: 'https://mcp.supabase.com/mcp',
  auth: { type: 'oauth' },
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
}

class MemoryKeytar {
  values = new Map<string, string>()

  async setPassword(_service: string, account: string, password: string): Promise<void> {
    this.values.set(account, password)
  }

  async getPassword(_service: string, account: string): Promise<string | null> {
    return this.values.get(account) ?? null
  }

  async deletePassword(_service: string, account: string): Promise<boolean> {
    return this.values.delete(account)
  }
}

test('MCP OAuth provider drops cached clients whose redirect URI no longer matches', async () => {
  const keytar = new MemoryKeytar()
  const secrets = new McpSecretStore({ service: 'Qubit Test', keytar })
  await secrets.saveClientInformation(server.id, {
    client_id: 'registered-client',
    client_secret: 'client-secret',
    redirect_uris: ['http://127.0.0.1:12345/mcp/oauth/callback'],
  })
  await secrets.saveTokens(server.id, { access_token: 'access-token', token_type: 'Bearer' })
  await secrets.set(secrets.codeVerifierAccount(server.id), 'verifier')

  const provider = new QubitMcpOAuthProvider(server, 'http://localhost:12345/mcp/oauth/callback', secrets)

  assert.equal(await provider.clientInformation(), undefined)
  assert.equal(await secrets.getClientInformation(server.id), undefined)
  assert.equal(await secrets.getTokens(server.id), undefined)
  assert.equal(await secrets.get(secrets.codeVerifierAccount(server.id)), undefined)
})

test('MCP OAuth provider keeps cached clients for the active redirect URI', async () => {
  const keytar = new MemoryKeytar()
  const secrets = new McpSecretStore({ service: 'Qubit Test', keytar })
  const clientInformation = {
    client_id: 'registered-client',
    client_secret: 'client-secret',
    redirect_uris: ['http://localhost:12345/mcp/oauth/callback'],
  }
  await secrets.saveClientInformation(server.id, clientInformation)

  const provider = new QubitMcpOAuthProvider(server, 'http://localhost:12345/mcp/oauth/callback', secrets)

  assert.deepEqual(await provider.clientInformation(), clientInformation)
})
