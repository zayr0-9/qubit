import type { Message, ToolCall } from "@hyper-labs/hyper-router";

type LooseRecord = Record<string, unknown>;

type QubitMessage = Message & {
  metadata?: LooseRecord;
};

export interface ReducedCompactionInput {
  prompt: string;
  preservedEditHistory: string;
  marker: string;
  sourcePreview: string;
  contextChars: number;
  readFiles: string[];
  latestTodoStatus?: string;
}

interface ToolResultMessage extends Message {
  role: "tool";
  name?: string;
  toolCallId?: string;
}

const textCap = 4000;
const assistantTextCap = 5000;
const preservedHistoryCap = 20000;

export function estimateMessagesContextTokens(messages: Message[]): number {
  const chars = messages.reduce((total, message) => {
    const toolCalls = Array.isArray((message as Message).toolCalls) ? JSON.stringify((message as Message).toolCalls) : "";
    return total
      + stringLength(message.role)
      + stringLength(message.content)
      + stringLength((message as Message).reasoningContent)
      + stringLength(toolCalls);
  }, 0);
  return Math.ceil(chars / 4);
}

export function buildCompactionInput(messages: Message[], sourceTitle = "Untitled chat"): ReducedCompactionInput {
  const toolResults = toolResultsById(messages);
  const transcript: string[] = [];
  const readFiles = new Set<string>();
  const editEntries: string[] = [];
  let latestTodoStatus = "";

  let step = 0;
  for (const message of messages) {
    if (!message) continue;
    if (message.role === "user") {
      const content = previewText(message.content, textCap).trim();
      if (content) transcript.push(`USER:\n${content}`);
      continue;
    }
    if (message.role === "assistant") {
      const content = previewText(message.content, assistantTextCap).trim();
      if (content && !isCompactionMarker(message as QubitMessage)) transcript.push(`ASSISTANT:\n${content}`);
      const toolCalls = Array.isArray(message.toolCalls) ? message.toolCalls : [];
      if (toolCalls.length > 0) {
        step += 1;
        for (const [index, toolCall] of toolCalls.entries()) {
          const toolName = toolCall.toolName || (toolCall as ToolCall & { name?: string }).name || "tool";
          const resultMessage = toolResults.get(toolCall.id || "");
          const parsedResult = parseToolResult(resultMessage?.content);
          collectReadFiles(toolName, toolCall.args, parsedResult, readFiles);
          if (toolName === "todoMd") latestTodoStatus = todoStatus(toolCall.args, parsedResult) || latestTodoStatus;
          const editEntry = preservedEditEntry(toolName, toolCall.args, parsedResult, `step ${step}.${index + 1}`);
          if (editEntry) editEntries.push(editEntry);
        }
      }
      continue;
    }
    if (message.role === "tool") {
      const toolMessage = message as ToolResultMessage;
      if (toolMessage.toolCallId) continue;
      const toolName = toolMessage.name || "tool";
      const parsedResult = parseToolResult(toolMessage.content);
      collectReadFiles(toolName, {}, parsedResult, readFiles);
      if (toolName === "todoMd") latestTodoStatus = todoStatus({}, parsedResult) || latestTodoStatus;
      const editEntry = preservedEditEntry(toolName, {}, parsedResult, `tool result ${editEntries.length + 1}`);
      if (editEntry) editEntries.push(editEntry);
    }
  }

  const sourcePreview = firstConversationPreview(messages);
  const marker = `>summarised session from ${sourcePreview || sourceTitle || "previous chat"}`;
  const readFileList = [...readFiles].sort();
  const preservedEditHistory = editEntries.length > 0
    ? capText(["## Preserved edit history", ...editEntries.map((entry, index) => `${index + 1}. ${entry}`)].join("\n"), preservedHistoryCap)
    : "## Preserved edit history\nNo file edit tool calls were preserved.";

  const sections = [
    "You are Qubit's context compactor. Summarize the reduced transcript for a future coding agent continuing the same task.",
    "Keep durable decisions, user requirements, current state, unresolved problems, validation results, and next steps. Do not invent facts.",
    "Tool outputs have been reduced: file reads list paths only, and exact preserved edit history will be appended by code after your summary.",
    `Source session title: ${sourceTitle || "Untitled chat"}`,
    readFileList.length > 0 ? `Files read by the model:\n${readFileList.map((file) => `- ${file}`).join("\n")}` : "Files read by the model: none captured.",
    latestTodoStatus ? `Latest todo status:\n${latestTodoStatus}` : "Latest todo status: none captured.",
    "Reduced transcript:",
    transcript.length > 0 ? transcript.join("\n\n---\n\n") : "No prior user/assistant text was captured.",
  ];

  const prompt = sections.join("\n\n");
  return {
    prompt,
    preservedEditHistory,
    marker,
    sourcePreview,
    contextChars: [...prompt].length,
    readFiles: readFileList,
    ...(latestTodoStatus ? { latestTodoStatus } : {}),
  };
}

