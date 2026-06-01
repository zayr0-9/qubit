import type { ModelProvider, ModelResponse } from "@hyper-labs/hyper-router";
import { CODEX_BASE_URL, CODEX_CLIENT_ID, CODEX_ORIGINATOR, type CodexGenerateInput, type CodexResponsesProviderOptions } from "./types.js";
import { getCodexBearerToken } from "./auth.js";
import { toCodexRequestParts } from "./responsesItems.js";
import { parseCodexSseResponse } from "./responsesSse.js";

export class CodexResponsesProvider implements ModelProvider {
  private readonly options: CodexResponsesProviderOptions;
  private readonly fetchImpl: typeof fetch;

  constructor(options: CodexResponsesProviderOptions) {
    this.options = options;
    this.fetchImpl = options.fetch || fetch;
  }

  async generate(input: CodexGenerateInput): Promise<ModelResponse> {
    input.signal?.throwIfAborted();
    const bearer = await getCodexBearerToken(this.options);
    const parts = toCodexRequestParts(input.messages, input.tools);
    const body = {
      model: input.model,
      ...(parts.instructions ? { instructions: parts.instructions } : {}),
      input: parts.input,
      ...(parts.tools.length ? { tools: parts.tools, tool_choice: "auto", parallel_tool_calls: true } : {}),
      store: false,
      stream: true,
      include: [],
    };
    const headers = new Headers({
      accept: "text/event-stream",
      "content-type": "application/json",
      authorization: `Bearer ${bearer}`,
      originator: this.options.originator || CODEX_ORIGINATOR,
      "user-agent": this.options.userAgent || "Qubit/0.1 Codex",
    });
    const requestId = input.runId || input.sessionId || `qubit-${Date.now()}`;
    if (input.sessionId) {
      headers.set("session-id", input.sessionId);
      headers.set("thread-id", input.sessionId);
    }
    headers.set("x-client-request-id", requestId);

    const response = await this.fetchImpl(this.responsesUrl(), {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      ...(input.signal ? { signal: input.signal } : {}),
    });
    const parsed = await parseCodexSseResponse(response);
    input.signal?.throwIfAborted();
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
    return modelResponse;
  }

  private responsesUrl(): string {
    return `${(this.options.baseURL || CODEX_BASE_URL).replace(/\/$/, "")}/responses`;
  }
}
