import type { CodexResponseParseOptions, CodexResponseParseResult } from "./types.js";

export type CodexResponseParseState = {
  content: string;
  reasoningParts: string[];
  toolCalls: NonNullable<CodexResponseParseResult["toolCalls"]>;
  generatedImages: NonNullable<CodexResponseParseResult["generatedImages"]>;
  providerStopReason: string;
  responseId: string;
  usage: unknown;
  outputItems: unknown[];
  debug: boolean;
  debugEventCounts: Map<string, number>;
  debugReasoningItemCount: number;
  debugReasoningDeltaCount: number;
  debugOutputItemTypes: Map<string, number>;
  options: CodexResponseParseOptions;
};

export function createCodexResponseParseState(options: CodexResponseParseOptions = {}): CodexResponseParseState {
  return {
    content: "",
    reasoningParts: [],
    toolCalls: [],
    generatedImages: [],
    providerStopReason: "",
    responseId: "",
    usage: undefined,
    outputItems: [],
    debug: process.env.QUBIT_CODEX_REASONING_DEBUG === "1",
    debugEventCounts: new Map<string, number>(),
    debugReasoningItemCount: 0,
    debugReasoningDeltaCount: 0,
    debugOutputItemTypes: new Map<string, number>(),
    options,
  };
}

export function codexResponseParseResult(state: CodexResponseParseState): CodexResponseParseResult {
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
    ...(state.responseId ? { responseId: state.responseId } : {}),
    ...(state.usage !== undefined ? { usage: state.usage } : {}),
    ...(state.outputItems.length ? { outputItems: state.outputItems } : {}),
  };
}

export function processCodexResponseEventText(text: string, state: CodexResponseParseState, eventHint = ""): void {
  let payload: any;
  try {
    payload = JSON.parse(text);
  } catch {
    return;
  }
  processCodexResponsePayload(payload, state, eventHint);
}

export function processCodexResponsePayload(payload: any, state: CodexResponseParseState, eventHint = ""): void {
  const eventType = eventHint || payload?.type || "";
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
      if (isHostedToolOutputItem(item)) state.outputItems.push(item);
      if (item?.type === "reasoning") state.debugReasoningItemCount += 1;
      collectOutputItem(item, state.toolCalls, state.generatedImages, (value) => appendReasoning(state, value), (value) => {
        if (!state.content) state.content += value;
      });
      break;
    }
    case "response.completed":
      state.providerStopReason = "response.completed";
      captureCompletedResponseMetadata(payload.response, state);
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

function captureCompletedResponseMetadata(response: any, state: CodexResponseParseState): void {
  if (!response || typeof response !== "object") return;
  if (typeof response.id === "string") state.responseId = response.id;
  if (response.usage !== undefined) state.usage = response.usage;
  if (Array.isArray(response.output)) state.outputItems = response.output;
}

function appendReasoning(state: CodexResponseParseState, value: unknown): void {
  const reasoningText = textFromUnknown(value);
  if (!reasoningText) return;
  const current = state.reasoningParts.join("");
  if (current.includes(reasoningText)) return;
  state.reasoningParts.push(reasoningText);
}

function recordOutputItemType(state: CodexResponseParseState, item: any): void {
  if (!state.debug) return;
  const type = typeof item?.type === "string" ? item.type : "unknown";
  state.debugOutputItemTypes.set(type, (state.debugOutputItemTypes.get(type) || 0) + 1);
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
  toolCalls: NonNullable<CodexResponseParseResult["toolCalls"]>,
  generatedImages: NonNullable<CodexResponseParseResult["generatedImages"]>,
  appendReasoning: (value: unknown) => void,
  appendContent: (value: string) => void,
): void {
  const output = response?.output;
  if (!Array.isArray(output)) return;
  for (const item of output) collectOutputItem(item, toolCalls, generatedImages, appendReasoning, appendContent);
}

function collectOutputItem(
  item: any,
  toolCalls: NonNullable<CodexResponseParseResult["toolCalls"]>,
  generatedImages: NonNullable<CodexResponseParseResult["generatedImages"]>,
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
    const dataUrl = dataUrlFromImageItem(item);
    const mimeType = mimeTypeFromImageItem(item);
    generatedImages.push({ ...(url ? { url } : {}), ...(dataUrl ? { dataUrl } : {}), ...(mimeType ? { mimeType } : {}) });
    return;
  }
  if (item.type === "message" || item.type === "output_message") {
    const text = outputTextFromItem(item);
    if (text) appendContent(text);
  }
}

function isHostedToolOutputItem(item: any): boolean {
  return item?.type === "web_search_call" || item?.type === "image_generation_call" || item?.type === "generated_image";
}

function dataUrlFromImageItem(item: any): string | undefined {
  const value = typeof item?.result === "string" ? item.result : typeof item?.dataUrl === "string" ? item.dataUrl : "";
  if (!value) return undefined;
  if (/^data:[^;]+;base64,/i.test(value)) return value;
  return `data:${mimeTypeFromImageItem(item) || "image/png"};base64,${value}`;
}

function mimeTypeFromImageItem(item: any): string | undefined {
  const explicit = typeof item?.mimeType === "string" ? item.mimeType : typeof item?.mime_type === "string" ? item.mime_type : "";
  if (explicit) return explicit;
  const format = String(item?.output_format || item?.format || "").trim().toLowerCase();
  if (format === "jpg" || format === "jpeg") return "image/jpeg";
  if (format === "webp") return "image/webp";
  if (format === "png") return "image/png";
  if (item?.result || item?.dataUrl) return "image/png";
  return undefined;
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

function dedupeToolCalls(toolCalls: NonNullable<CodexResponseParseResult["toolCalls"]>): NonNullable<CodexResponseParseResult["toolCalls"]> {
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
