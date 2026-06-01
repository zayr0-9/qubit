#!/usr/bin/env node
// @ts-nocheck
import readline from "node:readline";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createRuntime,
  defineAgent,
  StubProvider,
} from "@hyper-labs/hyper-router";
import { SqliteStorage } from "@hyper-labs/hyper-router/storage/sqlite";
import { GLMProvider } from "@hyper-labs/hyper-router/providers/glm";

const require = createRequire(import.meta.url);
const entryDir = dirname(fileURLToPath(import.meta.url));
const rootDir = basename(entryDir) === "dist" ? dirname(entryDir) : entryDir;
const dataDir = join(rootDir, ".qubit");
const storagePath = join(dataDir, "sessions.sqlite");
const indexPath = join(dataDir, "session-index.json");
const keyIndexPath = join(dataDir, "api-key-index.json");
const defaultSessionId = process.env.QUBIT_SESSION_ID || "qubit-default";
const model = process.env.GLM_MODEL || process.env.QUBIT_MODEL || "glm-4.6";
const keychainService = process.env.QUBIT_KEYCHAIN_SERVICE || "Qubit";
const envGLMAlias = "env:ZAI_API_KEY";

if (process.argv.includes("--check")) {
  console.log("runtime check ok");
  process.exit(0);
}

await mkdir(dataDir, { recursive: true });

let keytar = null;
let keytarLoadError = null;
try {
  const keytarModule = await import("keytar");
  keytar = keytarModule.default ?? keytarModule;
} catch (error) {
  keytarLoadError = error;
}

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
const hooks = {
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
};

let apiKeyIndex = await loadApiKeyIndex();
let runtimeState = await createRuntimeState();
let sessionIndex = await loadSessionIndex();
let activeSessionId = sessionIndex.activeSessionId;

function write(message) {
  process.stdout.write(`${JSON.stringify(redactMessage(message))}\n`);
}

