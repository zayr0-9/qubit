import * as assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { describe, it } from "node:test";
import { buildCodexWebSocketHeaders, buildCodexWebSocketRequestBody, codexWebSocketUrl, parseCodexWebSocketResponse } from "./responsesWebsocket.js";
import type { CodexWebSocketLike } from "./types.js";

class FakeSocket extends EventEmitter implements CodexWebSocketLike {
  sent: string[] = [];
  closed = false;

  send(data: string, callback?: (error?: Error) => void): void {
    this.sent.push(data);
    callback?.();
  }

  close(): void {
    this.closed = true;
  }
}

describe("Codex responses websocket helpers", () => {
  it("builds websocket responses URLs", () => {
    assert.equal(codexWebSocketUrl("https://chatgpt.com/backend-api/codex"), "wss://chatgpt.com/backend-api/codex/responses");
    assert.equal(codexWebSocketUrl("http://localhost:3000/base/"), "ws://localhost:3000/base/responses");
  });

  it("adds websocket beta header and preserves routing headers", () => {
    const headers = new Headers({ authorization: "Bearer token", accept: "text/event-stream", "session-id": "s1" });
    const wsHeaders = buildCodexWebSocketHeaders(headers);
    assert.equal(wsHeaders.authorization, "Bearer token");
    assert.equal(wsHeaders["session-id"], "s1");
    assert.equal(wsHeaders.accept, undefined);
    assert.equal(wsHeaders["OpenAI-Beta"], "responses_websockets=2026-02-06");
  });

  it("wraps request body as response.create", () => {
    const body = buildCodexWebSocketRequestBody({ model: "gpt", input: [], client_metadata: { existing: "1" } });
    assert.equal(body.type, "response.create");
    assert.equal(body.model, "gpt");
    assert.equal((body.client_metadata as any).existing, "1");
    assert.equal(typeof (body.client_metadata as any)["x-codex-ws-stream-request-start-ms"], "string");
  });

  it("streams text events from a fake websocket", async () => {
    let socket: FakeSocket | undefined;
    const parsedPromise = parseCodexWebSocketResponse({
      baseURL: "https://chatgpt.com/backend-api/codex",
      headers: new Headers({ authorization: "Bearer token" }),
      body: { model: "gpt", input: [], stream: true },
      webSocketFactory: () => {
        socket = new FakeSocket();
        queueMicrotask(() => socket?.emit("open"));
        return socket;
      },
      idleTimeoutMs: 1_000,
    });

    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(socket?.sent.length, 1);
    assert.equal(JSON.parse(socket!.sent[0]).type, "response.create");
    await new Promise((resolve) => setImmediate(resolve));
    socket?.emit("message", JSON.stringify({ type: "response.output_text.delta", delta: "hello" }), false);
    await new Promise((resolve) => setImmediate(resolve));
    socket?.emit("message", JSON.stringify({ type: "response.completed", response: { id: "resp-1", output: [] } }), false);

    const parsed = await parsedPromise;
    assert.equal(parsed.content, "hello");
    assert.equal(parsed.responseId, "resp-1");
    assert.equal(socket?.closed, true);
  });

  it("maps wrapped websocket errors to retryable errors", async () => {
    let socket: FakeSocket | undefined;
    const parsedPromise = parseCodexWebSocketResponse({
      baseURL: "https://chatgpt.com/backend-api/codex",
      headers: new Headers(),
      body: { model: "gpt", input: [], stream: true },
      webSocketFactory: () => {
        socket = new FakeSocket();
        queueMicrotask(() => socket?.emit("open"));
        return socket;
      },
      idleTimeoutMs: 1_000,
    });

    await new Promise((resolve) => setImmediate(resolve));
    socket?.emit("message", JSON.stringify({ type: "error", status: 429, error: { message: "rate limited" } }), false);

    await assert.rejects(parsedPromise, (error: any) => {
      assert.equal(error.status, 429);
      assert.equal(error.retryable, true);
      assert.match(error.message, /rate limited/);
      return true;
    });
  });
});


describe("Codex websocket listener cleanup", () => {
  it("does not accumulate message/error/close listeners across long streams", async () => {
    let socket: FakeSocket | undefined;
    const parsedPromise = parseCodexWebSocketResponse({
      baseURL: "https://chatgpt.com/backend-api/codex",
      headers: new Headers({ authorization: "Bearer token" }),
      body: { model: "gpt", input: [], stream: true },
      webSocketFactory: () => {
        socket = new FakeSocket();
        queueMicrotask(() => socket?.emit("open"));
        return socket;
      },
      idleTimeoutMs: 1_000,
    });

    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(socket?.listenerCount("open"), 0);

    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(socket?.listenerCount("message"), 1);
    assert.equal(socket?.listenerCount("error"), 1);
    assert.equal(socket?.listenerCount("close"), 1);

    for (let i = 0; i < 12; i += 1) {
      assert.equal(socket?.listenerCount("message"), 1);
      assert.equal(socket?.listenerCount("error"), 1);
      assert.equal(socket?.listenerCount("close"), 1);
      socket?.emit("message", JSON.stringify({ type: "response.output_text.delta", delta: String(i % 10) }), false);
      await new Promise((resolve) => setImmediate(resolve));
    }

    assert.equal(socket?.listenerCount("message"), 1);
    assert.equal(socket?.listenerCount("error"), 1);
    assert.equal(socket?.listenerCount("close"), 1);
    socket?.emit("message", JSON.stringify({ type: "response.completed", response: { id: "resp-listeners", output: [] } }), false);

    const parsed = await parsedPromise;
    assert.equal(parsed.content, "012345678901");
    assert.equal(socket?.listenerCount("message"), 0);
    assert.equal(socket?.listenerCount("error"), 0);
    assert.equal(socket?.listenerCount("close"), 0);
    assert.equal(socket?.closed, true);
  });
});
