import * as assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { describe, it } from "node:test";
import { QubitSqliteStorage } from "./qubitSqliteStorage.js";

const date = new Date("2026-06-04T00:00:00.000Z");

describe("QubitSqliteStorage message metadata", () => {
  it("persists message metadata and can attach Codex usage to latest assistant", async () => {
    const dir = await mkdtemp(join(tmpdir(), "qubit-storage-metadata-"));
    const storage = new QubitSqliteStorage({ filePath: join(dir, "sessions.sqlite") });

    await storage.saveMessages("session-1", [
      { role: "user", content: "hello", date },
      { role: "assistant", content: "first", date },
      { role: "assistant", content: "second", date },
    ]);

    await storage.attachLatestAssistantMetadata("session-1", {
      codexUsage: {
        inputTokens: 20,
        cachedTokens: 12,
        outputTokens: 8,
        model: "gpt-5.2-codex",
      },
    });

    const loaded = await storage.loadMessages("session-1") as Array<{ role: string; metadata?: Record<string, unknown> }>;
    assert.equal(loaded[1]?.metadata, undefined);
    assert.deepEqual(loaded[2]?.metadata?.codexUsage, {
      inputTokens: 20,
      cachedTokens: 12,
      outputTokens: 8,
      model: "gpt-5.2-codex",
    });

    storage.close();
  });
});
