import test from "node:test";
import assert from "node:assert/strict";

import { overlayActiveRunUserMessages } from "./activeRunOverlay.ts";

test("overlayActiveRunUserMessages appends matching active input", () => {
  const messages = overlayActiveRunUserMessages(
    [{ role: "user", content: "old question" }],
    ["latest question"],
  );

  assert.deepEqual(messages.map((message) => message.content), ["old question", "latest question"]);
  assert.equal(messages.at(-1)?.role, "user");
});

test("overlayActiveRunUserMessages ignores empty active inputs", () => {
  const messages = overlayActiveRunUserMessages(
    [{ role: "user", content: "old question" }],
    ["  "],
  );

  assert.deepEqual(messages.map((message) => message.content), ["old question"]);
});

test("overlayActiveRunUserMessages does not duplicate already persisted input", () => {
  const messages = overlayActiveRunUserMessages(
    [{ role: "user", content: "latest question" }],
    ["latest question"],
  );

  assert.equal(messages.length, 1);
  assert.equal(messages[0]?.content, "latest question");
});

test("overlayActiveRunUserMessages treats array content as duplicate text", () => {
  const messages = overlayActiveRunUserMessages(
    [{ role: "user", content: [{ text: "latest question" }] }],
    ["latest question"],
  );

  assert.equal(messages.length, 1);
});
