import { createHash, randomBytes } from "node:crypto";
import http, { type IncomingMessage, type ServerResponse } from "node:http";
import { CODEX_AUTH_ISSUER, CODEX_CLIENT_ID, CODEX_ORIGINATOR, CODEX_SCOPES, type CodexAuthJson, type CodexLoginCompleteResult, type CodexLoginStartResult, type CodexOAuthOptions } from "./types.js";
import { metadataFromAuth } from "./jwt.js";

const CALLBACK_PATH = "/auth/callback";
const PREFERRED_PORT = 1455;
// Keep in sync with the Codex CLI Hydra redirect URI allow-list. Arbitrary
// localhost ports cause `authorize_hydra_invalid_request` because their
// redirect_uri values are not registered for the Codex OAuth client.
const FALLBACK_PORT = 1457;

type PendingLogin = CodexLoginStartResult & { server: http.Server };
let pendingLogin: PendingLogin | null = null;

export async function startCodexLogin(options: CodexOAuthOptions): Promise<CodexLoginStartResult> {
  if (pendingLogin) {
    await pendingLogin.cancel();
    pendingLogin = null;
  }

  const issuer = (options.issuer || CODEX_AUTH_ISSUER).replace(/\/$/, "");
  const clientId = options.clientId || CODEX_CLIENT_ID;
  const originator = options.originator || CODEX_ORIGINATOR;
  const fetchImpl = options.fetch || fetch;
  const codeVerifier = base64Url(randomBytes(32));
  const codeChallenge = base64Url(createHash("sha256").update(codeVerifier).digest());
  // Match the working YggChat/Codex-style flow: 16 random bytes rendered as
  // 32 hex characters. Longer base64url state values are valid OAuth, but the
  // ChatGPT OAuth stub has proven sensitive to small shape differences.
  const state = randomHex(16);

  const serverState: { settled: boolean; resolve?: (value: CodexLoginCompleteResult) => void; reject?: (error: unknown) => void } = { settled: false };
  const completed = new Promise<CodexLoginCompleteResult>((resolve, reject) => {
    serverState.resolve = resolve;
    serverState.reject = reject;
  });

  let redirectUri = "";
  debugOAuth("prepared login parameters", {
    issuer,
    clientId,
    originator,
    scope: CODEX_SCOPES,
    stateLength: state.length,
    stateShape: /^[0-9a-f]+$/.test(state) ? "hex" : "other",
    codeVerifierLength: codeVerifier.length,
    codeChallengeLength: codeChallenge.length,
  });
  const server = http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url || "/", `http://localhost:${actualPort || PREFERRED_PORT}`);
      if (url.pathname === "/cancel") {
        respondHtml(res, 200, "Codex sign-in cancelled. You may close this tab.");
        settleReject(serverState, new Error("Codex login cancelled."));
        void closeServer(server);
        return;
      }
      if (url.pathname === "/success") {
        respondHtml(res, 200, "Codex sign-in completed. You may close this tab.");
        return;
      }
      if (url.pathname !== CALLBACK_PATH) {
        respondHtml(res, 404, "Not found.");
        return;
      }
      const returnedState = url.searchParams.get("state") || "";
      const code = url.searchParams.get("code") || "";
      const oauthError = url.searchParams.get("error") || "";
      debugOAuth("received callback", {
        path: url.pathname,
        hasCode: Boolean(code),
        codeLength: code.length,
        hasState: Boolean(returnedState),
        stateMatches: returnedState === state,
        hasError: Boolean(oauthError),
        redirectUri,
      });
      if (oauthError) throw new Error(`Codex OAuth failed: ${oauthError}`);
      if (!code) throw new Error("Codex OAuth callback did not include an authorization code.");
      if (returnedState !== state) throw new Error("Codex OAuth state mismatch.");
      const auth = await exchangeAuthorizationCode({ issuer, clientId, code, redirectUri, codeVerifier, fetchImpl });
      await options.tokenStore.save(auth);
      const metadata = metadataFromAuth(auth);
      respondHtml(res, 200, "Codex sign-in completed. Return to Qubit.");
      settleResolve(serverState, { ...metadata, status: "Signed in to ChatGPT Codex." });
      void closeServer(server);
    } catch (error) {
      debugOAuth("callback failed", { error: error instanceof Error ? error.message : String(error) });
      respondHtml(res, 400, "Codex sign-in failed. Return to Qubit for details.");
      settleReject(serverState, error);
      void closeServer(server);
    }
  });

  let actualPort = await listenWithFallback(server);
  redirectUri = `http://localhost:${actualPort}${CALLBACK_PATH}`;
  const authUrl = buildAuthorizeUrl({ issuer, clientId, redirectUri, codeChallenge, state, originator, allowedWorkspaceId: options.allowedWorkspaceId });
  debugOAuth("login server started", { localPort: actualPort, redirectUri, authUrl: sanitizeOAuthUrl(authUrl) });
  const cancel = async () => {
    settleReject(serverState, new Error("Codex login cancelled."));
    await closeServer(server);
    if (pendingLogin?.server === server) pendingLogin = null;
  };
  const result: PendingLogin = { authUrl, localPort: actualPort, cancel, completed, server };
  pendingLogin = result;
  completed.finally(() => {
    if (pendingLogin?.server === server) pendingLogin = null;
  }).catch(() => undefined);
  return result;
}

