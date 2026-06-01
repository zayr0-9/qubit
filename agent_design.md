# Qubit Design Agent Guide

This file is mandatory when working on Qubit's terminal UI design, visual styling, layout, colors, borders, spacing, or rendering behavior.

## Terminal Design Principles

Qubit's terminal interface should stay clean, minimal, and calm. Prefer readable text, whitespace, and small foreground-color accents over decorative containers.

## Visual Style Rules

- Keep the terminal UI minimal by default.
- Do not add boxes, panels, heavy borders, cards, framed sections, or decorative containers unless explicitly requested.
- Do not add background colors unless explicitly requested for a specific UI element or state.
- Prefer foreground text color, bold text, dim text, symbols, and spacing for visual hierarchy.
- Use color sparingly and consistently:
  - Accent color for active markers, prompts, titles, and important actions.
  - Cyan for command names, assistant identity, links, or secondary highlights.
  - Muted color for hints, metadata, shortcuts, and descriptions.
  - Red only for errors or destructive actions.
  - Green only for success or enabled/ready states.
- Avoid full-row selection highlights. Prefer a small colored marker such as `›`, `•`, or an accent-colored label.
- Avoid opaque full-width styles around chat, command palettes, pickers, and status areas unless a feature specifically requires it.
- Preserve terminal transparency where possible. Do not paint large rectangular areas behind content.
- User-selected themes are the explicit exception to the default no-background preference: `/theme` may apply a background/text palette across the app, while new UI features should still avoid extra decorative backgrounds unless requested.
- The selected `/theme` palette persists in `.qubit/theme.json` and should be loaded during model initialization before styles and spinner colors are created.

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
