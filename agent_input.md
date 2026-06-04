# Qubit Input Agent Guide

This file is mandatory when working on Qubit's Go TUI input handling: keyboard routing, composer behavior, paste/clipboard flows, mouse capture, mouse wheel routing, transcript selection, or terminal keyboard setup.

## Scope

Input behavior belongs in the Go Bubble Tea v2 CLI. Do not push keyboard, paste, mouse, selection, or terminal-input UX into the Node runtime unless a JSON protocol addition is explicitly required.

Primary implementation locations:

```txt
internal/tui/app.go                         Top-level Bubble Tea message dispatch
internal/tui/input_update.go                Keyboard routing, composer paste/newline/submit flow
internal/tui/input_view.go                  Composer rendering composition
internal/tui/composer.go                    TUI adapter around custom composer component
internal/tui/components/composer/           Custom multiline composer component
internal/tui/transcript_selection.go        Chat transcript mouse selection/copy behavior
internal/tui/transcript_selection_state.go  Transcript selection state
internal/tui/file_mentions.go               @file mention keyboard/paste interactions
internal/tui/terminal_setup.go              Windows Terminal keyboard enhancement setup
internal/tui/keys.go                        API key entry/paste behavior
internal/tui/md_editor.go                   Markdown editor text input/paste behavior
internal/tui/theme.go                       Theme entry text input/paste behavior
internal/tui/plan_clarification.go          Plan clarification manual input/paste behavior
```

## Composer Standards

- The chat composer uses Qubit's custom `composerModel` adapter in `internal/tui/composer.go`, backed by `internal/tui/components/composer`, not `textarea.Model` or `textinput.Model`.
- Preserve multiline input, source-index-based cursor/selection state, internal wrapping, max-height scrolling, and inline selection rendering.
- Do not post-process rendered ANSI text to implement composer selection/highlighting. Selection must be tracked before rendering with source indices so the chevron/prompt is never highlighted and selection stays behind input characters only.
- The composer prompt/chevron renders only on the first visible input row. Wrapped/continuation rows should use prompt-width spaces for alignment, not repeated chevrons.
- Recalculate layout after composer changes so the input area can grow/shrink up to its max height and the chat viewport height adjusts.
- When composer content exceeds max height, scroll inside the composer instead of expanding the app layout past the footer.

## Keyboard Behavior

- Keep `Update` as a dispatcher; move input behavior to focused helpers such as `input_update.go` or feature-specific update files.
- Plain Enter sends the current chat message.
- Modified Enter inserts a literal newline and must not submit:
  - `Shift+Enter` when terminal keyboard enhancements report it.
  - `Alt+Enter` when the terminal does not reserve it.
  - `Ctrl+J` is the reliable terminal fallback for inserting a newline.
- Composer keyboard behavior should remain editor-like:
  - Arrows/Home/End move in the composer.
  - `Ctrl+Left/Right` and Alt fallbacks move by word.
  - `Ctrl+Home/End` move to input begin/end.
  - `Ctrl+A` selects all.
  - `Shift+Arrow` selects when the terminal reports those keys.
  - `Ctrl+Shift+Left/Right` selects by word when supported.
  - `Ctrl+C` copies selected composer text and quits only when no composer selection exists.
  - `Esc` clears composer selection before quitting/cancelling broader state.
- `/terminal-setup` should patch Windows Terminal `settings.json` to map Shift+Enter to an enhanced keyboard escape sequence. It must be idempotent, create a timestamped backup before writing, remove the common misplaced top-level `command`/`keys` mistake, and report the settings/backup paths. It must not change non-Windows Terminal files.

## Paste and Clipboard Routing

- Bubble Tea terminal paste (`tea.PasteMsg`) and explicit clipboard paste (`composerPasteMsg` from `Ctrl+V`) are separate paths. Test both when changing paste behavior.
- Top-level paste routing should follow the focused input surface, in this order:
  1. plan clarification manual input, only when active/manual selected
  2. `/md-editor` active text target
  3. `/theme` entry
  4. `/keys` entry
  5. default chat composer when in chat mode
