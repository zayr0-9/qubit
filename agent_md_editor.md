# Qubit Markdown Editor Agent Guide

This file is mandatory when working on Qubit's `/md-editor` slash command, Markdown document list/editor UI, or runtime Markdown document protocol.

## Purpose

`/md-editor` is a terminal-first Markdown editor for project-local Qubit documents:

```txt
.qubit/plans/*.md
  Project plans, including files managed by planMd.

.qubit/user-docs/*.md
  User-created Markdown documents from the editor.
```

The editor lets users list, open, create, edit, save, and rename Markdown files without leaving Qubit.

## Architecture Boundaries

Keep the standard Qubit split:

- Go CLI owns slash command routing, full-screen list/editor UI, keyboard handling, dirty state, and rendering.
- Node runtime owns filesystem I/O and path validation through JSON-lines protocol messages.
- Do not push terminal UI state into the runtime.
- Do not edit generated `dist/runtime.js` directly; rebuild it with `pnpm run build:runtime` after runtime source changes.

## Raw Editing vs Rendered Preview

The editor source of truth is always raw Markdown text.

Use the chat Markdown renderer only for future read-only preview/display, never as the editable surface. Rendered Markdown loses source syntax and changes line/cell mapping, which makes cursor movement, selection, exact saves, and dirty-state comparison unreliable.

Current MVP is raw-only editing. If adding preview later, keep it as a toggle or split pane that renders `mdEditor.Editor.Value()` without replacing the raw buffer.

## Runtime Protocol

Known Markdown editor request/event types:

```txt
md.list
md.read
md.create
md.save
md.rename
md.created
md.saved
md.renamed
```

Requests:

- `md.list`: list `.qubit/plans/*.md` and `.qubit/user-docs/*.md`.
- `md.read`: read one allowed Markdown file by `path` or safe `{section,name}`.
- `md.create`: auto-create an empty user doc under `.qubit/user-docs` and return it opened.
- `md.save`: save raw UTF-8 content to an allowed `.md` path.
- `md.rename`: rename within the same allowed section.

Events:

- `md.list`: includes `files []mdFileInfo`.
- `md.read`: includes `file *mdFileInfo` and raw `content`.
- `md.created`: includes `file`, raw `content`, and optional `status`.
- `md.saved`: includes `file`, raw `content`, and optional `status`.
- `md.renamed`: includes `file`, previous `path`, and optional `status`.

Runtime path safety rules:

- Only `.md` files are allowed.
- Only files inside project `.qubit/plans` or `.qubit/user-docs` are allowed.
- UI-sent paths are not trusted; runtime must resolve and validate paths before reading/writing/renaming.
- Rename must stay in the current section and reject collisions.

## Go UI State and Behavior

Markdown editor UI lives in `internal/tui/md_editor.go` and uses `modeMdEditor` plus `mdEditorState`.

Editor subviews:

```txt
list
edit
rename
discard-confirm
```

List view:

- `Up/Down`, `k/j`: move selection.
- `Enter`: open selected file through `md.read`.
- `n`: auto-create user doc through `md.create`.
- `Esc`: close to chat.

Edit view:

- Uses a dedicated `composerModel` as the raw Markdown buffer.
- `Ctrl+S`: save through `md.save`.
- `Ctrl+R`: rename current file.
- `Esc`: return to list if clean; open discard confirmation if dirty.
- Newline keys (`Enter`, `Ctrl+J`, enhanced Shift+Enter/Alt+Enter) insert literal newlines.
- `Ctrl+C` copies selected editor text before quitting.

Rename view:

- `Ctrl+R` opens it from edit view.
- `Enter`: sends `md.rename`.
- `Esc`: cancels back to edit view.

Dirty discard confirmation:

- Default action is Cancel.
- Discard resets the editor buffer to `OriginalContent`, clears dirty state, and returns to list.

## Composer Height and Scrolling

Full-screen Markdown editing reuses `composerModel`, but it must be configured with the editor viewport height before keyboard navigation.

Always call `layoutMdEditorComposer()` before passing edit-view keypresses to `mdEditor.Editor.UpdateKey`. Otherwise `composerModel` may still think its height is the default/small composer height and will scroll prematurely when pressing Up/Down, even while the cursor is still visibly inside the editor box.

Rendering should use a copied editor model when setting render-only width/height, so `View()` does not mutate persisted editor scroll state.

## Design Rules

Follow `agent_design.md`:

- No heavy boxes, opaque panels, or decorative backgrounds.
- Use simple foreground styling, muted hints, and existing selection marker patterns.
- Use `visibleListWindow` for list paging.
- Keep footer hints mode-specific and terse.

## Testing Expectations

When changing `/md-editor`, add or update Go tests for the full model/protocol boundary:

- Slash command opens `modeMdEditor` and sends `md.list`.
- List renders both `plans` and `user-docs` entries.
- Enter sends `md.read` for selected file.
- `md.read` opens raw Markdown content and starts clean.
- Typing marks dirty.
- `Ctrl+S` sends exact raw `content` to `md.save`.
- `md.saved` clears dirty and updates `OriginalContent`.
- `n` sends `md.create`; `md.created` opens a user doc.
- `Ctrl+R` / `md.rename` updates current file metadata.
- Dirty `Esc` opens discard confirmation.
- Regression: pressing Up from the bottom visible editor line must not scroll until the cursor crosses the top edge.

Recommended validation:

```powershell
gofmt -w internal/tui/md_editor.go internal/tui/md_editor_test.go internal/tui/md_editor_state.go internal/tui/commands.go internal/tui/app.go internal/tui/view.go internal/tui/transcript_selection.go internal/tui/protocol/types.go
go test ./...
go vet ./...
go build -o bin\qubit.exe .
pnpm run build:runtime
pnpm run check:runtime
```
