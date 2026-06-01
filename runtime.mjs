#!/usr/bin/env node
import readline from "node:readline";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createRuntime,
  defineAgent,
  StubProvider,
} from "@hyper-labs/hyper-router";
import { SqliteStorage } from "@hyper-labs/hyper-router/storage/sqlite";
import { GLMProvider } from "@hyper-labs/hyper-router/providers/glm";

const require = createRequire(import.meta.url);
const rootDir = dirname(fileURLToPath(import.meta.url));
const dataDir = join(rootDir, ".qubit");
const storagePath = join(dataDir, "sessions.sqlite");
const indexPath = join(dataDir, "session-index.json");
const defaultSessionId = process.env.QUBIT_SESSION_ID || "qubit-default";
const model = process.env.GLM_MODEL || process.env.QUBIT_MODEL || "glm-4.6";
const useStub = process.env.QUBIT_STUB === "1" || !process.env.ZAI_API_KEY;

await mkdir(dataDir, { recursive: true });

const provider = useStub
  ? new StubProvider()
  : new GLMProvider({
      apiKey: process.env.ZAI_API_KEY,
      endpoint: process.env.GLM_ENDPOINT === "coding" ? "coding" : "general",
      thinking: process.env.GLM_THINKING === "1" ? { type: "enabled" } : { type: "disabled" },
    });

const agent = defineAgent({
  name: "qubit-chat",
  instructions:
    "You are Qubit, a concise terminal coding assistant MVP. Be helpful, direct, and practical. Keep answers brief unless the user asks for detail.",
  model,
});

const storage = new SqliteStorage({
  filePath: storagePath,
  locateFile: (file) => require.resolve(`sql.js/dist/${file}`),
});

const pendingPermissions = new Map();

const runtime = createRuntime({
  agent,
  provider,
  storage,
  toolPermission: {
    defaultMode: process.env.QUBIT_TOOL_PERMISSION_DEFAULT || "ask",
  },
  hooks: {
    async requestToolPermission(request) {
      write({
        type: "tool.permission.request",
        id: request.id,
        sessionId: request.sessionId,
        step: request.step,
        toolCallId: request.toolCallId,
        toolName: request.toolName,
        args: request.args,
        description: request.description,
        inputSchema: request.inputSchema,
        metadata: request.metadata,
      });

      return await new Promise((resolve) => {
        pendingPermissions.set(request.id, resolve);
      });
    },
  },
});

let sessionIndex = await loadSessionIndex();
let activeSessionId = sessionIndex.activeSessionId;

