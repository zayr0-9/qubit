import type { CodexSseParseResult } from "./types.js";
import { createRetryableCodexHttpError } from "./responsesRetry.js";

type CodexSseParseOptions = {
  onReasoningDelta?: (delta: string) => void;
};

type CodexSseParseState = {
  content: string;
  reasoningParts: string[];
  toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>;
  generatedImages: NonNullable<CodexSseParseResult["generatedImages"]>;
  providerStopReason: string;
  debug: boolean;
  debugEventCounts: Map<string, number>;
  debugReasoningItemCount: number;
  debugReasoningDeltaCount: number;
  debugOutputItemTypes: Map<string, number>;
  options: CodexSseParseOptions;
};

export async function parseCodexSseResponse(response: Response, options: CodexSseParseOptions = {}): Promise<CodexSseParseResult> {
  if (!response.ok) {
    const text = await response.text();
    throw createRetryableCodexHttpError(response.status, text);
  }
  if (!response.body) {
    return parseCodexSseText(await response.text(), options);
  }
  const state = createParseState(options);
  const decoder = new TextDecoder();
  const reader = response.body.getReader();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
    const frames = buffer.split(/\n\n+/);
    buffer = frames.pop() || "";
    for (const frame of frames) {
      if (frame.trim()) processFrameText(frame, state);
    }
  }
  buffer += decoder.decode();
  if (buffer.trim()) processFrameText(buffer, state);
  return stateResult(state);
}

export function parseCodexSseText(text: string, options: CodexSseParseOptions = {}): CodexSseParseResult {
  const state = createParseState(options);
  for (const frame of splitFrames(text)) processFrameText(frame, state);
  return stateResult(state);
}

function createParseState(options: CodexSseParseOptions): CodexSseParseState {
  return {
    content: "",
    reasoningParts: [],
    toolCalls: [],
    generatedImages: [],
    providerStopReason: "",
    debug: process.env.QUBIT_CODEX_REASONING_DEBUG === "1",
    debugEventCounts: new Map<string, number>(),
    debugReasoningItemCount: 0,
    debugReasoningDeltaCount: 0,
    debugOutputItemTypes: new Map<string, number>(),
    options,
  };
}

function stateResult(state: CodexSseParseState): CodexSseParseResult {
  const reasoningContent = state.reasoningParts.join("");
  if (state.debug) {
    console.error(`[codex-reasoning] events=${JSON.stringify(Object.fromEntries(state.debugEventCounts))} outputItemTypes=${JSON.stringify(Object.fromEntries(state.debugOutputItemTypes))} reasoningDeltas=${state.debugReasoningDeltaCount} reasoningItems=${state.debugReasoningItemCount} reasoningChars=${reasoningContent.length} outputChars=${state.content.length} toolCalls=${state.toolCalls.length} stop=${state.providerStopReason || ""}`);
  }
  return {
    content: state.content,
    ...(reasoningContent ? { reasoningContent } : {}),
    toolCalls: dedupeToolCalls(state.toolCalls),
    providerStopReason: state.providerStopReason || undefined,
    ...(state.generatedImages.length ? { generatedImages: state.generatedImages } : {}),
  };
}

function processFrameText(frame: string, state: CodexSseParseState): void {
  const parsed = parseFrame(frame);
  if (!parsed || parsed.data === "[DONE]") return;
  let payload: any;
  try {
    payload = JSON.parse(parsed.data);
  } catch {
    return;
  }
  const eventType = parsed.event || payload.type || "";
  if (state.debug) state.debugEventCounts.set(eventType || "unknown", (state.debugEventCounts.get(eventType || "unknown") || 0) + 1);
  switch (eventType) {
    case "response.output_text.delta":
      state.content += String(payload.delta ?? "");
      break;
    case "response.reasoning_text.delta":
    case "response.reasoning_summary_text.delta": {
      state.debugReasoningDeltaCount += 1;
      const delta = textFromUnknown(payload.delta);
      if (delta) state.options.onReasoningDelta?.(delta);
      appendReasoning(state, payload.delta);
      break;
    }
    case "response.output_item.done": {
      const item = payload.item || payload.output_item || payload;
      recordOutputItemType(state, item);
      if (item?.type === "reasoning") state.debugReasoningItemCount += 1;
      collectOutputItem(item, state.toolCalls, state.generatedImages, (value) => appendReasoning(state, value), (value) => {
        if (!state.content) state.content += value;
      });
      break;
    }
    case "response.completed":
      state.providerStopReason = "response.completed";
      if (state.debug) recordOutputItemTypes(payload.response?.output, state.debugOutputItemTypes);
      state.debugReasoningItemCount += countReasoningItems(payload.response?.output);
      collectResponseOutput(payload.response, state.toolCalls, state.generatedImages, (value) => appendReasoning(state, value), (value) => {
        if (!state.content) state.content += value;
      });
      break;
    case "response.failed":
      throw new Error(`Codex response failed${payload.response?.error?.message ? `: ${payload.response.error.message}` : ""}`);
    case "response.incomplete":
      throw new Error(`Codex response incomplete${payload.response?.incomplete_details?.reason ? `: ${payload.response.incomplete_details.reason}` : ""}`);
    default:
      if (payload.type === "function_call") collectOutputItem(payload, state.toolCalls, state.generatedImages, (value) => appendReasoning(state, value), (value) => {
        state.content += value;
      });
      break;
  }
}

