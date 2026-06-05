import { randomUUID } from "node:crypto";
import type { ModelProvider, ModelResponse } from "@hyper-labs/hyper-router";
import { CODEX_BASE_URL, CODEX_ORIGINATOR, type CodexGenerateInput, type CodexProviderCallLogEvent, type CodexResponsesProviderOptions, type CodexResponsesTransport } from "./types.js";
import { getCodexAuthContext } from "./auth.js";
import { toCodexRequestParts } from "./responsesItems.js";
import { parseCodexSseResponse } from "./responsesSse.js";
import { parseCodexWebSocketResponse } from "./responsesWebsocket.js";
import { codexErrorMessage, isCodexRetryableError, withCodexRetry } from "./responsesRetry.js";

export class CodexResponsesProvider implements ModelProvider {
  private readonly options: CodexResponsesProviderOptions;
  private readonly fetchImpl: typeof fetch;

  constructor(options: CodexResponsesProviderOptions) {
    this.options = options;
    this.fetchImpl = options.fetch || fetch;
  }

  async generate(input: CodexGenerateInput): Promise<ModelResponse> {
    input.signal?.throwIfAborted();
    const auth = await getCodexAuthContext(this.options);
    const parts = toCodexRequestParts(input.messages, input.tools);
    const requestId = input.runId || input.sessionId || `qubit-${Date.now()}`;
    const promptCacheKey = input.sessionId || requestId;
    const callId = randomUUID();
    const startedAtMs = Date.now();
    const startedAt = new Date(startedAtMs).toISOString();
    const body = {
      model: input.model,
      ...(parts.instructions ? { instructions: parts.instructions } : {}),
      input: parts.input,
      ...(parts.tools.length ? { tools: parts.tools, tool_choice: "auto", parallel_tool_calls: true } : {}),
      reasoning: {
        effort: this.options.reasoningEffort || "medium",
        ...(this.options.reasoningSummary === null ? {} : { summary: this.options.reasoningSummary || "auto" }),
      },
      store: false,
      stream: true,
      include: ["reasoning.encrypted_content", "web_search_call.action.sources"],
      prompt_cache_key: promptCacheKey,
      client_metadata: {
        "x-codex-installation-id": promptCacheKey,
      },
    };
    const headers = new Headers({
      accept: "text/event-stream",
      "content-type": "application/json",
      authorization: `Bearer ${auth.accessToken}`,
      originator: this.options.originator || CODEX_ORIGINATOR,
      "user-agent": this.options.userAgent || "Qubit/0.1 Codex",
    });
    if (auth.accountId) headers.set("ChatGPT-Account-ID", auth.accountId);
    if (input.sessionId) {
      headers.set("session-id", input.sessionId);
      headers.set("thread-id", input.sessionId);
    }
    headers.set("x-client-request-id", requestId);

    const debugReasoning = process.env.QUBIT_CODEX_REASONING_DEBUG === "1";
    if (debugReasoning) {
      console.error(`[codex-reasoning] request model=${input.model || "unknown"} reasoningEffort=${body.reasoning.effort} reasoningSummary=${"summary" in body.reasoning ? body.reasoning.summary : "disabled"} include=${JSON.stringify(body.include)}`);
    }
    try {
      let completedTransport: "http" | "websocket" = "http";
      const parsed = await withCodexRetry(async () => {
        const result = await this.generateWithTransport(input, headers, body);
        completedTransport = result.transport;
        return result.parsed;
      }, {
        onRetry: (event) => {
          this.logRetry(input, event.nextAttempt, event.maxAttempts, event.delayMs, event.reason);
        },
      });
      input.signal?.throwIfAborted();
      if (debugReasoning) {
        console.error(`[codex-reasoning] parsed reasoningChars=${parsed.reasoningContent?.length || 0} outputChars=${parsed.content?.length || 0} toolCalls=${parsed.toolCalls.length} stop=${parsed.providerStopReason || ""}`);
      }
      const modelResponse: ModelResponse = {
        toolCalls: parsed.toolCalls,
        stopReason: parsed.toolCalls.length > 0 ? "tool_calls" : "stop",
        ...(parsed.providerStopReason ? { providerStopReason: parsed.providerStopReason } : {}),
        ...(parsed.generatedImages?.length ? { generatedImages: parsed.generatedImages } : {}),
      };
      if (parsed.content || parsed.reasoningContent || parsed.toolCalls.length > 0) {
        modelResponse.message = {
          role: "assistant",
          content: parsed.content,
          date: new Date(),
          ...(parsed.reasoningContent ? { reasoningContent: parsed.reasoningContent } : {}),
          ...(parsed.toolCalls.length > 0 ? { toolCalls: parsed.toolCalls } : {}),
        };
      }
      await this.emitCallLog({
        callId,
        runId: input.runId,
        sessionId: input.sessionId,
        provider: "codex",
        model: input.model,
        requestId,
        promptCacheKey,
        status: "completed",
        transport: completedTransport,
        startedAt,
        ...this.finishedTiming(startedAtMs),
        ...(parsed.responseId ? { responseId: parsed.responseId } : {}),
        ...(parsed.usage !== undefined ? { usage: parsed.usage } : {}),
        result: {
          contextMessageCount: input.messages.length,
          contentChars: parsed.content.length,
          reasoningChars: parsed.reasoningContent?.length || 0,
          toolCallCount: parsed.toolCalls.length,
          generatedImageCount: parsed.generatedImages?.length || 0,
          stopReason: modelResponse.stopReason,
          ...(parsed.providerStopReason ? { providerStopReason: parsed.providerStopReason } : {}),
        },
      });
      return modelResponse;
    } catch (error) {
      await this.emitCallLog({
        callId,
        runId: input.runId,
        sessionId: input.sessionId,
        provider: "codex",
        model: input.model,
        requestId,
        promptCacheKey,
        status: input.signal?.aborted || isAbortError(error) ? "cancelled" : "failed",
        startedAt,
        ...this.finishedTiming(startedAtMs),
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private async generateWithTransport(input: CodexGenerateInput, headers: Headers, body: Record<string, unknown>): Promise<{ parsed: Awaited<ReturnType<typeof parseCodexSseResponse>>; transport: "http" | "websocket" }> {
    input.signal?.throwIfAborted();
    const transport = this.resolveTransport();
    if (transport === "http") return { parsed: await this.generateHttp(input, headers, body), transport: "http" };
    if (transport === "websocket") return { parsed: await this.generateWebSocket(input, headers, body), transport: "websocket" };

    try {
      return { parsed: await this.generateWebSocket(input, headers, body), transport: "websocket" };
    } catch (error) {
      input.signal?.throwIfAborted();
      if (!isCodexRetryableError(error)) throw error;
      console.error(`[codex-retry] websocket failed; falling back to HTTP model=${input.model || "unknown"} runId=${input.runId || ""} sessionId=${input.sessionId || ""} reason=${codexErrorMessage(error).replace(/\s+/g, " ").slice(0, 300)}`);
      return { parsed: await this.generateHttp(input, headers, body), transport: "http" };
    }
  }

  private async generateHttp(input: CodexGenerateInput, headers: Headers, body: Record<string, unknown>): Promise<Awaited<ReturnType<typeof parseCodexSseResponse>>> {
    input.signal?.throwIfAborted();
    const response = await this.fetchImpl(this.responsesUrl(), {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      ...(input.signal ? { signal: input.signal } : {}),
    });
    return await parseCodexSseResponse(response, {
      onReasoningDelta: (delta) => this.options.onReasoningDelta?.({ sessionId: input.sessionId, runId: input.runId, delta }),
    });
  }

  private async generateWebSocket(input: CodexGenerateInput, headers: Headers, body: Record<string, unknown>): Promise<Awaited<ReturnType<typeof parseCodexSseResponse>>> {
    return await parseCodexWebSocketResponse({
      baseURL: this.baseURL(),
      headers,
      body,
      ...(input.signal ? { signal: input.signal } : {}),
      ...(this.options.webSocketFactory ? { webSocketFactory: this.options.webSocketFactory } : {}),
      ...(this.options.websocketConnectTimeoutMs ? { connectTimeoutMs: this.options.websocketConnectTimeoutMs } : {}),
      ...(this.options.websocketIdleTimeoutMs ? { idleTimeoutMs: this.options.websocketIdleTimeoutMs } : {}),
      onReasoningDelta: (delta) => this.options.onReasoningDelta?.({ sessionId: input.sessionId, runId: input.runId, delta }),
    });
  }

  private resolveTransport(): CodexResponsesTransport {
    const value = (this.options.transport || process.env.QUBIT_CODEX_TRANSPORT || "auto").toLowerCase();
    return value === "http" || value === "websocket" || value === "auto" ? value : "auto";
  }

  private baseURL(): string {
    return (this.options.baseURL || CODEX_BASE_URL).replace(/\/$/, "");
  }

  private responsesUrl(): string {
    return `${this.baseURL()}/responses`;
  }

  private logRetry(input: CodexGenerateInput, attempt: number, maxAttempts: number, delayMs: number, reason: string): void {
    const safeReason = codexErrorMessage(reason).replace(/\s+/g, " ").slice(0, 300);
    console.error(`[codex-retry] retrying request attempt=${attempt}/${maxAttempts} delayMs=${delayMs} model=${input.model || "unknown"} runId=${input.runId || ""} sessionId=${input.sessionId || ""} reason=${safeReason}`);
  }

  private finishedTiming(startedAtMs: number): Pick<CodexProviderCallLogEvent, "finishedAt" | "durationMs"> {
    const finishedAtMs = Date.now();
    return {
      finishedAt: new Date(finishedAtMs).toISOString(),
      durationMs: finishedAtMs - startedAtMs,
    };
  }

  private async emitCallLog(event: CodexProviderCallLogEvent): Promise<void> {
    try {
      await this.options.onCallLog?.(event);
    } catch (error) {
      console.error(`[codex-call-log] failed to write call log: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}
