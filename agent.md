# Qubit Agent Guide

This file is the project standard for AI agents and human contributors working on Qubit.

## Project Purpose

Qubit is a terminal chat MVP for a Graviton/Qubit-style agent shell.

The project intentionally separates responsibilities:

```txt
hyper-router
  Pure TypeScript SDK dependency.
  No CLI, TUI, app shell, keybindings, or Qubit-specific UX concepts.

Qubit runtime
  Node.js sidecar process.
  Imports hyper-router.
  Owns provider setup, storage setup, session index management, and the JSON-lines bridge.

Qubit CLI
  Go Bubble Tea v2 terminal UI.
  Owns rendering, keyboard interaction, local UI state, slash command palette, and session picker.
```

Current MVP scope is basic chat plus session UI. Branch visualization, minimaps, archive/delete flows, transcript reload on session switch, and streaming can come later.

## Important Paths

```txt
D:\qubit
  package.json              Node runtime package config
  runtime.mjs               Node sidecar runtime
  go.mod                    Go module config
  main.go                   CLI entrypoint
  app.go                    Bubble Tea app model/update logic
  view.go                   TUI rendering
  commands.go               Slash commands and session picker interactions
  runtime_client.go         Go <-> Node JSON-lines client
  types.go                  Shared Go structs and message types
  styles.go                 Lip Gloss styling
  util.go                   Shared helpers
  bin\qubit.exe             Built Windows executable
  .qubit\sessions.sqlite    hyper-router SQLite transcript store
  .qubit\session-index.json Qubit-owned session index
  .qubit\runtime.log        Runtime diagnostic log
```

## Architecture Rules

1. Keep `hyper-router` pure.
   - Do not add Bubble Tea, terminal UI, app-specific sessions, Qubit keybindings, CLI code, or runtime sidecar code to `hyper-router`.
   - Treat it as a reusable SDK dependency.

2. Keep provider/runtime work in `runtime.mjs`.
   - GLM provider setup belongs in Node.
   - `SqliteStorage` setup belongs in Node.
   - Session transcript persistence through hyper-router belongs in Node.
   - The Qubit session index belongs in Node for now.

3. Keep terminal UX in Go.
   - Bubble Tea model/update/view code belongs in Go.
   - Slash command palette and session picker are Go UI concerns.
   - Do not push terminal-specific behavior into the Node runtime unless it is part of the JSON protocol.

4. Communicate across the process boundary with JSON lines only.
   - Go sends one JSON object per line to Node stdin.
   - Node sends one JSON object per line to Go stdout.
   - Runtime stderr is reserved for diagnostics and is copied to `.qubit/runtime.log`.

5. Prefer small, explicit protocol additions.
   - Add event/request types like `session.messages` instead of overloading existing messages.
   - Keep payloads JSON-serializable and stable.
   - Include `id` where request/response correlation matters.

## Current Runtime Protocol

Known request/event types include:

```txt
ready
session.list
session.new
session.activate
session.rename
run_started
assistant
run_finished
error
shutdown
chat
```

When adding protocol messages:

- Use lower-case dot-separated names: `session.messages`, `session.archived`.
- Keep names action-oriented for requests and state-oriented for events.
- Include `sessionId` whenever the message is session-scoped.
- Prefer additive fields over breaking existing shapes.

## Go Project Standards

### Package Layout

This is currently a small single-package Go application using `package main`. Keep files split by responsibility:

```txt
main.go            process entrypoint only
app.go             Bubble Tea model lifecycle and state transitions
view.go            all render/view functions
commands.go        slash commands and picker interactions
runtime_client.go  sidecar process/client code
types.go           shared structs/types
styles.go          UI styles/colors
util.go            small generic helpers
```

Add a new file only when it has a clear responsibility. Do not create tiny abstraction files without a reason.

### Go Style

Follow standard Go conventions:

- Run `gofmt` on every Go change.
- Prefer simple functions and explicit control flow.
- Keep names short but meaningful.
- Use `camelCase` for unexported identifiers.
- Export only what must be used outside the package. Since this is `package main`, most identifiers should stay unexported.
- Return errors instead of panicking, except in truly unrecoverable startup cases.
- Wrap errors with context using `fmt.Errorf("context: %w", err)` when returning them.
- Avoid global mutable state unless it is configuration/style data.
- Keep functions focused; if a function grows too large, extract behavior by responsibility.
- Prefer table-driven tests when adding tests.

