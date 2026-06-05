import * as assert from "node:assert/strict";
import http from "node:http";
import { describe, it } from "node:test";
import { buildAuthorizeUrl, cancelCodexLogin, startCodexLogin } from "./oauth.js";
import { CODEX_SCOPES, type CodexAuthJson, type CodexTokenStore } from "./types.js";

class MemoryTokenStore implements CodexTokenStore {
  saved: CodexAuthJson | null = null;

  async load() {
    return this.saved;
  }

  async save(auth: CodexAuthJson) {
    this.saved = auth;
  }

  async delete() {
    this.saved = null;
  }

  async status() {
    return { active: Boolean(this.saved) };
  }
}

function httpGet(url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      res.resume();
      res.on("end", resolve);
    });
    req.on("error", reject);
  });
}

describe("Codex OAuth", () => {
  it("builds the YggChat-compatible Codex authorize URL shape", () => {
    const authUrl = buildAuthorizeUrl({
      issuer: "https://auth.openai.com",
      clientId: "app_EMoamEEZ73f0CkXaXp7hrann",
      redirectUri: "http://localhost:1455/auth/callback",
      codeChallenge: "challenge",
      state: "state",
      originator: "codex_cli_rs",
    });

    const parsed = new URL(authUrl);
    assert.equal(parsed.origin, "https://auth.openai.com");
    assert.equal(parsed.pathname, "/oauth/authorize");
    assert.deepEqual(Array.from(parsed.searchParams.keys()), [
      "response_type",
      "client_id",
      "redirect_uri",
      "scope",
      "code_challenge",
      "code_challenge_method",
      "state",
      "id_token_add_organizations",
      "codex_cli_simplified_flow",
      "originator",
    ]);
    assert.equal(parsed.searchParams.get("scope"), CODEX_SCOPES);
    assert.equal(parsed.searchParams.get("id_token_add_organizations"), "true");
    assert.equal(parsed.searchParams.get("codex_cli_simplified_flow"), "true");
    assert.equal(parsed.searchParams.get("originator"), "codex_cli_rs");
    assert.equal(parsed.searchParams.has("allowed_workspace_id"), false);
  });

  it("exchanges callback codes with Codex-compatible form encoding", async () => {
    const previousPort = process.env.QUBIT_CODEX_OAUTH_PORT;
    process.env.QUBIT_CODEX_OAUTH_PORT = "1457";
    let capturedBody = "";
    const tokenStore = new MemoryTokenStore();

    try {
      const login = await startCodexLogin({
        tokenStore,
        fetch: (async (_url, init) => {
          capturedBody = String(init?.body || "");
          return new Response(JSON.stringify({
            access_token: "access-token",
            refresh_token: "refresh-token",
            id_token: "id-token",
          }), { status: 200 });
        }) as typeof fetch,
      });

      const authUrl = new URL(login.authUrl);
      assert.equal(authUrl.searchParams.get("scope"), CODEX_SCOPES);
      assert.equal(authUrl.searchParams.get("code_challenge")?.length, 43);
      assert.equal(authUrl.searchParams.get("state")?.length, 32);
      assert.match(authUrl.searchParams.get("state") || "", /^[0-9a-f]+$/);

      const callbackUrl = `http://127.0.0.1:${login.localPort}/auth/callback?code=test-code&state=${authUrl.searchParams.get("state")}`;
      await httpGet(callbackUrl);
      await login.completed;

      const body = new URLSearchParams(capturedBody);
      assert.deepEqual(Array.from(body.keys()), ["grant_type", "client_id", "code", "code_verifier", "redirect_uri"]);
      assert.equal(body.get("grant_type"), "authorization_code");
      assert.equal(body.get("code"), "test-code");
      assert.equal(body.get("redirect_uri"), "http://localhost:1457/auth/callback");
      assert.equal(body.get("client_id"), "app_EMoamEEZ73f0CkXaXp7hrann");
      assert.equal(body.get("code_verifier")?.length, 43);
      assert.equal(tokenStore.saved?.tokens?.refresh_token, "refresh-token");
    } finally {
      await cancelCodexLogin();
      if (previousPort === undefined) {
        delete process.env.QUBIT_CODEX_OAUTH_PORT;
      } else {
        process.env.QUBIT_CODEX_OAUTH_PORT = previousPort;
      }
    }
  });
});