export async function cancelCodexLogin(): Promise<boolean> {
  if (!pendingLogin) return false;
  await pendingLogin.cancel();
  pendingLogin = null;
  return true;
}

export function buildAuthorizeUrl({ issuer, clientId, redirectUri, codeChallenge, state, originator, allowedWorkspaceId }: { issuer: string; clientId: string; redirectUri: string; codeChallenge: string; state: string; originator: string; allowedWorkspaceId?: string }): string {
  const url = new URL("/oauth/authorize", issuer);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("client_id", clientId);
  url.searchParams.set("redirect_uri", redirectUri);
  url.searchParams.set("scope", CODEX_SCOPES);
  url.searchParams.set("code_challenge", codeChallenge);
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("id_token_add_organizations", "true");
  url.searchParams.set("codex_cli_simplified_flow", "true");
  url.searchParams.set("state", state);
  url.searchParams.set("originator", originator);
  if (allowedWorkspaceId) url.searchParams.set("allowed_workspace_id", normalizeAllowedWorkspaceIds(allowedWorkspaceId));
  return url.toString();
}

function normalizeAllowedWorkspaceIds(value: string): string {
  return value.split(/[\s,]+/).map((part) => part.trim()).filter(Boolean).join(",");
}

async function exchangeAuthorizationCode({ issuer, clientId, code, redirectUri, codeVerifier, fetchImpl }: { issuer: string; clientId: string; code: string; redirectUri: string; codeVerifier: string; fetchImpl: typeof fetch }): Promise<CodexAuthJson> {
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    client_id: clientId,
    code,
    code_verifier: codeVerifier,
    redirect_uri: redirectUri,
  });
  debugOAuth("exchanging authorization code", {
    tokenEndpoint: `${issuer}/oauth/token`,
    redirectUri,
    clientId,
    bodyKeys: Array.from(body.keys()),
    codeLength: code.length,
    codeVerifierLength: codeVerifier.length,
  });
  const response = await fetchImpl(`${issuer}/oauth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  const text = await response.text();
  let json: any = {};
  try {
    json = text.trim() ? JSON.parse(text) : {};
  } catch {
    json = {};
  }
  debugOAuth("token endpoint responded", {
    status: response.status,
    ok: response.ok,
    contentType: response.headers.get("content-type") || "",
    bodyPreview: sanitizeTokenResponsePreview(text),
    hasAccessToken: Boolean(json.access_token),
    hasRefreshToken: Boolean(json.refresh_token),
    hasIdToken: Boolean(json.id_token),
    expiresIn: typeof json.expires_in === "number" ? json.expires_in : undefined,
  });
  if (!response.ok) throw new Error(`Codex token exchange failed: HTTP ${response.status}${json.error ? ` ${json.error}` : text ? ` ${sanitizeTokenResponsePreview(text)}` : ""}`);
  if (!json.access_token || !json.refresh_token) throw new Error(`Codex token exchange returned an invalid token response: ${sanitizeTokenResponsePreview(text)}`);
  return {
    auth_mode: "chatgpt",
    tokens: {
      id_token: json.id_token,
      access_token: json.access_token,
      refresh_token: json.refresh_token,
      account_id: json.account_id || extractAccountId(json.access_token) || extractAccountId(json.id_token),
    },
    last_refresh: new Date().toISOString(),
    agent_identity: null,
  };
}

async function listenWithFallback(server: http.Server): Promise<number> {
  const explicitPort = Number(process.env.QUBIT_CODEX_OAUTH_PORT || 0);
  if (explicitPort && explicitPort !== PREFERRED_PORT && explicitPort !== FALLBACK_PORT) {
    throw new Error(`QUBIT_CODEX_OAUTH_PORT=${explicitPort} is not registered for the Codex OAuth client. Use ${PREFERRED_PORT} or ${FALLBACK_PORT}; arbitrary ports produce authorize_hydra_invalid_request.`);
  }

  const ports = explicitPort ? [explicitPort] : [PREFERRED_PORT, FALLBACK_PORT];
  let lastError: unknown;
  for (const port of ports) {
    try {
      return await listen(server, port);
    } catch (error: any) {
      lastError = error;
      if (error?.code !== "EADDRINUSE") continue;
      await tryCancelExisting(port);
      try {
        return await listen(server, port);
      } catch (retryError) {
        lastError = retryError;
      }
    }
  }
  const message = `Codex OAuth callback ports ${ports.join(" and ")} are already in use. Close the other Codex/YggChat/Qubit login window or stop the process using the port, then run /codex-login again.`;
  throw lastError instanceof Error ? new Error(`${message} Last error: ${lastError.message}`) : new Error(message);
}

function listen(server: http.Server, port: number): Promise<number> {
  return new Promise((resolve, reject) => {
    const onError = (error: Error) => {
      server.off("listening", onListening);
      reject(error);
    };
    const onListening = () => {
      server.off("error", onError);
      resolve(port);
    };
    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(port, "127.0.0.1");
  });
}

async function tryCancelExisting(port: number): Promise<void> {
  await new Promise<void>((resolve) => {
    const req = http.get({ hostname: "127.0.0.1", port, path: "/cancel", timeout: 500 }, (res: IncomingMessage) => {
      res.resume();
      res.on("end", resolve);
    });
    req.on("error", () => resolve());
    req.on("timeout", () => {
      req.destroy();
      resolve();
    });
  });
}

function closeServer(server: http.Server): Promise<void> {
  return new Promise((resolve) => server.close(() => resolve()));
}

function respondHtml(res: ServerResponse, status: number, message: string): void {
  res.writeHead(status, { "content-type": "text/html; charset=utf-8" });
  res.end(`<!doctype html><title>Qubit Codex</title><p>${escapeHtml(message)}</p>`);
}

function settleResolve(state: { settled: boolean; resolve?: (value: CodexLoginCompleteResult) => void }, value: CodexLoginCompleteResult): void {
  if (state.settled) return;
  state.settled = true;
  state.resolve?.(value);
}

function settleReject(state: { settled: boolean; reject?: (error: unknown) => void }, error: unknown): void {
  if (state.settled) return;
  state.settled = true;
  state.reject?.(error);
}

function base64Url(input: Buffer): string {
  return input.toString("base64").replace(/=/g, "").replace(/\+/g, "-").replace(/\//g, "_");
}

function randomHex(byteLength: number): string {
  return randomBytes(byteLength).toString("hex");
}

function extractAccountId(token?: string): string | undefined {
  if (!token || typeof token !== "string") return undefined;
  const parts = token.split(".");
  if (parts.length !== 3) return undefined;
  try {
    const payload = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = payload.padEnd(payload.length + ((4 - (payload.length % 4)) % 4), "=");
    const parsed = JSON.parse(Buffer.from(padded, "base64").toString("utf8"));
    return parsed?.["https://api.openai.com/auth"]?.chatgpt_account_id;
  } catch {
    return undefined;
  }
}

function debugOAuth(message: string, fields: Record<string, unknown> = {}): void {
  if (process.env.QUBIT_CODEX_OAUTH_DEBUG !== "1") return;
  const safe = Object.fromEntries(Object.entries(fields).map(([key, value]) => [key, sanitizeDebugValue(key, value)]));
  console.error(`[codex-oauth] ${message} ${JSON.stringify(safe)}`);
}

function sanitizeDebugValue(key: string, value: unknown): unknown {
  if (typeof value !== "string") return value;
  if (/url/i.test(key)) return sanitizeOAuthUrl(value);
  if (/code$|codeVerifier|codeChallenge|state|token|authorization/i.test(key)) return `[redacted:${value.length}]`;
  return value;
}

function sanitizeOAuthUrl(value: string): string {
  try {
    const url = new URL(value);
    for (const key of ["code", "state", "code_challenge", "access_token", "refresh_token", "id_token"]) {
      if (url.searchParams.has(key)) url.searchParams.set(key, `[redacted:${url.searchParams.get(key)?.length || 0}]`);
    }
    return url.toString();
  } catch {
    return value.replace(/(code|state|code_challenge|access_token|refresh_token|id_token)=([^&\s]+)/gi, "$1=[redacted]");
  }
}

function sanitizeTokenResponsePreview(text: string): string {
  if (!text) return "";
  try {
    const parsed = JSON.parse(text);
    for (const key of ["access_token", "refresh_token", "id_token"]) {
      if (typeof parsed?.[key] === "string") parsed[key] = `[redacted:${parsed[key].length}]`;
    }
    return JSON.stringify(parsed).slice(0, 1200);
  } catch {
    return text.replace(/\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/g, "[redacted-jwt]").slice(0, 1200);
  }
}

function escapeHtml(text: string): string {
  return text.replace(/[&<>"]/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[char] || char));
}