write({
  type: "ready",
  sessionId: activeSessionId,
  sessionTitle: activeSession()?.title,
  provider: runtimeState.providerName,
  activeProvider: runtimeState.providerName,
  activeKeyAlias: runtimeState.keyAlias,
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
    write({ type: "error", error: redactSecrets(error instanceof Error ? error.message : String(error)) });
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

  if (request.type === "key.list") {
    await writeKeyList(request.id);
    return;
  }

  if (request.type === "key.set") {
    await setApiKey({ provider: request.provider, alias: request.alias, apiKey: request.apiKey });
    runtimeState = await createRuntimeState();
    await writeKeyUpdated(request.id, `Saved and activated ${request.provider || "glm"}/${request.alias || ""}.`);
    return;
  }

  if (request.type === "key.use") {
    await activateApiKey({ provider: request.provider, alias: request.alias });
    runtimeState = await createRuntimeState();
    await writeKeyUpdated(request.id, `Activated ${request.provider || "glm"}/${request.alias || ""}.`);
    return;
  }

  if (request.type === "key.delete") {
    await deleteApiKey({ provider: request.provider, alias: request.alias });
    runtimeState = await createRuntimeState();
    await writeKeyUpdated(request.id, `Deleted ${request.provider || "glm"}/${request.alias || ""}.`);
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
    write({ type: "error", id: request.id, error: "Expected chat/session/key command" });
    return;
  }

  const runSessionId = request.sessionId || activeSessionId;
  await ensureSession(runSessionId);
  activeSessionId = runSessionId;
  write({ type: "run_started", id: request.id, sessionId: runSessionId });

  try {
    const result = await runtimeState.runtime.run({
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
      error: redactSecrets(error instanceof Error ? error.message : String(error)),
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

async function createRuntimeState() {
  const providerConfig = await resolveActiveProviderConfig("glm");
  const provider = createProvider(providerConfig);
  const runtime = createRuntime({
    agent,
    provider,
    storage,
    toolPermission: {
      defaultMode: process.env.QUBIT_TOOL_PERMISSION_DEFAULT || "ask",
    },
    hooks,
  });
  return { runtime, ...providerConfig };
}

function createProvider(config) {
  if (config.providerName === "stub") return new StubProvider();
  if (config.providerName !== "glm") {
    throw new Error(`Unsupported provider: ${config.providerName}`);
  }
  return new GLMProvider({
    apiKey: config.apiKey,
    endpoint: process.env.GLM_ENDPOINT === "coding" ? "coding" : "general",
    thinking: process.env.GLM_THINKING === "1" ? { type: "enabled" } : { type: "disabled" },
  });
}

async function resolveActiveProviderConfig(provider = "glm") {
  if (process.env.QUBIT_STUB === "1") {
    return { providerName: "stub", keyAlias: "stub", keySource: "stub", apiKey: undefined };
  }

  const normalizedProvider = normalizeProvider(provider);
  const activeAlias = apiKeyIndex.active?.[normalizedProvider];
  if (activeAlias) {
    if (activeAlias === envGLMAlias) {
      if (!process.env.ZAI_API_KEY) throw new Error("ZAI_API_KEY is not set.");
      return { providerName: normalizedProvider, keyAlias: activeAlias, keySource: "env", apiKey: process.env.ZAI_API_KEY };
    }
    const entry = findStoredKey(normalizedProvider, activeAlias);
    if (entry) {
      const apiKey = await getKeychainPassword(entry.account);
      if (apiKey) return { providerName: normalizedProvider, keyAlias: activeAlias, keySource: "keychain", apiKey };
    }
  }

  if (process.env.ZAI_API_KEY) {
    apiKeyIndex.active[normalizedProvider] = envGLMAlias;
    await saveApiKeyIndex();
    return { providerName: normalizedProvider, keyAlias: envGLMAlias, keySource: "env", apiKey: process.env.ZAI_API_KEY };
  }

  return { providerName: "stub", keyAlias: "stub", keySource: "stub", apiKey: undefined };
}

async function loadApiKeyIndex() {
  let parsed;
  try {
    parsed = JSON.parse(await readFile(keyIndexPath, "utf8"));
  } catch {
    parsed = null;
  }
  const keys = Array.isArray(parsed?.keys) ? parsed.keys : [];
  const normalized = keys
    .filter((key) => key && typeof key.provider === "string" && typeof key.alias === "string")
    .map((key) => ({
      provider: normalizeProvider(key.provider),
      alias: String(key.alias),
      source: "keychain",
      account: key.account || keychainAccount(key.provider, key.alias),
      createdAt: key.createdAt || new Date().toISOString(),
      updatedAt: key.updatedAt || key.createdAt || new Date().toISOString(),
    }));
  const index = {
    version: 1,
    active: typeof parsed?.active === "object" && parsed.active ? parsed.active : {},
    keys: normalized,
  };
  await saveApiKeyIndex(index);
  return index;
}

async function saveApiKeyIndex(index = apiKeyIndex) {
  await mkdir(dataDir, { recursive: true });
  await writeFile(keyIndexPath, `${JSON.stringify(index, null, 2)}\n`, { mode: 0o600 });
}

async function listApiKeys() {
  const activeProvider = runtimeState?.providerName === "stub" ? "glm" : runtimeState?.providerName || "glm";
  const activeAlias = apiKeyIndex.active?.glm || runtimeState?.keyAlias || "";
  const keys = [];

  if (process.env.ZAI_API_KEY) {
    keys.push({
      provider: "glm",
      alias: envGLMAlias,
      source: "env",
      active: activeProvider === "glm" && activeAlias === envGLMAlias,
      masked: maskApiKey(process.env.ZAI_API_KEY),
      readonly: true,
    });
  }

  for (const key of apiKeyIndex.keys) {
    let masked = "stored securely";
    try {
      const value = await getKeychainPassword(key.account);
      masked = value ? maskApiKey(value) : "missing from keychain";
    } catch {
      masked = "keychain unavailable";
    }
    keys.push({
      provider: key.provider,
      alias: key.alias,
      source: key.source,
      active: activeProvider === key.provider && activeAlias === key.alias,
      masked,
      readonly: false,
      createdAt: key.createdAt,
      updatedAt: key.updatedAt,
    });
  }

  return keys;
}

async function writeKeyList(id) {
  write({
    type: "key.list",
    id,
    provider: runtimeState.providerName,
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    keys: await listApiKeys(),
  });
}

async function writeKeyUpdated(id, status) {
  write({
    type: "key.updated",
    id,
    provider: runtimeState.providerName,
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    status,
    keys: await listApiKeys(),
  });
}

async function setApiKey({ provider, alias, apiKey }) {
  const normalizedProvider = normalizeProvider(provider || "glm");
  const normalizedAlias = normalizeAlias(alias);
  const value = String(apiKey || "").trim();
  if (!value) throw new Error("API key is required.");
  if (normalizedAlias.startsWith("env:")) throw new Error("Aliases beginning with env: are reserved.");

  const now = new Date().toISOString();
  const account = keychainAccount(normalizedProvider, normalizedAlias);
  await setKeychainPassword(account, value);

  const existing = findStoredKey(normalizedProvider, normalizedAlias);
  if (existing) {
    existing.updatedAt = now;
  } else {
    apiKeyIndex.keys.push({
      provider: normalizedProvider,
      alias: normalizedAlias,
      source: "keychain",
      account,
      createdAt: now,
      updatedAt: now,
    });
  }
  apiKeyIndex.active[normalizedProvider] = normalizedAlias;
  await saveApiKeyIndex();
}

async function activateApiKey({ provider, alias }) {
  const normalizedProvider = normalizeProvider(provider || "glm");
  const normalizedAlias = normalizeAlias(alias);
  if (normalizedAlias === envGLMAlias) {
    if (!process.env.ZAI_API_KEY) throw new Error("ZAI_API_KEY is not set.");
    apiKeyIndex.active[normalizedProvider] = normalizedAlias;
    await saveApiKeyIndex();
    return;
  }
  const entry = findStoredKey(normalizedProvider, normalizedAlias);
  if (!entry) throw new Error(`Unknown API key alias: ${normalizedProvider}/${normalizedAlias}`);
  const value = await getKeychainPassword(entry.account);
  if (!value) throw new Error(`API key ${normalizedProvider}/${normalizedAlias} is missing from the OS keychain.`);
  apiKeyIndex.active[normalizedProvider] = normalizedAlias;
  await saveApiKeyIndex();
}

async function deleteApiKey({ provider, alias }) {
  const normalizedProvider = normalizeProvider(provider || "glm");
  const normalizedAlias = normalizeAlias(alias);
  if (normalizedAlias === envGLMAlias) throw new Error("Environment API keys are read-only and cannot be deleted.");
  const index = apiKeyIndex.keys.findIndex((key) => key.provider === normalizedProvider && key.alias === normalizedAlias);
  if (index < 0) throw new Error(`Unknown API key alias: ${normalizedProvider}/${normalizedAlias}`);
  const [entry] = apiKeyIndex.keys.splice(index, 1);
  await deleteKeychainPassword(entry.account);
  if (apiKeyIndex.active?.[normalizedProvider] === normalizedAlias) {
    if (process.env.ZAI_API_KEY) apiKeyIndex.active[normalizedProvider] = envGLMAlias;
    else delete apiKeyIndex.active[normalizedProvider];
  }
  await saveApiKeyIndex();
}

function normalizeProvider(provider) {
  const normalized = String(provider || "glm").trim().toLowerCase();
  if (normalized !== "glm") throw new Error(`Unsupported provider: ${normalized || "empty"}`);
  return normalized;
}

function normalizeAlias(alias) {
  const normalized = String(alias || "").trim();
  if (!normalized) throw new Error("API key alias is required.");
  if (!/^[A-Za-z0-9_.:-]{1,64}$/.test(normalized)) {
    throw new Error("API key alias may contain only letters, numbers, dash, underscore, dot, and colon.");
  }
  return normalized;
}

function findStoredKey(provider, alias) {
  return apiKeyIndex.keys.find((key) => key.provider === provider && key.alias === alias);
}

function keychainAccount(provider, alias) {
  return `${normalizeProvider(provider)}:${normalizeAlias(alias)}`;
}

async function setKeychainPassword(account, password) {
  ensureKeychainAvailable();
  await keytar.setPassword(keychainService, account, password);
}

async function getKeychainPassword(account) {
  ensureKeychainAvailable();
  return await keytar.getPassword(keychainService, account);
}

async function deleteKeychainPassword(account) {
  ensureKeychainAvailable();
  await keytar.deletePassword(keychainService, account);
}

function ensureKeychainAvailable() {
  if (keytar && typeof keytar.setPassword === "function" && typeof keytar.getPassword === "function" && typeof keytar.deletePassword === "function") return;
  const reason = keytarLoadError instanceof Error ? keytarLoadError.message : String(keytarLoadError || "keytar module did not expose the expected OS keychain API");
  throw new Error(`Secure OS keychain storage is unavailable: ${reason}. Install Qubit's keychain dependency and required OS keychain support, or use ZAI_API_KEY.`);
}

function maskApiKey(value) {
  const text = String(value || "");
  if (text.length <= 8) return "••••";
  return `${text.slice(0, 4)}…${text.slice(-4)}`;
}

function redactSecrets(value) {
  return String(value || "").replace(/\b(?:sk|zai|key|token)[-_][A-Za-z0-9_.-]{8,}/gi, "[redacted]");
}

function redactMessage(message) {
  if (!message || typeof message !== "object") return message;
  const clone = { ...message };
  if (typeof clone.apiKey === "string") clone.apiKey = "[redacted]";
  if (typeof clone.error === "string") clone.error = redactSecrets(clone.error);
  return clone;
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
      provider: session.provider || runtimeState.providerName,
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
    provider: runtimeState.providerName,
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
  session.provider = runtimeState.providerName;
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
