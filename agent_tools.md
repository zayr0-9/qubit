# Qubit Tool Context

This file follows Qubit's `agent_<category>.md` context-file naming scheme.

Reading this file is mandatory before planning or changing Qubit's model-callable tools, tool registration, runtime tool permissions, or shared filesystem/path infrastructure.

Update this file whenever tool-related changes affect durable standards, architecture, workflows, file locations, behavior, migration order, testing expectations, or pitfalls.

## Current Tool Surface

Qubit currently registers migrated tools from `tools/index.ts`:

```txt
tools/index.ts
  Exports qubitTools for runtime registration.

runtime.ts
  Imports qubitTools and passes them to defineAgent({ tools: qubitTools }).
```

Registered tools:

```txt
readFile
  Reads one text/code/config file.
  Supports maxBytes, startLine/endLine, ranges, includeHash, metadata, binary detection, and cwd workspace restriction.

readFileContinuation
  Reads a line-based continuation chunk after a given line number.
  Implemented in tools/readFile.ts as companion functionality for readFile.

readFiles
  Reads multiple files concurrently using readFile.
  Returns per-file results and a combined content string with relative filename headers.

glob
  Finds files using glob patterns from a resolved cwd.
  Supports ignore patterns, dotfiles, absolute/mark/nodir/follow/realpath/stat options, timeout, and match limits.

ripgrep
  Searches files using rg.
  Uses native Windows rg for Windows paths when available and WSL rg for WSL/POSIX paths on Windows.
  Supports line numbers, count/files-with-matches modes, globs, hidden/no-ignore flags, context, timeout, and output limits.

bash
  Runs Bash commands.
  On Windows, requires a WSL/Linux cwd and runs through WSL; it intentionally does not run PowerShell for native Windows cwd paths.
  Permission mode is ask.

powershell
  Runs PowerShell commands.
  On Windows, WSL/Linux cwd paths are converted to UNC for native PowerShell; on non-Windows it uses pwsh.
  Permission mode is ask.

createFile
  Creates or overwrites text files within the default/supplied workspace.
  Uses shared path resolution and cwd containment; permission mode is ask.

editFile
  Edits files with replace, replace_first, or append operations.
  Preserves layered matching, optional backups, validation metadata checks, line hints, and operationMode plan guard. Fuzzy matching is disabled by default because broad fuzzy replacements can corrupt brace-sensitive code; callers must explicitly set `enableFuzzyMatching: true` to use it. In plan mode, editFile may only modify files inside the project `.qubit/plans` directory; all other paths are blocked by the tool. Permission mode is ask, but the runtime/Go permission bridge auto-allows plan-scoped `.qubit/plans` editFile permission requests in plan mode.

multiEdit
  Applies editFile-style operations sequentially across one or more files.
  Stops on first failure by default and shares editFile path/validation behavior; permission mode is ask.

multiCall
  Executes multiple registered Qubit tool calls sequentially in one tool invocation using simple items shaped as { tool, args }.
  Stops on first failure by default unless stopOnError=false.
  The outer tool is always-allowed so read-only chains do not prompt; nested ask/never tools are checked individually through the existing permission dialog path, so edit mode auto-allows and plan mode asks for non-whitelisted tools.

deleteFile
  Deletes files within the default/supplied workspace.
  Supports optional allowedExtensions, rejects directories/sensitive paths, and permission mode is ask.

todoMd
  Manages Markdown todo lists stored in the project .qubit/todos directory under the default/supplied workspace.
  Supports create/list/read/edit actions, optional named create IDs, auto-generated lowercase dash names when create omits a name, single or batched line replacement edits, and permission mode is ask.

planMd
  Manages Markdown plans stored in the project .qubit/plans directory.
  Supports create/list/read/edit/display/clarify actions and is always-allowed so planning can create/update/display plans or ask clarifying questions without opening a permission modal. The clarify action emits a plan.clarification.request event so Go can collect one or more user answers in the bottom overlay above the input and return them as the tool result. The display action emits a UI-only plan.view event so Go can render the selected Markdown plan in chat; Qubit's runtime replaces the tool result sent back to the model with a short acknowledgement so the displayed plan Markdown is not sent to the model as tool-call output. On transcript reload, stored planMd display tool calls are hydrated back into UI-only plan preview messages from the current plan file so displayed plans remain visible after restarting Qubit. In hidden subagent runs, display/clarify must not emit UI events or wait for user input; clarify returns a safe cancelled result.

subagent
  Delegates one or more tasks to hidden persisted Qubit child sessions. Supports parallel and linear execution, uses `prompts/subagent.md`, and permission mode ask for the parent tool call. Once the parent call is allowed, hidden child runs auto-borrow gated tool permission while preserving never-deny tools. Child run internals are not rendered in Go; the parent sees only the subagent tool lifecycle and result summary.
```

