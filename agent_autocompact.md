# Qubit Auto-Compaction Guide

This file is mandatory context when working on Qubit's auto-compaction flow, including context estimation, `/compact`, compaction trigger thresholds, compaction request/response handling, compact-fork transcript shaping, or how compaction state is surfaced in the UI.

## Boundary

Auto-compaction is a cross-cutting runtime/TUI feature.

- Go decides when to trigger compaction before sending a user message.
- The runtime performs the actual compact fork, transcript reduction, and session metadata updates.
- The TUI reflects compacting state, pending input, and the resulting compacted session.

Do not rewrite the source session in place. Compaction should create a new compact fork/session and preserve the original source session history.

## Triggering Rules

- The Go UI checks compaction before sending a normal user message.
- Trigger auto-compaction when estimated context reaches 80% of the selected model max context.
- `/compact` should use the same runtime flow as auto-triggered compaction.
- Skip compaction when there is no active max context, the input is blank, compaction is already in progress, or the current session already compacted as the current source.

## Context Estimation

- Use the current model's max context when available.
- Prefer live provider usage when it is available and relevant to the active run.
- Fall back to a local estimate based on visible messages, reasoning content, and any tool-call context that is preserved for compaction.
- Keep the estimate stable and conservative enough to trigger before hard context limits.

## Compaction Reduction Rules

- Preserve exact edit history for `editFile`, `multiEdit`, and nested `multiCall` editFile operations.
- Reduce `readFile`, `readFiles`, and `readFileContinuation` to file names and ranges only; do not retain full contents in compacted history.
- Keep tool summaries useful but bounded.
- Persist a `>summarised session from ...` marker in the compacted transcript.
- Store compact context in session metadata and use it for future provider calls.

## Provider Injection

- Inject compacted context into future provider calls as `system` for normal providers.
- Inject compacted context as `developer` for the local Codex provider.
- Keep this provider-specific behavior inside the runtime, not the Go UI.

## TUI Expectations

- While compaction is running, the UI should show compacting state and preserve any pending user input.
- When compaction completes with auto-continue enabled, the UI should resume with the pending input in the new compacted session.
- Keep user-visible status terse and avoid exposing raw reduction internals.

## Testing Expectations

When changing compaction behavior, add or update tests that cover:

- The 80% threshold trigger.
- Manual `/compact` invoking the same flow.
- Pending input preservation.
- Auto-continue after compaction.
- Reduced tool history preserving edit operations and trimming read-only tool content.
- Session/branch state after compaction, including source vs compacted session handling.
