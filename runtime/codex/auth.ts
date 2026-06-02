import { CODEX_AUTH_ISSUER, CODEX_CLIENT_ID, type CodexAuthJson, type CodexAuthOptions } from "./types.js";
import { isJwtExpiringSoon, metadataFromAuth } from "./jwt.js";

const refreshLocks = new WeakMap<object, Promise<CodexAuthJson>>();

export interface CodexAuthContext {
  accessToken: string;
  accountId?: string;
}

export async function getCodexBearerToken(options: CodexAuthOptions): Promise<string> {
  return (await getCodexAuthContext(options)).accessToken;
}

export async function getCodexAuthContext(options: CodexAuthOptions): Promise<CodexAuthContext> {
  const direct = process.env.CODEX_ACCESS_TOKEN;
  if (direct?.trim()) {
    return {
      accessToken: direct.trim(),
      ...(process.env.CODEX_ACCOUNT_ID?.trim() ? { accountId: process.env.CODEX_ACCOUNT_ID.trim() } : {}),
    };
  }

  const auth = await options.tokenStore.load();
  if (!auth?.tokens?.access_token) {
    throw new Error("Codex is not signed in. Run /codex-login to sign in with ChatGPT Codex.");
  }
  if (!isJwtExpiringSoon(auth.tokens.access_token)) return authContextFromAuth(auth);
  const refreshed = await refreshCodexAuth(options, auth);
  if (!refreshed.tokens?.access_token) throw new Error("Codex token refresh did not return an access token. Run /codex-login again.");
  return authContextFromAuth(refreshed);
}

export async function refreshCodexAuth(options: CodexAuthOptions, currentAuth?: CodexAuthJson): Promise<CodexAuthJson> {
  const key = options.tokenStore as object;
  const existing = refreshLocks.get(key);
  if (existing) return existing;
  const promise = doRefreshCodexAuth(options, currentAuth).finally(() => refreshLocks.delete(key));
  refreshLocks.set(key, promise);
  return promise;
}

async function doRefreshCodexAuth(options: CodexAuthOptions, currentAuth?: CodexAuthJson): Promise<CodexAuthJson> {
  const auth = currentAuth || await options.tokenStore.load();
  const refreshToken = auth?.tokens?.refresh_token;
  if (!refreshToken) throw new Error("Codex refresh token is missing. Run /codex-login again.");
  const issuer = (options.issuer || CODEX_AUTH_ISSUER).replace(/\/$/, "");
  const clientId = options.clientId || CODEX_CLIENT_ID;
  const fetchImpl = options.fetch || fetch;
  const response = await fetchImpl(`${issuer}/oauth/token`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ client_id: clientId, grant_type: "refresh_token", refresh_token: refreshToken }),
  });
  const text = await response.text();
  const json = text.trim() ? JSON.parse(text) : {};
  if (!response.ok) throw new Error(`Codex token refresh failed: HTTP ${response.status}${json.error ? ` ${json.error}` : ""}`);
  const refreshed: CodexAuthJson = {
    ...auth,
    auth_mode: auth?.auth_mode || "chatgpt",
    tokens: {
      ...(auth?.tokens || {}),
      ...(json.id_token ? { id_token: json.id_token } : {}),
      ...(json.access_token ? { access_token: json.access_token } : {}),
      ...(json.refresh_token ? { refresh_token: json.refresh_token } : {}),
      ...(json.account_id ? { account_id: json.account_id } : {}),
    },
    last_refresh: new Date().toISOString(),
  };
  await options.tokenStore.save(refreshed, metadataFromAuth(refreshed));
  return refreshed;
}

function authContextFromAuth(auth: CodexAuthJson): CodexAuthContext {
  const accessToken = auth.tokens?.access_token;
  if (!accessToken) throw new Error("Codex auth is missing an access token. Run /codex-login again.");
  return {
    accessToken,
    ...(auth.tokens?.account_id ? { accountId: auth.tokens.account_id } : {}),
  };
}
