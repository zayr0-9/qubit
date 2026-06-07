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
        id: "resp-123",
        usage: {
          input_tokens: 20,
          input_tokens_details: { cached_tokens: 12 },
          output_tokens: 8,
        },
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
    assert.equal(parsed.responseId, "resp-123");
    assert.deepEqual(parsed.usage, {
      input_tokens: 20,
      input_tokens_details: { cached_tokens: 12 },
      output_tokens: 8,
    });
    assert.deepEqual(parsed.outputItems, [
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
    ]);
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

  it("replaces partial streamed reasoning with fuller completed reasoning", () => {
    const parsed = parseCodexSseText([
      frame("response.reasoning_summary_text.delta", { type: "response.reasoning_summary_text.delta", delta: "I see that the is asking me." }),
      frame("response.output_item.done", {
        type: "response.output_item.done",
        item: { type: "reasoning", summary: [{ type: "summary_text", text: "I see that the user is asking me." }] },
      }),
    ].join("\n"));

    assert.equal(parsed.reasoningContent, "I see that the user is asking me.");
  });

  it("does not duplicate reasoning seen in output item done and completed", () => {
    const parsed = parseCodexSseText([
      frame("response.output_item.done", {
        type: "response.output_item.done",
        item: { type: "reasoning", summary: [{ type: "summary_text", text: "Inspect code and patch." }] },
      }),
      frame("response.completed", {
        type: "response.completed",
        response: { output: [{ type: "reasoning", summary: [{ type: "summary_text", text: "Inspect code and patch." }] }] },
      }),
    ].join("\n"));

    assert.equal(parsed.reasoningContent, "Inspect code and patch.");
  });

  it("keeps hosted web search calls in outputItems without creating local tool calls", () => {
    const webSearchCall = {
      id: "ws_123",
      type: "web_search_call",
      status: "completed",
      action: { type: "search", queries: ["current news"] },
    };
    const parsed = parseCodexSseText(frame("response.output_item.done", {
      type: "response.output_item.done",
      item: webSearchCall,
    }));

    assert.deepEqual(parsed.toolCalls, []);
    assert.deepEqual(parsed.outputItems, [webSearchCall]);
  });

  it("normalizes image generation result base64 into generated image data URLs", () => {
    const parsed = parseCodexSseText(frame("response.completed", {
      type: "response.completed",
      response: {
        output: [
          {
            id: "ig_123",
            type: "image_generation_call",
            status: "completed",
            output_format: "jpeg",
            result: "aW1hZ2UtYnl0ZXM=",
          },
        ],
      },
    }));

    assert.deepEqual(parsed.generatedImages, [
      { dataUrl: "data:image/jpeg;base64,aW1hZ2UtYnl0ZXM=", mimeType: "image/jpeg" },
    ]);
  });
});
