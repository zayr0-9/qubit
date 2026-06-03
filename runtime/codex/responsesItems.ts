import { z } from "zod/v4";
import type { AnyToolDefinition, Message, ToolCall } from "@hyper-labs/hyper-router";
import type { CodexRequestParts } from "./types.js";

export function toCodexRequestParts(messages: Message[], tools: AnyToolDefinition[]): CodexRequestParts {
  const instructions = messages
    .filter((message) => message.role === "system" && message.content)
    .map((message) => message.content)
    .join("\n\n") || undefined;
  const input = messages.flatMap((message, index) => messageToCodexItems(message, index));
  return { instructions, input, tools: [...tools.map(toCodexTool), ...codexHostedTools()] };
}

function codexHostedTools(): unknown[] {
  return [
    { type: "web_search" },
    { type: "image_generation" },
  ];
}

function messageToCodexItems(message: Message, index: number): unknown[] {
  if (message.role === "system") return [];
  if (message.role === "tool") {
    return [{ type: "function_call_output", call_id: message.toolCallId || `${message.name || "tool"}-result-${index}`, output: message.content || "" }];
  }
  if (message.role === "user") {
    return [{ type: "message", role: "user", content: [{ type: "input_text", text: message.content || "" }] }];
  }
  if (message.role === "assistant") {
    const items: unknown[] = [];
    if (message.content) {
      items.push({ type: "message", role: "assistant", content: [{ type: "output_text", text: message.content }] });
    }
    for (const toolCall of message.toolCalls || []) items.push(toolCallToCodexItem(toolCall));
    return items;
  }
  return [];
}

function toolCallToCodexItem(toolCall: ToolCall): unknown {
  return {
    type: "function_call",
    name: toolCall.toolName,
    arguments: JSON.stringify(toolCall.args ?? {}),
    call_id: toolCall.id || `call_${toolCall.toolName}`,
  };
}

function toCodexTool(tool: AnyToolDefinition): unknown {
  return {
    type: "function",
    name: tool.name,
    description: tool.description || "",
    strict: false,
    parameters: toJsonSchema(tool.inputSchema),
  };
}

function toJsonSchema(inputSchema: unknown): unknown {
  if (!inputSchema) return { type: "object", properties: {}, additionalProperties: true };
  if (isJsonSchemaLike(inputSchema)) return inputSchema;
  if (isZodSchema(inputSchema)) return z.toJSONSchema(inputSchema as never);
  throw new Error("Codex tool schema must be JSON Schema or Zod.");
}

function isZodSchema(value: unknown): boolean {
  return Boolean(value && typeof value === "object" && typeof (value as any).safeParse === "function" && typeof (value as any).parse === "function");
}

function isJsonSchemaLike(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const schema = value as Record<string, unknown>;
  return schema.type !== undefined || schema.properties !== undefined || schema.required !== undefined || schema.additionalProperties !== undefined || schema.items !== undefined || schema.anyOf !== undefined || schema.oneOf !== undefined || schema.allOf !== undefined || schema.enum !== undefined;
}
