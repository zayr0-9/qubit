import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import test from "node:test";

import { listMdDocuments } from "./mdDocuments.js";

test("listMdDocuments returns user docs before agent plans", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "qubit-md-docs-"));
  try {
    const dataDir = path.join(root, ".qubit");
    await fs.mkdir(path.join(dataDir, "plans"), { recursive: true });
    await fs.mkdir(path.join(dataDir, "user-docs"), { recursive: true });
    await fs.writeFile(path.join(dataDir, "plans", "launch.md"), "# Launch Plan\n", "utf8");
    await fs.writeFile(path.join(dataDir, "user-docs", "notes.md"), "# User Notes\n", "utf8");

    const files = await listMdDocuments(dataDir);

    assert.deepEqual(files.map((file) => `${file.section}/${file.name}`), ["user-docs/notes", "plans/launch"]);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