## Important Files

```txt
tools/index.ts
  Tool registry exported to runtime.ts.

tools/readFile.ts
  readFile + readFileContinuation implementation and tool definitions.

tools/readFiles.ts
  Multi-file reader built on readTextFile.

tools/glob.ts
  Glob search implementation and tool definition.

tools/ripgrep.ts
  Ripgrep implementation and tool definition.

tools/bash.ts
  Bash command implementation and tool definition.

tools/powershell.ts
  PowerShell command implementation and tool definition.

tools/createFile.ts
  File creation implementation and tool definition.

tools/editFile.ts
  editFile + multiEdit implementation and tool definitions.
  LSP integration from the old harness is intentionally deferred/removed.

tools/multiCall.ts
  Sequential tool-chain implementation and tool definition.
  Wraps registered Qubit tools and checks nested gated tools through the runtime permission requester.

tools/deleteFile.ts
  File deletion implementation and tool definition.

tools/todoMd.ts
  Markdown todo-list implementation and tool definition.
  Stores todo files in project .qubit/todos without Electron dependencies.

tools/planMd.ts
  planMd
    Markdown plan implementation and tool definition.
    Stores plan files in project .qubit/plans, can emit UI-only plan.view events for displayed plans, and can request plan-mode clarifications through plan.clarification.request/response.

tools/subagent.ts
  subagent
    Tool definition and validation for delegated hidden subagent runs. Runtime installs the executor from `runtime.ts` with `setSubagentExecutor(...)`.
utils/qubitProject.ts
  Resolves Qubit-owned internal paths under the active project .qubit directory for project-scoped stores that should not be blocked by normal cwd file-tool containment.

tools/shellShared.ts
  Shared shell output limiting, timeout, stderr filtering, and process-tree cleanup helpers.

utils/toolWorkspace.ts
  Stores the default tool cwd captured from the terminal directory where Qubit was launched.

utils/wslBridge.ts
  Cross-platform Windows/WSL helper functions.
  Detects path formats, converts Windows paths to WSL-style paths, converts WSL/Linux paths to Windows UNC paths, and discovers the default WSL distro.

utils/pathSafety.ts
  Central path resolver and workspace safety checks.
  Produces display/fs/comparison paths and enforces cwd containment.

tools_to_migrate/
  Source/reference copies from the previous harness.
  Do not expose these files directly as Qubit tools.
```

## Path Policy

All filesystem tools should use shared path utilities instead of ad-hoc path logic.

Qubit must support these path styles from the start:

```txt
Native Windows cwd/path:
  D:\qubit
  D:/qubit
  C:\Users\name\project\file.ts

WSL/Linux paths while Qubit runs on Windows:
  /home/name/project/file.ts
  /mnt/d/qubit/file.ts

UNC WSL paths:
  \\wsl$\Ubuntu\home\name\project

Relative paths:
  src/foo.ts

Relative cwd values:
  cwd = .
  cwd = ./src
```

Relative tool cwd values are resolved against the launch/default workspace cwd, not filesystem root or the Node runtime process cwd. For example, if Qubit was launched in `/home/user/project`, `cwd: "."` means `/home/user/project` and `cwd: "./src"` means `/home/user/project/src` while still remaining subject to cwd containment.

Path resolver expectations:

```ts
resolveToolPath(inputPath, { cwd, mode })
```

The resolved shape should include:

```ts
{
  inputPath,
  displayPath,
  fsPath,
  pathType,
  comparisonPath,
  comparisonKind,
}
```

Meanings:

- `displayPath`: path to show in results/errors.
- `fsPath`: path Node filesystem APIs can use.
- `comparisonPath`: normalized path used for containment checks.
- `comparisonKind`: `win32` or `posix`; never compare Windows and WSL/POSIX paths with the same path module.

On Windows, WSL/Linux absolute paths must be converted to UNC before Node filesystem APIs read or write them.

## Default Tool Workspace

Qubit captures the terminal directory where the Go CLI was opened before launching the Node runtime. The Go sidecar launcher passes that path as `QUBIT_WORKSPACE_CWD`, and `runtime.ts` initializes `utils/toolWorkspace.ts` with it.

Tool implementations should default omitted `cwd` values to `cwdOrDefault(...)`, not raw `process.cwd()`. This keeps model-callable tools scoped to the user's launch directory even though the Node runtime process itself runs from Qubit's app root so it can load `dist/runtime.js` and `.qubit` data.

`ready` events include `workspaceCwd` for UI/status/debug visibility.

## Workspace Safety

If cwd blocking is enabled, and `cwd` is supplied or defaulted for a filesystem tool:

