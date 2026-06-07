import test from 'node:test'
import assert from 'node:assert/strict'
import { createMcpTools } from './toolFactory.js'
import { setMcpToolMappings, summarizeMcpArgs, summarizeMcpResult } from './summarize.js'
import type { McpClientManager } from './clientManager.js'
import type { McpServerConfig, McpToolInfo } from './types.js'

const server: McpServerConfig = {
  id: 'linear',
  name: 'Linear',
  enabled: true,
  transport: 'streamable_http',
  url: 'https://mcp.linear.app/mcp',
  auth: { type: 'none' },
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
}

const tools: McpToolInfo[] = [{ name: 'search-issues', title: 'Search issues', description: 'Find issues', inputSchema: { type: 'object', properties: { query: { type: 'string' } } } }]

test('createMcpTools wraps MCP tools with prefixed names and calls manager', async () => {
  const calls: unknown[] = []
  const manager = {
    async callTool(serverId: string, toolName: string, args: Record<string, unknown>) {
      calls.push({ serverId, toolName, args })
      return { content: [{ type: 'text', text: 'issue result' }] }
    },
  } as unknown as McpClientManager

  const { tools: wrapped, mappings } = createMcpTools([server], new Map([[server.id, tools]]), manager)
  assert.equal(wrapped.length, 1)
  assert.equal(wrapped[0].name, 'mcp__linear__search_issues')
  assert.equal(mappings.get(wrapped[0].name)?.toolName, 'search-issues')
  assert.equal(wrapped[0].permission?.mode, 'ask')

  const result = await wrapped[0].execute({ query: 'bug' }, { sessionId: 's', step: 1 })
  assert.equal(result.ok, true)
  assert.deepEqual(calls, [{ serverId: 'linear', toolName: 'search-issues', args: { query: 'bug' } }])
})

test('MCP summaries include server/tool display metadata and redact args', () => {
  const manager = { async callTool() { return {} } } as unknown as McpClientManager
  const { tools: wrapped, mappings } = createMcpTools([server], new Map([[server.id, tools]]), manager)
  setMcpToolMappings(mappings)
  const args = summarizeMcpArgs(wrapped[0].name, { access_token: 'secret', query: 'bug' })
  assert.equal(args.serverName, 'Linear')
  assert.equal(args.toolName, 'search-issues')
  assert.match(String(args.argsPreview), /\[redacted\]/)
  assert.match(String(args.argsPreview), /bug/)

  const result = summarizeMcpResult(wrapped[0].name, { ok: true, data: { result: { content: [{ type: 'text', text: 'hello from mcp' }] } } })
  assert.equal(result.serverName, 'Linear')
  assert.match(String(result.contentPreview), /hello from mcp/)
})
