import test from 'node:test'
import assert from 'node:assert/strict'
import { mcpStarterCatalog } from './catalog.js'
import { normalizeMcpConfig } from './configStore.js'
import { mcpWrapperToolName, parseMcpWrapperToolName } from './names.js'
import { redactMcpSecrets } from './redact.js'

test('starter catalog contains five hosted entries with no raw secrets', () => {
  assert.equal(mcpStarterCatalog.length, 5)
  assert.deepEqual(mcpStarterCatalog.map(entry => entry.id), ['supabase', 'notion', 'linear', 'cloudflare-docs', 'sentry'])
  for (const entry of mcpStarterCatalog) {
    assert.equal(entry.transport, 'streamable_http')
    assert.match(entry.url, /^https:\/\//)
    assert.ok(entry.docsUrl)
    assert.ok(entry.caveat)
    assert.ok(!JSON.stringify(entry).toLowerCase().includes('secret'))
  }
})

test('normalizes catalog MCP server config', () => {
  const config = normalizeMcpConfig({ servers: [{ id: 'Supabase!', catalogId: 'supabase', enabled: true, auth: { type: 'bearer', source: 'env', envVar: 'SUPABASE_ACCESS_TOKEN' } }] })
  assert.equal(config.version, 1)
  assert.equal(config.servers.length, 1)
  assert.equal(config.servers[0].id, 'supabase')
  assert.equal(config.servers[0].url, 'https://mcp.supabase.com/mcp')
  assert.equal(config.servers[0].auth.type, 'bearer')
  assert.equal(config.servers[0].auth.envVar, 'SUPABASE_ACCESS_TOKEN')
})

test('drops invalid stdio server configs and duplicate ids', () => {
  const config = normalizeMcpConfig({ servers: [
    { id: 'custom', transport: 'stdio', args: ['x'] },
    { id: 'custom', transport: 'stdio', command: 'npx', args: ['-y', 'server'] },
    { id: 'custom', transport: 'stdio', command: 'node', args: ['server.js'] },
  ] })
  assert.equal(config.servers.length, 1)
  assert.equal(config.servers[0].command, 'npx')
})

test('MCP wrapper names are sanitized and parseable', () => {
  const name = mcpWrapperToolName('Cloudflare Docs', 'search-cloudflare.documentation')
  assert.equal(name, 'mcp__cloudflare_docs__search_cloudflare_documentation')
  assert.deepEqual(parseMcpWrapperToolName(name), { serverId: 'cloudflare_docs', toolName: 'search_cloudflare_documentation' })
})

test('redacts nested MCP secret values by key and bearer text', () => {
  const redacted = redactMcpSecrets({ Authorization: 'Bearer abc123', nested: { refresh_token: 'rt-secret', ok: 'visible' } })
  assert.deepEqual(redacted, { Authorization: '[redacted]', nested: { refresh_token: '[redacted]', ok: 'visible' } })
})
