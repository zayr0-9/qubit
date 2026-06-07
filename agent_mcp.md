# Qubit MCP Agent Guide

This file is mandatory when working on Qubit's Model Context Protocol (MCP) support: MCP server config, OAuth/auth, dynamic MCP tool registration, `/mcp` UI, MCP integration tests, or MCP tool-call display.

## Architecture

- Runtime-side MCP support lives under `runtime/mcp/`.
- User-global non-secret MCP configuration is stored in `<config>/mcp.json`.
- Raw MCP secrets/tokens are stored in the OS keychain through `keytar` using the Qubit keychain service.
- MCP servers are user-global configuration, not project `.qubit` state.
- MCP tool wrappers are generated dynamically during runtime state creation and are added alongside Qubit's built-in tools.
- Wrapper tool names use `mcp__<serverId>__<toolName>` to avoid collisions with built-in tools.

## Current MCP Runtime Files

```txt
runtime/mcp/types.ts         Shared MCP config/catalog/tool DTOs.
runtime/mcp/catalog.ts       Starter hosted MCP catalog.
runtime/mcp/configStore.ts   `<config>/mcp.json` load/save/normalization.
runtime/mcp/secretStore.ts   Keychain-backed MCP secret/token storage.
runtime/mcp/oauth.ts         OAuth provider for Streamable HTTP MCP servers.
runtime/mcp/clientManager.ts Lazy stdio/Streamable HTTP MCP clients, list/call/test/auth.
runtime/mcp/toolFactory.ts   Hyper-router tool wrappers for MCP tools.
runtime/mcp/summarize.ts     Bounded/redacted MCP tool summaries for UI events.
runtime/mcp/names.ts         MCP wrapper-name sanitization/parsing.
runtime/mcp/redact.ts        MCP secret redaction and previews.
```

## Starter Catalog

The default hosted starter catalog currently includes:

1. Supabase — `https://mcp.supabase.com/mcp`
2. Notion — `https://mcp.notion.com/mcp`
3. Linear — `https://mcp.linear.app/mcp`
4. Cloudflare Docs — `https://docs.mcp.cloudflare.com/mcp`
5. Sentry — `https://mcp.sentry.dev`

Use primary vendor docs/repos when updating entries. Keep caveats visible in `/mcp`; server defaults are allowed, but Qubit permission prompts still gate model-initiated MCP tool calls.

## Runtime Protocol

MCP request/event types use lower-case dot names:

```txt
mcp.catalog
mcp.list
mcp.add
mcp.update
mcp.delete
mcp.secret.set
mcp.test
mcp.auth.start
mcp.updated
mcp.test.started
mcp.test.finished
mcp.auth.started
mcp.auth.completed
mcp.auth.error
```

MCP manager responses are request-scoped. Do not broadcast `/mcp` manager state to other attached TUIs.

## Go TUI

- `/mcp` opens `modeMcpManager`.
- MCP manager supports add, test, OAuth, bearer-token entry, enable/disable, delete, refresh, details, and close.
- Bearer token entry uses `modeMcpSecretEntry`; rendered text must be masked and paste must route to the secret entry, not the chat composer.
- MCP calls render through existing tool-call rows. `mcp__` tool groups are labeled as MCP calls and display server/tool metadata from summarized args/results.

## OAuth Notes

- MCP OAuth callback URLs use `http://localhost:<port>/mcp/oauth/callback` rather than `127.0.0.1`; Supabase rejects the `127.0.0.1` redirect URI during hosted MCP OAuth. If the callback host/port shape changes, invalidate cached OAuth client registrations whose stored `redirect_uris` do not include the active callback URL.

## Security Rules

- Never write raw MCP secrets, bearer tokens, OAuth tokens, or authorization headers to JSON config, stdout/stderr, runtime logs, or Go UI details.
- Use `runtime/mcp/redact.ts` for runtime previews and summaries.
- Store raw bearer tokens/OAuth tokens in keychain only.
- Environment-variable references are allowed and should remain read-only metadata.
- Stdio MCP server env values may come from literal/env/keychain refs, but raw secret literals should not be introduced by UI flows.

## Testing

Default MCP tests must not require live vendor credentials. Use catalog/config/unit tests and mocked manager/tool wrapper tests.

Live starter smoke tests should be opt-in with env flags/credentials and must not be reported as passed unless actually run. At minimum, a live smoke should initialize and list tools; call only clearly safe read-only tools.

Recommended checks for MCP changes:

```sh
pnpm run test -- runtime/mcp/*.test.ts
go test ./internal/tui -run 'TestSlashMcp|TestMcp|TestToolCall'
pnpm run check:runtime
go test ./...
```
