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

Current MVP scope is basic chat plus session UI, including transcript reload on session switch, session forking through `/fork`, edit/reroll forks from prior user messages, a keyboard-first fork tree through `/tree`, and frontend-simulated assistant streaming. The `/tree` view is a symbolic horizontal message/fork map: left/right navigates parent/child lineage, Up/Down jumps to parallel branch nodes, and `j`/`k` preserve linear order navigation. Archive/delete flows and true provider token streaming can come later.

## Extra Context Files

Some focused project guidance lives in separate context files using the naming scheme `agent_<category>.md`.

Agents must read the relevant category context file before planning or changing that area, in addition to this guide. If a task spans multiple categories, read every relevant `agent_<category>.md` file first.

Agents must also update the relevant category context files when a change affects standards, architecture, workflows, file locations, tool behavior, testing expectations, or other durable guidance for that category.

```txt
agent_design.md
  Mandatory when working on terminal UI design, visual styling, layout, colors, borders, spacing, or rendering behavior.

agent_input.md
  Mandatory when working on Go TUI keyboard routing, composer behavior, paste/clipboard flows, mouse capture, mouse wheel routing, transcript selection, terminal keyboard setup, or text input areas.

agent_tools.md
  Mandatory when working on model-callable tools, tool registration, runtime tool permissions, or shared filesystem/path infrastructure.

agent_codex.md
  Mandatory when working on Qubit's local Codex provider, ChatGPT OAuth flow, Codex token storage/refresh, or Codex Responses API integration.

agent_server.md
  Mandatory when working on Qubit's singleton Node runtime server, multi-TUI attachment, runtime process lifetime, client routing, or JSON-lines transport.

agent_md_editor.md
  Mandatory when working on Qubit's `/md-editor` slash command, Markdown document list/editor UI, or runtime Markdown document protocol.
```

When adding a new major subsystem or extracting detailed guidance from this file, create a focused `agent_<category>.md` context file and list it here with when it is mandatory to read.

## Important Paths

```txt
Project root
  package.json                         Node runtime package config
  runtime.ts                           Node sidecar runtime source
  dist/runtime.js                      Compiled Node sidecar runtime launched by Go
  go.mod                               Go module config
  main.go                              CLI entrypoint only; calls internal/tui.Run
  internal/tui/run.go                  Exported TUI package entrypoint used by main.go
  internal/tui/app.go                  Bubble Tea app model/update dispatcher
  internal/tui/input_update.go         Keyboard/composer submit flow
  internal/tui/runtime_events.go       Runtime event dispatcher
  internal/tui/run_lifecycle.go        Assistant/reasoning/streaming/notification lifecycle
  internal/tui/session_events.go       Session list/message event handling
  internal/tui/errors_layout.go        Runtime/send error handling and layout helpers
  internal/tui/view.go                 Top-level TUI rendering composition
  internal/tui/header_footer_view.go   Header, footer, and status rendering
  internal/tui/input_view.go           Composer/input rendering
  internal/tui/message_view.go         Chat viewport/message rendering helpers
  internal/tui/markdown_view.go        Glamour Markdown rendering and ANSI cleanup
  internal/tui/session_picker_view.go  Session picker rendering
  internal/tui/modal_view.go           Modal rendering
  internal/tui/plan_view.go            Plan view rendering
  internal/tui/reasoning_view.go       Reasoning block rendering
  internal/tui/layout_render.go        Transparent fixed-height render helpers
  internal/tui/commands.go             Slash commands and command palette interactions
  internal/tui/keys.go                 API key picker and masked key-entry UI
  internal/tui/runtime_client.go       TUI-side Tea command adapter around runtimeclient
  internal/tui/runtime_state.go        TUI runtime client wrapper state for tests/adapters
  internal/tui/protocol_aliases.go     Internal aliases to protocol DTOs
  internal/tui/model_struct.go         Main model struct
  internal/tui/*_state.go              Focused UI state structs split by feature/domain
  internal/tui/styles.go               Lip Gloss styling
  internal/tui/util.go                 Shared helpers
  internal/tui/protocol/types.go       JSON-lines protocol DTOs
  internal/tui/runtimeclient/client.go Runtime process/server client implementation
  internal/tui/components/composer/    Custom multiline composer component
  internal/tui/components/listwindow/  Visible list-window helper
  internal/tui/platform/platform.go    Browser/config-dir/notification platform helpers
  internal/tui/storage/input_history.go
                                       Project-local input history store
  bin/qubit                            Built Linux/macOS executable
  bin/qubit.exe                        Built Windows executable
  .qubit/sessions.sqlite               hyper-router SQLite transcript store in the terminal launch cwd
  .qubit/session-index.json            Qubit-owned session index in the terminal launch cwd; may include session metadata such as favouritedAt
  .qubit/runtime.log                   Runtime diagnostic log in the terminal launch cwd
  .qubit/codex-provider-calls.log
                                       JSON-lines Codex provider call log in the terminal launch cwd
  .qubit/input-history.json            Persisted non-secret composer history in the terminal launch cwd
  %APPDATA%\Qubit or ~/.config/qubit
                                       User-global Qubit config directory, overrideable with QUBIT_CONFIG_DIR
  <config>/theme.json                  User-global selected `/theme` palette
  <config>/settings.json               User-global non-secret app defaults, including default provider, per-provider default models, and the global subagent provider/model
  .qubit/todos/*.md                    Project todo lists managed by todoMd
  .qubit/plans/*.md                    Project plans managed by planMd
```

