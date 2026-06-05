import assert from "node:assert/strict";
import test from "node:test";

import { broadcastTo, writeTo, type RuntimeClientTarget } from "./writer.js";

class FakeTarget implements RuntimeClientTarget {
  lines: string[] = [];
  constructor(public id: string) {}
  write(line: string): void {
    this.lines.push(line);
  }
  messages(): unknown[] {
    return this.lines.map((line) => JSON.parse(line));
  }
}

test("writeTo delivers request scoped response only to target", () => {
  const a = new FakeTarget("a");
  const b = new FakeTarget("b");

  assert.equal(writeTo(a, { type: "session.list", apiKey: "secret" }, { redactor: (message) => ({ ...(message as object), apiKey: "[redacted]" }), serverMode: true }), true);

  assert.deepEqual(a.messages(), [{ type: "session.list", apiKey: "[redacted]" }]);
  assert.deepEqual(b.messages(), []);
});

test("writeTo drops untargeted server-mode writes", () => {
  const logs: string[] = [];
  const wrote = writeTo(null, { type: "ready" }, { serverMode: true, logger: (line) => logs.push(line) });

  assert.equal(wrote, false);
  assert.match(logs[0] || "", /dropped untargeted message type=ready/);
});

test("broadcastTo writes redacted message to every client", () => {
  const a = new FakeTarget("a");
  const b = new FakeTarget("b");

  const count = broadcastTo([a, b], { type: "notice", token: "secret" }, { redactor: (message) => ({ ...(message as object), token: "[redacted]" }), serverMode: true });

  assert.equal(count, 2);
  assert.deepEqual(a.messages(), [{ type: "notice", token: "[redacted]" }]);
  assert.deepEqual(b.messages(), [{ type: "notice", token: "[redacted]" }]);
});
