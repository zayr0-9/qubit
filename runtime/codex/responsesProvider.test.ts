import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { CodexResponsesProvider } from "./responsesProvider.js";
import type { CodexAuthJson, CodexTokenStore } from "./types.js";

const VALID_ACCESS_TOKEN = `header.${Buffer.from(JSON.stringify({ exp: 4_102_444_800 })).toString("base64url")}.signature`;

class MemoryTokenStore implements CodexTokenStore {
  constructor(private readonly auth: CodexAuthJson) {}

  async load() {
    return this.auth;
  }

  async save() {
    return undefined;
  }

  async delete() {
    return undefined;
  }

  async status() {
    return { active: true as const };
  }
}

describe("CodexResponsesProvider", () => {
  it("sends ChatGPT account routing headers and Codex request metadata", async () => {
    let capturedUrl = "";
    let capturedHeaders: Headers | undefined;
    let capturedBody: any;
    const tokenStore = new MemoryTokenStore({
      tokens: {
        access_token: VALID_ACCESS_TOKEN,
        refresh_token: "refresh-token",
        account_id: "workspace-123",
      },
    });
    const provider = new CodexResponsesProvider({
      tokenStore,
      baseURL: "https://chatgpt.com/backend-api/codex",
      fetch: (async (url, init) => {
        capturedUrl = String(url);
        capturedHeaders = new Headers(init?.headers);
        capturedBody = JSON.parse(String(init?.body));
        return new Response([
          "event: response.completed",
          'data: {"type":"response.completed","response":{"id":"resp-1","output":[]}}',
          "",
          "",
        ].join("\n"), { status: 200 });
      }) as typeof fetch,
    });

    await provider.generate({
      model: "gpt-5.2-codex",
      sessionId: "session-1",
      runId: "run-1",
      messages: [{ role: "user", content: "hello" } as any],
      tools: [],
    });

    assert.equal(capturedUrl, "https://chatgpt.com/backend-api/codex/responses");
    assert.equal(capturedHeaders?.get("authorization"), `Bearer ${VALID_ACCESS_TOKEN}`);
    assert.equal(capturedHeaders?.get("ChatGPT-Account-ID"), "workspace-123");
    assert.equal(capturedHeaders?.get("session-id"), "session-1");
    assert.equal(capturedHeaders?.get("thread-id"), "session-1");
    assert.equal(capturedHeaders?.get("x-client-request-id"), "run-1");
    assert.equal(capturedBody.prompt_cache_key, "session-1");
    assert.deepEqual(capturedBody.client_metadata, { "x-codex-installation-id": "session-1" });
  });
});


describe("CodexResponsesProvider reasoning", () => {
  it("returns reasoningContent from completed Codex reasoning output items", async () => {
    const tokenStore = new MemoryTokenStore({
      tokens: {
        access_token: VALID_ACCESS_TOKEN,
        refresh_token: "refresh-token",
      },
    });
    const provider = new CodexResponsesProvider({
      tokenStore,
      baseURL: "https://chatgpt.com/backend-api/codex",
      fetch: (async () => new Response([
        "event: response.completed",
        `data: ${JSON.stringify({
          type: "response.completed",
          response: {
            id: "resp-1",
            output: [
              { type: "reasoning", summary: [{ type: "summary_text", text: "Inspected code and found parser gap." }] },
              { type: "message", content: [{ type: "output_text", text: "Implemented." }] },
            ],
          },
        })}`,
        "",
        "",
      ].join("\n"), { status: 200 })) as typeof fetch,
    });

    const response = await provider.generate({
      model: "gpt-5.2-codex",
      sessionId: "session-1",
      runId: "run-1",
      messages: [{ role: "user", content: "hello" } as any],
      tools: [],
    });

    assert.equal(response.message?.content, "Implemented.");
    assert.equal(response.message?.reasoningContent, "Inspected code and found parser gap.");
  });
});
