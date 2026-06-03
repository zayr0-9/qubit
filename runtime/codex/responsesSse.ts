import type { CodexSseParseResult } from "./types.js";
import { createRetryableCodexHttpError } from "./responsesRetry.js";

export async function parseCodexSseResponse(response: Response): Promise<CodexSseParseResult> {
  const text = await response.text();
  if (!response.ok) {
    throw createRetryableCodexHttpError(response.status, text);
  }
  return parseCodexSseText(text);
}

export function parseCodexSseText(text: string): CodexSseParseResult {
  let content = "";
  const reasoningParts: string[] = [];
  const toolCalls: NonNullable<CodexSseParseResult["toolCalls"]> = [];
  const generatedImages: NonNullable<CodexSseParseResult["generatedImages"]> = [];
  let providerStopReason = "";

  const appendReasoning = (value: unknown) => {
    const reasoningText = textFromUnknown(value);
    if (!reasoningText) return;
    const current = reasoningParts.join("");
    if (current.includes(reasoningText)) return;
    reasoningParts.push(reasoningText);
  };

  for (const frame of splitFrames(text)) {
    const parsed = parseFrame(frame);
    if (!parsed) continue;
    if (parsed.data === "[DONE]") break;
    let payload: any;
    try {
      payload = JSON.parse(parsed.data);
    } catch {
      continue;
    }
    const eventType = parsed.event || payload.type || "";
    switch (eventType) {
      case "response.output_text.delta":
        content += String(payload.delta ?? "");
        break;
      case "response.reasoning_text.delta":
      case "response.reasoning_summary_text.delta":
        appendReasoning(payload.delta);
        break;
      case "response.output_item.done":
        collectOutputItem(payload.item || payload.output_item || payload, toolCalls, generatedImages, appendReasoning, (value) => {
          content += value;
        });
        break;
      case "response.completed":
        providerStopReason = "response.completed";
        collectResponseOutput(payload.response, toolCalls, generatedImages, appendReasoning, (value) => {
          if (!content.includes(value)) content += value;
        });
        break;
      case "response.failed":
        throw new Error(`Codex response failed${payload.response?.error?.message ? `: ${payload.response.error.message}` : ""}`);
      case "response.incomplete":
        throw new Error(`Codex response incomplete${payload.response?.incomplete_details?.reason ? `: ${payload.response.incomplete_details.reason}` : ""}`);
      default:
        if (payload.type === "function_call") collectOutputItem(payload, toolCalls, generatedImages, appendReasoning, (value) => {
          content += value;
        });
        break;
    }
  }

  const reasoningContent = reasoningParts.join("");
  return {
    content,
    ...(reasoningContent ? { reasoningContent } : {}),
    toolCalls: dedupeToolCalls(toolCalls),
    providerStopReason: providerStopReason || undefined,
    ...(generatedImages.length ? { generatedImages } : {}),
  };
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