## Architecture Rules

1. Keep `hyper-router` pure.
   - Do not add Bubble Tea, terminal UI, app-specific sessions, Qubit keybindings, CLI code, or runtime sidecar code to `hyper-router`.
   - Treat it as a reusable SDK dependency.
   - Codex provider exception: Codex is implemented inside Qubit runtime, not `hyper-router`, because it uses ChatGPT OAuth, local token storage, and the ChatGPT Codex backend. It still implements `hyper-router`'s `ModelProvider` interface.

2. Keep provider/runtime work in TypeScript runtime source.
   - GLM provider setup belongs in Node TypeScript runtime files.
   - `SqliteStorage` setup belongs in Node TypeScript runtime files.
   - Session transcript persistence through hyper-router belongs in Node TypeScript runtime files.
   - The Qubit session index belongs in Node for now.
   - Go launches the compiled `dist\runtime.js`; do not edit `dist` directly.

3. Keep terminal UX in Go.
   - Bubble Tea model/update/view code belongs in Go.
   - Slash command palette and session picker are Go UI concerns.
   - Do not push terminal-specific behavior into the Node runtime unless it is part of the JSON protocol.

4. Communicate across the process boundary with JSON lines only.
   - Go TUIs attach to one singleton Node runtime server per terminal launch cwd/project `.qubit` directory when possible.
   - The first TUI starts `dist\\runtime.js` with `QUBIT_RUNTIME_ADDR`; later TUIs connect to the same localhost JSON-lines server instead of starting independent runtimes against the same DB.
   - In direct/stdin check mode, Go sends one JSON object per line to Node stdin and Node sends one JSON object per line to Go stdout.
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
session.favourite
session.favourited
session.fork
session.tree
key.list
key.set
key.use
key.delete
key.updated
run_started
assistant
run_finished
plan.view
plan.clarification.request
plan.clarification.response
subagent.config
subagent.provider.use
subagent.model.use
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

The root binary is intentionally tiny: `main.go` imports `github.com/qubit/graviton-cli/internal/tui` and calls `tui.Run()`. Most app orchestration still lives in package `internal/tui` so white-box tests and unexported model internals remain usable, but low-risk leaf packages have been extracted.

