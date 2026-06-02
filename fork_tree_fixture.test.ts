import assert from "node:assert/strict";
import test from "node:test";

function buildMessageTreeNodesForSessionsFixture(sessions, rawMessagesBySession) {
  const byId = new Map();
  for (const session of sessions) {
    if (session?.id) byId.set(session.id, session);
  }

  const sessionMessages = new Map();
  for (const session of sessions) {
    sessionMessages.set(session.id, textTreeMessagesFromRawMessages(rawMessagesBySession.get(session.id) || []));
  }

  const rootSessions = sessions
    .filter((session) => !session?.forkedFromSessionId || !byId.has(session.forkedFromSessionId))
    .sort(compareSessionsForTree);
  const childSessions = new Map();
  for (const session of sessions) {
    const parentId = session?.forkedFromSessionId;
    if (!parentId || !byId.has(parentId)) continue;
    const children = childSessions.get(parentId) || [];
    children.push(session);
    childSessions.set(parentId, children);
  }
  for (const children of childSessions.values()) {
    children.sort(compareSessionsForTree);
  }

  const nodesBySession = new Map();
  const globalNodeIds = new Map();
  const visited = new Set();

  const addNode = (session, message, index, parentId, continued = false) => {
    const key = `${session.id}:${index}`;
    const id = `msg:${session.id}:${index}`;
    globalNodeIds.set(key, id);
    const node = {
      id,
      parentId: parentId || "",
      sessionId: session.id,
      sessionTitle: session.title || "Untitled chat",
      role: message.role,
      content: message.content,
      messageIndex: index,
      continued,
    };
    const nodes = nodesBySession.get(session.id) || [];
    nodes.push(node);
    nodesBySession.set(session.id, nodes);
    return id;
  };

  const walkSession = (session, inheritedParentId = "") => {
    if (!session?.id || visited.has(session.id)) return;
    visited.add(session.id);

    const messages = sessionMessages.get(session.id) || [];
    const rawForkIndex = Number.isFinite(session.forkedFromMessageIndex) ? Math.trunc(session.forkedFromMessageIndex) : 0;
    const startsAt = session.forkedFromSessionId && byId.has(session.forkedFromSessionId)
      ? textTreeStartIndexForRawIndex(rawMessagesBySession.get(session.id) || [], rawForkIndex)
      : 0;

    let parentId = inheritedParentId;
    if (session.forkedFromSessionId && byId.has(session.forkedFromSessionId)) {
      parentId = forkParentMessageNodeId(session, byId, rawMessagesBySession, globalNodeIds);
    }

    for (let index = startsAt; index < messages.length; index += 1) {
      parentId = addNode(session, messages[index], index, parentId, index >= startsAt);
    }

    for (const child of childSessions.get(session.id) || []) {
      walkSession(child, parentId);
    }
  };

  for (const root of rootSessions) {
    walkSession(root, "");
  }
  for (const session of sessions) {
    walkSession(session, "");
  }

  return nodesBySession;
}

function forkParentMessageNodeId(session, sessionsById, rawMessagesBySession, globalNodeIds) {
  const parentId = session?.forkedFromSessionId || "";
  if (!parentId) return "";
  const parentRawMessages = rawMessagesBySession.get(parentId) || [];
  const rawForkIndex = Number.isFinite(session.forkedFromMessageIndex) ? Math.trunc(session.forkedFromMessageIndex) : parentRawMessages.length;
  const parentMessageIndex = textTreeParentIndexForRawIndex(parentRawMessages, rawForkIndex);
  if (parentMessageIndex < 0) return "";
  return sharedPrefixMessageNodeId(parentId, parentMessageIndex, sessionsById, rawMessagesBySession, globalNodeIds);
}

function sharedPrefixMessageNodeId(sessionId, messageIndex, sessionsById, rawMessagesBySession, globalNodeIds) {
  const directNodeId = globalNodeIds.get(`${sessionId}:${messageIndex}`);
  if (directNodeId) return directNodeId;

  const session = sessionsById.get(sessionId);
  const parentId = session?.forkedFromSessionId || "";
  if (!parentId || !sessionsById.has(parentId)) return "";

  const rawForkIndex = Number.isFinite(session.forkedFromMessageIndex) ? Math.trunc(session.forkedFromMessageIndex) : 0;
  const copiedTextCount = textTreeStartIndexForRawIndex(rawMessagesBySession.get(sessionId) || [], rawForkIndex);
  if (messageIndex >= copiedTextCount) return "";
  return sharedPrefixMessageNodeId(parentId, messageIndex, sessionsById, rawMessagesBySession, globalNodeIds);
}

function textTreeParentIndexForRawIndex(rawMessages, rawIndex) {
  const target = Math.max(0, Math.min(rawMessages.length, rawIndex));
  let textIndex = -1;
  for (let index = 0; index < target; index += 1) {
    if (treeTextMessageFromRaw(rawMessages[index])) textIndex += 1;
  }
  return textIndex;
}

function textTreeStartIndexForRawIndex(rawMessages, rawIndex) {
  return Math.max(0, textTreeParentIndexForRawIndex(rawMessages, rawIndex) + 1);
}

