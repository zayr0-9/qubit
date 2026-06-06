import * as assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { describe, it } from "node:test";
import { AgentRuntime, defineAgent } from "@hyper-labs/hyper-router";
import type { Message, ModelProvider, ModelResponse } from "@hyper-labs/hyper-router";
import { QubitSqliteStorage } from "./storage/qubitSqliteStorage.js";

const date = new Date("2026-06-06T00:00:00.000Z");

class RecordingProvider implements ModelProvider {
  calls: Message[][] = [];

  async generate(input: Parameters<ModelProvider["generate"]>[0]): Promise<ModelResponse> {
    this.calls.push(input.messages.map((message) => ({ ...message })));
    return {
      message: {
        role: "assistant",
        content: "edited assistant response",
        date,
      },
      stopReason: "stop",
    };
  }
}

function message(role: Message["role"], content: string): Message {
  return { role, content, date } as Message;
}

function nonSystemContents(messages: Message[]): string[] {
  return messages
    .filter((message) => message.role !== "system")
    .map((message) => String(message.content ?? ""));
}

function rawMessageStartIndexForUiMessageIndex(messages: Message[], uiMessageIndex: number): number {
  if (uiMessageIndex <= 0) return 0;
  const visibleStartIndexes: number[] = [];
  const consumedToolResults = new Set<string>();

  for (let index = 0; index < messages.length; index += 1) {
    const current = messages[index] as (Message & { toolCallId?: string; toolCalls?: Array<{ id?: string }>; reasoningContent?: string }) | undefined;
    if (!current || current.role === "system") continue;

    if (current.role === "user") {
      const content = typeof current.content === "string" ? current.content : String(current.content ?? "");
      if (content.trim()) visibleStartIndexes.push(index);
      continue;
    }

    if (current.role === "assistant") {
      const toolCalls = Array.isArray(current.toolCalls) ? current.toolCalls : [];
      if (current.reasoningContent) visibleStartIndexes.push(index);
      if (toolCalls.length > 0) visibleStartIndexes.push(index);
      for (const toolCall of toolCalls) {
        const toolCallId = toolCall.id || "";
        if (!toolCallId) continue;
        const resultIndex = messages.findIndex((candidate, candidateIndex) => {
          const toolMessage = candidate as (Message & { toolCallId?: string }) | undefined;
          return candidateIndex > index && toolMessage?.role === "tool" && toolMessage.toolCallId === toolCallId;
        });
        if (resultIndex >= 0) consumedToolResults.add(toolCallId);
      }
      const content = typeof current.content === "string" ? current.content : String(current.content ?? "");
      if (content.trim()) visibleStartIndexes.push(index);
      continue;
    }

    if (current.role === "tool" && current.toolCallId && !consumedToolResults.has(current.toolCallId)) {
      visibleStartIndexes.push(index);
      consumedToolResults.add(current.toolCallId);
    }
  }

  if (uiMessageIndex >= visibleStartIndexes.length) return messages.length;
  return visibleStartIndexes[uiMessageIndex] ?? messages.length;
}

describe("edited fork provider context", () => {
  it("sends only the fork prefix plus edited message to the model", async () => {
    const dir = await mkdtemp(join(tmpdir(), "qubit-fork-edit-context-"));
    const storage = new QubitSqliteStorage({ filePath: join(dir, "sessions.sqlite") });
    const provider = new RecordingProvider();
    const runtime = new AgentRuntime({
      agent: defineAgent({
        name: "fork-edit-context-test",
        instructions: "test instructions",
        model: "test-model",
        tools: [],
      }),
      provider,
      storage,
    });

    try {
      const parentMessages: Message[] = [
        message("user", "message 1"),
        message("assistant", "answer 1"),
        message("user", "message 2"),
        message("assistant", "answer 2"),
        message("user", "original message 4"),
        message("assistant", "original answer 4"),
      ];
      await storage.saveMessages("parent-session", parentMessages);

      const selectedUiMessageIndex = 4;
      const forkCutoff = rawMessageStartIndexForUiMessageIndex(parentMessages, selectedUiMessageIndex);
      assert.equal(forkCutoff, 4, "test fixture should cut immediately before original message 4");

      const forkMessages = parentMessages.slice(0, forkCutoff);
      await storage.saveMessages("edited-fork-session", forkMessages);

      await runtime.run({
        runId: "run-edit-fork",
        sessionId: "edited-fork-session",
        input: "edited message 4a",
        maxSteps: 1,
      });

      assert.equal(provider.calls.length, 1, "provider should be called once");
      const sentContents = nonSystemContents(provider.calls[0] ?? []);
      assert.deepEqual(sentContents, [
        "message 1",
        "answer 1",
        "message 2",
        "answer 2",
        "edited message 4a",
      ]);
      assert.ok(!sentContents.includes("original message 4"), "original selected message leaked into provider context");
      assert.ok(!sentContents.includes("original answer 4"), "original answer after selected message leaked into provider context");

      const persistedForkContents = nonSystemContents(await storage.loadMessages("edited-fork-session"));
      assert.deepEqual(persistedForkContents, [
        "message 1",
        "answer 1",
        "message 2",
        "answer 2",
        "edited message 4a",
        "edited assistant response",
      ]);
    } finally {
      storage.close();
    }
  });
});

