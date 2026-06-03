import type { AnyToolDefinition, Message, ModelResponse, ToolCall } from "@hyper-labs/hyper-router";

export const CODEX_AUTH_ISSUER = "https://auth.openai.com";
export const CODEX_CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann";
export const CODEX_BASE_URL = "https://chatgpt.com/backend-api/codex";
export const CODEX_ORIGINATOR = "codex_cli_rs";
export const CODEX_SCOPES = "openid profile email offline_access";
export const CODEX_KEYCHAIN_ACCOUNT = "codex:chatgpt";

export interface CodexTokens {
  id_token?: string;
  access_token?: string;
  refresh_token?: string;
  account_id?: string;
  [key: string]: unknown;
}

export interface CodexAuthJson {
  auth_mode?: string;
  OPENAI_API_KEY?: string;
  tokens?: CodexTokens;
  last_refresh?: string;
  agent_identity?: Record<string, unknown> | null;
  [key: string]: unknown;
}

export interface CodexAuthMetadata {
  accountEmail?: string;
  accountId?: string;
  planType?: string;
  updatedAt?: string;
  source?: "keychain" | "file" | "env";
}

export interface CodexAuthStatus extends CodexAuthMetadata {
  active: boolean;
  hasAccessToken?: boolean;
  hasRefreshToken?: boolean;
  expiresAt?: string;
  storage?: "keychain" | "file" | "env";
}

export interface CodexTokenStore {
  load(): Promise<CodexAuthJson | null>;
  save(auth: CodexAuthJson, metadata?: CodexAuthMetadata): Promise<void>;
  delete(): Promise<void>;
  status(): Promise<CodexAuthStatus>;
}

export interface CodexTokenStoreOptions {
  dataDir: string;
  legacyDataDir?: string;
  keychainService?: string;
  keychainAccount?: string;
  keytar?: KeytarLike | null;
}

export interface KeytarLike {
  getPassword(service: string, account: string): Promise<string | null>;
  setPassword(service: string, account: string, password: string): Promise<void>;
  deletePassword(service: string, account: string): Promise<boolean>;
}

export interface CodexOAuthOptions {
  issuer?: string;
  clientId?: string;
  originator?: string;
  allowedWorkspaceId?: string;
  tokenStore: CodexTokenStore;
  fetch?: typeof fetch;
}

export interface CodexLoginStartResult {
  authUrl: string;
  localPort: number;
  cancel(): Promise<void>;
  completed: Promise<CodexLoginCompleteResult>;
}

export interface CodexLoginCompleteResult extends CodexAuthMetadata {
  status: string;
}

export interface CodexAuthOptions {
  issuer?: string;
  clientId?: string;
  tokenStore: CodexTokenStore;
  fetch?: typeof fetch;
}

export interface CodexResponsesProviderOptions extends CodexAuthOptions {
  baseURL?: string;
  originator?: string;
  userAgent?: string;
  reasoningEffort?: "minimal" | "low" | "medium" | "high";
  reasoningSummary?: "auto" | "concise" | "detailed" | null;
  onReasoningDelta?: (event: { sessionId?: string; runId?: string; delta: string }) => void;
}

export interface CodexRequestParts {
  instructions?: string;
  input: unknown[];
  tools: unknown[];
}

export interface CodexSseParseResult {
  content: string;
  reasoningContent?: string;
  toolCalls: ToolCall[];
  providerStopReason?: string;
  generatedImages?: NonNullable<ModelResponse["generatedImages"]>;
}

export type CodexGenerateInput = {
  sessionId?: string;
  runId?: string;
  model: string;
  messages: Message[];
  tools: AnyToolDefinition[];
  signal?: AbortSignal;
};