```txt
main.go
  Process entrypoint only.

internal/tui/run.go
  Exported Run() entrypoint: start runtime, run Bubble Tea, return errors.

internal/tui/app.go
  `initialModel`, `Init`, and `Update` dispatcher only.

internal/tui/input_update.go
  Keyboard routing, composer updates, submit flow, chat-run setup, and edit/reroll submit paths.

internal/tui/runtime_events.go
  Runtime event dispatcher and run-scoped event acceptance.

internal/tui/run_lifecycle.go
  Assistant/reasoning events, frontend-simulated streaming, cancellation, and run-complete notifications.

internal/tui/session_events.go
  Session list/activity reconciliation and session message/favourite/delete event handling.

internal/tui/errors_layout.go
  Runtime/send error handling, terminal setup result handling, viewport/layout helpers.

internal/tui/view.go
  Top-level View composition, app view setup, main area routing, and bottom overlay selection.

internal/tui/header_footer_view.go
  Header, footer, badges, and input status rendering.

internal/tui/input_view.go
  Composer/input rendering and prompt helpers.

internal/tui/session_picker_view.go
  Session picker rendering and row formatting.

internal/tui/message_view.go
  Viewport refresh, message separators, view-message rendering, message icons, and render cache lookup.

internal/tui/markdown_view.go
  Glamour Markdown rendering, no-background style config, and ANSI background stripping.

internal/tui/plan_view.go
  Plan view-message rendering.

internal/tui/reasoning_view.go
  Reasoning block rendering helpers.

internal/tui/layout_render.go
  Transparent fixed-height/chat render helpers.

internal/tui/commands.go, keys.go, md_editor.go, fork_tree.go, tool_calls.go, todo_overlay.go, transcript_selection.go, file_mentions.go, theme.go, modal.go
  Feature interaction/update/render helpers that still need direct model access.

internal/tui/model_struct.go
  Main Bubble Tea model struct.

internal/tui/*_state.go and internal/tui/runtime_messages.go
  Focused UI state structs and Tea message wrappers split out from the former catch-all types file.

internal/tui/protocol/types.go
  JSON-lines protocol DTOs shared by the runtime client adapter and TUI.
  `internal/tui/protocol_aliases.go` keeps package-local aliases for existing TUI code; preserve JSON tags when editing DTOs.

internal/tui/runtimeclient/client.go
  Concrete Go <-> Node runtime process/server client. It owns app-root discovery, singleton server attach/start, JSON-lines send/read, diagnostics logging, and shutdown. Keep Bubble Tea adapters in `internal/tui/runtime_client.go`.

internal/tui/components/composer/
  Custom multiline composer component with public methods for value/cursor/selection/layout configuration.

internal/tui/components/listwindow/
  Pure visible-list-window helper.

internal/tui/platform/
  Browser opener, user config directory helper, notification interface/default backend.

internal/tui/storage/
  Project-local input history JSON store.
```


Future extraction should proceed incrementally from `internal/tui` into subpackages such as `internal/tui/features/...` only when boundaries are clear and tests can move with the extracted code. Keep app-level Tea command adapters and model-bound behavior in `internal/tui`; keep leaf packages free of broad model/business logic. Do not add unrelated logic to `internal/tui/app.go` or `internal/tui/view.go`; prefer the focused files above or a new cohesive file/component when a feature has its own state, update/render rules, tests, or lifecycle. Avoid tiny abstraction files that only add indirection.


### Go Style

Follow standard Go conventions:

- Run `gofmt` on every Go change.
- Prefer simple functions and explicit control flow.
- Keep names short but meaningful.
- Use `camelCase` for unexported identifiers.
- Export only what must be used outside the package. The root `package main` should normally only call `tui.Run`; most TUI identifiers in `internal/tui` should stay unexported until a real subpackage boundary needs them.
- Return errors instead of panicking, except in truly unrecoverable startup cases.
- Wrap errors with context using `fmt.Errorf("context: %w", err)` when returning them.
- Avoid global mutable state unless it is configuration/style data.
- Keep functions focused; if a function grows too large, extract behavior by responsibility.
- Keep files focused; if a feature starts adding multiple helpers, tests, or state fields, consider a dedicated file/component instead of expanding an already-large file.
- Prefer small composed helpers and self-contained UI components for modal/picker/streaming-style features.
- Prefer table-driven tests when adding tests.

### Testing Standards

- New features and bug fixes must include meaningful tests at the right level, not only compile checks.
- For user-visible feature work, add integration-style model/runtime-flow tests that exercise the complete interaction path where practical. For example: user input or keypress -> model update -> outbound runtime JSON command -> runtime event response -> final model/message/viewport state.
- Session, slash-command, runtime-protocol, and transcript-loading changes should assert the exact outbound request type and important fields (`sessionId`, `title`, `input`, etc.), then simulate the expected runtime event and assert the resulting UI state.
- Fork/edit/reroll flows must be tested end-to-end at the model/protocol boundary: selecting a fork point or edited message -> outbound `session.fork`, `chat`, or `session.messages` request -> runtime events -> final active session and transcript. Edited reroll forks must not merge the original edited-away message/response back into the loaded chat history.
- Bubble Tea `tea.Cmd` and `tea.Batch` values should be tested carefully. Do not run blocking commands like `waitRuntimeEvent` sequentially in tests; use a recording/fake runtime client, inspect outbound JSON writes, and use short timeouts when probing batched commands.
- Prefer focused integration-style tests in Go for CLI behavior over manual-only verification. Manual smoke checks are still useful for rendering/terminal behavior, but they should not be the only coverage for core flows.
- When fixing a regression, add a test that would have failed before the fix.