### Bubble Tea Standards

- Keep `Update` as a dispatcher; move meaningful logic to helper methods.
- Keep `View` as composition; move large sections to `renderX` helpers.
- Do not perform blocking work inside `Update` or `View`.
- All side effects should happen through `tea.Cmd` where possible.
- Maintain value semantics for the model unless pointer mutation is clearer for local helpers.
- Use `tea.Batch` for multiple commands.
- Keep mouse capture disabled unless the user explicitly asks for mouse-driven UI:

```go
view.MouseMode = tea.MouseModeNone
```

- Alt-screen is currently enabled. If normal terminal scrollback becomes more important, revisit:

```go
view.AltScreen = false
```

### Runtime Client Standards

- Keep the Go runtime client responsible only for process management, logging, JSON encoding/decoding, and Tea commands.
- Do not put business logic in `runtime_client.go`.
- Keep stdout parsing strict JSON-lines.
- Log stdout/stderr lines to `.qubit/runtime.log` for copyable diagnostics.
- Avoid swallowing send/decode/runtime errors silently.

### UI/UX Standards

- Chat should stay usable and calm by default.
- Slash commands should be discoverable by typing `/`.
- Commands requiring arguments should insert a trailing space after completion.
- `/sessions` should open an interactive picker, not print repeated lists into chat.
- Session picker should support:
  - Up/down selection
  - Enter activation
  - Esc close
- Preserve normal terminal selection/copy behavior by keeping mouse capture disabled.

### Node Runtime Standards

- Use pnpm for Node dependency management.
- Keep `runtime.mjs` ESM.
- Keep provider selection environment-driven:

```powershell
$env:ZAI_API_KEY = "your-zai-key"
$env:GLM_MODEL = "glm-4.6"
$env:GLM_ENDPOINT = "coding" # optional
```

- Support stub mode for local development:

```powershell
$env:QUBIT_STUB = "1"
```

- Persist transcripts through hyper-router SQLite storage.
- Maintain listable session metadata in `.qubit/session-index.json` until hyper-router exposes a suitable public session listing API.
- If adding sql.js or packaging behavior, verify WASM resolution carefully.

## Validation Checklist

Before reporting a Go/runtime change as complete, run the narrowest useful checks.

Recommended commands from `D:\qubit` on Windows:

```powershell
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$files = Get-ChildItem -Filter *.go | ForEach-Object { $_.FullName }
gofmt -w $files
go test ./...
go vet ./...
go build -o bin\qubit.exe .
node --check runtime.mjs
```

For manual smoke testing:

```powershell
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$env:QUBIT_STUB = "1"
D:\qubit\bin\qubit.exe
```

Verify:

1. Chat sends and receives a stub response.
2. Typing `/` opens command suggestions.
3. Typing `/re` filters to `/rename`.
4. Enter/Tab accepts the highlighted command.
5. `/sessions` opens the session picker.
6. Up/down changes the highlighted session.
7. Enter activates the selected session.
8. Session lists are not repeatedly appended after assistant responses.
9. Terminal text selection/copy still works normally.

## Suggested Future Work

Prefer incremental changes in this order:

1. Improve session picker display: active marker, updated time, message count, truncation.
2. Add transcript reload when switching sessions via a `session.messages` protocol command.
3. Add archive/delete with confirmation.
4. Add `/clear` with clearly defined behavior.
5. Add better error surfaces and `/logs`.
6. Add session auto-title generation.
7. Add real or simulated token streaming.
8. Add keyboard-first branch/session minimap later.

## Agent Operating Rules

When making changes:

1. Inspect before editing.
2. Keep changes scoped to the user request.
3. Preserve the runtime/TUI/SDK separation.
4. Prefer boring, maintainable code over clever abstractions.
5. Do not introduce unrelated formatting churn.
6. Do not mutate generated files, `.qubit` data, or `bin/qubit.exe` unless rebuilding is part of the task.
7. Do not install dependencies unless clearly required.
8. Run validation before claiming success.
9. If an interactive TUI issue cannot be copied, inspect:

```powershell
Get-Content D:\qubit\.qubit\runtime.log -Raw
```

## Definition of Done

A change is done when:

- The code is formatted.
- The relevant build/check commands pass, or any failure is clearly explained as unrelated/pre-existing.
- The architecture boundaries remain intact.
- The final response summarizes what changed, files touched, and validation performed.
