import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { buildCompactionInput, compactSummaryWithPreservedEdits, compactionProviderMessage, compactionSummaryFromMetadata } from "./compaction.js";

describe("compaction helpers", () => {
  it("reduces readFile/readFiles contents to filenames and preserves edit history", () => {
    const messages: any[] = [
      { role: "user", content: "please edit", date: new Date() },
      {
        role: "assistant",
        content: "",
        date: new Date(),
        toolCalls: [
          { id: "read-1", toolName: "readFile", args: { path: "agent.md", cwd: "." } },
          { id: "edit-1", toolName: "editFile", args: { path: "runtime.ts", operation: "append", content: "secret large replacement" } },
          { id: "bash-1", toolName: "bash", args: { command: "cat secret" } },
        ],
      },
      { role: "tool", name: "readFile", toolCallId: "read-1", content: JSON.stringify({ ok: true, data: { content: "VERY LARGE FILE CONTENT", totalLines: 10 } }), date: new Date() },
      { role: "tool", name: "editFile", toolCallId: "edit-1", content: JSON.stringify({ ok: true, data: { success: true, replacements: 1, message: "ok" } }), date: new Date() },
      { role: "tool", name: "bash", toolCallId: "bash-1", content: JSON.stringify({ ok: true, data: { stdout: "do not keep" } }), date: new Date() },
    ];

    const reduced = buildCompactionInput(messages, "Test");
    assert.match(reduced.prompt, /\.\/agent\.md/);
    assert.doesNotMatch(reduced.prompt, /VERY LARGE FILE CONTENT/);
    assert.match(reduced.preservedEditHistory, /editFile/);
    assert.match(reduced.preservedEditHistory, /runtime\.ts/);
    assert.doesNotMatch(reduced.preservedEditHistory, /bash/);
    assert.doesNotMatch(reduced.preservedEditHistory, /secret large replacement/);
  });

  it("preserves nested multiCall editFile calls only", () => {
    const messages: any[] = [
      {
        role: "assistant",
        content: "",
        date: new Date(),
        toolCalls: [{ id: "multi-1", toolName: "multiCall", args: { calls: [
          { tool: "readFile", args: { path: "README.md" } },
          { tool: "editFile", args: { path: "src/a.ts", operation: "replace" } },
          { tool: "bash", args: { command: "echo no" } },
        ] } }],
      },
      { role: "tool", name: "multiCall", toolCallId: "multi-1", content: JSON.stringify({ ok: true, data: { results: [
        { index: 0, tool: "readFile", ok: true, data: { content: "contents" } },
        { index: 1, tool: "editFile", ok: true, data: { success: true, replacements: 2 } },
        { index: 2, tool: "bash", ok: true, data: { stdout: "no" } },
      ] } }), date: new Date() },
    ];

    const reduced = buildCompactionInput(messages, "Test");
    assert.match(reduced.prompt, /README\.md/);
    assert.match(reduced.preservedEditHistory, /src\/a\.ts/);
    assert.doesNotMatch(reduced.preservedEditHistory, /echo no/);
  });

  it("extracts latest todo status and appends deterministic preserved edits", () => {
    const messages: any[] = [
      { role: "assistant", content: "", date: new Date(), toolCalls: [{ id: "todo-1", toolName: "todoMd", args: { action: "read", name: "work" } }] },
      { role: "tool", name: "todoMd", toolCallId: "todo-1", content: JSON.stringify({ ok: true, data: { content: "# Work\n- [x] inspect\n- [ ] implement\n" } }), date: new Date() },
    ];
    const reduced = buildCompactionInput(messages, "Test");
    assert.match(reduced.prompt, /- \[x\] inspect/);
    assert.match(compactSummaryWithPreservedEdits("Summary", "## Preserved edit history\nNone"), /Summary\n\n## Preserved edit history/);
  });

  it("builds provider messages as system for normal providers and developer for Codex", () => {
    assert.equal(compactionProviderMessage("summary", "glm")?.role, "system");
    assert.equal(compactionProviderMessage("summary", "codex")?.role, "developer");
    assert.equal(compactionSummaryFromMetadata({ custom: { qubit: { compactionSummary: "saved" } } }), "saved");
  });
});