function appendReasoning(state: CodexSseParseState, value: unknown): void {
  const reasoningText = textFromUnknown(value);
  if (!reasoningText) return;
  const current = state.reasoningParts.join("");
  if (current.includes(reasoningText)) return;
  state.reasoningParts.push(reasoningText);
}

function recordOutputItemType(state: CodexSseParseState, item: any): void {
  if (!state.debug) return;
  const type = typeof item?.type === "string" ? item.type : "unknown";
  state.debugOutputItemTypes.set(type, (state.debugOutputItemTypes.get(type) || 0) + 1);
}

function splitFrames(text: string): string[] {
  return text.replace(/\r\n/g, "\n").split(/\n\n+/).filter((frame) => frame.trim());
}

function parseFrame(frame: string): { event?: string; data: string } | null {
  let event = "";
  const data: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  if (data.length === 0) return null;
  return { ...(event ? { event } : {}), data: data.join("\n") };
}

function countReasoningItems(output: unknown): number {
  if (!Array.isArray(output)) return 0;
  return output.filter((item: any) => item?.type === "reasoning").length;
}

function recordOutputItemTypes(output: unknown, counts: Map<string, number>): void {
  if (!Array.isArray(output)) return;
  for (const item of output) {
    const type = typeof item?.type === "string" ? item.type : "unknown";
    counts.set(type, (counts.get(type) || 0) + 1);
  }
}

function collectResponseOutput(
  response: any,
  toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>,
  generatedImages: NonNullable<CodexSseParseResult["generatedImages"]>,
  appendReasoning: (value: unknown) => void,
  appendContent: (value: string) => void,
): void {
  const output = response?.output;
  if (!Array.isArray(output)) return;
  for (const item of output) collectOutputItem(item, toolCalls, generatedImages, appendReasoning, appendContent);
}

function collectOutputItem(
  item: any,
  toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>,
  generatedImages: NonNullable<CodexSseParseResult["generatedImages"]>,
  appendReasoning: (value: unknown) => void,
  appendContent: (value: string) => void,
): void {
  if (!item || typeof item !== "object") return;
  if (item.type === "reasoning") {
    appendReasoning(reasoningTextFromItem(item));
    return;
  }
  if (item.type === "function_call") {
    toolCalls.push({ id: item.call_id || item.id, toolName: String(item.name || "unknown_tool"), args: parseArgs(item.arguments) });
    return;
  }
  if (item.type === "image_generation_call" || item.type === "generated_image") {
    const url = item.url || item.image_url;
    const dataUrl = item.result && typeof item.result === "string" ? item.result : undefined;
    generatedImages.push({ ...(url ? { url } : {}), ...(dataUrl ? { dataUrl } : {}) });
    return;
  }
  if (item.type === "message" || item.type === "output_message") {
    const text = outputTextFromItem(item);
    if (text) appendContent(text);
  }
}

function reasoningTextFromItem(item: any): string {
  return [
    item.summary,
    item.content,
    item.text,
    item.delta,
  ].map(textFromUnknown).filter(Boolean).join("");
}

function outputTextFromItem(item: any): string {
  return textPartsFromUnknown(item.content, new Set(["output_text", "text"])).join("");
}

function textFromUnknown(value: unknown): string {
  return textPartsFromUnknown(value).join("");
}

function textPartsFromUnknown(value: unknown, allowedTypes?: Set<string>): string[] {
  if (value === undefined || value === null) return [];
  if (typeof value === "string") return [value];
  if (typeof value === "number" || typeof value === "boolean") return [];
  if (Array.isArray(value)) return value.flatMap((item) => textPartsFromUnknown(item, allowedTypes));
  if (typeof value !== "object") return [];

  const record = value as Record<string, unknown>;
  const type = typeof record.type === "string" ? record.type : "";
  if (allowedTypes && type && !allowedTypes.has(type)) return [];

  const parts: string[] = [];
  if (typeof record.text === "string") parts.push(record.text);
  if (typeof record.delta === "string") parts.push(record.delta);
  if (typeof record.content === "string") parts.push(record.content);
  return parts;
}

function parseArgs(value: unknown): unknown {
  if (typeof value !== "string") return value ?? {};
  if (!value.trim()) return {};
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

function dedupeToolCalls(toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>): NonNullable<CodexSseParseResult["toolCalls"]> {
  const seen = new Set<string>();
  const result = [];
  for (const call of toolCalls) {
    const key = call.id || `${call.toolName}:${JSON.stringify(call.args)}`;
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(call);
  }
  return result;
}
