# Qubit Codex Provider Guide

This file is mandatory context when working on Qubit's local Codex provider, ChatGPT OAuth flow, Codex token storage/refresh, Codex Responses API integration, or provider/model switching that includes Codex.

## Architecture Boundary

Codex support is a Qubit runtime/backend exception, not a `hyper-router` provider. Do not add Codex provider code to `D:\hyper-router` or to the vendored `@hyper-labs/hyper-router` dependency.

The local provider lives under:

```txt
runtime/codex/
```

It implements `hyper-router`'s `ModelProvider` interface so the existing Qubit runtime/tool loop can use it unchanged.

## Auth and Secret Handling

- Default storage is the OS keychain (`service=Qubit`, `account=codex:chatgpt`).
- The keychain secret is a Codex-compatible auth JSON blob.
- Windows Credential Manager can reject large token blobs with `The stub received bad data.`; Qubit stores large Codex auth JSON as chunked keychain entries behind a small manifest.
- User-global `<config>/codex-auth-index.json` may contain sanitized metadata only.
- Do not log, print, or send to Go any raw access token, refresh token, ID token, authorization code, OAuth state, PKCE verifier/challenge, or bearer header.
- Plaintext auth-file storage is an explicit escape hatch only via `QUBIT_CODEX_AUTH_STORAGE=file` or `QUBIT_CODEX_AUTH_FILE`.

## OAuth Constants

```txt
issuer: https://auth.openai.com
client id: app_EMoamEEZ73f0CkXaXp7hrann
preferred callback: http://localhost:1455/auth/callback
fallback callback: http://localhost:1457/auth/callback
scope: openid profile email offline_access
```

Only callback ports `1455` and `1457` are registered for this OAuth client. Do not fall back to arbitrary ports; Hydra rejects unregistered redirect URIs with `authorize_hydra_invalid_request`.

The flow uses authorization code + PKCE (`S256`). Match the known-working YggChat/Codex flow shape:

- 32-character hex `state` from 16 random bytes.
- `code_verifier` from 32 random bytes base64url encoded without padding.
- Token exchange body order: `grant_type`, `client_id`, `code`, `code_verifier`, `redirect_uri`.
- Token exchange header: `Content-Type: application/x-www-form-urlencoded`.
- Authorize params include `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, and `originator=codex_cli_rs`.
- Do not send `allowed_workspace_id` unless the Qubit-specific `QUBIT_CODEX_ALLOWED_WORKSPACE_ID` escape hatch is set.

## Runtime Protocol

Codex OAuth uses explicit protocol messages, not API-key `key.set`:

```txt
codex.status
codex.login.start
codex.login.cancel
codex.logout
```

Events include:

```txt
codex.status
codex.login.started
codex.login.completed
codex.login.cancelled
codex.logout.completed
codex.error
```

Payloads must stay sanitized.

## Provider and Model Switching

Qubit provider switching is runtime-owned and should update both the active provider and active model. Provider slash commands should route through the runtime instead of only mutating Go UI state.

Provider selection should be exposed through:

```txt
/providers
```

`/providers` opens a selectable provider list. Do not add individual top-level provider commands like `/codex` or `/openrouter` unless explicitly requested.

When switching to Codex:

- Select the Codex default model unless a Codex-specific model override is configured.
- The `/models` list should show Codex models, not the previous provider's models.
- If OAuth is not signed in, provider selection may still switch to Codex, but chat generation will require `/codex-login` first.
- Keep Codex provider implementation Qubit-local under `runtime/codex/`; do not add it to `hyper-router`.

## Codex Responses Backend

Codex model metadata is currently Qubit-local. For the MVP, Codex model entries expose `maxContext: 400000` so the Go UI can estimate consumed context with a rough 1 token = 4 characters heuristic. Other providers should get provider-specific context limits later.

Default endpoint:

```txt
https://chatgpt.com/backend-api/codex/responses
```

Requests use `Authorization: Bearer <ChatGPT access token>` and `Accept: text/event-stream`.

Important event types include:

```txt
response.output_text.delta
response.reasoning_text.delta
response.reasoning_summary_text.delta
response.output_item.done
response.completed
response.failed
response.incomplete
```

Reasoning capture must handle both streaming deltas and completed output items. Codex may return reasoning as `response.reasoning_text.delta` / `response.reasoning_summary_text.delta`, or later as `item.type === "reasoning"` inside `response.output_item.done` / `response.completed` output arrays with `summary` or `content` text parts. Preserve extracted text on `message.reasoningContent` so the Go TUI can surface a separate reasoning block.

## Validation

Before live OAuth testing, run at least:

```powershell
pnpm run check:runtime
go test ./...
```

Inspect `.qubit/runtime.log` and stdout protocol messages to ensure secrets are redacted.
