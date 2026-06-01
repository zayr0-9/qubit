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

Current MVP scope is basic chat plus session UI, including transcript reload on session switch and frontend-simulated assistant streaming. Branch visualization, minimaps, archive/delete flows, and true provider token streaming can come later.

## Important Paths

```txt
D:\qubit
  package.json              Node runtime package config
  runtime.mjs               Node sidecar runtime
  go.mod                    Go module config
  main.go                   CLI entrypoint
  app.go                    Bubble Tea app model/update logic
  streaming.go              Frontend-simulated assistant streaming helpers, if/when split out
  view.go                   TUI rendering, including Glow/Glamour Markdown message rendering
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
session.messages
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
view.go            top-level rendering composition and shared render helpers
commands.go        slash commands and session picker interactions
modal.go           reusable modal state/update/render helpers
streaming.go       frontend-simulated assistant streaming helpers when streaming logic grows
runtime_client.go  sidecar process/client code
types.go           shared structs/types
styles.go          UI styles/colors
util.go            small generic helpers
```

Keep the application maintainable as it grows. Split cohesive behavior into focused files or self-contained components when a feature has its own state/update/render rules, tests, or lifecycle. Do not keep adding unrelated logic to `app.go` or `view.go` until they become giant catch-all files. A new file is appropriate when it has a clear responsibility, reduces coupling, and makes the code easier to navigate; avoid tiny abstraction files that only add indirection.

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
- Keep files focused; if a feature starts adding multiple helpers, tests, or state fields, consider a dedicated file/component instead of expanding an already-large file.
- Prefer small composed helpers and self-contained UI components for modal/picker/streaming-style features.
- Prefer table-driven tests when adding tests.

### Bubble Tea Standards

- Keep `Update` as a dispatcher; move meaningful logic to helper methods or focused feature files.
- Keep `View` as composition; move large sections to `renderX` helpers or component render functions.
- Do not perform blocking work inside `Update` or `View`.
- All side effects should happen through `tea.Cmd` where possible.
- Maintain value semantics for the model unless pointer mutation is clearer for local helpers.
- Use `tea.Batch` for multiple commands.
- The chat composer uses Qubit's custom `composerModel` in `composer.go`, not `textarea.Model` or `textinput.Model`. Preserve multiline input, source-index-based cursor/selection state, internal wrapping, max-height scrolling, and inline selection rendering.
- Do not post-process rendered ANSI text to implement composer selection/highlighting. Selection must be tracked before rendering with source indices so the chevron/prompt is never highlighted and selection stays behind input characters only.
- The composer prompt/chevron renders only on the first visible input row. Wrapped/continuation rows should use prompt-width spaces for alignment, not repeated chevrons.
- Plain Enter sends the current chat message. Modified Enter (`Shift+Enter` when available, and `Alt+Enter` when the terminal does not reserve it) inserts a literal newline and must not submit. `Ctrl+J` is the reliable terminal fallback for inserting a newline.
- Composer keyboard behavior should remain editor-like: arrows/Home/End move in the composer; `Ctrl+Left/Right` and Alt fallbacks move by word; `Ctrl+Home/End` move to input begin/end; `Ctrl+A` selects all; `Shift+Arrow` selects when the terminal reports those keys; `Ctrl+Shift+Left/Right` selects by word when supported; `Ctrl+C` copies selected composer text and quits only when no composer selection exists; Esc clears selection before quitting.
- Recalculate layout after composer changes so the input area can grow/shrink up to its max height and the chat viewport height adjusts. When composer content exceeds max height, scroll inside the composer instead of expanding the app layout past the footer.
- Recalculate layout before refreshing/replacing chat viewport content on transcript-load paths such as `session.messages`. A loaded long conversation must not render with stale viewport dimensions from the loading placeholder or previous screen state; otherwise a large blank gap can appear above the input until the next composer edit triggers layout. The safe order is update messages/state -> `m.layout()` -> `m.refreshViewport()`.
- Keep scrolling app-contained: Qubit runs in Bubble Tea alt-screen and captures cell-motion mouse events so mouse wheel scrolls only the chat message viewport, not the whole terminal above the Qubit title/header:

```go
view.AltScreen = true
view.MouseMode = tea.MouseModeCellMotion
```

- `MouseModeCellMotion` is the accepted tradeoff for contained wheel scrolling: it enables click/release/wheel plus drag events, but common terminals will not support normal drag-to-select text while the app is capturing mouse events. Do not upgrade to `MouseModeAllMotion` unless passive movement events are explicitly needed.
- Mouse wheel handling should scroll the chat viewport directly: wheel up disables auto-scroll, wheel down resumes auto-scroll only when the viewport reaches bottom. `PgUp`/`PgDn` should keep matching this behavior.
- If text selection is needed while mouse capture is enabled, use the terminal's modifier-based selection override when available (for example, holding Shift while dragging in many terminals), or copy through composer selection/clipboard flows where applicable.
- If chat scroll appears to work only after pressing `PgUp`/`PgDn`, inspect whether viewport auto-scroll/layout refresh is forcing `GotoBottom()`. Preserve viewport offset after generic viewport updates, content refreshes, and resize/layout, and only resume auto-scroll when the user submits a new message or explicitly reaches/jumps to the bottom.
- Avoid full layout or full Markdown re-rendering on every generic viewport update or stream tick. Cache rendered historical message content by role/content/width, and do not cache the actively streaming partial message.

