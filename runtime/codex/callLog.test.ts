import * as assert from "node:assert/strict";
import { mkdtemp, readFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { describe, it } from "node:test";
import { CodexCallLogWriter } from "./callLog.js";

describe("CodexCallLogWriter", () => {
  it("writes JSON lines and preserves token counts while redacting secrets", async () => {
    const dir = await mkdtemp(join(tmpdir(), "qubit-codex-log-"));
    const path = join(dir, "codex-provider-calls.log");
    const writer = new CodexCallLogWriter(path);

    await writer.append({
      callId: "call-1",
      runId: "run-1",
      sessionId: "session-1",
      provider: "codex",
      model: "gpt-5.2-codex",
      requestId: "run-1",
      promptCacheKey: "session-1",
      status: "completed",
      startedAt: "2026-06-03T00:00:00.000Z",
      finishedAt: "2026-06-03T00:00:00.010Z",
      durationMs: 10,
      request: {
        input: [{ type: "message", content: [{ type: "input_text", text: "hello" }] }],
        access_token: "secret-token",
      },
      usage: {
        input_tokens: 20,
        input_tokens_details: { cached_tokens: 12 },
        output_tokens: 8,
      },
    });

    const lines = (await readFile(path, "utf8")).trim().split("\n");
    assert.equal(lines.length, 1);
    const parsed = JSON.parse(lines[0]!);
    assert.equal(parsed.request.access_token, "[redacted]");
    assert.equal(parsed.usage.input_tokens, 20);
    assert.equal(parsed.usage.input_tokens_details.cached_tokens, 12);
    assert.equal(parsed.usage.output_tokens, 8);
  });
});