### Bubble Tea Standards

- Keep `Update` as a dispatcher; move meaningful logic to helper methods or focused feature files.
- Keep `View` as composition; move large sections to `renderX` helpers or component render functions.
- Do not perform blocking work inside `Update` or `View`.
- All side effects should happen through `tea.Cmd` where possible.
- Maintain value semantics for the model unless pointer mutation is clearer for local helpers.
- Use `tea.Batch` for multiple commands.
- Input, paste, mouse, and transcript-selection behavior is covered by `agent_input.md`; read it before changing those areas. In short: preserve the custom composer, route paste to the focused input surface, keep app-contained wheel scrolling/selection semantics, recalculate layout after composer changes, and avoid full Markdown re-renders during mouse/viewport churn.
- Recalculate layout before refreshing/replacing chat viewport content on transcript-load paths such as `session.messages`. A loaded long conversation must not render with stale viewport dimensions from the loading placeholder or previous screen state; otherwise a large blank gap can appear above the input until the next composer edit triggers layout. The safe order is update messages/state -> `m.layout()` -> `m.refreshViewport()`.

### Runtime Client Standards

- Keep the Go runtime client responsible only for process management, singleton server attach/start, logging, JSON encoding/decoding, and Tea commands.
- Do not put business logic in `internal/tui/runtime_client.go` or `internal/tui/runtimeclient/client.go`.
- Prefer attaching to an existing runtime server for the same project `.qubit` directory; only start a new Node process when no server is reachable.
- Normal TUI shutdown must close only that TUI's runtime socket. Do not kill the singleton Node server or remove `.qubit/runtime-server.lock` on normal exit.
- Keep runtime socket/stdout parsing strict JSON-lines.
- Log runtime socket events and server diagnostics to `.qubit/runtime.log` for copyable diagnostics; a server process that outlives its starter TUI should write diagnostics directly to that file.
- Unexpected runtime socket disconnects should trigger reconnect/failover before becoming permanent UI errors.
- Avoid swallowing send/decode/runtime errors silently.

### UI/UX Standards

- Chat should stay usable and calm by default.
- Render user and assistant message bodies as Markdown through `internal/tui/message_view.go` and `internal/tui/markdown_view.go` using the Glow/Glamour renderer path. Preserve fenced code blocks, lists, headings, and intentional newlines.
- If Markdown appears flattened, inspect `.qubit/runtime.log` first to confirm whether message content contains real `\n` characters before changing the renderer.
- Slash commands should be discoverable by typing `/` and must render in a reserved visible area above the input, not appended below the viewport where they can be clipped.
- When reserving terminal layout space, preserve the composer/footer visibility without painting opaque chat/message rows. Avoid using `Style.Width(...).Height(...).Render(...)` or `lipgloss.Place(...)` around the chat viewport when the style has a background, because Lip Gloss/viewport padding can create full-line black bars behind messages and role names. Prefer transparent styles plus explicit blank-line padding helpers such as `renderFixedHeight`/`renderChat`.
- Prefer existing Lip Gloss APIs already used by the project; do not assume newer helpers such as `lipgloss.WithWhitespaceBackground` are available without confirming the pinned dependency supports them.
- Commands requiring arguments should insert a trailing space after completion.
- Slash command palette filtering should rank command-name matches before description-only matches so command names remain the primary selection signal.
- Slash commands that open interactive UI directly from the palette should mark `slashCommand.OpensOnSelect` so Enter/Tab clears the composer and opens the UI instead of inserting command text.
- Reusable modal selector lists use `modalState.Options` plus `OptionCursor`: Up/down moves the option cursor, left/right or tab/shift+tab moves actions, Enter resolves the selected action, and Esc cancels selector-style modals.
- `/models` should open the model selector modal backed by runtime `model.list`/`model.use` protocol data, not a hardcoded demo list. The model selector offers Use now and Set default actions; Set default sends `model.use` with `persistDefault: true` so the runtime stores a non-secret per-provider default model in user-global `<config>/settings.json`.
- `/subagents` opens the subagent provider/model selector backed by runtime `subagent.config`, `subagent.provider.use`, and `subagent.model.use` protocol data. The selected subagent provider/model is stored in `<config>/settings.json` under `subagent` and must not mutate the main active provider/model.
- `/sessions` should open an interactive picker, not print repeated lists into chat.
- Session picker results should be sorted by most recent activity (`updatedAt`, falling back to `createdAt`) so chats with new messages surface above merely newer-created sessions.
- Session switching should happen through the interactive picker. Do not add a `/use` slash command unless explicitly requested.
- Session picker should support:
  - Up/down selection
  - Enter activation
  - Esc close
