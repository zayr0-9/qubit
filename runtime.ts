#!/usr/bin/env node
// @ts-nocheck
import readline from "node:readline";
import net from "node:net";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { basename, dirname, join } from "node:path";
import os from "node:os";
import { fileURLToPath } from "node:url";
import {
  createRuntime,
  defineAgent,
  StubProvider,
} from "@hyper-labs/hyper-router";
import { QubitSqliteStorage } from "./runtime/storage/qubitSqliteStorage.js";
import { GLMProvider } from "@hyper-labs/hyper-router/providers/glm";
import { OpenAIProvider } from "@hyper-labs/hyper-router/providers/openai-vai";
import { AmazonBedrockVAIProvider } from "@hyper-labs/hyper-router/providers/amazon-bedrock-vai";
import { OpenRouterProvider } from "@hyper-labs/hyper-router/providers/openrouter";
import { qubitTools } from "./tools/index.js";
import { setPlanViewEmitter } from "./tools/planMd.js";
import { setMultiCallLifecycleEmitter, setMultiCallPermissionRequester } from "./tools/multiCall.js";
import { getDefaultToolCwd, setDefaultToolCwd } from "./utils/toolWorkspace.js";
import { CodexResponsesProvider, QubitCodexTokenStore, cancelCodexLogin, startCodexLogin } from "./runtime/codex/index.js";
import { overlayActiveRunUserMessages } from "./runtime/activeRunOverlay.js";
import { assistantReasoningContent } from "./runtime/assistantReasoning.js";

const entryDir = dirname(fileURLToPath(import.meta.url));
const rootDir = basename(entryDir) === "dist" ? dirname(entryDir) : entryDir;
const initialWorkspaceCwd = process.env.QUBIT_WORKSPACE_CWD || process.cwd();
setDefaultToolCwd(initialWorkspaceCwd);
const dataDir = process.env.QUBIT_PROJECT_DIR || join(initialWorkspaceCwd, ".qubit");
const configDir = resolveConfigDir();
const storagePath = join(dataDir, "sessions.sqlite");
const indexPath = join(dataDir, "session-index.json");
const legacyKeyIndexPath = join(dataDir, "api-key-index.json");
const legacySettingsPath = join(dataDir, "settings.json");
const keyIndexPath = join(configDir, "api-key-index.json");
const settingsPath = join(configDir, "settings.json");
const promptsDir = join(rootDir, "prompts");
const baseInstructions = "You are Qubit, a concise terminal coding assistant MVP. Be helpful, direct, and practical. Keep answers brief unless the user asks for detail.";
const defaultModePrompts = {
  plan: "You are in plan mode: reason carefully and propose changes before editing.",
  edit: "You are in edit mode: implement changes directly and validate them.",
};
const defaultSessionId = process.env.QUBIT_SESSION_ID || "qubit-default";
function resolveConfigDir() {
  if (process.env.QUBIT_CONFIG_DIR) return process.env.QUBIT_CONFIG_DIR;
  if (process.platform === "win32") return join(process.env.APPDATA || join(os.homedir(), "AppData", "Roaming"), "Qubit");
  if (process.platform === "darwin") return join(os.homedir(), "Library", "Application Support", "Qubit");
  return join(process.env.XDG_CONFIG_HOME || join(os.homedir(), ".config"), "qubit");
}

const providerDefinitions = {
  glm: {
    aliases: ["zai"],
    envKeys: ["ZAI_API_KEY", "GLM_API_KEY"],
    modelEnvKeys: ["GLM_MODEL", "QUBIT_MODEL"],
    defaultModel: "glm-5.1",
  },
  hyperrouter: {
    aliases: ["hyper-router", "hyper_router", "hyperouter"],
    envKeys: ["HYPERROUTER_API_KEY", "HYPER_ROUTER_API_KEY"],
    modelEnvKeys: ["HYPERROUTER_MODEL", "HYPER_ROUTER_MODEL"],
    defaultModel: "claude-sonnet-4-6",
  },
  openai: {
    envKeys: ["OPENAI_API_KEY"],
    modelEnvKeys: ["OPENAI_MODEL"],
    defaultModel: "gpt-5.2",
  },
  bedrock: {
    aliases: ["amazon-bedrock", "amazon_bedrock", "amazonbedrock", "aws-bedrock", "aws_bedrock"],
    envKeys: ["AWS_BEARER_TOKEN_BEDROCK", "BEDROCK_API_KEY"],
    modelEnvKeys: ["BEDROCK_MODEL", "AWS_BEDROCK_MODEL"],
    defaultModel: "anthropic.claude-opus-4-7",
  },
  openrouter: {
    aliases: ["open-router", "open_router"],
    envKeys: ["OPENROUTER_API_KEY", "OPEN_ROUTER_API_KEY"],
    modelEnvKeys: ["OPENROUTER_MODEL", "OPEN_ROUTER_MODEL"],
    defaultModel: "openai/gpt-5.4",
  },
  codex: {
    aliases: ["openai-codex", "chatgpt-codex", "chatgpt"],
    envKeys: ["CODEX_ACCESS_TOKEN"],
    modelEnvKeys: ["CODEX_MODEL", "QUBIT_MODEL"],
    defaultModel: "gpt-5.2-codex",
  },
};
const providerAliasMap = new Map(Object.entries(providerDefinitions).flatMap(([name, definition]) => [name, ...(definition.aliases || [])].map((alias) => [alias, name])));
const providerNames = Object.keys(providerDefinitions);
let settings = await loadSettings();
const modePrompts = await loadModePrompts();
const defaultProviderName = normalizeProvider(process.env.QUBIT_PROVIDER || settings.defaultProvider || "glm");
let activeProviderName = defaultProviderName;
let model = defaultModelForProvider(activeProviderName);
let reasoningLevel = normalizeReasoningLevel(process.env.QUBIT_REASONING || settings.reasoningLevel || "medium");
const providerModelLists = {
  glm: [
    { id: "glm-5.1", name: "glm-5.1", description: "Latest flagship GLM model for coding and long-horizon agent tasks" },
    { id: "glm-5", name: "glm-5", description: "GLM-5 foundation model for coding and agent workflows" },
    { id: "glm-5-turbo", name: "glm-5-turbo", description: "Fast GLM-5 variant for tool use and long chains" },
    { id: "glm-4.7", name: "glm-4.7", description: "GLM-4.7 coding, reasoning, and agent model" },
    { id: "glm-4.7-flashx", name: "glm-4.7-flashx", description: "Fast GLM-4.7 variant" },
    { id: "glm-4.7-flash", name: "glm-4.7-flash", description: "Lightweight GLM-4.7 flash variant" },
    { id: "glm-4.6", name: "glm-4.6", description: "Previous flagship GLM coding/chat model" },
    { id: "glm-4.5", name: "glm-4.5", description: "Previous GLM generation" },
    { id: "glm-4.5-x", name: "glm-4.5-x", description: "Higher-performance GLM-4.5 variant" },
    { id: "glm-4.5-air", name: "glm-4.5-air", description: "Lower-cost GLM-4.5 Air model" },
    { id: "glm-4.5-flash", name: "glm-4.5-flash", description: "Lightweight GLM-4.5 flash variant" },
  ],
  openai: [
    { id: "gpt-5.2", name: "GPT-5.2", description: "Flagship OpenAI model for coding and agentic tasks" },
    { id: "gpt-5.2-chat-latest", name: "GPT-5.2 Chat", description: "GPT-5.2 model used for ChatGPT-style chat" },
    { id: "gpt-5.2-codex", name: "GPT-5.2-Codex", description: "Coding-optimized GPT-5.2 model" },
    { id: "gpt-5.2-pro", name: "GPT-5.2 pro", description: "Higher-compute GPT-5.2 variant" },
    { id: "gpt-5.1", name: "GPT-5.1", description: "Agentic coding model with configurable reasoning" },
    { id: "gpt-5", name: "GPT-5", description: "Previous intelligent reasoning model" },
    { id: "gpt-5-mini", name: "GPT-5 mini", description: "Faster, cost-efficient GPT-5 variant" },
    { id: "gpt-5-nano", name: "GPT-5 nano", description: "Fastest, most cost-efficient GPT-5 variant" },
    { id: "gpt-4.1", name: "GPT-4.1", description: "Smart non-reasoning model" },
    { id: "gpt-4.1-mini", name: "GPT-4.1 mini", description: "Smaller, faster GPT-4.1 variant" },
  ],
  bedrock: [
    { id: "anthropic.claude-sonnet-4-5-20250929-v1:0", name: "Claude Sonnet 4.5", description: "Anthropic coding and agent model on Amazon Bedrock" },
    { id: "global.anthropic.claude-sonnet-4-5-20250929-v1:0", name: "Claude Sonnet 4.5 global", description: "Cross-region inference profile" },
    { id: "us.anthropic.claude-sonnet-4-5-20250929-v1:0", name: "Claude Sonnet 4.5 US", description: "US cross-region inference profile" },
    { id: "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", name: "Claude Sonnet 4.5 EU", description: "EU cross-region inference profile" },
    { id: "anthropic.claude-3-7-sonnet-20250219-v1:0", name: "Claude 3.7 Sonnet", description: "Previous Claude Sonnet generation" },
    { id: "anthropic.claude-3-5-sonnet-20241022-v2:0", name: "Claude 3.5 Sonnet v2", description: "Earlier Claude coding/chat model" },
  ],
  openrouter: [
    { id: "openai/gpt-5.4", name: "OpenAI: GPT-5.4", description: "Latest OpenAI flagship through OpenRouter" },
    { id: "openai/gpt-5.4-mini", name: "OpenAI: GPT-5.4 Mini", description: "Fast GPT-5.4 model through OpenRouter" },
    { id: "openai/gpt-5.2", name: "OpenAI: GPT-5.2", description: "OpenAI GPT-5.2 through OpenRouter" },
    { id: "openai/gpt-chat-latest", name: "OpenAI: GPT Chat Latest", description: "OpenAI chat alias through OpenRouter" },
    { id: "anthropic/claude-opus-4.8", name: "Anthropic: Claude Opus 4.8", description: "Latest Claude Opus through OpenRouter" },
    { id: "anthropic/claude-sonnet-4.6", name: "Anthropic: Claude Sonnet 4.6", description: "Claude Sonnet coding model through OpenRouter" },
    { id: "google/gemini-3.5-flash", name: "Google: Gemini 3.5 Flash", description: "Latest Gemini Flash through OpenRouter" },
    { id: "x-ai/grok-4.3", name: "xAI: Grok 4.3", description: "Latest Grok model through OpenRouter" },
    { id: "qwen/qwen3.7-max", name: "Qwen: Qwen3.7 Max", description: "Latest Qwen Max through OpenRouter" },
    { id: "z-ai/glm-5.1", name: "Z.ai: GLM 5.1", description: "Latest GLM model through OpenRouter" },
  ],
  codex: [
    { id: "gpt-5.5", name: "GPT-5.5", description: "ChatGPT/Codex backend frontier model", maxContext: 400000 },
    { id: "gpt-5.3-codex", name: "GPT-5.3 Codex", description: "Current ChatGPT Codex coding model", maxContext: 400000 },
    { id: "gpt-5.2-codex", name: "GPT-5.2 Codex", description: "ChatGPT Codex Responses backend via local OAuth", maxContext: 400000 },
    { id: "gpt-5.2", name: "GPT-5.2", description: "ChatGPT/Codex backend model", maxContext: 400000 },
  ],
};
let openRouterModelsCache = null;
let openRouterModelsCacheAt = 0;
const keychainService = process.env.QUBIT_KEYCHAIN_SERVICE || "Qubit";

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

