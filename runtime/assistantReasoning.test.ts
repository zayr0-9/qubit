import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { assistantReasoningContent } from "./assistantReasoning.js";

describe("assistantReasoningContent", () => {
  it("aggregates reasoning from assistant tool-call and final messages", () => {
    const reasoning = assistantReasoningContent([
      { role: "user", content: "inspect" },
      { role: "assistant", content: "", reasoningContent: "Need inspect files.", toolCalls: [{ id: "call_1" }] },
      { role: "tool", content: "{}" },
      { role: "assistant", content: "Done.", reasoningContent: "Summarized result." },
    ]);

    assert.equal(reasoning, "Need inspect files.\n\nSummarized result.");
  });

  it("deduplicates repeated reasoning from later assistant messages", () => {
    const reasoning = assistantReasoningContent([
      { role: "assistant", content: "", reasoningContent: "same reasoning" },
      { role: "assistant", content: "Done.", reasoningContent: "same reasoning" },
    ]);

    assert.equal(reasoning, "same reasoning");
  });
});
