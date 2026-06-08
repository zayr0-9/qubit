import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { toCodexRequestParts } from "./responsesItems.js";

describe("toCodexRequestParts", () => {
  it("does not replay surfaced reasoningContent as synthetic reasoning input", () => {
    const parts = toCodexRequestParts([
      {
        role: "assistant",
        content: "Final answer.",
        reasoningContent: "Display-only reasoning summary.",
      } as any,
      { role: "user", content: "Continue." } as any,
    ], []);

    assert.deepEqual(parts.input, [
      { type: "message", role: "assistant", content: [{ type: "output_text", text: "Final answer." }] },
      { type: "message", role: "user", content: [{ type: "input_text", text: "Continue." }] },
    ]);
  });

  it("adds Codex hosted web search and image generation tools after local functions", () => {
    const parts = toCodexRequestParts([
      { role: "user", content: "hello" } as any,
    ], [
      {
        name: "readFile",
        description: "Read a file",
        inputSchema: { type: "object", properties: { path: { type: "string" } }, required: ["path"] },
      } as any,
    ]);

    assert.deepEqual(parts.tools, [
      {
        type: "function",
        name: "readFile",
        description: "Read a file",
        strict: false,
        parameters: { type: "object", properties: { path: { type: "string" } }, required: ["path"] },
      },
      { type: "web_search" },
      { type: "image_generation" },
    ]);
  });
});

it("folds developer messages into instructions without replaying them as input", () => {
  const parts = toCodexRequestParts([
    { role: "system", content: "System rules" } as any,
    { role: "developer", content: "Compacted context" } as any,
    { role: "user", content: "Continue" } as any,
  ], []);

  assert.equal(parts.instructions, "System rules\n\n<developer>\nCompacted context\n</developer>");
  assert.deepEqual(parts.input, [
    { type: "message", role: "user", content: [{ type: "input_text", text: "Continue" }] },
  ]);
});
