import type { CodexResponseParseOptions, CodexResponseParseResult } from "./types.js";
import { createRetryableCodexHttpError } from "./responsesRetry.js";
import { codexResponseParseResult, createCodexResponseParseState, processCodexResponseEventText } from "./responsesEvents.js";

export async function parseCodexSseResponse(response: Response, options: CodexResponseParseOptions = {}): Promise<CodexResponseParseResult> {
  if (!response.ok) {
    const text = await response.text();
    throw createRetryableCodexHttpError(response.status, text);
  }
  if (!response.body) {
    return parseCodexSseText(await response.text(), options);
  }
  const state = createCodexResponseParseState(options);
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
  return codexResponseParseResult(state);
}

export function parseCodexSseText(text: string, options: CodexResponseParseOptions = {}): CodexResponseParseResult {
  const state = createCodexResponseParseState(options);
  for (const frame of splitFrames(text)) processFrameText(frame, state);
  return codexResponseParseResult(state);
}

function processFrameText(frame: string, state: ReturnType<typeof createCodexResponseParseState>): void {
  const parsed = parseFrame(frame);
  if (!parsed || parsed.data === "[DONE]") return;
  processCodexResponseEventText(parsed.data, state, parsed.event);
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
