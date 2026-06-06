import WebSocket from "ws";
import type { CodexResponseParseOptions, CodexResponseParseResult, CodexWebSocketFactory, CodexWebSocketLike } from "./types.js";
import { codexResponseParseResult, createCodexResponseParseState, processCodexResponseEventText } from "./responsesEvents.js";
import { createRetryableCodexWebSocketError } from "./responsesRetry.js";

const RESPONSES_WEBSOCKETS_V2_BETA_HEADER_VALUE = "responses_websockets=2026-02-06";
const WEBSOCKET_CONNECTION_LIMIT_REACHED_CODE = "websocket_connection_limit_reached";
const DEFAULT_CONNECT_TIMEOUT_MS = 10_000;
const DEFAULT_IDLE_TIMEOUT_MS = 120_000;

export type CodexWebSocketRequestOptions = CodexResponseParseOptions & {
  baseURL: string;
  headers: Headers;
  body: Record<string, unknown>;
  signal?: AbortSignal;
  webSocketFactory?: CodexWebSocketFactory;
  connectTimeoutMs?: number;
  idleTimeoutMs?: number;
};

export function codexWebSocketUrl(baseURL: string): string {
  const url = new URL(`${baseURL.replace(/\/$/, "")}/responses`);
  if (url.protocol === "https:") url.protocol = "wss:";
  else if (url.protocol === "http:") url.protocol = "ws:";
  return url.toString();
}

export function buildCodexWebSocketHeaders(headers: Headers): Record<string, string> {
  const result: Record<string, string> = {};
  headers.forEach((value, key) => {
    if (key.toLowerCase() !== "accept") result[key] = value;
  });
  result["OpenAI-Beta"] = RESPONSES_WEBSOCKETS_V2_BETA_HEADER_VALUE;
  return result;
}

export function buildCodexWebSocketRequestBody(body: Record<string, unknown>): Record<string, unknown> {
  const clientMetadata = {
    ...((body.client_metadata && typeof body.client_metadata === "object" && !Array.isArray(body.client_metadata)) ? body.client_metadata as Record<string, unknown> : {}),
    "x-codex-ws-stream-request-start-ms": String(Date.now()),
  };
  return {
    type: "response.create",
    ...body,
    client_metadata: clientMetadata,
  };
}

export async function parseCodexWebSocketResponse(options: CodexWebSocketRequestOptions): Promise<CodexResponseParseResult> {
  const socket = await connectCodexWebSocket(options);
  let completed = false;
  const state = createCodexResponseParseState({ onReasoningDelta: options.onReasoningDelta });

  try {
    const requestBody = buildCodexWebSocketRequestBody(options.body);
    await sendWithTimeout(socket, JSON.stringify(requestBody), idleTimeoutMs(options.idleTimeoutMs), options.signal);
    while (!completed) {
      const message = await nextMessage(socket, idleTimeoutMs(options.idleTimeoutMs), options.signal);
      if (message.kind === "close") {
        throw createRetryableCodexWebSocketError(`websocket closed by server before response.completed${message.reason ? `: ${message.reason}` : ""}`, { retryable: true });
      }
      if (message.kind === "error") throw createRetryableCodexWebSocketError(message.error.message, { retryable: true, cause: message.error });
      if (message.isBinary) throw createRetryableCodexWebSocketError("unexpected binary websocket event", { retryable: false });

      const text = message.text;
      const wrappedError = parseWrappedWebSocketError(text);
      if (wrappedError) throw mapWrappedWebSocketError(wrappedError, text);

      let eventType = "";
      try {
        eventType = JSON.parse(text)?.type || "";
      } catch {
        // Shared parser ignores malformed events; keep doing that for WS non-error payloads.
      }
      processCodexResponseEventText(text, state);
      if (eventType === "response.completed") completed = true;
    }
    return codexResponseParseResult(state);
  } finally {
    closeSocket(socket);
  }
}

async function connectCodexWebSocket(options: CodexWebSocketRequestOptions): Promise<CodexWebSocketLike> {
  const factory = options.webSocketFactory || defaultWebSocketFactory;
  const socket = factory({
    url: codexWebSocketUrl(options.baseURL),
    headers: buildCodexWebSocketHeaders(options.headers),
  });
  if (options.signal?.aborted) {
    closeSocket(socket);
    options.signal.throwIfAborted();
  }

  await new Promise<void>((resolve, reject) => {
    let settled = false;
    const timeout = setTimeout(() => finish(createRetryableCodexWebSocketError("websocket connect timeout", { retryable: true })), connectTimeoutMs(options.connectTimeoutMs));
    const onAbort = () => finish(abortError());
    const onOpen = () => finish();
    const onError = (error: Error) => finish(createRetryableCodexWebSocketError(error.message, { retryable: true, cause: error }));
    const finish = (error?: unknown) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      options.signal?.removeEventListener("abort", onAbort);
      socket.off("open", onOpen);
      socket.off("error", onError);
      if (error) {
        closeSocket(socket);
        reject(error);
      } else {
        resolve();
      }
    };
    options.signal?.addEventListener("abort", onAbort, { once: true });
    socket.on("open", onOpen);
    socket.on("error", onError);
  });
  return socket;
}

