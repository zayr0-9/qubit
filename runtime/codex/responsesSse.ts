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
  const summaryParts: string[] = [];
  const toolCalls: NonNullable<CodexSseParseResult["toolCalls"]> = [];
  const generatedImages: NonNullable<CodexSseParseResult["generatedImages"]> = [];
  let providerStopReason = "";

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
        reasoningParts.push(String(payload.delta ?? ""));
        break;
      case "response.reasoning_summary_text.delta":
        summaryParts.push(String(payload.delta ?? ""));
        break;
      case "response.output_item.done":
        collectOutputItem(payload.item || payload.output_item || payload, toolCalls, generatedImages);
        break;
      case "response.completed":
        providerStopReason = "response.completed";
        collectResponseOutput(payload.response, toolCalls, generatedImages);
        break;
      case "response.failed":
        throw new Error(`Codex response failed${payload.response?.error?.message ? `: ${payload.response.error.message}` : ""}`);
      case "response.incomplete":
        throw new Error(`Codex response incomplete${payload.response?.incomplete_details?.reason ? `: ${payload.response.incomplete_details.reason}` : ""}`);
      default:
        if (payload.type === "function_call") collectOutputItem(payload, toolCalls, generatedImages);
        break;
    }
  }

  return {
    content,
    ...(reasoningParts.length || summaryParts.length ? { reasoningContent: [...reasoningParts, ...summaryParts].join("") } : {}),
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

function collectResponseOutput(response: any, toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>, generatedImages: NonNullable<CodexSseParseResult["generatedImages"]>): void {
  const output = response?.output;
  if (!Array.isArray(output)) return;
  for (const item of output) collectOutputItem(item, toolCalls, generatedImages);
}

function collectOutputItem(item: any, toolCalls: NonNullable<CodexSseParseResult["toolCalls"]>, generatedImages: NonNullable<CodexSseParseResult["generatedImages"]>): void {
  if (!item || typeof item !== "object") return;
  if (item.type === "function_call") {
    toolCalls.push({ id: item.call_id || item.id, toolName: String(item.name || "unknown_tool"), args: parseArgs(item.arguments) });
    return;
  }
  if (item.type === "image_generation_call" || item.type === "generated_image") {
    const url = item.url || item.image_url;
    const dataUrl = item.result && typeof item.result === "string" ? item.result : undefined;
    generatedImages.push({ ...(url ? { url } : {}), ...(dataUrl ? { dataUrl } : {}) });
  }
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