export function compactSummaryWithPreservedEdits(summary: string, preservedEditHistory: string): string {
  const normalized = String(summary || "").trim() || "The prior session was compacted, but the summarizer returned no text. Continue from the preserved conversation constraints and edit history below.";
  return `${normalized}\n\n${preservedEditHistory.trim()}`.trim();
}

export function compactionSummaryFromMetadata(metadata: unknown): string {
  const custom = plainObject(metadata)?.custom;
  const qubit = plainObject(custom)?.qubit;
  const value = plainObject(qubit)?.compactionSummary;
  return typeof value === "string" ? value : "";
}

export function compactionProviderMessage(summary: string, providerName: string): Message | (Message & { role: "developer" }) | null {
  const content = String(summary || "").trim();
  if (!content) return null;
  const role = providerName === "codex" ? "developer" : "system";
  return { role, content: `Compacted prior session context:\n\n${content}`, date: new Date() } as Message & { role: "developer" };
}

function toolResultsById(messages: Message[]): Map<string, ToolResultMessage> {
  const results = new Map<string, ToolResultMessage>();
  for (const message of messages) {
    if (message?.role !== "tool") continue;
    const toolMessage = message as ToolResultMessage;
    if (toolMessage.toolCallId) results.set(toolMessage.toolCallId, toolMessage);
  }
  return results;
}

function collectReadFiles(toolName: string, args: unknown, result: unknown, readFiles: Set<string>): void {
  const source = plainObject(args);
  const payload = resultPayload(result);
  const data = plainObject(payload);
  if (toolName === "readFile" || toolName === "readFileContinuation") {
    addReadFile(readFiles, source.path, source.cwd, source.startLine ?? source.afterLine, source.endLine);
    addReadFile(readFiles, data.path, data.cwd, data.startLine, data.endLine);
    return;
  }
  if (toolName === "readFiles") {
    if (Array.isArray(source.paths)) {
      for (const path of source.paths) addReadFile(readFiles, path, source.cwd, source.startLine, source.endLine);
    }
    if (Array.isArray(data.files)) {
      for (const file of data.files) {
        const fileRecord = plainObject(file);
        addReadFile(readFiles, fileRecord.filename ?? fileRecord.path, source.cwd, fileRecord.startLine, fileRecord.endLine);
      }
    }
    return;
  }
  if (toolName === "multiCall") {
    const calls = Array.isArray(source.calls) ? source.calls : [];
    const results = Array.isArray(data.results) ? data.results : [];
    for (const [index, call] of calls.entries()) {
      const callRecord = plainObject(call);
      const nestedTool = typeof callRecord.tool === "string" ? callRecord.tool : "";
      const resultRecord = plainObject(results[index]);
      collectReadFiles(nestedTool, callRecord.args, { ok: resultRecord.ok, data: resultRecord.data ?? resultRecord.output, error: resultRecord.error }, readFiles);
    }
  }
}

function addReadFile(readFiles: Set<string>, pathValue: unknown, cwdValue?: unknown, start?: unknown, end?: unknown): void {
  if (typeof pathValue !== "string" || !pathValue.trim()) return;
  let label = pathValue.trim();
  if (typeof cwdValue === "string" && cwdValue.trim()) label = `${cwdValue.trim()}/${label}`;
  const line = lineSuffix(start, end);
  readFiles.add(line ? `${label} ${line}` : label);
}

function lineSuffix(start: unknown, end: unknown): string {
  const left = Number(start);
  const right = Number(end);
  if (Number.isFinite(left) && Number.isFinite(right)) return `(lines ${left}-${right})`;
  if (Number.isFinite(left)) return `(from line ${left})`;
  return "";
}

function preservedEditEntry(toolName: string, args: unknown, result: unknown, label: string): string {
  const source = plainObject(args);
  const payload = resultPayload(result);
  const data = plainObject(payload);
  if (toolName === "editFile") {
    return `${label} editFile ${jsonLine({ args: summarizeEditArgs(source), result: summarizeEditResult(data, result) })}`;
  }
  if (toolName === "multiEdit") {
    return `${label} multiEdit ${jsonLine({ args: summarizeMultiEditArgs(source), result: summarizeMultiEditResult(data, result) })}`;
  }
  if (toolName === "multiCall") {
    const calls = Array.isArray(source.calls) ? source.calls : [];
    const results = Array.isArray(data.results) ? data.results : [];
    const editCalls = calls.flatMap((call, index) => {
      const callRecord = plainObject(call);
      const nestedTool = typeof callRecord.tool === "string" ? callRecord.tool : "";
      if (nestedTool !== "editFile") return [];
      const resultRecord = plainObject(results[index]);
      return [{ index, tool: nestedTool, args: summarizeEditArgs(plainObject(callRecord.args)), result: summarizeEditResult(plainObject(resultRecord.data ?? resultRecord.output), resultRecord) }];
    });
    if (editCalls.length === 0) return "";
    return `${label} multiCall editFile calls ${jsonLine({ calls: editCalls })}`;
  }
  return "";
}