### Runtime Client Standards

- Keep the Go runtime client responsible only for process management, logging, JSON encoding/decoding, and Tea commands.
- Do not put business logic in `runtime_client.go`.
- Keep stdout parsing strict JSON-lines.
- Log stdout/stderr lines to `.qubit/runtime.log` for copyable diagnostics.
- Avoid swallowing send/decode/runtime errors silently.

### UI/UX Standards

- Chat should stay usable and calm by default.
- Render user and assistant message bodies as Markdown in `view.go` using the Glow/Glamour renderer path. Preserve fenced code blocks, lists, headings, and intentional newlines.
- If Markdown appears flattened, inspect `.qubit/runtime.log` first to confirm whether message content contains real `\n` characters before changing the renderer.
- Slash commands should be discoverable by typing `/` and must render in a reserved visible area above the input, not appended below the viewport where they can be clipped.
- When reserving terminal layout space, preserve the composer/footer visibility without painting opaque chat/message rows. Avoid using `Style.Width(...).Height(...).Render(...)` or `lipgloss.Place(...)` around the chat viewport when the style has a background, because Lip Gloss/viewport padding can create full-line black bars behind messages and role names. Prefer transparent styles plus explicit blank-line padding helpers such as `renderFixedHeight`/`renderChat`.
- Prefer existing Lip Gloss APIs already used by the project; do not assume newer helpers such as `lipgloss.WithWhitespaceBackground` are available without confirming the pinned dependency supports them.
- Commands requiring arguments should insert a trailing space after completion.
- `/sessions` should open an interactive picker, not print repeated lists into chat.
- Session switching should happen through the interactive picker. Do not add a `/use` slash command unless explicitly requested.
- `/terminal-setup` should patch Windows Terminal `settings.json` to map Shift+Enter to an enhanced keyboard escape sequence. It must be idempotent, create a timestamped backup before writing, remove the common misplaced top-level `command`/`keys` mistake, and report the settings/backup paths. It must not change non-Windows Terminal files.
- Session picker should support:
  - Up/down selection
  - Enter activation
  - Esc close
- Preserve normal terminal selection/copy behavior by keeping mouse capture disabled unless richer mouse interaction is explicitly requested.
- Assistant responses may be frontend-simulated streamed: the runtime can send a complete `assistant` event, and the Go UI may progressively reveal it. Keep this fake streaming as terminal UX logic, not provider/runtime business logic, until true provider token streaming is explicitly added to the protocol.

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
7. Enter activates the selected session and reloads that session's transcript.
8. Long loaded transcripts start with the correct chat viewport height: there should not be a large blank gap above the input area that only disappears after typing.
9. Session lists are not repeatedly appended after assistant responses.
10. Terminal text selection/copy works via the terminal's mouse-capture override where available, or through supported keyboard/clipboard flows.
11. Enter sends a message; Ctrl+J inserts a newline in the composer and expands the input area up to the composer max height. Shift+Enter should also work when keyboard enhancements are supported. Alt+Enter works only if the terminal does not reserve it for fullscreen.
12. Composer editing works: arrows/Home/End move the cursor, Ctrl+Left/Right moves by word when the terminal reports those keys, Ctrl+A selects all, Ctrl+C copies selected composer text, typing replaces selected text, Esc clears selection before quitting, and Shift+Arrow selection works where supported.
13. Composer rendering stays stable: selection highlight appears only behind input characters, the chevron appears only on the first visible input row, continuation rows are aligned with spaces, and long selected/multiline input does not expand beyond max height.
14. `/terminal-setup` patches Windows Terminal settings for Shift+Enter newline support, or reports a clear failure/manual snippet.
15. Pasted or typed fenced Markdown code blocks retain line breaks and render as Markdown in chat.

## Suggested Future Work

Prefer incremental changes in this order:

1. Improve session picker display: updated time and better truncation.
2. Add archive/delete with confirmation.
3. Add `/clear` with clearly defined behavior.
4. Add better error surfaces and `/logs`.
5. Add session auto-title generation.
6. Add true provider token streaming when the runtime/provider protocol supports it.
7. Add normal terminal paste handling improvements if any terminal still flattens multiline paste before Bubble Tea receives it.
8. Add keyboard-first branch/session minimap later.

## Agent Operating Rules

When making changes:

1. Inspect before editing.
2. Keep changes scoped to the user request.
3. Preserve the runtime/TUI/SDK separation.
4. Prefer boring, maintainable code over clever abstractions.
5. Keep the app maintainable: create focused files/components for cohesive features when that improves navigation and prevents giant files.
6. Do not introduce unrelated formatting churn.
7. Do not mutate generated files, `.qubit` data, or `bin/qubit.exe` unless rebuilding is part of the task.
8. Do not install dependencies unless clearly required.
9. Run validation before claiming success.
10. If an interactive TUI issue cannot be copied, inspect:

```powershell
Get-Content D:\qubit\.qubit\runtime.log -Raw
```

## Definition of Done

A change is done when:

- The code is formatted.
- The relevant build/check commands pass, or any failure is clearly explained as unrelated/pre-existing.
- The architecture boundaries remain intact.
- The final response summarizes what changed, files touched, and validation performed.
