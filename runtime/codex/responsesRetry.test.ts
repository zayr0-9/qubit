import * as assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createRetryableCodexHttpError, isCodexRetryableError, withCodexRetry } from "./responsesRetry.js";

class AbortError extends Error {
  constructor() {
    super("The operation was aborted");
    this.name = "AbortError";
  }
}

describe("Codex response retry helpers", () => {
  it("retries transient fetch failures", async () => {
    let calls = 0;
    const retryEvents: unknown[] = [];

    const result = await withCodexRetry(async () => {
      calls += 1;
      if (calls === 1) throw new TypeError("fetch failed");
      return "ok";
    }, {
      sleep: async () => undefined,
      onRetry: (event) => retryEvents.push(event),
    });

    assert.equal(result, "ok");
    assert.equal(calls, 2);
    assert.equal(retryEvents.length, 1);
  });

  it("retries retryable HTTP errors", async () => {
    let calls = 0;

    const result = await withCodexRetry(async () => {
      calls += 1;
      if (calls < 3) throw createRetryableCodexHttpError(502, "bad gateway");
      return "ok";
    }, { sleep: async () => undefined });

    assert.equal(result, "ok");
    assert.equal(calls, 3);
  });

  it("does not retry auth or bad request HTTP errors", async () => {
    let calls = 0;

    await assert.rejects(
      () => withCodexRetry(async () => {
        calls += 1;
        throw createRetryableCodexHttpError(401, "unauthorized");
      }, { sleep: async () => undefined }),
      /HTTP 401/
    );

    assert.equal(calls, 1);
  });

  it("does not retry abort errors", async () => {
    let calls = 0;

    await assert.rejects(
      () => withCodexRetry(async () => {
        calls += 1;
        throw new AbortError();
      }, { sleep: async () => undefined }),
      /aborted/
    );

    assert.equal(calls, 1);
  });

  it("decorates final retry exhaustion error", async () => {
    await assert.rejects(
      () => withCodexRetry(async () => {
        throw new TypeError("fetch failed");
      }, { maxAttempts: 2, sleep: async () => undefined }),
      /Codex request failed after 2\/2 attempts: fetch failed/
    );
  });

  it("classifies common undici transport errors as retryable", () => {
    assert.equal(isCodexRetryableError(new Error("UND_ERR_CONNECT_TIMEOUT")), true);
    assert.equal(isCodexRetryableError(new Error("socket hang up")), true);
    assert.equal(isCodexRetryableError(new Error("validation failed")), false);
  });
});
