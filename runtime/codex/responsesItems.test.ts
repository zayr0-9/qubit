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
});
