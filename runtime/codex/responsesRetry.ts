const DEFAULT_MAX_ATTEMPTS = 3;
const DEFAULT_BASE_DELAY_MS = 500;
const DEFAULT_MAX_DELAY_MS = 4_000;
const RETRYABLE_HTTP_STATUSES = new Set([408, 409, 425, 429, 500, 502, 503, 504]);

export interface CodexRetryOptions {
  maxAttempts?: number;
  baseDelayMs?: number;
  maxDelayMs?: number;
  sleep?: (delayMs: number) => Promise<void>;
  onRetry?: (event: CodexRetryEvent) => void;
}

export interface CodexRetryEvent {
  attempt: number;
  nextAttempt: number;
  maxAttempts: number;
  delayMs: number;
  reason: string;
}

export interface CodexRetriableError extends Error {
  retryable?: boolean;
  status?: number;
  cause?: unknown;
}

export async function withCodexRetry<T>(operation: () => Promise<T>, options: CodexRetryOptions = {}): Promise<T> {
  const maxAttempts = clampPositiveInteger(options.maxAttempts, DEFAULT_MAX_ATTEMPTS);
  const baseDelayMs = clampPositiveInteger(options.baseDelayMs, DEFAULT_BASE_DELAY_MS);
  const maxDelayMs = clampPositiveInteger(options.maxDelayMs, DEFAULT_MAX_DELAY_MS);
  const sleep = options.sleep || sleepMs;
  let lastError: unknown;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await operation();
    } catch (error) {
      lastError = error;
      if (attempt >= maxAttempts || !isCodexRetryableError(error)) {
        throw decorateFinalCodexError(error, attempt, maxAttempts);
      }

      const delayMs = retryDelayMs(attempt, baseDelayMs, maxDelayMs);
      options.onRetry?.({
        attempt,
        nextAttempt: attempt + 1,
        maxAttempts,
        delayMs,
        reason: codexErrorMessage(error),
      });
      await sleep(delayMs);
    }
  }

  throw decorateFinalCodexError(lastError, maxAttempts, maxAttempts);
}

export function createRetryableCodexHttpError(status: number, text?: string): CodexRetriableError {
  const trimmed = text?.trim();
  const error = new Error(`Codex Responses request failed: HTTP ${status}${trimmed ? ` ${safeErrorText(trimmed)}` : ""}`) as CodexRetriableError;
  error.status = status;
  error.retryable = isRetryableCodexHttpStatus(status);
  return error;
}

export function createRetryableCodexWebSocketError(message: string, options: { status?: number; retryable?: boolean; cause?: unknown } = {}): CodexRetriableError {
  const error = new Error(`Codex Responses websocket failed: ${safeErrorText(message)}`) as CodexRetriableError;
  if (typeof options.status === "number") error.status = options.status;
  error.retryable = options.retryable ?? (typeof options.status === "number" ? isRetryableCodexHttpStatus(options.status) : true);
  if (options.cause !== undefined) error.cause = options.cause;
  return error;
}

export function isCodexRetryableError(error: unknown): boolean {
  if (isAbortLikeError(error)) return false;
  const candidate = error as CodexRetriableError | undefined;
  if (candidate?.retryable === true) return true;
  if (typeof candidate?.status === "number") return isRetryableCodexHttpStatus(candidate.status);

  const message = codexErrorMessage(error).toLowerCase();
  return [
    "fetch failed",
    "network error",
    "websocket",
    "socket hang up",
    "connection reset",
    "connection refused",
    "connect timeout",
    "headers timeout",
    "body timeout",
    "terminated",
    "econnreset",
    "etimedout",
    "enotfound",
    "eai_again",
    "econnrefused",
    "und_err_connect_timeout",
    "und_err_headers_timeout",
    "und_err_body_timeout",
    "und_err_socket",
  ].some((needle) => message.includes(needle));
}

export function isRetryableCodexHttpStatus(status: number): boolean {
  return RETRYABLE_HTTP_STATUSES.has(status);
}

export function codexErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    const cause = (error as { cause?: unknown }).cause;
    const causeMessage = cause instanceof Error ? `: ${cause.message}` : "";
    return `${error.message}${causeMessage}`;
  }
  return String(error);
}

function decorateFinalCodexError(error: unknown, attempts: number, maxAttempts: number): Error {
  const message = codexErrorMessage(error);
  if (attempts <= 1) return error instanceof Error ? error : new Error(message);
  const finalError = new Error(`Codex request failed after ${attempts}/${maxAttempts} attempts: ${message}`) as CodexRetriableError;
  if (error instanceof Error) finalError.cause = error;
  const source = error as CodexRetriableError | undefined;
  if (typeof source?.status === "number") finalError.status = source.status;
  if (source?.retryable !== undefined) finalError.retryable = source.retryable;
  return finalError;
}

function isAbortLikeError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return error.name === "AbortError" || error.message.toLowerCase().includes("aborted");
}

function retryDelayMs(attempt: number, baseDelayMs: number, maxDelayMs: number): number {
  return Math.min(maxDelayMs, baseDelayMs * 2 ** Math.max(0, attempt - 1));
}

function clampPositiveInteger(value: unknown, fallback: number): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? Math.floor(numeric) : fallback;
}

function sleepMs(delayMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, delayMs));
}

function safeErrorText(text: string): string {
  return text.slice(0, 1200);
}
