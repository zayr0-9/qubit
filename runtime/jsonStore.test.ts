import assert from "node:assert/strict";
import { mkdtemp, readFile, readdir, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { writeJsonAtomic } from "./jsonStore.js";

test("writeJsonAtomic writes formatted JSON and replaces existing target", async () => {
  const root = await mkdtemp(join(tmpdir(), "qubit-json-store-"));
  try {
    const filePath = join(root, "nested", "settings.json");

    await writeJsonAtomic(filePath, { version: 1, value: "old" }, { mode: 0o600 });
    await writeJsonAtomic(filePath, { version: 2, value: "new" }, { mode: 0o600 });

    assert.equal(await readFile(filePath, "utf8"), '{\n  "version": 2,\n  "value": "new"\n}\n');
    const mode = (await stat(filePath)).mode & 0o777;
    if (process.platform !== "win32") assert.equal(mode, 0o600);

    const entries = await readdir(join(root, "nested"));
    assert.deepEqual(entries, ["settings.json"]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
