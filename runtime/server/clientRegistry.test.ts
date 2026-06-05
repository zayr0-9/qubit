import assert from "node:assert/strict";
import test from "node:test";

import { ClientRegistry, selectedSessionIdForContext } from "./clientRegistry.js";
import type { RuntimeClientTarget } from "./writer.js";

class FakeTarget implements RuntimeClientTarget {
  constructor(public id: string) {}
  write(_line: string): void {}
}

test("ClientRegistry keeps selected sessions isolated per client", () => {
  const registry = new ClientRegistry<FakeTarget>();
  const a = new FakeTarget("a");
  const b = new FakeTarget("b");

  const stateA = registry.add(a, { selectedSessionId: "session-a", now: 1 });
  const stateB = registry.add(b, { selectedSessionId: "session-b", now: 2 });
  stateA.selectedSessionId = "session-a2";

  assert.equal(registry.size, 2);
  assert.equal(registry.stateFor(a)?.selectedSessionId, "session-a2");
  assert.equal(registry.stateFor(b)?.selectedSessionId, "session-b");
  assert.equal(selectedSessionIdForContext({ target: a, clientState: stateA }, "default"), "session-a2");
  assert.equal(selectedSessionIdForContext({ target: null }, "default"), "default");

  assert.deepEqual(registry.remove(a), stateA);
  assert.equal(registry.size, 1);
  assert.equal(registry.stateFor(a), undefined);
  assert.equal(registry.stateFor(b), stateB);
});