function textTreeMessagesFromRawMessages(rawMessages) {
  return rawMessages.map((message) => treeTextMessageFromRaw(message)).filter(Boolean);
}

function treeTextMessageFromRaw(message) {
  if (!message || message.role === "system") return null;
  const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
  if (!content.trim()) return null;
  return { role: message.role, content };
}

function compareSessionsForTree(a, b) {
  const left = a?.forkedAt || a?.createdAt || a?.updatedAt || "";
  const right = b?.forkedAt || b?.createdAt || b?.updatedAt || "";
  if (left === right) return String(a?.title || "").localeCompare(String(b?.title || ""));
  return left.localeCompare(right);
}

function message(role, content) {
  return { role, content };
}

function session(id, title, forkedFromSessionId = "", forkedFromMessageIndex = 0, forkedAt = "2026-01-01T00:00:00.000Z") {
  return {
    id,
    title,
    forkedFromSessionId,
    forkedFromMessageIndex,
    forkedAt,
    createdAt: forkedAt,
    updatedAt: forkedAt,
  };
}

test("runtime tree data parents repeated nested /fork 1 branches to the same first message node", () => {
  const root = session("root", "Root", "", 0, "2026-01-01T00:00:00.000Z");
  const forkOne = session("fork-one", "Fork one", "root", 1, "2026-01-01T00:01:00.000Z");
  const forkTwo = session("fork-two", "Fork two", "fork-one", 1, "2026-01-01T00:02:00.000Z");
  const forkThree = session("fork-three", "Fork three", "fork-two", 1, "2026-01-01T00:03:00.000Z");
  const sessions = [root, forkOne, forkTwo, forkThree];
  const rawMessagesBySession = new Map([
    ["root", [
      message("user", "root user 1"),
      message("assistant", "root agent 1"),
      message("user", "root user 2"),
      message("assistant", "root agent 2"),
    ]],
    ["fork-one", [
      message("user", "root user 1"),
      message("user", "fork one user 1"),
      message("assistant", "fork one agent 1"),
      message("user", "fork one user 2"),
      message("assistant", "fork one agent 2"),
    ]],
    ["fork-two", [
      message("user", "root user 1"),
      message("user", "fork two user 1"),
      message("assistant", "fork two agent 1"),
      message("user", "fork two user 2"),
      message("assistant", "fork two agent 2"),
    ]],
    ["fork-three", [
      message("user", "root user 1"),
      message("user", "fork three user 1"),
      message("assistant", "fork three agent 1"),
      message("user", "fork three user 2"),
      message("assistant", "fork three agent 2"),
    ]],
  ]);

  const nodesBySession = buildMessageTreeNodesForSessionsFixture(sessions, rawMessagesBySession);
  const rootNodes = nodesBySession.get("root");
  assert.deepEqual(rootNodes.map((node) => [node.id, node.parentId, node.content]), [
    ["msg:root:0", "", "root user 1"],
    ["msg:root:1", "msg:root:0", "root agent 1"],
    ["msg:root:2", "msg:root:1", "root user 2"],
    ["msg:root:3", "msg:root:2", "root agent 2"],
  ]);

  for (const forkId of ["fork-one", "fork-two", "fork-three"]) {
    const nodes = nodesBySession.get(forkId);
    assert.equal(nodes[0].parentId, "msg:root:0", `${forkId} first divergent node should attach to shared /fork 1 parent`);
    assert.equal(nodes[0].messageIndex, 1, `${forkId} first divergent node should be message index 1`);
    for (let index = 1; index < nodes.length; index += 1) {
      assert.equal(nodes[index].parentId, nodes[index - 1].id, `${forkId} node ${index} should continue its own lineage`);
    }
  }
});


test("runtime tree data makes /fork 1 edited branches root-level instead of inheriting branch tail", () => {
  const root = session("root", "Root", "", 0, "2026-01-01T00:00:00.000Z");
  const editOne = session("edit-one", "Edit one", "root", 0, "2026-01-01T00:01:00.000Z");
  const editTwo = session("edit-two", "Edit two", "edit-one", 0, "2026-01-01T00:02:00.000Z");
  const sessions = [root, editOne, editTwo];
  const rawMessagesBySession = new Map([
    ["root", [
      message("user", "root user 1"),
      message("assistant", "root agent 1"),
      message("user", "root user 2"),
      message("assistant", "root agent 2"),
    ]],
    ["edit-one", [
      message("user", "edited one user 1"),
      message("assistant", "edited one agent 1"),
      message("user", "edited one user 2"),
      message("assistant", "edited one agent 2"),
    ]],
    ["edit-two", [
      message("user", "edited two user 1"),
      message("assistant", "edited two agent 1"),
      message("user", "edited two user 2"),
      message("assistant", "edited two agent 2"),
    ]],
  ]);

  const nodesBySession = buildMessageTreeNodesForSessionsFixture(sessions, rawMessagesBySession);
  assert.equal(nodesBySession.get("edit-one")[0].parentId, "", "first /fork 1 edit should be a root-level sibling, not child of root tail");
  assert.equal(nodesBySession.get("edit-two")[0].parentId, "", "repeated /fork 1 edit should also be root-level, not child of previous edit tail");
});