1. Relative paths resolve against `cwd`.
2. Absolute paths must be inside the launch workspace cwd.
3. A supplied tool `cwd` or search/shell root must also be inside the launch workspace cwd.
4. Windows paths and WSL/POSIX paths are treated as different filesystem styles.
5. Mixed-style workspace escapes must fail unless cwd blocking is explicitly disabled.

Cwd blocking is enabled by default. `/cwd-remove-block` disables this containment gate for subsequent runs in the current TUI session, allowing tools to access absolute paths outside the launch cwd such as `D:\\Hyper-router` while still respecting normal tool permission prompts. `/cwd-enable-block` restores the default containment gate. The status below the input renders the current state next to the permission mode as `cwd block` or `cwd open`.

Current policy intentionally does not migrate old managed-path exceptions from the previous harness. Default behavior stays inside the launch cwd unless the user explicitly runs `/cwd-remove-block`.

Do not reintroduce old `YGG_*` concepts. Qubit-owned project storage is the hidden `.qubit` directory under the terminal launch cwd. Internal project stores such as `.qubit/todos` and `.qubit/plans` use `utils/qubitProject.ts` instead of the normal file-tool cwd gate, while ordinary read/edit/delete tools remain restricted to the workspace.

```txt
.qubit/
.qubit/todos/
.qubit/plans/
.qubit/tools/
.qubit/generated/
```

## Runtime Permission Modes

Qubit currently supports three user-facing permission modes for gated tools:

```txt
ask
  The Go client opens the permission modal when the runtime emits tool.permission.request, except planMd and subagent are always-allowed for planning workflows and editFile is auto-allowed only for plan-mode requests that the runtime has verified are restricted to project `.qubit/plans` files.

always_allow
  The Go client immediately sends tool.permission.response with allow=true for runtime permission requests and uses the edit prompt.

allow_all
  The Go client immediately sends tool.permission.response with allow=true for runtime permission requests while keeping the plan prompt. Choosing Allow all in the permission modal enables this for the rest of the current TUI session.
```

The permission mode is Go UI/session state. Keep the runtime/tool definitions responsible for declaring static tool safety (`permission: { mode: 'ask' }` for gated tools and `permission: { mode: 'always' }` for intrinsically safe read/search/planning tools), but keep user-facing auto-approval gating in the Go client. Do not conflate tool-level `always` with user-level `always_allow` or `allow_all`.

For now, permission mode is changed with `/permission <plan|edit|allow-all>` or cycled in the chat UI with Shift+Tab. The current mode is rendered as a minimal bright `plan` / `edit` / `allow all` label in a dedicated status section below the input area and is not persisted across process restarts. The same status section also shows cwd containment state as `cwd block` or `cwd open`; `/cwd-remove-block` and `/cwd-enable-block` toggle that per-TUI-session setting. Plan mode maps to ask-before-gated-tools behavior with planMd and subagent allowed and editFile auto-allowed only after runtime path validation confirms the target is inside project `.qubit/plans`; edit mode maps to always-allow gated tools plus the edit prompt; allow-all mode maps to always-allow gated tools while retaining the plan prompt. If persistence is added later, document the settings file and migration behavior here.

## Tool Definition Standards

Use Hyper Router tool definitions for model-callable tools:

```ts
import { defineTool } from '@hyper-labs/hyper-router'
```

Tool implementations should return Hyper Router `ToolResult` objects:

```ts
{ ok: true, data: ... }
{ ok: false, error: '...' }
```

Guidelines:

- Keep read-only tools permission mode `always` unless a reason emerges to ask.
- Write/shell/network tools should be permission-gated when migrated.
- Keep user-facing permission-mode behavior in Go: ask mode shows the modal except always-allowed tools such as planMd and subagent; always-allow and allow-all modes auto-approve runtime permission requests.
- Keep input schemas explicit and JSON-serializable.
- Validate required arguments before doing filesystem work.
- Catch implementation errors inside `execute` and return `{ ok: false, error }`.
- Keep reusable logic exported separately from tool definitions so tests and future tools can call it directly.

## Migrating More Tools

Batch 4 migrated:

1. `todoMd`

`todoMd` now stores todo Markdown files under the workspace-scoped `.qubit/todos` directory and does not depend on Electron, old Ygg app storage, or `unique-names-generator`.

Expected path behavior retained for migrated shell/search/write tools:

```txt
ripgrep
  Prefer native Windows rg.exe for Windows paths.
  Use WSL rg for Linux paths.

bash
  On Windows + Linux cwd: run through WSL.
  On non-Windows: run native bash.
  Clarify semantics before making bash run PowerShell for Windows paths.

powershell
  On Windows + WSL cwd: convert cwd to UNC.
  On non-Windows: use pwsh.

createFile/editFile/multiEdit/deleteFile
  Use resolveRestrictedToolPath and the same cwd containment rule.
  Writes to WSL paths use UNC conversion when Node filesystem APIs are used.
  Write tools are permission-gated with ask.
  operationMode: "plan" blocks write operations for createFile, multiEdit, and deleteFile. editFile has the only plan-mode write exception: it may patch existing Markdown plan files under project `.qubit/plans` and blocks every other path, including traversal attempts.
  editFile intentionally excludes old LSP integration for now.

todoMd
  Store under .qubit/todos in cwdOrDefault(cwd).
  Use resolveRestrictedToolPath for the storage directory so native Windows and WSL workspaces stay scoped correctly.
  Keep todo IDs lowercase dash-separated and avoid adding old Electron/Ygg storage assumptions. Create accepts an optional name; if omitted, it generates a random ID that must be used for later read/edit calls. Named create must not overwrite an existing todo file.
  Edit action accepts legacy search/replacement for one line replacement, or an edits array for multiple replacements in one tool call. Batched edits are all-or-nothing: if any search misses, no changes are written.
```

Defer these until separate decisions are made:

```txt
braveSearch
  Needs Qubit key-storage/config decision.

browseWeb
  Old Electron implementation should not be copied directly into Node sidecar.
```

Codex-hosted `web_search` and `image_generation` are not Qubit local tools and are not part of this migration list. They are added only inside `runtime/codex/` request construction as OpenAI Responses hosted tools, execute server-side, and do not use Qubit's local permission lifecycle. Generated image payloads from Codex are saved under `.qubit/generated` and represented in chat by saved file paths.

## Tool Call UI Lifecycle

Qubit bridges Hyper Router tool lifecycle hooks into the JSON-lines protocol:

```txt
tool.call.start
  Emitted after permission is allowed and immediately before a registered tool executes.
  Includes sessionId, step, toolCallId, toolName, status=running, summarized args, and startedAt.

tool.call.finish
  Emitted when a tool reaches a terminal outcome.
  Status can be completed, failed, denied, or unknown_tool.
  Includes summarized args/result plus timing fields when available.
```

Runtime summary helpers must keep these events user-relevant and bounded. Do not send unbounded file contents, stdout/stderr, edit replacements, or raw tool payloads over stdout. Summaries should include paths, commands, counts, success/error messages, and capped previews with obvious secrets redacted.

The Go UI groups tool calls by `(step, toolName)` and renders them as expandable tool rows in the chat transcript. Collapsed rows show compact summaries such as `Read 2 files`, `Searched 3 times · 12 matches`, or `Ran 2 subagents`; clicking a row expands per-call details. When expanding a `multiCall` row, the details should list each nested tool invocation as its own row using the summarized nested args/results, not just the outer multiCall wrapper payload. When expanding a `subagent` row, details show per-task status and bounded response/error previews while hiding hidden session/run IDs unless developer details are enabled. `session.messages` reconstructs persisted tool groups from Hyper Router assistant `toolCalls` plus matching `role: "tool"` messages so session reloads preserve tool-call UI rows instead of dropping tool activity.

## Testing Expectations

For runtime/tool source changes, run:

```powershell
pnpm run check:runtime
go test ./...
```

When changing `multiCall`, test read-only chains, gated nested tools in both plan and edit permission modes, stopOnError behavior, and unknown/nested multiCall rejection. When changing `subagent`, test argument validation, executor injection, hidden-run permission borrowing/suppression, linear stop-on-error, parallel all-results behavior, and Go tool-row summaries.

When changing path handling, also test representative cases where available:

```txt
cwd = D:\qubit, path = agent.md
path = D:\qubit\agent.md
cwd = /mnt/d/qubit, path = agent.md
cwd = /home/<user>/some-project, path = file.txt
cwd = \\wsl$\Ubuntu\home\<user>\some-project
cwd = D:\qubit, path = ..\some-other-file  # should fail
cwd = D:\qubit, path = /home/user/file.txt # should fail as mixed/escaped workspace
```

If WSL is unavailable on the development machine, document that WSL smoke cases were not run.

## Common Pitfalls

- Do not edit generated `dist/` files directly.
- Do not compare Windows paths with `path.posix`, or WSL/POSIX paths with `path.win32`.
- Do not use Node filesystem APIs on raw `/home/...` paths while running on Windows; convert to UNC first.
- Do not migrate `tools_to_migrate/managedToolPaths.ts` verbatim; it contains old harness-specific assumptions.
- Do not expose support modules like `wslBridge` as model-callable tools.
- Do not let `multiCall` bypass nested tool permissions; read-only nested tools may run directly, but gated nested tools must route through the existing permission dialog/auto-allow path.
- Do not silently widen filesystem access outside `cwd`.
- Do not use raw `process.cwd()` as a tool default; use `cwdOrDefault(...)` so tools operate under the terminal launch directory.
- Keep dependency additions intentional and update `pnpm-lock.yaml` with `pnpm install`.