function write(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

write({
  type: "ready",
  sessionId: activeSessionId,
  sessionTitle: activeSession()?.title,
  provider: useStub ? "stub" : "glm",
  model,
  storagePath,
  indexPath,
});

const rl = readline.createInterface({
  input: process.stdin,
  crlfDelay: Infinity,
});

let queue = Promise.resolve();

rl.on("line", (line) => {
  if (handleImmediateLine(line)) return;

  queue = queue.then(() => handleLine(line)).catch((error) => {
    write({ type: "error", error: error instanceof Error ? error.message : String(error) });
  });
});

function handleImmediateLine(line) {
  if (!line.trim()) return true;

  let request;
  try {
    request = JSON.parse(line);
  } catch {
    return false;
  }

  if (request.type !== "tool.permission.response") {
    return false;
  }

  handlePermissionResponse(request);
  return true;
}

async function handleLine(line) {
  if (!line.trim()) return;

  let request;
  try {
    request = JSON.parse(line);
  } catch (error) {
    write({ type: "error", error: `Invalid JSON request: ${error.message}` });
    return;
  }

  if (request.type === "shutdown") {
    write({ type: "shutdown" });
    process.exit(0);
  }

  if (request.type === "tool.permission.response") {
    handlePermissionResponse(request);
    return;
  }

  if (request.type === "session.list") {
    writeSessionList(request.id);
    return;
  }

  if (request.type === "session.new") {
    const session = await createSession({ title: request.title });
    write({ type: "session.created", id: request.id, session, sessionId: activeSessionId, sessionTitle: session.title });
    writeSessionList(request.id);
    return;
  }

  if (request.type === "session.activate") {
    const session = await activateSession(String(request.sessionId || ""));
    if (!session) {
      write({ type: "error", id: request.id, error: `Unknown session: ${request.sessionId}` });
      return;
    }
    write({ type: "session.activated", id: request.id, sessionId: activeSessionId, sessionTitle: session.title, session });
    return;
  }

  if (request.type === "session.messages") {
    const sessionId = String(request.sessionId || activeSessionId);
    const session = sessionIndex.sessions.find((candidate) => candidate.id === sessionId);
    if (!session) {
      write({ type: "error", id: request.id, error: `Unknown session: ${sessionId}` });
      return;
    }

    try {
      const messages = await loadSessionMessages(sessionId);
      write({
        type: "session.messages",
        id: request.id,
        sessionId,
        sessionTitle: session.title,
        messages,
      });
    } catch (error) {
      write({
        type: "error",
        id: request.id,
        sessionId,
        error: `Failed to load session messages: ${error instanceof Error ? error.message : String(error)}`,
      });
    }
    return;
  }

  if (request.type === "session.rename") {
    const session = await renameSession(String(request.sessionId || activeSessionId), String(request.title || ""));
    if (!session) {
      write({ type: "error", id: request.id, error: `Unknown session: ${request.sessionId || activeSessionId}` });
      return;
    }
    write({ type: "session.renamed", id: request.id, sessionId: session.id, sessionTitle: session.title, session });
    return;
  }

  if (request.type !== "chat" || typeof request.input !== "string") {
    write({ type: "error", id: request.id, error: "Expected chat/session command" });
    return;
  }

  const runSessionId = request.sessionId || activeSessionId;
  await ensureSession(runSessionId);
  activeSessionId = runSessionId;
  write({ type: "run_started", id: request.id, sessionId: runSessionId });

  try {
    const result = await runtime.run({
      sessionId: runSessionId,
      input: request.input,
      maxSteps: 4,
    });

    const assistant = [...result.messages].reverse().find((message) => message.role === "assistant");
    await touchSession(runSessionId, {
      title: titleFromInput(request.input),
      messageCount: result.messages.filter((message) => message.role !== "system").length,
    });

    write({
      type: "assistant",
      id: request.id,
      sessionId: runSessionId,
      status: result.status,
      content: assistant?.content || "",
      reasoningContent: assistant?.reasoningContent,
    });
    write({ type: "run_finished", id: request.id, sessionId: runSessionId, status: result.status });
  } catch (error) {
    write({
      type: "error",
      id: request.id,
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

function handlePermissionResponse(request) {
  const resolve = pendingPermissions.get(request.id);
  if (!resolve) return;
  pendingPermissions.delete(request.id);
  resolve(
    request.allow
      ? { type: "allow" }
      : { type: "deny", reason: request.reason || "Denied by user." },
  );
}

async function loadSessionIndex() {
  let parsed;
  try {
    parsed = JSON.parse(await readFile(indexPath, "utf8"));
  } catch (error) {
    parsed = null;
  }

  const now = new Date().toISOString();
  const sessions = Array.isArray(parsed?.sessions) ? parsed.sessions : [];
  let normalized = sessions
    .filter((session) => session && typeof session.id === "string")
    .map((session) => ({
      id: session.id,
      title: session.title || "Untitled chat",
      createdAt: session.createdAt || now,
      updatedAt: session.updatedAt || session.createdAt || now,
      provider: session.provider || (useStub ? "stub" : "glm"),
      model: session.model || model,
      messageCount: Number.isFinite(session.messageCount) ? session.messageCount : 0,
    }));

  if (normalized.length === 0) {
    normalized = [newSession(defaultSessionId, "Default chat", now)];
  }

  const active = parsed?.activeSessionId && normalized.some((session) => session.id === parsed.activeSessionId)
    ? parsed.activeSessionId
    : normalized[0].id;

  const index = { version: 1, activeSessionId: active, sessions: normalized };
  await saveSessionIndex(index);
  return index;
}

async function saveSessionIndex(index = sessionIndex) {
  await mkdir(dataDir, { recursive: true });
  await writeFile(indexPath, `${JSON.stringify(index, null, 2)}\n`);
}

function newSession(id = createSessionId(), title = "New chat", now = new Date().toISOString()) {
  return {
    id,
    title,
    createdAt: now,
    updatedAt: now,
    provider: useStub ? "stub" : "glm",
    model,
    messageCount: 0,
  };
}

async function createSession(options = {}) {
  const title = String(options.title || "New chat").trim() || "New chat";
  const session = newSession(createSessionId(), title);
  sessionIndex.sessions.unshift(session);
  sessionIndex.activeSessionId = session.id;
  activeSessionId = session.id;
  await saveSessionIndex();
  return session;
}

async function ensureSession(sessionId) {
  if (sessionIndex.sessions.some((session) => session.id === sessionId)) return;
  sessionIndex.sessions.unshift(newSession(sessionId, sessionId === defaultSessionId ? "Default chat" : "Imported chat"));
  await saveSessionIndex();
}

async function activateSession(sessionId) {
  const session = sessionIndex.sessions.find((candidate) => candidate.id === sessionId);
  if (!session) return null;
  sessionIndex.activeSessionId = session.id;
  activeSessionId = session.id;
  session.updatedAt = new Date().toISOString();
  await saveSessionIndex();
  return session;
}

async function renameSession(sessionId, title) {
  const session = sessionIndex.sessions.find((candidate) => candidate.id === sessionId);
  if (!session) return null;
  const trimmed = title.trim();
  if (trimmed) session.title = trimmed;
  session.updatedAt = new Date().toISOString();
  await saveSessionIndex();
  return session;
}

async function touchSession(sessionId, patch = {}) {
  const session = sessionIndex.sessions.find((candidate) => candidate.id === sessionId);
  if (!session) return;
  if (patch.title && (!session.title || session.title === "New chat" || session.title === "Default chat" || session.title === "Untitled chat")) {
    session.title = patch.title;
  }
  if (Number.isFinite(patch.messageCount)) {
    session.messageCount = patch.messageCount;
  }
  session.provider = useStub ? "stub" : "glm";
  session.model = model;
  session.updatedAt = new Date().toISOString();
  sessionIndex.activeSessionId = session.id;
  await saveSessionIndex();
}

function activeSession() {
  return sessionIndex.sessions.find((session) => session.id === activeSessionId) || sessionIndex.sessions[0];
}

function writeSessionList(id) {
  write({
    type: "session.list",
    id,
    sessionId: activeSessionId,
    sessionTitle: activeSession()?.title,
    sessions: sessionIndex.sessions,
  });
}

async function loadSessionMessages(sessionId) {
  const messages = await storage.loadMessages(sessionId);
  return messages
    .filter((message) => message?.role === "user" || message?.role === "assistant")
    .map((message) => ({
      role: message.role,
      content: typeof message.content === "string" ? message.content : String(message.content ?? ""),
    }))
    .filter((message) => message.content.trim() !== "");
}

function createSessionId() {
  return `sess_${new Date().toISOString().replace(/[-:.TZ]/g, "").slice(0, 14)}_${Math.random().toString(36).slice(2, 8)}`;
}

function titleFromInput(input) {
  const cleaned = input.replace(/\s+/g, " ").trim();
  if (!cleaned) return "New chat";
  return cleaned.length > 48 ? `${cleaned.slice(0, 45)}...` : cleaned;
}

rl.on("close", () => {
  queue.finally(() => process.exit(0));
});
