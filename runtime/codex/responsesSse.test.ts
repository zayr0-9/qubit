import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { parseCodexSseText } from "./responsesSse.js";

function frame(event: string, data: unknown): string {
  return [`event: ${event}`, `data: ${JSON.stringify(data)}`, ""].join("\n");
}

describe("parseCodexSseText", () => {
  it("captures reasoning text and summary deltas", () => {
    const parsed = parseCodexSseText([
      frame("response.reasoning_text.delta", { type: "response.reasoning_text.delta", delta: "thinking" }),
      frame("response.reasoning_summary_text.delta", { type: "response.reasoning_summary_text.delta", delta: " summary" }),
      frame("response.output_text.delta", { type: "response.output_text.delta", delta: "answer" }),
    ].join("\n"));

    assert.equal(parsed.content, "answer");
    assert.equal(parsed.reasoningContent, "thinking summary");
  });

  it("emits reasoning delta callbacks as reasoning frames are parsed", () => {
    const deltas: string[] = [];
    parseCodexSseText([
      frame("response.reasoning_summary_text.delta", { type: "response.reasoning_summary_text.delta", delta: "step " }),
      frame("response.reasoning_summary_text.delta", { type: "response.reasoning_summary_text.delta", delta: "one" }),
    ].join("\n"), { onReasoningDelta: (delta) => deltas.push(delta) });

    assert.deepEqual(deltas, ["step ", "one"]);
  });

  it("captures completed reasoning output item summaries", () => {
    const parsed = parseCodexSseText([
      frame("response.output_item.done", {
        type: "response.output_item.done",
        item: {
          type: "reasoning",
          summary: [
            { type: "summary_text", text: "Need inspect. " },
            { type: "summary_text", text: "Then patch." },
          ],
        },
      }),
      frame("response.output_text.delta", { type: "response.output_text.delta", delta: "done" }),
    ].join("\n"));

    assert.equal(parsed.reasoningContent, "Need inspect. Then patch.");
    assert.equal(parsed.content, "done");
  });

  it("captures reasoning output from completed response output arrays", () => {
    const parsed = parseCodexSseText(frame("response.completed", {
      type: "response.completed",
      response: {
        output: [
          {
            type: "reasoning",
            content: [
              { type: "reasoning_text", text: "Look at parser. " },
              { type: "reasoning_text", text: "Fix extraction." },
            ],
          },
          {
            type: "message",
            content: [{ type: "output_text", text: "final answer" }],
          },
        ],
      },
    }));

    assert.equal(parsed.reasoningContent, "Look at parser. Fix extraction.");
    assert.equal(parsed.content, "final answer");
    assert.equal(parsed.providerStopReason, "response.completed");
  });

  it("does not duplicate completed message text after streamed output text", () => {
    const parsed = parseCodexSseText([
      frame("response.output_text.delta", { type: "response.output_text.delta", delta: "final answer" }),
      frame("response.completed", {
        type: "response.completed",
        response: {
          output: [
            {
              type: "message",
              content: [{ type: "output_text", text: "final answer" }],
            },
          ],
        },
      }),
    ].join("\n"));

    assert.equal(parsed.content, "final answer");
  });

  it("does not duplicate output item done message text after streamed output text", () => {
    const parsed = parseCodexSseText([
      frame("response.output_text.delta", { type: "response.output_text.delta", delta: "Read " }),
      frame("response.output_text.delta", { type: "response.output_text.delta", delta: "`agent.md`." }),
      frame("response.output_item.done", {
        type: "response.output_item.done",
        item: {
          type: "message",
          content: [{ type: "output_text", text: "Read `agent.md`." }],
        },
      }),
      frame("response.completed", {
        type: "response.completed",
        response: {
          output: [
            {
              type: "message",
              content: [{ type: "output_text", text: "Read `agent.md`." }],
            },
          ],
        },
      }),
    ].join("\n"));

    assert.equal(parsed.content, "Read `agent.md`.");
  });

  it("deduplicates repeated reasoning from deltas and completed items", () => {
    const parsed = parseCodexSseText([
      frame("response.reasoning_summary_text.delta", { type: "response.reasoning_summary_text.delta", delta: "same reasoning" }),
      frame("response.output_item.done", {
        type: "response.output_item.done",
        item: { type: "reasoning", summary: [{ type: "summary_text", text: "same reasoning" }] },
      }),
    ].join("\n"));

    assert.equal(parsed.reasoningContent, "same reasoning");
  });
});
