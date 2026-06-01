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
  Preserves layered matching, optional backups, validation metadata checks, line hints, and operationMode plan guard; permission mode is ask.

multiEdit
  Applies editFile-style operations sequentially across one or more files.
  Stops on first failure by default and shares editFile path/validation behavior; permission mode is ask.

deleteFile
  Deletes files within the default/supplied workspace.
  Supports optional allowedExtensions, rejects directories/sensitive paths, and permission mode is ask.

todoMd
  Manages Markdown todo lists stored in .qubit/todos under the default/supplied workspace.
  Supports create/list/read/edit actions, auto-generated lowercase dash names, line replacement edits, and permission mode is ask.
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

tools/deleteFile.ts
  File deletion implementation and tool definition.

tools/todoMd.ts
  Markdown todo-list implementation and tool definition.
  Stores todo files in workspace-scoped .qubit/todos without Electron dependencies.

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
```

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

If `cwd` is supplied or defaulted for a filesystem tool:

1. Relative paths resolve against `cwd`.
2. Absolute paths must be inside `cwd`.
3. Windows paths and WSL/POSIX paths are treated as different filesystem styles.
4. Mixed-style workspace escapes must fail unless a future explicit exception is added.

Current policy intentionally does not migrate old managed-path exceptions from the previous harness. Keep behavior simple: stay inside `cwd`.

Do not reintroduce old `YGG_*` concepts. If Qubit later needs managed internal exceptions, design them as Qubit-owned paths such as:

```txt
.qubit/
.qubit/tools/
.qubit/generated/
```

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
  operationMode: "plan" blocks write operations.
  editFile intentionally excludes old LSP integration for now.

todoMd
  Store under .qubit/todos in cwdOrDefault(cwd).
  Use resolveRestrictedToolPath for the storage directory so native Windows and WSL workspaces stay scoped correctly.
  Keep generated names lowercase dash-separated and avoid adding old Electron/Ygg storage assumptions.
```

Defer these until separate decisions are made:

```txt
braveSearch
  Needs Qubit key-storage/config decision.

browseWeb
  Old Electron implementation should not be copied directly into Node sidecar.
```

## Testing Expectations

For runtime/tool source changes, run:

```powershell
pnpm run check:runtime
go test ./...
```

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
- Do not silently widen filesystem access outside `cwd`.
- Do not use raw `process.cwd()` as a tool default; use `cwdOrDefault(...)` so tools operate under the terminal launch directory.
- Keep dependency additions intentional and update `pnpm-lock.yaml` with `pnpm install`.