- `/providers` should open the provider selector modal. The selector offers Use now and Set default actions; Set default sends `provider.use` with `persistDefault: true` so the runtime stores the non-secret default provider in user-global `<config>/settings.json` for future launches across cwd/projects.
- `/keys` should open the API key manager, not require users to put raw API keys in slash-command text.
- API key manager should support:
  - Listing masked keys for each provider, including read-only environment keys such as `env:ZAI_API_KEY`.
  - Up/down selection.
  - Enter activation/switching.
  - `a` to add a key through masked input.
  - `d` to delete stored keychain keys after a confirmation modal, while blocking deletion of env keys.
  - Esc close/cancel.
- API key entry must never render raw secret text. Pasted or typed keys should be displayed only as mask bullets, and tests should cover paste -> save flows, not only programmatic insertion.
- Plan/edit mode maps the UI's permission mode to runtime prompt mode: plan uses ask-before-gated-tools behavior and the `prompts/plan.md` system prompt addendum; edit uses always-allow gated-tool behavior and the `prompts/edit.md` system prompt addendum. Subagent runs use `prompts/subagent.md` inside hidden child sessions. Keep the Markdown files as the editable source for these prompt addenda. In plan mode, `planMd` can use `action: "clarify"` to ask one or more user clarification questions before the final plan; Go renders these in the bottom overlay above the input, always includes a final manual-entry option, and returns all answers to the model as the tool result.
- Cwd blocking is enabled by default for model-callable filesystem/search/shell tools. `/cwd-remove-block` allows subsequent runs in the current TUI session to access paths outside the launch cwd, and `/cwd-enable-block` restores the block. Render this state beside the plan/edit/allow mode below the input text area.
- Assistant responses may be frontend-simulated streamed: the runtime can send a complete `assistant` event, and the Go UI may progressively reveal it. During a running chat, Esc sends `chat.cancel` with the active `runId` so the Node runtime can abort the hyper-router model call; any assistant text already visible in the Go UI is preserved. If the full `assistant` event already arrived and only the frontend reveal is still streaming, Esc stops that reveal while keeping the visible partial text. Keep fake reveal streaming as terminal UX logic; true provider token streaming should be added explicitly to the protocol when needed.
- Agent-done notifications are Go-side UI lifecycle concerns. Fire run-complete notifications only after `run_finished` has arrived and any frontend-simulated assistant streaming has fully drained; do not notify on partial assistant events, aborts, stale run IDs, or session loads. Keep notification delivery behind the `notifier` interface in `notifications.go` so Windows/macOS/Linux implementations can be added without changing streaming lifecycle logic.

### Node Runtime Standards

- Use pnpm for Node dependency management.
- Use plain `tsc` for the Node sidecar; do not use Vite or bundling for runtime/tool code.
- Keep runtime source in TypeScript (`runtime.ts` and future `runtime/**/*.ts` modules).
- Do not edit generated `dist` files directly.
- Go launches the compiled `dist/runtime.js`.
- Keep provider setup, key resolution, storage setup, and tool registration in TypeScript runtime files.
- Run `pnpm run build:runtime` after runtime/tool source changes.
- Native runtime dependencies (`keytar`, `better-sqlite3`) must be allowed in `package.json` `pnpm.onlyBuiltDependencies`; after dependency changes run `pnpm rebuild better-sqlite3` if the native SQLite smoke test cannot locate `better_sqlite3.node`.
- Linux local development needs common build/search tools such as `git`, `ripgrep`, `gcc`, `make`, `pkg-config`, and Secret Service/libsecret development packages for `keytar`.
- Validate native SQLite on the active Node environment with: `node -e "import Database from 'better-sqlite3'; const db = new Database(':memory:'); db.exec('select 1'); db.close(); console.log('ok')"`.
- Support environment-driven provider configuration for automation and fallback:

```powershell
$env:ZAI_API_KEY = "your-zai-key"
$env:GLM_MODEL = "glm-4.6"
$env:GLM_ENDPOINT = "coding" # optional
```

```sh
export ZAI_API_KEY=your-zai-key
export GLM_MODEL=glm-4.6
export GLM_ENDPOINT=coding # optional
```

- Support secure in-app API key management through OS keychain integration (`keytar`).
  - Raw API keys must be stored in the OS keychain, not plaintext `.qubit` JSON files.
  - `<config>\api-key-index.json` may store only non-secret user-global metadata such as provider, alias, active key, source, account name, and timestamps.
  - Runtime protocol events must only expose masked keys and metadata; never send raw key material back to Go.
  - Runtime stdout/stderr and `.qubit\runtime.log` must not contain raw API keys. Redaction should avoid hiding useful non-secret diagnostics such as function names.
  - `keytar` ESM import shape should be handled as `module.default ?? module`; verify `setPassword`, `getPassword`, and `deletePassword` exist.
  - `pnpm` native build approval may be required for `keytar`; keep `package.json`/lockfile in sync and verify a keychain smoke test when changing this area.
  - Environment keys such as `ZAI_API_KEY` should appear as read-only virtual keys and must not be deleted from the UI.
  - Switching the active key should rebuild the provider/runtime state before the next chat run.
- Support stub mode for local development:

```powershell
$env:QUBIT_STUB = "1"
```

```sh
export QUBIT_STUB=1
```

- Keep dev/debug UI details behind explicit environment flags. Do not expose raw internal IDs, fork parent IDs, durations, payload sizes, modal internals, or similar developer metadata in normal UI unless the user explicitly asks for it.
  - `QUBIT_DEV=1` is the broad local developer override for developer-only UI details.
  - Prefer a feature-scoped flag for new debug surfaces when possible, and document it here.
  - Current feature-scoped flags:
    - `QUBIT_MODAL_DEV=1` shows extra permission modal internals.
    - `QUBIT_DEV_TOOL_DETAILS=1` shows tool-call duration/size/details.
    - `QUBIT_DEV_FORK_TREE=1` shows fork-tree session titles, session IDs, and fork parent IDs inside the tree/preview.

- Persist transcripts through hyper-router SQLite storage.
- Maintain listable session metadata in `.qubit/session-index.json` until hyper-router exposes a suitable public session listing API. Session favourites are stored there as `favouritedAt` metadata.
- If adding sql.js or packaging behavior, verify WASM resolution carefully.

## Validation Checklist

Before reporting a Go/runtime change as complete, run the narrowest useful checks.

Recommended commands on Linux:

```sh
pnpm run build:runtime
pnpm run check:runtime
go test ./...
go vet ./...
go build -o bin/qubit .
node -e "import('keytar').then(async mod => { const keytar = mod.default ?? mod; const account = 'qubit-smoke-' + Date.now(); await keytar.setPassword('Qubit Test', account, 'secret'); const got = await keytar.getPassword('Qubit Test', account); await keytar.deletePassword('Qubit Test', account); if (got !== 'secret') throw new Error('keytar round trip failed'); console.log('keytar round trip ok'); })"
```

Recommended commands on Windows:

```powershell
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$files = Get-ChildItem -Recurse -Filter *.go -File | Where-Object { $_.FullName -notmatch '\\(dist|node_modules|bin)\\' } | ForEach-Object { $_.FullName }
gofmt -w $files
go test ./...
go vet ./...
go build -o bin\qubit.exe .
pnpm run build:runtime
pnpm run check:runtime
node -e "import('keytar').then(async mod => { const keytar = mod.default ?? mod; const account = 'qubit-smoke-' + Date.now(); await keytar.setPassword('Qubit Test', account, 'secret'); const got = await keytar.getPassword('Qubit Test', account); await keytar.deletePassword('Qubit Test', account); if (got !== 'secret') throw new Error('keytar round trip failed'); console.log('keytar round trip ok'); })"
```

For manual smoke testing:

```sh
QUBIT_STUB=1 ./bin/qubit
```

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
9. `/keys` opens the API key manager.
10. Adding a key through `a` uses masked input, accepts paste, saves to OS keychain, and returns a masked/listed key without showing the raw secret.
11. Switching/deleting keys updates the key list and active provider metadata; env keys remain read-only.
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
7. Do not mutate generated files, `.qubit` data, or built binaries under `bin/` unless rebuilding is part of the task.
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
