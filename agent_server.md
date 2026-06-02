# Qubit Runtime Server Guide

This file is mandatory context when working on Qubit's singleton Node runtime server, multi-TUI attachment, runtime process lifetime, client routing, or JSON-lines transport.

## Purpose

Qubit supports multiple terminal UI instances attached to one runtime server for the same project `.qubit` directory. This prevents independent Node sidecars from concurrently mutating the same SQLite transcript DB and Qubit-owned JSON state files.

The intended shape is:

```txt
Terminal A qubit TUI ─┐
Terminal B qubit TUI ─┼─ localhost JSON-lines runtime server ─ .qubit/sessions.sqlite
Terminal C qubit TUI ─┘                                      ├─ .qubit/session-index.json
                                                              ├─ .qubit/settings.json
                                                              └─ .qubit/api-key-index.json
```

The first TUI for a project starts the Node runtime server. Later TUIs connect to that existing server. The first TUI must still be a normal usable TUI; it is not server-only.

## Current Implementation

- Go startup is in `runtime_client.go`.
- Node runtime server mode is in `runtime.ts`.
- Go computes a deterministic localhost address from the project `.qubit` directory.
- Go first tries to connect to the existing runtime server.
- If no server is reachable, Go acquires `.qubit/runtime-server.lock`, starts `dist/runtime.js` with `QUBIT_RUNTIME_ADDR`, then connects to the server as a client.
- Node uses a TCP JSON-lines server bound to localhost.
- Direct stdio mode remains available when `QUBIT_RUNTIME_ADDR` is not set, including `node dist/runtime.js --check`.

## Runtime Server Ownership Rules

1. One runtime server per project `.qubit` directory.
   - The identity is the terminal launch cwd's `.qubit` path.
   - Different project directories should use different runtime server addresses and DBs.

2. The runtime server owns persistent shared state.
   - Session transcript persistence through SQLite.
   - `session-index.json`.
   - `settings.json`.
   - `api-key-index.json` metadata.
   - Provider/model/key runtime state.

3. Each TUI owns local UI state.
   - Selected session/fork view.
   - Composer contents and cursor/selection.
   - Slash/modal open state.
   - Viewport scroll position.
   - Local rendering cache.

4. A TUI action must not mirror local UI state into other TUIs.
   - Opening `/sessions`, selecting a session, opening a modal, or loading transcript in Terminal A must not change Terminal B's selected view.
   - Request/response messages should normally go only to the requesting client.

## JSON-lines Routing Standards

The runtime server has two classes of outbound messages:

### Request-scoped responses

These go only to the client that sent the request.

Examples:

```txt
ready
session.list
session.activated
session.messages
session.tree
model.list
model.updated
key.list
key.updated
session.renamed
error for a specific request
```

Use the current response target in `runtime.ts` when handling a client line so helper functions that call `write(...)` route back to the initiating socket.

### Broadcast events

These may be delivered to every attached TUI only when the event represents shared runtime activity that all clients may need to know about.

Examples/future candidates:

```txt
run_started
assistant / assistant.delta
run_finished
tool.call.start
tool.call.finish
tool.permission.request
session.updated
```

Broadcast/run-scoped request events must include enough identifiers for the runtime server and clients to route, show, badge, or ignore them:

```txt
runId
sessionId
branchId/nodeId when same-session branch streams exist
request id where relevant
```

Tool lifecycle and permission events are run-scoped. Hyper-router hook payloads (`requestToolPermission`, `onToolCallStart`, and `onToolCallFinish`) are run-aware and should include `runId` whenever emitted from an active agent run. Qubit routes these events by `runId` first, using `sessionId` only as a defensive fallback for older payloads.

Do not broadcast local navigation responses such as `session.activated` and `session.messages`; that causes every TUI to mirror one TUI's selected session.

## Client Attachment Behavior

When a TUI starts:

1. Resolve launch cwd and project `.qubit` directory.
2. Compute the deterministic localhost runtime address.
3. Try to connect briefly.
4. If connected, attach as a secondary client.
5. If not connected, acquire `.qubit/runtime-server.lock` and start Node with:

```txt
QUBIT_WORKSPACE_CWD=<launch cwd>
QUBIT_PROJECT_DIR=<launch cwd>\.qubit
QUBIT_RUNTIME_ADDR=<host:port>
```

6. Connect to the newly started server.
7. Treat `[runtime-server] ...` stderr diagnostics as log-only, not TUI runtime errors.

If a stale lock is found and no server becomes reachable, Go may remove the stale lock and retry ownership.

## Server Lifetime Standards

Current MVP behavior:

- The owner TUI starts the Node server.
- Secondary TUIs attach to the existing server.
- Attached secondary TUIs should close only their socket on exit.
- The owner TUI may still terminate the Node server on exit in the current implementation.

Future preferred behavior:

- Server lifetime should be independent from any single TUI.
- The server should stay alive while clients are connected or active runs exist.
- The server may exit after an idle timeout when no clients/runs remain.

Do not regress the first TUI into a server-only process. It must connect to the server it starts and behave like every other TUI.

## Error and Logging Standards

- Runtime socket/stdout parsing remains strict JSON-lines.
- Runtime diagnostics go to `.qubit/runtime.log`.
- Log-only stderr prefixes include:
  - `[runtime-server]`
  - `[codex-oauth]`
  - `[codex-retry]`
- Do not surface expected server lifecycle diagnostics as user-visible runtime errors.
- Do surface actual runtime crashes, JSON decode failures, and send failures clearly.

## Persistence and Concurrency Standards

- Prefer all Qubit writes to shared `.qubit` state to happen through the singleton runtime server.
- Do not add a second independent runtime sidecar path that writes the same DB/index files.
- If adding new shared JSON files, write them from the runtime server where practical and use atomic write patterns.
- SQLite access should remain coordinated by the singleton runtime where possible.

## Testing Expectations

Changes to this area should include or update tests where practical and must be manually smoke-tested with at least two terminals when behavior is UI-lifecycle related.

Recommended manual smoke test:

1. Close all Qubit terminals.
2. Start Terminal A with `pnpm run chat`.
3. Verify Terminal A is usable and can type `/`.
4. Start Terminal B with `pnpm run chat`.
5. Verify Terminal B attaches and is usable.
6. In Terminal A, open `/sessions` and activate a different session.
7. Verify Terminal B does not mirror Terminal A's selected session.
8. In Terminal B, open `/providers` or `/models` and verify Terminal A does not open/mirror that modal.
9. Send a normal chat in one terminal and verify the other terminal does not corrupt its local selected view.
10. Inspect `.qubit/runtime.log` if startup or routing looks wrong.

Automated tests should prefer focused Go tests for `runtime_client.go` address/attach behavior where possible and TypeScript/runtime tests for client routing functions if the runtime server is further modularized.

## Future Work

- Add explicit client IDs to connection state and logs.
- Add protocol-level `clientId` only if needed for diagnostics; do not expose it in normal UI.
- Make runtime server lifetime independent from the owner TUI.
- Add active run snapshots so newly attached TUIs can discover in-flight runs.
- Continue hardening per-run/per-stream routing with `runId`, `sessionId`, and branch IDs.
- Add robust broadcast policy for background stream events after single-TUI parallel stream state is implemented.