function assistantToolCallMessage(content: string, toolCallId: string): Message {
  return {
    role: "assistant",
    content,
    date,
    toolCalls: [{ id: toolCallId, toolName: "readFile", args: { path: "agent.md" } }],
  } as Message;
}

function toolResultMessage(content: string, toolCallId: string): Message {
  return {
    role: "tool",
    content,
    date,
    name: "readFile",
    toolCallId,
  } as Message;
}

describe("edited fork provider context with grouped runtime messages", () => {
  it("cuts before the selected user message when prior assistant messages include reasoning and tool calls", async () => {
    const dir = await mkdtemp(join(tmpdir(), "qubit-fork-edit-context-rich-"));
    const storage = new QubitSqliteStorage({ filePath: join(dir, "sessions.sqlite") });
    const provider = new RecordingProvider();
    const runtime = new AgentRuntime({
      agent: defineAgent({
        name: "fork-edit-context-rich-test",
        instructions: "test instructions",
        model: "test-model",
        tools: [],
      }),
      provider,
      storage,
    });

    try {
      const parentMessages: Message[] = [
        message("user", "message 1"),
        { ...message("assistant", "answer 1"), reasoningContent: "reasoning before answer 1" } as Message,
        message("user", "message 2"),
        assistantToolCallMessage("", "tool-call-1"),
        toolResultMessage("tool result before answer 2", "tool-call-1"),
        message("assistant", "answer 2 after tool"),
        message("user", "original message 4"),
        { ...message("assistant", "original answer 4"), reasoningContent: "original reasoning 4" } as Message,
      ];
      await storage.saveMessages("rich-parent-session", parentMessages);
      const storedParentMessages = await storage.loadMessages("rich-parent-session");

      // UI-visible messages are:
      // 0 user message 1
      // 1 assistant reasoning before answer 1
      // 2 assistant answer 1
      // 3 user message 2
      // 4 assistant tool-call group
      // 5 assistant answer 2 after tool
      // 6 user original message 4
      // 7 assistant original reasoning 4
      // 8 assistant original answer 4
      const selectedUiMessageIndex = 6;
      const forkCutoff = rawMessageStartIndexForUiMessageIndex(storedParentMessages, selectedUiMessageIndex);
      assert.equal(forkCutoff, 6, "rich fixture should cut immediately before original message 4 raw message");

      const forkMessages = storedParentMessages.slice(0, forkCutoff);
      await storage.saveMessages("rich-edited-fork-session", forkMessages);

      await runtime.run({
        runId: "run-rich-edit-fork",
        sessionId: "rich-edited-fork-session",
        input: "edited message 4a",
        maxSteps: 1,
      });

      assert.equal(provider.calls.length, 1, "provider should be called once");
      const sentContents = nonSystemContents(provider.calls[0] ?? []);
      assert.deepEqual(sentContents, [
        "message 1",
        "answer 1",
        "message 2",
        "",
        "tool result before answer 2",
        "answer 2 after tool",
        "edited message 4a",
      ]);
      assert.ok(!sentContents.includes("original message 4"), "original selected message leaked into rich provider context");
      assert.ok(!sentContents.includes("original answer 4"), "original answer after selected message leaked into rich provider context");

      const providerMessages = provider.calls[0] ?? [];
      assert.equal(providerMessages.some((current) => current.role === "assistant" && current.reasoningContent === "original reasoning 4"), false, "original reasoning after selected message leaked into provider context");
      assert.equal(providerMessages.some((current) => current.role === "assistant" && current.reasoningContent === "reasoning before answer 1"), true, "prior reasoning before fork point should be preserved");
      assert.equal(providerMessages.some((current) => current.role === "tool" && (current as Message & { toolCallId?: string }).toolCallId === "tool-call-1"), true, "prior tool result before fork point should be preserved");

      const persistedForkContents = nonSystemContents(await storage.loadMessages("rich-edited-fork-session"));
      assert.deepEqual(persistedForkContents, [
        "message 1",
        "answer 1",
        "message 2",
        "",
        "tool result before answer 2",
        "answer 2 after tool",
        "edited message 4a",
        "edited assistant response",
      ]);
    } finally {
      storage.close();
    }
  });
});
