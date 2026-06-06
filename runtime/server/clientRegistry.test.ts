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

test("ClientRegistry can reidentify a reconnecting logical client", () => {
  const registry = new ClientRegistry<FakeTarget>();
  const oldTarget = new FakeTarget("old-socket");
  const newTarget = new FakeTarget("new-socket");

  const oldState = registry.add(oldTarget, { selectedSessionId: "session-a", requestedClientId: "client-a", now: 1 });
  const newState = registry.add(newTarget, { selectedSessionId: oldState.selectedSessionId, requestedClientId: "pending", now: 2 });
  registry.reidentify(newTarget, "client-a");

  assert.equal(registry.targetForClientId("client-a"), newTarget);
  assert.equal(registry.stateFor(oldTarget), undefined);
  assert.equal(registry.stateFor(newTarget)?.clientId, "client-a");
  assert.equal(newState.selectedSessionId, "session-a");
});