function createQubitAgent(mode = "plan") {
  return defineAgent({
    name: "qubit-chat",
    instructions: instructionsForMode(mode),
    model,
    tools: qubitTools,
  });
}

function instructionsForMode(mode) {
  const normalized = normalizePromptMode(mode);
  const modePrompt = modePrompts[normalized] || defaultModePrompts[normalized];
  return [baseInstructions, modePrompt].filter(Boolean).join("\n\n");
}

function normalizePromptMode(mode) {
  const normalized = String(mode || "").trim().toLowerCase();
  if (["edit", "always", "always_allow", "always-allow", "allow"].includes(normalized)) return "edit";
  return "plan";
}

function normalizeReasoningLevel(value) {
  const normalized = String(value || "medium").trim().toLowerCase();
  if (["none", "off", "disabled", "disable", "0"].includes(normalized)) return "none";
  if (["low", "l"].includes(normalized)) return "low";
  if (["medium", "med", "m", ""].includes(normalized)) return "medium";
  if (["high", "h"].includes(normalized)) return "high";
  throw new Error("Reasoning level must be none, low, medium, or high.");
}

async function loadModePrompts() {
  const entries = await Promise.all(Object.keys(defaultModePrompts).map(async (mode) => {
    try {
      const content = (await readFile(join(promptsDir, `${mode}.md`), "utf8")).trim();
      return [mode, content || defaultModePrompts[mode]];
    } catch {
      return [mode, defaultModePrompts[mode]];
    }
  }));
  return Object.fromEntries(entries);
}

const storage = new QubitSqliteStorage({
  filePath: storagePath,
});

const pendingPermissions = new Map();
const activeRuns = new Map();
function targetForRunEvent(event) {
  if (event?.runId && activeRuns.has(event.runId)) return activeRuns.get(event.runId).target || null;
  if (event?.sessionId) {
    const active = [...activeRuns.values()].find((run) => run.sessionId === event.sessionId);
    if (active?.target) return active.target;
  }
  return null;
}

async function requestToolPermission(request) {
  const target = targetForRunEvent(request);
  write({
    type: "tool.permission.request",
    id: request.id,
    sessionId: request.sessionId,
    runId: request.runId,
    step: request.step,
    toolCallId: request.toolCallId,
    toolName: request.toolName,
    args: summarizeToolArgs(request.toolName, request.args),
    description: request.description,
    inputSchema: request.inputSchema,
    metadata: request.metadata,
  }, target);

  return await new Promise((resolve) => {
    pendingPermissions.set(request.id, resolve);
  });
}

setMultiCallPermissionRequester(requestToolPermission);
setMultiCallLifecycleEmitter((event) => {
  const target = targetForRunEvent(event);
  if (event.type === "start") {
    write({
      type: "tool.call.start",
      sessionId: event.sessionId,
      runId: event.runId,
      step: event.step,
      toolCallId: event.toolCallId,
      toolName: event.toolName,
      status: event.status,
      args: summarizeToolArgs(event.toolName, event.args),
      startedAt: event.startedAt,
    }, target);
    return;
  }
  write({
    type: "tool.call.finish",
    sessionId: event.sessionId,
    runId: event.runId,
    step: event.step,
    toolCallId: event.toolCallId,
    toolName: event.toolName,
    status: event.status,
    args: summarizeToolArgs(event.toolName, event.args),
    result: summarizeToolResult(event.toolName, event.result),
    contextChars: toolCallContextChars(event.toolName, event.args, event.result),
    startedAt: event.startedAt,
    finishedAt: event.finishedAt,
    durationMs: event.durationMs,
  }, target);
});
setPlanViewEmitter((event) => {
  write({ type: "plan.view", name: event.name, path: event.path, cwd: event.cwd, content: event.content });
});

const hooks = {
  requestToolPermission,

  async onToolCallStart(event) {
    const target = targetForRunEvent(event);
    write({
      type: "tool.call.start",
      sessionId: event.sessionId,
      runId: event.runId,
      step: event.step,
      toolCallId: event.toolCallId,
      toolName: event.toolName,
      status: event.status,
      args: summarizeToolArgs(event.toolName, event.args),
      startedAt: event.startedAt,
    }, target);
  },
  async onToolCallFinish(event) {
    const target = targetForRunEvent(event);
    write({
      type: "tool.call.finish",
      sessionId: event.sessionId,
      runId: event.runId,
      step: event.step,
      toolCallId: event.toolCallId,
      toolName: event.toolName,
      status: event.status,
      args: summarizeToolArgs(event.toolName, event.args),
      result: summarizeToolResult(event.toolName, event.result),
      contextChars: toolCallContextChars(event.toolName, event.args, event.result),
      startedAt: event.startedAt,
      finishedAt: event.finishedAt,
      durationMs: event.durationMs,
    }, target);
  },
};

let apiKeyIndex = await loadApiKeyIndex();
const codexTokenStore = new QubitCodexTokenStore({ dataDir: configDir, legacyDataDir: dataDir, keychainService, keytar });
let runtimeState = await createRuntimeState("plan");
let sessionIndex = await loadSessionIndex();
let activeSessionId = sessionIndex.activeSessionId;

const clients = new Set();

function readyMessage(id) {
  return {
    type: "ready",
    id,
    sessionId: activeSessionId,
    sessionTitle: activeSession()?.title,
    provider: runtimeState.providerName,
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    model,
    maxContext: activeModelMaxContext(),
    reasoningLevel,
    storagePath,
    indexPath,
    workspaceCwd: getDefaultToolCwd(),
  };
}

let currentResponseTarget = null;

function write(message, target = null) {
  const line = `${JSON.stringify(redactMessage(message))}\n`;
  const destination = target || currentResponseTarget;
  if (destination) {
    destination.write(line);
    return;
  }
  if (clients.size > 0) {
    for (const client of clients) {
      client.write(line);
    }
    return;
  }
  process.stdout.write(line);
}

const serverAddress = process.env.QUBIT_RUNTIME_ADDR || "";
if (serverAddress) {
  await startRuntimeServer(serverAddress);
} else {
  startStdioClient();
  write(readyMessage());
}

function startStdioClient() {
  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });
  attachLineHandler(rl, null);
  rl.on("close", () => {
    queue.finally(() => process.exit(0));
  });
}

async function startRuntimeServer(address) {
  const server = net.createServer((socket) => {
    clients.add(socket);
    socket.setEncoding("utf8");
    const rl = readline.createInterface({ input: socket, crlfDelay: Infinity });
    attachLineHandler(rl, socket);
    write(readyMessage(), socket);
    socket.on("close", () => {
      clients.delete(socket);
      rl.close();
    });
    socket.on("error", () => {
      clients.delete(socket);
    });
  });
  server.on("error", (error) => {
    console.error(`[runtime-server] ${error instanceof Error ? error.message : String(error)}`);
    process.exit(1);
  });
  const [host, portText] = String(address).split(":");
  const port = Number(portText);
  await new Promise((resolve) => server.listen(port, host || "127.0.0.1", resolve));
  console.error(`[runtime-server] listening ${address}`);
}

let queue = Promise.resolve();

function attachLineHandler(rl, target) {
  rl.on("line", (line) => {
    const previousTarget = currentResponseTarget;
    currentResponseTarget = target;
    try {
      if (handleImmediateLine(line)) return;
    } finally {
      currentResponseTarget = previousTarget;
    }

    queue = queue.then(async () => {
      const previousTarget = currentResponseTarget;
      currentResponseTarget = target;
      try {
        await handleLine(line);
      } finally {
        currentResponseTarget = previousTarget;
      }
    }).catch((error) => {
      write({ type: "error", error: redactSecrets(error instanceof Error ? error.message : String(error)) }, target);
    });
  });
}