- Do not paste into hidden chat composer while a picker, modal, fork tree, or other non-text surface is focused.
- Pasted multiline chat input should preserve line breaks. Pasted or typed fenced Markdown code blocks must retain line breaks and render as Markdown after send.
- Pasting while a composer selection exists should replace only the selected text and clear the selection.
- After chat composer paste or text edits, clear input-history navigation state, refresh file mention palette bounds if visible, and call `m.layout()`.
- API key entry must never render raw secret text. Pasted or typed keys should display only as mask bullets, and tests should cover paste -> save flows, not only programmatic insertion.

## Mouse and Scrolling Behavior

Qubit currently uses Bubble Tea alt-screen plus cell-motion mouse capture so wheel scrolling stays app-contained and transcript selection/link hitboxes can be owned by the app:

```go
view.AltScreen = true
view.MouseMode = tea.MouseModeCellMotion
```

- `MouseModeCellMotion` is the accepted tradeoff: it enables click/release/wheel plus drag events, but common terminals will not support normal drag-to-select text while the app captures mouse events.
- Do not upgrade to `MouseModeAllMotion` unless passive movement events are explicitly needed.
- If a future change disables mouse capture to restore native terminal selection, it must also replace contained wheel scrolling and app-owned transcript selection/link behavior.
- Mouse wheel handling should route to the visible interactive surface:
  - In chat mode, wheel up disables auto-scroll.
  - In chat mode, wheel down resumes auto-scroll only when the chat viewport reaches bottom.
  - `PgUp`/`PgDn` should keep matching chat wheel behavior.
  - In picker/modal/list modes, wheel should move the visible list cursor or list viewport instead of scrolling hidden chat content.
- If chat scroll appears to work only after pressing `PgUp`/`PgDn`, inspect whether viewport auto-scroll/layout refresh is forcing `GotoBottom()`.
- Preserve viewport offset after generic viewport updates, content refreshes, and resize/layout. Only resume auto-scroll when the user submits a new message or explicitly reaches/jumps to the bottom.

## Transcript Selection and Links

- Qubit owns chat transcript mouse selection while mouse capture is enabled.
- Drag in the chat viewport to select rendered transcript text.
- Use wheel/edge-drag to keep scrolling while selecting.
- `Ctrl+C` copies composer selection first, then transcript selection when no composer selection exists.
- `Esc` clears transcript selection.
- Composer selection remains separate and has copy priority.
- Selection drag updates must repaint from cached rendered transcript content instead of running the full Markdown/render-cache refresh path on every mouse motion.
- Terminal-native modifier overrides such as Shift-drag may still work in some terminals, but they are a fallback rather than the primary selection path.
- Qubit owns transcript link opening through app-generated hitboxes over rendered transcript text, not terminal-native URL detection.
- Build link hitboxes from ANSI-stripped rendered lines and display-cell coordinates so Unicode prefixes align correctly.
- Open links only on Ctrl+left-click release when no drag occurred. Plain clicks continue to feed transcript selection/tool/reasoning toggles, and Ctrl+drag must remain text selection rather than browser launch.
- Footer/help copy should say Ctrl+click opens links only when the terminal forwards the mouse event, because some terminals intercept Ctrl+click URLs before Bubble Tea receives them.

## Performance Rules

- Avoid full layout or full Markdown re-rendering on every generic viewport update or stream tick.
- Cache rendered historical message content by role/content/width, and do not cache the actively streaming partial message.
- Mouse selection drag updates should use cached rendered transcript content rather than full render-cache refresh.

## Testing Expectations

- New input, paste, mouse, or selection regressions must include focused Go tests where practical.
- Prefer model/update tests that exercise the actual message path, for example `tea.PasteMsg` through `m.Update`, keypress through `m.updateKey`, or mouse message through routed mouse handlers.
- For clipboard paste, test the app-level `composerPasteMsg` path separately from terminal bracketed paste.
- Manual smoke checks are still useful for terminal-dependent behavior such as Shift+Enter reporting, mouse capture, drag selection, and clipboard integration, but they should not be the only coverage for core routing logic.