function defaultWebSocketFactory(options: { url: string; headers: Record<string, string> }): CodexWebSocketLike {
  return new WebSocket(options.url, { headers: options.headers, perMessageDeflate: true }) as unknown as CodexWebSocketLike;
}

type SocketMessage =
  | { kind: "message"; text: string; isBinary: boolean }
  | { kind: "error"; error: Error }
  | { kind: "close"; code: number; reason: string };

function nextMessage(socket: CodexWebSocketLike, timeoutMs: number, signal?: AbortSignal): Promise<SocketMessage> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const timeout = setTimeout(() => finish(undefined, createRetryableCodexWebSocketError("idle timeout waiting for websocket", { retryable: true })), timeoutMs);
    const onAbort = () => finish(undefined, abortError());
    const onMessage = (data: unknown, isBinary?: boolean) => finish({ kind: "message", text: socketDataToText(data), isBinary: Boolean(isBinary) });
    const onError = (error: Error) => finish({ kind: "error", error });
    const onClose = (code: number, reason: Buffer) => finish({ kind: "close", code, reason: reason?.toString("utf8") || "" });
    const finish = (message?: SocketMessage, error?: unknown) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      signal?.removeEventListener("abort", onAbort);
      socket.off("message", onMessage);
      socket.off("error", onError);
      socket.off("close", onClose);
      if (error) reject(error);
      else resolve(message!);
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    socket.on("message", onMessage);
    socket.on("error", onError);
    socket.on("close", onClose);
  });
}

function sendWithTimeout(socket: CodexWebSocketLike, data: string, timeoutMs: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const timeout = setTimeout(() => finish(createRetryableCodexWebSocketError("idle timeout sending websocket request", { retryable: true })), timeoutMs);
    const onAbort = () => finish(abortError());
    const finish = (error?: unknown) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      signal?.removeEventListener("abort", onAbort);
      if (error) reject(error);
      else resolve();
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    try {
      socket.send(data, (error) => finish(error ? createRetryableCodexWebSocketError(error.message, { retryable: true, cause: error }) : undefined));
    } catch (error) {
      finish(createRetryableCodexWebSocketError(error instanceof Error ? error.message : String(error), { retryable: true, cause: error }));
    }
  });
}

function parseWrappedWebSocketError(payload: string): any | null {
  try {
    const parsed = JSON.parse(payload);
    return parsed?.type === "error" ? parsed : null;
  } catch {
    return null;
  }
}

function mapWrappedWebSocketError(event: any, originalPayload: string): Error {
  const status = Number(event.status ?? event.status_code);
  const code = typeof event.error?.code === "string" ? event.error.code : "";
  if (code === WEBSOCKET_CONNECTION_LIMIT_REACHED_CODE) {
    return createRetryableCodexWebSocketError("Responses websocket connection limit reached. Create a new websocket connection to continue.", { retryable: true, status });
  }
  const message = typeof event.error?.message === "string" ? event.error.message : originalPayload;
  return createRetryableCodexWebSocketError(`websocket error${Number.isFinite(status) ? ` HTTP ${status}` : ""}: ${message}`, {
    status: Number.isFinite(status) ? status : undefined,
  });
}

function socketDataToText(data: unknown): string {
  if (typeof data === "string") return data;
  if (Buffer.isBuffer(data)) return data.toString("utf8");
  if (Array.isArray(data)) return Buffer.concat(data as Buffer[]).toString("utf8");
  if (data instanceof ArrayBuffer) return Buffer.from(data).toString("utf8");
  return String(data);
}

function closeSocket(socket: CodexWebSocketLike): void {
  try {
    socket.close();
  } catch {
    try {
      socket.terminate?.();
    } catch {
      // ignore close failures
    }
  }
}

function abortError(): Error {
  return new DOMException("Aborted", "AbortError");
}

function connectTimeoutMs(value: unknown): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? numeric : DEFAULT_CONNECT_TIMEOUT_MS;
}

function idleTimeoutMs(value: unknown): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? numeric : DEFAULT_IDLE_TIMEOUT_MS;
}