function handleImmediateLine(line) {
  if (!line.trim()) return true;

  let request;
  try {
    request = JSON.parse(line);
  } catch {
    return false;
  }

  if (request.type === "tool.permission.response") {
    handlePermissionResponse(request);
    return true;
  }

  if (request.type === "chat.cancel") {
    handleCancelRequest(request);
    return true;
  }

  return false;
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
    if (!serverAddress) {
      process.exit(0);
    }
    return;
  }

  if (request.type === "tool.permission.response") {
    handlePermissionResponse(request);
    return;
  }

  if (request.type === "chat.cancel") {
    handleCancelRequest(request);
    return;
  }

  if (request.type === "key.list") {
    await writeKeyList(request.id);
    return;
  }

  if (request.type === "codex.status") {
    await writeCodexStatus(request.id);
    return;
  }

  if (request.type === "codex.login.start") {
    await handleCodexLoginStart(request.id);
    return;
  }

  if (request.type === "codex.login.cancel") {
    await handleCodexLoginCancel(request.id);
    return;
  }

  if (request.type === "codex.logout") {
    await handleCodexLogout(request.id);
    return;
  }

  if (request.type === "model.list") {
    await writeModelList(request.id);
    return;
  }

  if (request.type === "provider.use") {
    await switchProvider(request.id, request.provider, request.persistDefault === true);
    return;
  }

  if (request.type === "reasoning.set") {
    try {
      reasoningLevel = normalizeReasoningLevel(request.level);
      settings.reasoningLevel = reasoningLevel;
      await saveSettings();
      runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
      write({ type: "reasoning.updated", id: request.id, reasoningLevel, status: `Reasoning: ${reasoningLevel}.` });
    } catch (error) {
      write({ type: "error", id: request.id, error: error instanceof Error ? error.message : String(error) });
    }
    return;
  }

  if (request.type === "model.use") {
    try {
      model = normalizeModel(request.model);
      if (request.persistDefault === true) {
        settings.defaultModels[activeProviderName] = model;
        await saveSettings();
      }
      runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
      const defaultStatus = request.persistDefault === true ? " Saved as default." : "";
      await writeModelUpdated(request.id, `Using ${runtimeState.providerName} model ${model}.${defaultStatus}`);
    } catch (error) {
      write({ type: "error", id: request.id, error: error instanceof Error ? error.message : String(error) });
    }
    return;
  }

  if (request.type === "key.set") {
    await setApiKey({ provider: request.provider, alias: request.alias, apiKey: request.apiKey });
    runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
    await writeKeyUpdated(request.id, `Saved and activated ${normalizeProvider(request.provider || defaultProviderName)}/${request.alias || ""}.`);
    return;
  }

  if (request.type === "key.use") {
    await activateApiKey({ provider: request.provider, alias: request.alias });
    runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
    await writeKeyUpdated(request.id, `Activated ${normalizeProvider(request.provider || defaultProviderName)}/${request.alias || ""}.`);
    return;
  }

  if (request.type === "key.delete") {
    await deleteApiKey({ provider: request.provider, alias: request.alias });
    runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
    await writeKeyUpdated(request.id, `Deleted ${normalizeProvider(request.provider || defaultProviderName)}/${request.alias || ""}.`);
    return;
  }

  if (request.type === "session.list") {
    writeSessionList(request.id);
    return;
  }

  if (request.type === "session.delete") {
    const deleted = await deleteSession(String(request.sessionId || ""));
    if (!deleted) {
      write({ type: "error", id: request.id, error: `Unknown session: ${request.sessionId}` });
      return;
    }
    write({ type: "session.deleted", id: request.id, sessionId: deleted.id, sessionTitle: deleted.title });
    writeSessionList(request.id);
    return;
  }

  if (request.type === "session.tree") {
    const requestedSessionId = String(request.sessionId || activeSessionId || "");
    const focalSession = sessionIndex.sessions.find((session) => session.id === requestedSessionId) || activeSession();
    const focalSessionId = focalSession?.id || activeSessionId;
    const nodes = await buildSessionTreeNodes(focalSessionId);
    write({
      type: "session.tree",
      id: request.id,
      sessionId: focalSessionId,
      sessionTitle: focalSession?.title,
      nodes,
    });
    return;
  }

  if (request.type === "session.new") {
    const session = await createSession({ title: request.title });
    write({ type: "session.created", id: request.id, session, sessionId: activeSessionId, sessionTitle: session.title });
    writeSessionList(request.id);
    return;
  }

  if (request.type === "session.fork") {
    const session = await forkSession({
      sourceSessionId: String(request.sessionId || activeSessionId),
      messageIndex: Number(request.messageIndex),
      title: request.title,
    });
    if (!session) {
      write({ type: "error", id: request.id, error: `Unknown session: ${request.sessionId || activeSessionId}` });
      return;
    }
    write({ type: "session.forked", id: request.id, session, sessionId: activeSessionId, sessionTitle: session.title });
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

  if (request.type === "chat" && typeof request.input === "string") {
    const responseTarget = currentResponseTarget;
    void handleChatRequest(request, responseTarget).catch((error) => {
      write({
        type: "error",
        id: request.id,
        runId: request.runId,
        sessionId: request.sessionId,
        error: redactSecrets(error instanceof Error ? error.message : String(error)),
      }, responseTarget);
    });
    return;
  }

  write({ type: "error", id: request.id, error: "Expected chat/session/key command" });
}

async function handleChatRequest(request, responseTarget = null) {
  const runId = String(request.runId || request.id || createRunId());
  const controller = new AbortController();
  let runSessionId = request.sessionId || activeSessionId;
  const replaceFromMessageIndex = Number(request.replaceFromMessageIndex);
  if (Number.isFinite(replaceFromMessageIndex)) {
    const sourceSessionId = String(request.sessionId || activeSessionId);
    const source = sessionIndex.sessions.find((candidate) => candidate.id === sourceSessionId);
    if (!source) {
      write({ type: "error", id: request.id, error: `Unknown session: ${sourceSessionId}` }, responseTarget);
      return;
    }
    const session = await forkSession({
      sourceSessionId,
      messageIndex: replaceFromMessageIndex,
      title: request.title || `Edit: ${source.title || "Untitled chat"}`,
      includeUiMessageAtIndex: false,
    });
    if (!session) {
      write({ type: "error", id: request.id, error: `Unknown session: ${sourceSessionId}` }, responseTarget);
      return;
    }
    runSessionId = session.id;
  } else if (request.newSession === true || !request.sessionId) {
    const session = await createSession({ title: request.title || titleFromInput(request.input) });
    runSessionId = session.id;
  } else {
    await ensureSession(runSessionId);
    activeSessionId = runSessionId;
  }

  const requestedReasoningLevel = normalizeReasoningLevel(request.reasoningLevel || reasoningLevel);
  if (requestedReasoningLevel !== reasoningLevel) {
    reasoningLevel = requestedReasoningLevel;
    runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
  }

  const promptMode = normalizePromptMode(request.systemPromptMode || request.mode);
  if (runtimeState.promptMode !== promptMode) {
    runtimeState = await createRuntimeState(promptMode);
  }

  await touchSession(runSessionId, { title: titleFromInput(request.input) });

  const runRuntimeState = runtimeState;
  activeRuns.set(runId, { runId, requestId: request.id, sessionId: runSessionId, input: request.input, controller, runtime: runRuntimeState.runtime, target: responseTarget });
  write({ type: "run_started", id: request.id, runId, sessionId: runSessionId }, responseTarget);

  try {
    const result = await runRuntimeState.runtime.run({
      runId,
      signal: controller.signal,
      sessionId: runSessionId,
      input: request.input,
      maxSteps: 400,
      metadata: { workspaceCwd: getDefaultToolCwd() },
    });

    const assistant = [...result.messages].reverse().find((message) => message.role === "assistant" && String(message.content || "").trim());
    const fallbackAssistant = [...result.messages].reverse().find((message) => message.role === "assistant");
    const reasoningContent = assistantReasoningContent(result.messages);
    await touchSession(runSessionId, {
      title: titleFromInput(request.input),
      messageCount: result.messages.filter((message) => message.role !== "system").length,
    });

    if (result.status !== "cancelled" || assistant?.content) {
      write({
        type: "assistant",
        id: request.id,
        runId,
        sessionId: runSessionId,
        status: result.status,
        content: assistant?.content || fallbackAssistant?.content || "",
        reasoningContent,
      }, responseTarget);
    }
    write({ type: "run_finished", id: request.id, runId, sessionId: runSessionId, status: result.status }, responseTarget);
  } catch (error) {
    if (controller.signal.aborted || isAbortError(error)) {
      write({ type: "run_finished", id: request.id, runId, sessionId: runSessionId, status: "cancelled" }, responseTarget);
    } else {
      write({
        type: "error",
        id: request.id,
        runId,
        sessionId: runSessionId,
        error: redactSecrets(error instanceof Error ? error.message : String(error)),
      }, responseTarget);
    }
  } finally {
    activeRuns.delete(runId);
  }
}

function handleCancelRequest(request) {
  const requesterTarget = currentResponseTarget;
  const runId = String(request.runId || "");
  const active = activeRuns.get(runId);
  if (!runId || !active) {
    write({ type: "run_cancelled", id: request.id, runId, status: "not_found" }, requesterTarget);
    return;
  }

  try {
    active.runtime?.cancel?.(runId);
  } catch {
    // Best-effort: the AbortController below is the local cancellation path.
  }
  active.controller?.abort();

  const message = { type: "run_cancelled", id: request.id, runId, sessionId: active.sessionId, status: "cancelled" };
  write(message, requesterTarget);
  if (active.target && active.target !== requesterTarget) {
    write(message, active.target);
  }
}

function defaultModelForProvider(provider) {
  const normalizedProvider = normalizeProvider(provider || defaultProviderName);
  return firstEnvValue(providerDefinitions[normalizedProvider].modelEnvKeys) || settings.defaultModels?.[normalizedProvider] || providerDefinitions[normalizedProvider].defaultModel;
}

