# Qubit Design Agent Guide

This file is mandatory when working on Qubit's terminal UI design, visual styling, layout, colors, borders, spacing, or rendering behavior.

## Terminal Design Principles

Qubit's terminal interface should stay clean, minimal, and calm. Prefer readable text, whitespace, and small foreground-color accents over decorative containers.

## Visual Style Rules

- Keep the terminal UI minimal by default.
- Do not add boxes, panels, heavy borders, cards, framed sections, or decorative containers unless explicitly requested.
- Do not add background colors unless explicitly requested for a specific UI element or state. Inline edit diffs should use red/green foreground text for removed/added lines, not colored row backgrounds.
- Prefer foreground text color, bold text, dim text, symbols, and spacing for visual hierarchy.
- Use color sparingly and consistently:
  - Accent color for active markers, prompts, titles, and important actions.
  - Cyan for command names, assistant identity, links, or secondary highlights.
  - Muted color for hints, metadata, shortcuts, and descriptions.
  - Red only for errors or destructive actions.
  - Green only for success or enabled/ready states.
- Reasoning blocks use the theme-provided `Reasoning` foreground color and a compact sparkle marker (`✦`) rather than the assistant/message colors.
- Tool-call rows use theme-provided foreground colors by category (`ToolRead`, `ToolSearch`, `ToolWrite`, `ToolShell`, `ToolOther`) and communicate running/completed/failed state through colored symbols rather than status words. Expanded `multiCall` rows should show each nested tool invocation as a compact individual detail row.
- Avoid full-row selection highlights. Prefer a small colored marker such as `›`, `•`, or an accent-colored label.
- Avoid opaque full-width styles around chat, command palettes, pickers, and status areas unless a feature specifically requires it.
- Preserve terminal transparency where possible. Do not paint large rectangular areas behind content.
- User-selected themes are the explicit exception to the default no-background preference: `/theme` may apply a background/text palette across the app, while new UI features should still avoid extra decorative backgrounds unless requested.
- The selected `/theme` palette persists in user-global `<config>/theme.json` and should be loaded during model initialization before styles and spinner colors are created.

## Layout Rules

- Prefer simple vertical lists and aligned text over boxed widgets.
- Use blank lines and indentation instead of borders for grouping.
- Keep command palettes, pickers, modals, and status messages compact.
- Do not add visual chrome that reduces available chat space without a clear usability benefit.
- Long text should truncate or wrap according to existing project patterns without creating large blocks of colored background.

## Implementation Guidance

- In Lip Gloss styles, avoid `.Background(...)`, `.Border(...)`, and box-like padding unless the task explicitly asks for those visuals.
- Before adding a background or border, confirm it is necessary and that the user requested it.
- For selected rows, style only the marker and/or foreground text. Do not apply a background to the whole row.
- When changing visual design, run `gofmt` and relevant Go tests.
- If a design change affects durable UI conventions, update this file and `agent.md` if needed.

## Context Status

- The compact input status row may show approximate context usage next to mode/reasoning (for example `plan · medium · ctx 2.1k/400k`). Keep it terse and foreground-only.
- Context usage is currently an MVP estimate using 1 token = 4 characters and includes visible chat messages, surfaced reasoning blocks, and tool-call full-context character counts when available. Tool-call preview truncation is visual only and should not reduce the status estimate. When Codex usage is available from the latest run log, append it tersely beside `ctx` (for example `ctx 2.1k/400k log in 12k/cache 8k/out 900`).

## Transcript Mouse Links

- Qubit owns transcript link opening through app-generated hitboxes over rendered transcript text, not terminal-native URL detection. Build hitboxes from ANSI-stripped rendered lines and display-cell coordinates so Unicode prefixes align correctly.
- Open links only on Ctrl+left-click release when no drag occurred. Plain clicks continue to feed transcript selection/tool/reasoning toggles, and Ctrl+drag must remain text selection rather than browser launch.
- Footer/help copy should say Ctrl+click opens links only when the terminal forwards the mouse event, because some terminals intercept Ctrl+click URLs before Bubble Tea receives them.