function summarizeEditArgs(source: LooseRecord): LooseRecord {
  return compactObject(source, ["path", "operation", "cwd", "approxStartLine", "approxEndLine", "createBackup", "operationMode", "validateContent", "expectedHash"]);
}

function summarizeMultiEditArgs(source: LooseRecord): LooseRecord {
  return {
    ...compactObject(source, ["cwd", "stopOnError", "createBackup", "operationMode", "validateContent"]),
    edits: Array.isArray(source.edits) ? source.edits.slice(0, 40).map((edit) => summarizeEditArgs(plainObject(edit))) : undefined,
  };
}

function summarizeEditResult(data: LooseRecord, raw: unknown): LooseRecord {
  const base = plainObject(raw);
  return compactObject({ ...data, ok: base.ok, error: base.error }, ["ok", "success", "path", "operation", "sizeBytes", "replacements", "message", "backup", "matchStrategy", "lineInfo", "error"]);
}

function summarizeMultiEditResult(data: LooseRecord, raw: unknown): LooseRecord {
  const base = plainObject(raw);
  return {
    ...compactObject({ ...data, ok: base.ok, error: base.error }, ["ok", "success", "message", "applied", "failed", "stoppedEarly", "error"]),
    results: Array.isArray(data.results) ? data.results.slice(0, 40).map((item) => summarizeEditResult(plainObject(item), item)) : undefined,
  };
}

function todoStatus(args: unknown, result: unknown): string {
  const source = plainObject(args);
  const payload = resultPayload(result);
  const data = plainObject(payload);
  const content = typeof data.content === "string" ? data.content : typeof data.contentPreview === "string" ? data.contentPreview : "";
  const lines = content.split(/\r?\n/).filter((line) => /^\s*- \[[ xX-]\]/.test(line)).slice(0, 30);
  const heading = [`todoMd ${typeof source.action === "string" ? source.action : ""}${typeof source.name === "string" ? ` ${source.name}` : ""}`.trim()];
  if (typeof data.message === "string" && data.message.trim()) heading.push(data.message.trim());
  if (lines.length > 0) return [...heading, ...lines].join("\n");
  if (content.trim()) return [...heading, previewText(content, 1600)].join("\n");
  return heading.length > 1 ? heading.join("\n") : "";
}

function firstConversationPreview(messages: Message[]): string {
  for (const message of messages) {
    if (message?.role !== "user" && message?.role !== "assistant") continue;
    if (isCompactionMarker(message as QubitMessage)) continue;
    const content = String(message.content || "").replace(/\s+/g, " ").trim();
    if (content) return previewText(content, 50);
  }
  return "previous chat";
}

function isCompactionMarker(message: QubitMessage): boolean {
  const qubit = plainObject(plainObject(message.metadata)?.qubit);
  return qubit.kind === "compaction_marker";
}

function parseToolResult(content: unknown): unknown {
  if (typeof content !== "string" || !content.trim()) return null;
  try {
    return JSON.parse(content) as unknown;
  } catch {
    return { ok: false, error: content };
  }
}

function resultPayload(result: unknown): unknown {
  const source = plainObject(result);
  if (source.data !== undefined) return source.data;
  if (source.output !== undefined) return source.output;
  return result;
}

function plainObject(value: unknown): LooseRecord {
  return value && typeof value === "object" && !Array.isArray(value) ? value as LooseRecord : {};
}

function compactObject(source: LooseRecord, keys: string[]): LooseRecord {
  const output: LooseRecord = {};
  for (const key of keys) {
    const value = source[key];
    if (value !== undefined && value !== null && value !== "") output[key] = value;
  }
  return output;
}

function jsonLine(value: unknown): string {
  return JSON.stringify(value, (_key, item) => typeof item === "string" ? previewText(item, 1200) : item);
}

function previewText(value: unknown, maxChars: number): string {
  const text = typeof value === "string" ? value : value === undefined || value === null ? "" : JSON.stringify(value);
  return capText(text, maxChars);
}

function capText(text: string, maxChars: number): string {
  const chars = [...String(text || "")];
  return chars.length > maxChars ? `${chars.slice(0, maxChars).join("")}…` : chars.join("");
}

function stringLength(value: unknown): number {
  if (typeof value !== "string") return 0;
  return [...value].length;
}