function activeModelMaxContext() {
  const models = providerModelLists[activeProviderName] || [];
  const active = models.find((candidate) => candidate.id === model);
  return Number.isFinite(active?.maxContext) ? active.maxContext : 0;
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

async function createRuntimeState(promptMode = "plan") {
  const normalizedPromptMode = normalizePromptMode(promptMode);
  const providerConfig = await resolveActiveProviderConfig(activeProviderName);
  const provider = createProvider(providerConfig);
  const runtime = createRuntime({
    agent: createQubitAgent(normalizedPromptMode),
    provider,
    storage,
    toolPermission: {
      defaultMode: process.env.QUBIT_TOOL_PERMISSION_DEFAULT || "ask",
    },
    hooks,
  });
  return { runtime, promptMode: normalizedPromptMode, ...providerConfig };
}

function providerReasoningOptions() {
  return reasoningLevel === "none" ? false : { enabled: true, effort: reasoningLevel, capture: true, includeInMessages: true };
}

function codexReasoningEffort() {
  return reasoningLevel === "none" ? "minimal" : reasoningLevel;
}

function createProvider(config) {
  if (config.providerName === "stub") return new StubProvider();

  switch (config.providerName) {
    case "glm":
      return new GLMProvider({
        apiKey: config.apiKey,
        endpoint: process.env.GLM_ENDPOINT === "coding" ? "coding" : "general",
        thinking: process.env.GLM_THINKING === "1" ? { type: "enabled" } : { type: "disabled" },
      });
    case "hyperrouter":
      return new OpenAIProvider({
        apiKey: config.apiKey,
        baseURL: process.env.HYPERROUTER_BASE_URL || process.env.HYPER_ROUTER_BASE_URL || "https://hyperrouter.cloud/v1",
        name: "hyperrouter",
        reasoning: providerReasoningOptions(),
      });
    case "openai":
      return new OpenAIProvider({
        apiKey: config.apiKey,
        baseURL: process.env.OPENAI_BASE_URL,
        organization: process.env.OPENAI_ORG_ID || process.env.OPENAI_ORGANIZATION,
        project: process.env.OPENAI_PROJECT,
        reasoning: providerReasoningOptions(),
      });
    case "bedrock":
      return new AmazonBedrockVAIProvider({
        apiKey: config.apiKey,
        region: process.env.AWS_REGION || process.env.AWS_DEFAULT_REGION || process.env.BEDROCK_REGION,
        accessKeyId: process.env.AWS_ACCESS_KEY_ID,
        secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY,
        sessionToken: process.env.AWS_SESSION_TOKEN,
        baseURL: process.env.BEDROCK_BASE_URL,
      });
    case "openrouter":
      return new OpenRouterProvider({
        apiKey: config.apiKey,
      });
    case "codex":
      return new CodexResponsesProvider({
        tokenStore: codexTokenStore,
        baseURL: process.env.CODEX_BASE_URL || "https://chatgpt.com/backend-api/codex",
        issuer: process.env.CODEX_ISSUER || "https://auth.openai.com",
        clientId: process.env.CODEX_CLIENT_ID || "app_EMoamEEZ73f0CkXaXp7hrann",
        originator: process.env.CODEX_ORIGINATOR || "codex_cli_rs",
        reasoningEffort: codexReasoningEffort(),
        reasoningSummary: process.env.QUBIT_CODEX_REASONING_SUMMARY === "off" ? null : (process.env.QUBIT_CODEX_REASONING_SUMMARY || "auto"),
        onReasoningDelta: (event) => {
          write({
            type: "reasoning.delta",
            sessionId: event.sessionId,
            runId: event.runId,
            content: event.delta,
          }, targetForRunEvent(event));
        },
      });
    default:
      throw new Error(`Unsupported provider: ${config.providerName}`);
  }
}

async function resolveActiveProviderConfig(provider = defaultProviderName) {
  if (process.env.QUBIT_STUB === "1") {
    return { providerName: "stub", keyAlias: "stub", keySource: "stub", apiKey: undefined };
  }

  const normalizedProvider = normalizeProvider(provider);
  if (normalizedProvider === "codex") {
    if (process.env.CODEX_ACCESS_TOKEN) return { providerName: "codex", keyAlias: "env:CODEX_ACCESS_TOKEN", keySource: "env", apiKey: process.env.CODEX_ACCESS_TOKEN };
    const status = await codexTokenStore.status();
    if (status.active) return { providerName: "codex", keyAlias: "chatgpt", keySource: status.storage || "keychain", apiKey: undefined };
    return { providerName: "codex", keyAlias: "not-signed-in", keySource: "oauth", apiKey: undefined };
  }
  const activeAlias = apiKeyIndex.active?.[normalizedProvider];
  if (activeAlias) {
    const envKeyName = envKeyNameFromAlias(normalizedProvider, activeAlias);
    if (envKeyName) {
      const envValue = process.env[envKeyName];
      if (!envValue) throw new Error(`${envKeyName} is not set.`);
      return { providerName: normalizedProvider, keyAlias: activeAlias, keySource: "env", apiKey: envValue };
    }
    const entry = findStoredKey(normalizedProvider, activeAlias);
    if (entry) {
      const apiKey = await getKeychainPassword(entry.account);
      if (apiKey) return { providerName: normalizedProvider, keyAlias: activeAlias, keySource: "keychain", apiKey };
    }
  }

  const envKey = firstProviderEnvKey(normalizedProvider);
  if (envKey) {
    const envAlias = envAliasFor(envKey);
    apiKeyIndex.active[normalizedProvider] = envAlias;
    await saveApiKeyIndex();
    return { providerName: normalizedProvider, keyAlias: envAlias, keySource: "env", apiKey: process.env[envKey] };
  }

  return { providerName: "stub", keyAlias: "stub", keySource: "stub", apiKey: undefined };
}

async function loadSettings() {
  let parsed;
  try {
    parsed = JSON.parse(await readFile(settingsPath, "utf8"));
  } catch {
    try {
      parsed = JSON.parse(await readFile(legacySettingsPath, "utf8"));
    } catch {
      parsed = null;
    }
  }

  let defaultProvider = "";
  if (typeof parsed?.defaultProvider === "string" && parsed.defaultProvider.trim()) {
    try {
      defaultProvider = normalizeProvider(parsed.defaultProvider);
    } catch {
      defaultProvider = "";
    }
  }

  const defaultModels = {};
  if (parsed?.defaultModels && typeof parsed.defaultModels === "object") {
    for (const [provider, value] of Object.entries(parsed.defaultModels)) {
      try {
        const normalizedProvider = normalizeProvider(provider);
        defaultModels[normalizedProvider] = normalizeModel(value);
      } catch {
        // Drop invalid persisted providers/models.
      }
    }
  }

  let reasoningLevel = "medium";
  try {
    reasoningLevel = normalizeReasoningLevel(parsed?.reasoningLevel || "medium");
  } catch {
    reasoningLevel = "medium";
  }

  const normalized = { version: 1, defaultProvider, defaultModels, reasoningLevel };
  await saveSettings(normalized);
  return normalized;
}
async function saveSettings(nextSettings = settings) {
  await mkdir(configDir, { recursive: true });
  await writeFile(settingsPath, `${JSON.stringify(nextSettings, null, 2)}\n`, { mode: 0o600 });
}

async function loadApiKeyIndex() {
  let parsed;
  try {
    parsed = JSON.parse(await readFile(keyIndexPath, "utf8"));
  } catch {
    try {
      parsed = JSON.parse(await readFile(legacyKeyIndexPath, "utf8"));
    } catch {
      parsed = null;
    }
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
  const active = {};
  if (typeof parsed?.active === "object" && parsed.active) {
    for (const [provider, alias] of Object.entries(parsed.active)) {
      try {
        active[normalizeProvider(provider)] = String(alias);
      } catch {
        // Drop active provider entries that are no longer supported.
      }
    }
  }
  const index = {
    version: 1,
    active,
    keys: normalized,
  };
  await saveApiKeyIndex(index);
  return index;
}
async function saveApiKeyIndex(index = apiKeyIndex) {
  await mkdir(configDir, { recursive: true });
  await writeFile(keyIndexPath, `${JSON.stringify(index, null, 2)}\n`, { mode: 0o600 });
}

async function listApiKeys() {
  const activeProvider = runtimeState?.providerName === "stub" ? activeProviderName : runtimeState?.providerName || activeProviderName;
  const activeAlias = apiKeyIndex.active?.[activeProvider] || runtimeState?.keyAlias || "";
  const keys = [];

  for (const providerName of providerNames) {
    for (const envKey of providerDefinitions[providerName].envKeys || []) {
      const value = process.env[envKey];
      if (!value) continue;
      const alias = envAliasFor(envKey);
      keys.push({
        provider: providerName,
        alias,
        source: "env",
        active: activeProvider === providerName && activeAlias === alias,
        masked: maskApiKey(value),
        readonly: true,
      });
    }
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

async function listModels() {
  const activeProvider = runtimeState?.providerName === "stub" ? activeProviderName : runtimeState?.providerName || activeProviderName;
  const knownModels = activeProvider === "openrouter"
    ? await listOpenRouterModels()
    : providerModelLists[activeProvider] || [];
  const known = knownModels.some((candidate) => candidate.id === model)
    ? knownModels
    : [{ id: model, name: model, description: "Configured model" }, ...knownModels];
  return known.map((candidate) => ({ ...candidate, active: candidate.id === model }));
}

async function listOpenRouterModels() {
  const fallback = providerModelLists.openrouter;
  const now = Date.now();
  if (openRouterModelsCache && now - openRouterModelsCacheAt < 5 * 60 * 1000) {
    return openRouterModelsCache;
  }

  try {
    const apiKey = await resolveProviderApiKey("openrouter");
    const headers = {
      "HTTP-Referer": process.env.OPENROUTER_HTTP_REFERER || "https://github.com/zayr0-9/qubit",
      "X-Title": process.env.OPENROUTER_APP_TITLE || "Qubit",
      ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
    };
    const response = await fetch("https://openrouter.ai/api/v1/models", { headers });
    if (!response.ok) throw new Error(`OpenRouter models request failed: HTTP ${response.status}`);
    const payload = await response.json();
    const remoteModels = Array.isArray(payload?.data)
      ? payload.data
        .filter((item) => item && typeof item.id === "string")
        .map((item) => ({
          id: item.id,
          name: item.name || item.id,
          description: openRouterModelDescription(item),
        }))
      : [];
    if (remoteModels.length === 0) return fallback;
    openRouterModelsCache = remoteModels;
    openRouterModelsCacheAt = now;
    return remoteModels;
  } catch (error) {
    return fallback;
  }
}

function openRouterModelDescription(item) {
  const contextLength = Number.isFinite(item?.context_length) ? `${item.context_length.toLocaleString()} context` : "OpenRouter model";
  const pricing = item?.pricing;
  const prompt = pricing?.prompt;
  const completion = pricing?.completion;
  if (prompt !== undefined && completion !== undefined) {
    return `${contextLength} · prompt ${prompt}/token · completion ${completion}/token`;
  }
  return contextLength;
}

async function resolveProviderApiKey(provider) {
  const normalizedProvider = normalizeProvider(provider);
  const activeAlias = apiKeyIndex.active?.[normalizedProvider];
  if (activeAlias) {
    const envKeyName = envKeyNameFromAlias(normalizedProvider, activeAlias);
    if (envKeyName) return process.env[envKeyName] || "";
    const entry = findStoredKey(normalizedProvider, activeAlias);
    if (entry) {
      try {
        return await getKeychainPassword(entry.account) || "";
      } catch {
        return "";
      }
    }
  }
  const envKey = firstProviderEnvKey(normalizedProvider);
  return envKey ? process.env[envKey] || "" : "";
}

async function switchProvider(id, provider, persistDefault = false) {
  try {
    const normalizedProvider = normalizeProvider(provider);
    activeProviderName = normalizedProvider;
    if (persistDefault) {
      settings.defaultProvider = normalizedProvider;
      await saveSettings();
    }
    model = defaultModelForProvider(normalizedProvider);
    runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
    const defaultStatus = persistDefault ? " Saved as default provider." : "";
    await writeModelUpdated(id, `Using provider ${runtimeState.providerName} with model ${model}.${defaultStatus}`);
  } catch (error) {
    write({ type: "error", id, error: error instanceof Error ? error.message : String(error) });
  }
}

async function writeModelList(id) {
  write({
    type: "model.list",
    id,
    provider: runtimeState.providerName,
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    model,
    maxContext: activeModelMaxContext(),
    reasoningLevel,
    models: await listModels(),
  });
}

async function writeModelUpdated(id, status) {
  write({
    type: "model.updated",
    id,
    provider: runtimeState.providerName,
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    model,
    maxContext: activeModelMaxContext(),
    reasoningLevel,
    status,
    models: await listModels(),
  });
}

function normalizeModel(value) {
  const normalized = String(value || "").trim();
  if (!normalized) throw new Error("Model is required.");
  if (!/^[A-Za-z0-9_.:/-]{1,128}$/.test(normalized)) {
    throw new Error("Model may contain only letters, numbers, dash, underscore, dot, colon, and slash.");
  }
  return normalized;
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

async function writeCodexStatus(id) {
  const status = await codexTokenStore.status();
  write({
    type: "codex.status",
    id,
    provider: "codex",
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.providerName === "codex" ? runtimeState.keyAlias : undefined,
    model,
    ...status,
    status: status.active ? "Signed in to ChatGPT Codex." : "Not signed in to ChatGPT Codex.",
  });
}

async function handleCodexLoginStart(id) {
  try {
    const login = await startCodexLogin({
      tokenStore: codexTokenStore,
      issuer: process.env.CODEX_ISSUER || "https://auth.openai.com",
      clientId: process.env.CODEX_CLIENT_ID || "app_EMoamEEZ73f0CkXaXp7hrann",
      originator: process.env.CODEX_ORIGINATOR || "codex_cli_rs",
      allowedWorkspaceId: process.env.QUBIT_CODEX_ALLOWED_WORKSPACE_ID,
    });
    write({ type: "codex.login.started", id, authUrl: login.authUrl, localPort: login.localPort });
    login.completed.then(async (result) => {
      if (activeProviderName === "codex") runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
      write({
        type: "codex.login.completed",
        id,
        provider: "codex",
        activeProvider: runtimeState.providerName,
        activeKeyAlias: runtimeState.providerName === "codex" ? runtimeState.keyAlias : "chatgpt",
        model,
        ...result,
      });
    }).catch((error) => {
      write({ type: "codex.error", id, error: redactSecrets(error instanceof Error ? error.message : String(error)) });
    });
  } catch (error) {
    write({ type: "codex.error", id, error: redactSecrets(error instanceof Error ? error.message : String(error)) });
  }
}

async function handleCodexLoginCancel(id) {
  const cancelled = await cancelCodexLogin();
  write({ type: cancelled ? "codex.login.cancelled" : "codex.status", id, status: cancelled ? "Codex login cancelled." : "No Codex login is active." });
}

async function handleCodexLogout(id) {
  await codexTokenStore.delete();
  runtimeState = await createRuntimeState(runtimeState?.promptMode || "plan");
  write({
    type: "codex.logout.completed",
    id,
    provider: "codex",
    activeProvider: runtimeState.providerName,
    activeKeyAlias: runtimeState.keyAlias,
    status: "Signed out of ChatGPT Codex.",
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
  const normalizedProvider = normalizeProvider(provider || defaultProviderName);
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
  const normalizedProvider = normalizeProvider(provider || defaultProviderName);
  const normalizedAlias = normalizeAlias(alias);
  const envKeyName = envKeyNameFromAlias(normalizedProvider, normalizedAlias);
  if (envKeyName) {
    if (!process.env[envKeyName]) throw new Error(`${envKeyName} is not set.`);
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
  const normalizedProvider = normalizeProvider(provider || defaultProviderName);
  const normalizedAlias = normalizeAlias(alias);
  if (envKeyNameFromAlias(normalizedProvider, normalizedAlias)) throw new Error("Environment API keys are read-only and cannot be deleted.");
  const index = apiKeyIndex.keys.findIndex((key) => key.provider === normalizedProvider && key.alias === normalizedAlias);
  if (index < 0) throw new Error(`Unknown API key alias: ${normalizedProvider}/${normalizedAlias}`);
  const [entry] = apiKeyIndex.keys.splice(index, 1);
  await deleteKeychainPassword(entry.account);
  if (apiKeyIndex.active?.[normalizedProvider] === normalizedAlias) {
    const envKey = firstProviderEnvKey(normalizedProvider);
    if (envKey) apiKeyIndex.active[normalizedProvider] = envAliasFor(envKey);
    else delete apiKeyIndex.active[normalizedProvider];
  }
  await saveApiKeyIndex();
}

function normalizeProvider(provider) {
  const normalized = String(provider || "glm").trim().toLowerCase();
  const canonical = providerAliasMap.get(normalized);
  if (!canonical) throw new Error(`Unsupported provider: ${normalized || "empty"}. Supported providers: ${providerNames.join(", ")}.`);
  return canonical;
}

function firstEnvValue(names = []) {
  const name = names.find((candidate) => process.env[candidate]);
  return name ? process.env[name] : undefined;
}

function firstProviderEnvKey(provider) {
  const definition = providerDefinitions[normalizeProvider(provider)];
  return (definition.envKeys || []).find((name) => process.env[name]);
}

function envAliasFor(envKey) {
  return `env:${envKey}`;
}

function envKeyNameFromAlias(provider, alias) {
  const normalizedProvider = normalizeProvider(provider);
  const normalizedAlias = String(alias || "").trim();
  if (!normalizedAlias.startsWith("env:")) return "";
  const envKey = normalizedAlias.slice(4);
  return (providerDefinitions[normalizedProvider].envKeys || []).includes(envKey) ? envKey : "";
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
  throw new Error(`Secure OS keychain storage is unavailable: ${reason}. Install Qubit's keychain dependency and required OS keychain support, or use an environment API key for the selected provider.`);
}

function maskApiKey(value) {
  const text = String(value || "");
  if (text.length <= 8) return "••••";
  return `${text.slice(0, 4)}…${text.slice(-4)}`;
}

function redactSecrets(value) {
  return String(value || "")
    .replace(/(authorization\s*[:=]\s*bearer\s+)[A-Za-z0-9._~+/-]+/gi, "$1[redacted]")
    .replace(/(\/auth\/callback\?[^\s]*?\bstate=)([^\s&#]+)/gi, "$1[redacted]")
    .replace(/\b(?:access_token|refresh_token|id_token|subject_token|requested_token|code)=([^\s&#]+)/gi, (match, _value) => match.replace(/=([^\s&#]+)/, "=[redacted]"))
    .replace(/\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/g, "[redacted-jwt]")
    .replace(/\b(?:sk|zai|key|token)[-_][A-Za-z0-9_.-]{8,}/gi, "[redacted]");
}

function plainObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function compactObject(value, allowedKeys) {
  const source = plainObject(value);
  const compact = {};
  for (const key of allowedKeys) {
    if (source[key] !== undefined) compact[key] = source[key];
  }
  return compact;
}

function previewText(value, maxChars = 1200) {
  if (value === undefined || value === null) return "";
  const text = typeof value === "string" ? value : JSON.stringify(value, null, 2);
  const redacted = redactSecrets(text || "");
  return redacted.length > maxChars ? `${redacted.slice(0, maxChars)}…` : redacted;
}

function contextCharCount(value) {
  if (value === undefined || value === null) return 0;
  const text = typeof value === "string" ? value : JSON.stringify(value, null, 2);
  return [...redactSecrets(text || "")].length;
}

function resultPayload(result) {
  if (!result || typeof result !== "object") return result;
  if (result.data !== undefined) return result.data;
  if (result.output !== undefined) return result.output;
  return result;
}

function summarizeToolArgs(toolName, args) {
  const source = plainObject(args);
  switch (toolName) {
    case "readFile":
      return compactObject(source, ["path", "cwd", "maxBytes", "startLine", "endLine", "ranges", "includeHash"]);
    case "readFileContinuation":
      return compactObject(source, ["path", "cwd", "afterLine", "numLines", "maxBytes", "includeHash"]);
    case "readFiles":
      return compactObject(source, ["paths", "cwd", "baseDir", "maxBytes", "startLine", "endLine", "ranges"]);
    case "glob":
      return compactObject(source, ["pattern", "cwd", "ignore", "dot", "absolute", "nodir", "maxMatches"]);
    case "ripgrep":
      return compactObject(source, ["pattern", "searchPath", "cwd", "glob", "caseSensitive", "lineNumbers", "count", "filesWithMatches", "maxCount", "contextLines"]);
    case "bash":
    case "powershell":
      return compactObject(source, ["command", "description", "cwd", "timeoutMs", "maxOutputChars"]);
    case "createFile":
      return { ...compactObject(source, ["path", "cwd", "createParentDirs", "overwrite", "executable", "operationMode"]), contentPreview: previewText(source.content, 600) };
    case "editFile":
      return { ...compactObject(source, ["path", "operation", "cwd", "approxStartLine", "approxEndLine", "createBackup", "operationMode", "validateContent"]), searchPreview: previewText(source.searchPattern, 400), replacementPreview: previewText(source.replacement, 400), contentPreview: previewText(source.content, 400) };
    case "multiEdit":
      return { ...compactObject(source, ["cwd", "stopOnError", "createBackup", "operationMode", "validateContent"]), edits: Array.isArray(source.edits) ? source.edits.map((edit) => ({ ...compactObject(edit, ["path", "operation", "approxStartLine", "approxEndLine"]), searchPreview: previewText(edit?.searchPattern, 400), replacementPreview: previewText(edit?.replacement, 400), contentPreview: previewText(edit?.content, 400) })).slice(0, 20) : undefined };
    case "multiCall":
      return { ...compactObject(source, ["stopOnError"]), calls: Array.isArray(source.calls) ? source.calls.map((call) => {
        const tool = typeof call?.tool === "string" ? call.tool : "";
        return { tool, args: tool && tool !== "multiCall" ? summarizeToolArgs(tool, call.args || {}) : compactObject(call.args || {}, ["stopOnError"]) };
      }).slice(0, 20) : undefined };
    case "deleteFile":
      return compactObject(source, ["path", "cwd", "allowedExtensions", "operationMode"]);
    case "todoMd":
      return { ...compactObject(source, ["action", "name", "cwd", "search"]), contentPreview: previewText(source.content, 600), replacementPreview: previewText(source.replacement, 600) };
    case "planMd":
      return { ...compactObject(source, ["action", "name", "cwd", "search"]), contentPreview: previewText(source.content, 600), replacementPreview: previewText(source.replacement, 600) };
    default:
      return JSON.parse(JSON.stringify(source, (_key, value) => typeof value === "string" ? previewText(value, 1000) : value));
  }
}

function toolCallContextChars(toolName, args, result) {
  const sourceArgs = plainObject(args);
  const payload = resultPayload(result);
  const data = plainObject(payload);
  switch (toolName) {
    case "readFile":
    case "readFileContinuation":
      return contextCharCount(data.content);
    case "readFiles":
      return Array.isArray(data.files) ? data.files.reduce((total, file) => total + contextCharCount(file?.content), 0) : 0;
    case "bash":
    case "powershell":
      return contextCharCount(data.stdout) + contextCharCount(data.stderr) + contextCharCount(data.error);
    case "multiCall": {
      const outerArgs = plainObject(args);
      const argCalls = Array.isArray(outerArgs.calls) ? outerArgs.calls : [];
      return Array.isArray(data.results) ? data.results.reduce((total, item) => {
        const index = Number.isFinite(Number(item?.index)) ? Number(item.index) : -1;
        const argCall = index >= 0 ? argCalls[index] : undefined;
        const nestedTool = item?.tool || argCall?.tool;
        const nestedArgs = argCall?.args || {};
        const nestedPayload = item?.data !== undefined ? item.data : item?.output;
        return total + toolCallContextChars(nestedTool, nestedArgs, { ok: item?.ok, data: nestedPayload, error: item?.error });
      }, 0) : 0;
    }
    default:
      return contextCharCount(sourceArgs) + contextCharCount(payload);
  }
}

function summarizeToolResult(toolName, result) {
  const base = plainObject(result);
  const payload = resultPayload(result);
  const data = plainObject(payload);
  const summary = { ok: Boolean(base.ok), ...(base.error ? { error: previewText(base.error, 1200) } : {}) };

  switch (toolName) {
    case "readFile":
    case "readFileContinuation":
      return { ...summary, ...compactObject(data, ["truncated", "sizeBytes", "totalLines", "startLine", "endLine", "ranges", "metadata"]), contentPreview: previewText(data.content, 2400) };
    case "readFiles":
      return { ...summary, fileCount: Array.isArray(data.files) ? data.files.length : undefined, files: Array.isArray(data.files) ? data.files.slice(0, 20).map((file) => ({ filename: file.filename, totalLines: file.totalLines, contentPreview: previewText(file.content, 1200) })) : undefined };
    case "glob":
      return { ...summary, success: data.success, pattern: data.pattern, cwd: data.cwd, error: previewText(data.error, 1200), matchCount: Array.isArray(data.matches) ? data.matches.length : 0, matches: Array.isArray(data.matches) ? data.matches.slice(0, 30) : undefined };
    case "ripgrep":
      return { ...summary, success: data.success, searchPath: data.searchPath, command: data.command, error: previewText(data.error, 1200), matchCount: Array.isArray(data.matches) ? data.matches.length : 0, matches: Array.isArray(data.matches) ? data.matches.slice(0, 30) : undefined };
    case "bash":
    case "powershell":
      return { ...summary, ...compactObject(data, ["success", "cwd", "exitCode", "timedOut", "durationMs", "command"]), stdoutPreview: previewText(data.stdout, 2400), stderrPreview: previewText(data.stderr, 1600), error: previewText(data.error || base.error, 1200) };
    case "createFile":
    case "deleteFile":
      return { ...summary, ...compactObject(data, ["success", "created", "deleted", "sizeBytes", "path", "message"]) };
    case "editFile":
      return { ...summary, ...compactObject(data, ["success", "sizeBytes", "replacements", "message", "backup", "matchStrategy", "lineInfo"]) };
    case "multiEdit":
      return { ...summary, ...compactObject(data, ["success", "message", "applied", "failed", "stoppedEarly"]), results: Array.isArray(data.results) ? data.results.slice(0, 20).map((item) => compactObject(item, ["path", "operation", "success", "replacements", "message", "matchStrategy", "lineInfo", "searchPreview", "replacementPreview", "contentPreview"])) : undefined };
    case "multiCall":
      return { ...summary, ...compactObject(data, ["success", "message", "completed", "failed", "stoppedEarly"]), results: Array.isArray(data.results) ? data.results.slice(0, 20).map((item) => {
        const nestedPayload = item.data !== undefined ? item.data : item.output;
        return { ...compactObject(item, ["index", "tool", "ok", "permission", "error"]), result: summarizeToolResult(item.tool, { ok: item.ok, data: nestedPayload, error: item.error }) };
      }) : undefined };
    case "todoMd":
      return { ...summary, ...compactObject(data, ["id", "created", "exists", "success", "message", "modifiedAt"]), contentPreview: previewText(data.content, 1600) };
    case "planMd":
      return { ...summary, ...compactObject(data, ["name", "created", "exists", "success", "message", "modifiedAt", "displayed", "path"]), planCount: Array.isArray(payload) ? payload.length : undefined, contentPreview: data.displayed ? undefined : previewText(data.content, 1600) };
    default:
      return { ...summary, payloadPreview: previewText(payload, 2400) };
  }
}

function redactMessage(message) {
  if (!message || typeof message !== "object") return message;
  return redactMessageValue(message);
}

function redactMessageValue(value) {
  if (typeof value === "string") return redactSecrets(value);
  if (Array.isArray(value)) return value.map((item) => redactMessageValue(item));
  if (!value || typeof value !== "object") return value;
  const clone = {};
  for (const [key, item] of Object.entries(value)) {
    if (/apiKey|access_token|refresh_token|id_token|authorization|subject_token|code_verifier/i.test(key)) clone[key] = "[redacted]";
    else clone[key] = redactMessageValue(item);
  }
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
      ...(typeof session.forkedFromSessionId === "string" ? { forkedFromSessionId: session.forkedFromSessionId } : {}),
      ...(Number.isFinite(session.forkedFromMessageIndex) ? { forkedFromMessageIndex: session.forkedFromMessageIndex } : {}),
      ...(typeof session.forkedAt === "string" ? { forkedAt: session.forkedAt } : {}),
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

async function forkSession({ sourceSessionId, messageIndex, title, includeUiMessageAtIndex = true } = {}) {
  const source = sessionIndex.sessions.find((candidate) => candidate.id === sourceSessionId);
  if (!source) return null;

  const sourceMessages = await storage.loadMessages(sourceSessionId);
  const sourceUiMessages = transcriptMessagesForUi(sourceMessages);
  const requestedIndex = Number.isFinite(messageIndex) ? Math.trunc(messageIndex) : sourceUiMessages.length;
  const normalizedUiIndex = Math.max(0, Math.min(sourceUiMessages.length, requestedIndex));
  const normalizedIndex = includeUiMessageAtIndex
    ? (normalizedUiIndex >= sourceUiMessages.length
      ? sourceMessages.length
      : rawMessageIndexForUiMessageIndex(sourceMessages, normalizedUiIndex))
    : rawMessageStartIndexForUiMessageIndex(sourceMessages, normalizedUiIndex);
  const forkMessages = sourceMessages.slice(0, normalizedIndex);
  const forkTitle = String(title || "").trim() || `Fork: ${source.title || "Untitled chat"}`;
  const now = new Date().toISOString();
  const session = {
    ...newSession(createSessionId(), forkTitle, now),
    forkedFromSessionId: source.id,
    forkedFromMessageIndex: normalizedIndex,
    forkedAt: now,
    messageCount: forkMessages.filter((message) => message.role !== "system").length,
  };

  await storage.saveMessages(session.id, forkMessages);
  const metadata = storage.getSessionMetadata ? await storage.getSessionMetadata(sourceSessionId) : null;
  if (metadata && storage.setSessionMetadata) {
    await storage.setSessionMetadata(session.id, {
      ...metadata,
      updatedAt: now,
      custom: {
        ...(metadata.custom || {}),
        forkedFromSessionId: source.id,
        forkedFromMessageIndex: normalizedIndex,
        forkedAt: now,
      },
    });
  }

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
  await saveSessionIndex();
  return session;
}

async function deleteSession(sessionId) {
  const index = sessionIndex.sessions.findIndex((candidate) => candidate.id === sessionId);
  if (index < 0) return null;
  if (sessionIndex.sessions.length <= 1) throw new Error("Cannot delete the only session.");
  const [deleted] = sessionIndex.sessions.splice(index, 1);
  if (storage.deleteSession) await storage.deleteSession(sessionId);
  if (activeSessionId === sessionId || sessionIndex.activeSessionId === sessionId) {
    const next = sessionIndex.sessions[Math.min(index, sessionIndex.sessions.length - 1)] || sessionIndex.sessions[0];
    activeSessionId = next.id;
    sessionIndex.activeSessionId = next.id;
  }
  await saveSessionIndex();
  return deleted;
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
  activeSessionId = session.id;
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
    sessions: sessionsByRecentActivity(sessionIndex.sessions),
  });
}

function sessionsByRecentActivity(sessions = []) {
  return [...sessions].sort((a, b) => {
    const left = sessionRecentTimestamp(a);
    const right = sessionRecentTimestamp(b);
    if (left !== right) return right.localeCompare(left);
    return String(a?.id || "").localeCompare(String(b?.id || ""));
  });
}

function sessionRecentTimestamp(session) {
  return session?.updatedAt || session?.createdAt || "";
}

async function loadSessionMessages(sessionId) {
  const messages = await storage.loadMessages(sessionId);
  const uiMessages = transcriptMessagesForUi(messages);
  return await hydratePlanViewMessages(uiMessages);
}

async function hydratePlanViewMessages(uiMessages) {
  const hydrated = [];
  for (const message of uiMessages) {
    hydrated.push(message);
    const planViews = await planViewMessagesForToolGroup(message?.toolGroup);
    for (const planView of planViews) {
      hydrated.push(planView);
    }
  }
  return hydrated;
}

async function planViewMessagesForToolGroup(group) {
  if (!group || group.name !== "planMd" || !Array.isArray(group.calls)) return [];
  const views = [];
  for (const call of group.calls) {
    const args = plainObject(call.args);
    const result = plainObject(call.result);
    if (!(args.action === "display" && result.displayed === true)) continue;
    const name = String(result.name || args.name || "plan");
    const planPath = String(result.path || join(dataDir, "plans", `${name}.md`));
    let content = "";
    try {
      content = await readFile(planPath, "utf8");
    } catch {
      content = `Plan \"${name}\" was displayed, but ${planPath} could not be loaded.`;
    }
    views.push({ role: "view", viewType: "plan", title: `Plan: ${name}`, path: planPath, content });
  }
  return views;
}

async function buildSessionTreeNodes(focalSessionId = activeSessionId) {
  const previewCache = new Map();
  const sessions = relatedForkSessions(sessionIndex.sessions, focalSessionId);
  const messageNodesBySession = await buildMessageTreeNodesForSessions(sessions, previewCache);
  const nodes = [];
  for (const session of sessions) {
    const node = {
      id: session.id,
      sessionId: session.id,
      sessionTitle: session.title || "Untitled chat",
      parentSessionId: typeof session.forkedFromSessionId === "string" ? session.forkedFromSessionId : "",
      forkedFromMessageIndex: Number.isFinite(session.forkedFromMessageIndex) ? session.forkedFromMessageIndex : 0,
      forkedAt: typeof session.forkedAt === "string" ? session.forkedAt : "",
      createdAt: session.createdAt || "",
      updatedAt: session.updatedAt || "",
      messageCount: Number.isFinite(session.messageCount) ? session.messageCount : 0,
      messageRole: "",
      messageContent: "",
      assistantRole: "",
      assistantContent: "",
      lineageMessages: [],
      messageNodes: messageNodesBySession.get(session.id) || [],
    };

    let preview = null;
    if (node.parentSessionId) {
      preview = await divergentTextPreviewForFork(session.id, node.forkedFromMessageIndex, previewCache)
        || await textPreviewForFork(node.parentSessionId, node.forkedFromMessageIndex, previewCache);
    } else {
      preview = await firstTextPreviewForSession(session.id, previewCache);
    }
    if (preview) {
      node.messageRole = preview.role;
      node.messageContent = preview.content;
    }
    const assistantPreview = await assistantTextPreviewForSession(session.id, previewCache);
    if (assistantPreview) {
      node.assistantRole = assistantPreview.role;
      node.assistantContent = assistantPreview.content;
      if (!node.messageContent) {
        node.messageRole = assistantPreview.role;
        node.messageContent = assistantPreview.content;
      }
    }
    node.lineageMessages = await lineageTextMessagesForSession(session.id, previewCache);
    nodes.push(node);
  }
  return nodes;
}

async function buildMessageTreeNodesForSessions(sessions, cache) {
  const byId = new Map();
  for (const session of sessions) {
    if (session?.id) byId.set(session.id, session);
  }

  const rawMessagesBySession = new Map();
  const sessionMessages = new Map();
  for (const session of sessions) {
    const rawMessages = await rawSessionMessagesCached(session.id, cache);
    rawMessagesBySession.set(session.id, rawMessages);
    sessionMessages.set(session.id, textTreeMessagesFromRawMessages(rawMessages));
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
  return treeTextMessage({ role: message.role, content: message.content });
}

function compareSessionsForTree(a, b) {
  const left = a?.forkedAt || a?.createdAt || a?.updatedAt || "";
  const right = b?.forkedAt || b?.createdAt || b?.updatedAt || "";
  if (left === right) return String(a?.title || "").localeCompare(String(b?.title || ""));
  return left.localeCompare(right);
}

function relatedForkSessions(sessions, focalSessionId = activeSessionId) {
  if (!Array.isArray(sessions) || sessions.length === 0) return [];

  const idToSession = new Map();
  for (const session of sessions) {
    if (session && typeof session.id === "string" && session.id) {
      idToSession.set(session.id, session);
    }
  }
  if (idToSession.size === 0) return [];

  let startId = typeof focalSessionId === "string" ? focalSessionId : "";
  if (!idToSession.has(startId)) {
    startId = idToSession.has(activeSessionId) ? activeSessionId : sessions.find((session) => idToSession.has(session?.id))?.id;
  }
  if (!startId || !idToSession.has(startId)) return [];

  const adjacency = new Map();
  for (const id of idToSession.keys()) adjacency.set(id, new Set());
  for (const session of idToSession.values()) {
    const parentId = typeof session.forkedFromSessionId === "string" ? session.forkedFromSessionId : "";
    if (!parentId || !idToSession.has(parentId) || parentId === session.id) continue;
    adjacency.get(session.id).add(parentId);
    adjacency.get(parentId).add(session.id);
  }

  const visited = new Set();
  const queue = [startId];
  while (queue.length > 0) {
    const id = queue.shift();
    if (!id || visited.has(id)) continue;
    visited.add(id);
    for (const nextId of adjacency.get(id) || []) {
      if (!visited.has(nextId)) queue.push(nextId);
    }
  }

  return sessions.filter((session) => visited.has(session?.id));
}

async function textPreviewForFork(sessionId, rawForkIndex, cache) {
  const messages = await rawSessionMessagesCached(sessionId, cache);
  const normalizedIndex = Math.max(0, Math.min(messages.length, Number.isFinite(rawForkIndex) ? Math.trunc(rawForkIndex) : messages.length));
  return lastTextPreview(messages.slice(0, normalizedIndex));
}

async function divergentTextPreviewForFork(sessionId, rawForkIndex, cache) {
  const messages = await rawSessionMessagesCached(sessionId, cache);
  const normalizedIndex = Math.max(0, Math.min(messages.length, Number.isFinite(rawForkIndex) ? Math.trunc(rawForkIndex) : 0));
  return firstTextPreview(messages.slice(normalizedIndex));
}

async function firstTextPreviewForSession(sessionId, cache) {
  const messages = await rawSessionMessagesCached(sessionId, cache);
  const uiMessages = transcriptMessagesForUi(messages);
  for (const message of uiMessages) {
    const preview = treeTextMessage(message);
    if (preview) return preview;
  }
  return null;
}

async function lineageTextMessagesForSession(sessionId, cache) {
  const messages = await rawSessionMessagesCached(sessionId, cache);
  return transcriptMessagesForUi(messages)
    .map((message) => treeTextMessage(message))
    .filter(Boolean);
}

async function assistantTextPreviewForSession(sessionId, cache) {
  const messages = await rawSessionMessagesCached(sessionId, cache);
  const uiMessages = transcriptMessagesForUi(messages);
  for (const message of uiMessages) {
    const preview = treeTextMessage(message);
    if (preview?.role === "assistant") return preview;
  }
  return null;
}

async function rawSessionMessagesCached(sessionId, cache) {
  if (cache.has(sessionId)) return cache.get(sessionId);
  let messages = [];
  try {
    messages = await storage.loadMessages(sessionId);
  } catch {
    messages = [];
  }
  messages = overlayActiveRunUserMessages(messages, activeRunInputsForSession(sessionId));
  cache.set(sessionId, messages);
  return messages;
}

function activeRunInputsForSession(sessionId) {
  const normalizedSessionId = String(sessionId || "");
  return [...activeRuns.values()]
    .filter((run) => run?.sessionId === normalizedSessionId && typeof run.input === "string" && run.input.trim())
    .map((run) => run.input);
}

function lastTextPreview(messages) {
  const uiMessages = transcriptMessagesForUi(messages);
  for (let index = uiMessages.length - 1; index >= 0; index -= 1) {
    const message = uiMessages[index];
    const preview = treeTextMessage(message);
    if (preview) return preview;
  }
  return null;
}

function firstTextPreview(messages) {
  const uiMessages = transcriptMessagesForUi(messages);
  for (const message of uiMessages) {
    const preview = treeTextMessage(message);
    if (preview) return preview;
  }
  return null;
}

function treeTextMessage(message) {
  const role = normalizeTextMessageRole(message?.role);
  if (!role) return null;
  const content = textContentFromMessage(message).trim();
  return content ? { role, content } : null;
}

function normalizeTextMessageRole(role) {
  if (role === "user") return "user";
  if (role === "assistant" || role === "agent") return "assistant";
  return "";
}

function textContentFromMessage(message) {
  const content = message?.content;
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .map((part) => {
        if (!part || typeof part !== "object") return "";
        if (typeof part.text === "string") return part.text;
        if (typeof part.content === "string") return part.content;
        return "";
      })
      .filter(Boolean)
      .join("\n");
  }
  return String(content ?? "");
}

function rawMessageStartIndexForUiMessageIndex(messages, uiMessageIndex) {
  if (uiMessageIndex <= 0) return 0;
  const visibleStartIndexes = [];
  const consumedToolResults = new Set();

  for (let index = 0; index < messages.length; index += 1) {
    const message = messages[index];
    if (!message || message.role === "system") continue;

    if (message.role === "user") {
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) visibleStartIndexes.push(index);
      continue;
    }

    if (message.role === "assistant") {
      const toolCalls = Array.isArray(message.toolCalls) ? message.toolCalls : [];
      if (message.reasoningContent) visibleStartIndexes.push(index);
      if (toolCalls.length > 0) visibleStartIndexes.push(index);
      for (const toolCall of toolCalls) {
        const toolCallId = toolCall.id || "";
        if (!toolCallId) continue;
        const resultIndex = messages.findIndex((candidate, candidateIndex) => candidateIndex > index && candidate?.role === "tool" && candidate.toolCallId === toolCallId);
        if (resultIndex >= 0) consumedToolResults.add(toolCallId);
      }
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) visibleStartIndexes.push(index);
      continue;
    }

    if (message.role === "tool" && message.toolCallId && !consumedToolResults.has(message.toolCallId)) {
      visibleStartIndexes.push(index);
      consumedToolResults.add(message.toolCallId);
    }
  }

  if (uiMessageIndex >= visibleStartIndexes.length) return messages.length;
  return visibleStartIndexes[uiMessageIndex];
}

function rawMessageIndexForUiMessageIndex(messages, uiMessageIndex) {
  if (uiMessageIndex <= 0) return 0;
  const visibleIndexes = [];
  const consumedToolResults = new Set();

  for (let index = 0; index < messages.length; index += 1) {
    const message = messages[index];
    if (!message || message.role === "system") continue;

    if (message.role === "user") {
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) visibleIndexes.push(index + 1);
      continue;
    }

    if (message.role === "assistant") {
      const toolCalls = Array.isArray(message.toolCalls) ? message.toolCalls : [];
      if (message.reasoningContent) visibleIndexes.push(index + 1);
      if (toolCalls.length > 0) {
        let groupEnd = index + 1;
        for (const toolCall of toolCalls) {
          const toolCallId = toolCall.id || "";
          if (!toolCallId) continue;
          const resultIndex = messages.findIndex((candidate, candidateIndex) => candidateIndex > index && candidate?.role === "tool" && candidate.toolCallId === toolCallId);
          if (resultIndex >= 0) {
            consumedToolResults.add(toolCallId);
            groupEnd = Math.max(groupEnd, resultIndex + 1);
          }
        }
        visibleIndexes.push(groupEnd);
      }
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) visibleIndexes.push(index + 1);
      continue;
    }

    if (message.role === "tool" && message.toolCallId && !consumedToolResults.has(message.toolCallId)) {
      visibleIndexes.push(index + 1);
      consumedToolResults.add(message.toolCallId);
    }
  }

  if (uiMessageIndex >= visibleIndexes.length) return messages.length;
  return visibleIndexes[uiMessageIndex];
}

function transcriptMessagesForUi(messages) {
  const toolResults = new Map();
  for (const message of messages) {
    if (message?.role !== "tool") continue;
    const toolCallId = message.toolCallId || "";
    if (!toolCallId) continue;
    toolResults.set(toolCallId, message);
  }

  const uiMessages = [];
  let reconstructedStep = 0;
  const consumedToolResults = new Set();

  for (const message of messages) {
    if (!message || message.role === "system") continue;

    if (message.role === "user") {
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) uiMessages.push({ role: "user", content });
      continue;
    }

    if (message.role === "assistant") {
      const reasoningContent = typeof message.reasoningContent === "string" ? message.reasoningContent : String(message.reasoningContent ?? "");
      if (reasoningContent.trim()) uiMessages.push({ role: "reasoning", content: reasoningContent });
      const toolCalls = Array.isArray(message.toolCalls) ? message.toolCalls : [];
      if (toolCalls.length > 0) {
        reconstructedStep += 1;
        uiMessages.push(...toolGroupsFromStoredToolCalls(toolCalls, toolResults, consumedToolResults, reconstructedStep));
      }
      const content = typeof message.content === "string" ? message.content : String(message.content ?? "");
      if (content.trim()) uiMessages.push({ role: "assistant", content, ...(reasoningContent.trim() ? { reasoningContent } : {}) });
      continue;
    }

    if (message.role === "tool" && message.toolCallId && !consumedToolResults.has(message.toolCallId)) {
      reconstructedStep += 1;
      uiMessages.push(toolGroupMessageFromStoredToolResult(message, reconstructedStep));
      consumedToolResults.add(message.toolCallId);
    }
  }

  return uiMessages;
}

function toolGroupsFromStoredToolCalls(toolCalls, toolResults, consumedToolResults, step) {
  const groups = [];
  const byToolName = new Map();
  for (const [index, toolCall] of toolCalls.entries()) {
    const toolName = toolCall.toolName || toolCall.name || "tool";
    let group = byToolName.get(toolName);
    if (!group) {
      group = {
        id: `stored-tool-${step}-${toolName}-${groups.length}`,
        name: toolName,
        step,
        calls: [],
        expanded: false,
      };
      byToolName.set(toolName, group);
      groups.push(group);
    }

    const toolCallId = toolCall.id || `${toolName}-${step}-${index}`;
    const storedResult = toolResults.get(toolCallId);
    if (storedResult) consumedToolResults.add(toolCallId);
    const parsedResult = parseStoredToolResult(storedResult?.content);
    group.calls.push({
      id: toolCallId,
      name: toolName,
      step,
      status: storedToolStatus(toolName, parsedResult),
      args: summarizeToolArgs(toolName, toolCall.args),
      result: summarizeToolResult(toolName, parsedResult ?? { ok: false, error: "Missing stored tool result" }),
      contextChars: toolCallContextChars(toolName, toolCall.args, parsedResult ?? storedResult?.content ?? ""),
    });
  }
  return groups.map((group) => ({ role: "tool", content: "", toolGroup: group }));
}

function toolGroupMessageFromStoredToolResult(message, step) {
  const toolName = message.name || "tool";
  const parsedResult = parseStoredToolResult(message.content);
  return {
    role: "tool",
    content: "",
    toolGroup: {
      id: `stored-tool-${step}-${toolName}-0`,
      name: toolName,
      step,
      calls: [{
        id: message.toolCallId || `${toolName}-${step}-0`,
        name: toolName,
        step,
        status: storedToolStatus(toolName, parsedResult),
        args: {},
        result: summarizeToolResult(toolName, parsedResult ?? { ok: false, error: "Missing stored tool result" }),
        contextChars: toolCallContextChars(toolName, {}, parsedResult ?? message.content ?? ""),
      }],
      expanded: false,
    },
  };
}

function parseStoredToolResult(content) {
  if (typeof content !== "string" || !content.trim()) return null;
  try {
    return JSON.parse(content);
  } catch {
    return { ok: false, error: content };
  }
}

function storedToolStatus(toolName, parsedResult) {
  if (!parsedResult) return "failed";
  if (parsedResult.ok === false) {
    const error = String(parsedResult.error || "").toLowerCase();
    if (error.includes("permission denied")) return "denied";
    if (error.includes("unknown tool")) return "unknown_tool";
    return "failed";
  }
  const payload = resultPayload(parsedResult);
  const data = plainObject(payload);
  if ((toolName === "bash" || toolName === "powershell") && data.success === false) return "failed";
  if (data.success === false) return "failed";
  return "completed";
}

function createSessionId() {
  return `sess_${new Date().toISOString().replace(/[-:.TZ]/g, "").slice(0, 14)}_${Math.random().toString(36).slice(2, 8)}`;
}

function createRunId() {
  return `run_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
}

function isAbortError(error) {
  if (!error) return false;
  if (error.name === "AbortError") return true;
  const message = error instanceof Error ? error.message : String(error);
  return /\babort(?:ed)?\b|\bcancell?ed\b/i.test(message);
}

const sessionTitleMaxChars = 96;

function titleFromInput(input) {
  const cleaned = input.replace(/\s+/g, " ").trim();
  if (!cleaned) return "New chat";
  return cleaned.length > sessionTitleMaxChars ? `${cleaned.slice(0, sessionTitleMaxChars - 3)}...` : cleaned;
}
