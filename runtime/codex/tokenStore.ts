import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { CODEX_KEYCHAIN_ACCOUNT, type CodexAuthJson, type CodexAuthMetadata, type CodexAuthStatus, type CodexTokenStore, type CodexTokenStoreOptions, type KeytarLike } from "./types.js";
import { jwtExpiresAt, metadataFromAuth } from "./jwt.js";

const INDEX_FILE = "codex-auth-index.json";
const KEYCHAIN_CHUNK_SIZE = 1500;
const KEYCHAIN_CHUNK_PREFIX = ":chunk:";
const MAX_KEYCHAIN_CHUNKS_TO_CLEAN = 32;

interface KeychainChunkManifest {
  qubitCodexAuthChunks: true;
  version: 1;
  chunks: number;
}

export class QubitCodexTokenStore implements CodexTokenStore {
  private readonly dataDir: string;
  private readonly keychainService: string;
  private readonly keychainAccount: string;
  private readonly keytar: KeytarLike | null;

  constructor(options: CodexTokenStoreOptions) {
    this.dataDir = options.dataDir;
    this.keychainService = options.keychainService || "Qubit";
    this.keychainAccount = options.keychainAccount || CODEX_KEYCHAIN_ACCOUNT;
    this.keytar = options.keytar || null;
  }

  async load(): Promise<CodexAuthJson | null> {
    const filePath = this.fileAuthPath();
    if (this.useFileStorage() || filePath) {
      return await this.loadFromFile(filePath || this.defaultFileAuthPath());
    }
    this.ensureKeychain();
    const secret = await this.loadFromKeychain();
    if (!secret) return null;
    return this.parseAuth(secret);
  }

  async save(auth: CodexAuthJson, metadata: CodexAuthMetadata = {}): Promise<void> {
    const now = new Date().toISOString();
    const withRefresh = { ...auth, last_refresh: auth.last_refresh || now };
    const derived = { ...metadataFromAuth(withRefresh), ...metadata, updatedAt: metadata.updatedAt || now };
    const filePath = this.fileAuthPath();
    if (this.useFileStorage() || filePath) {
      await this.saveToFile(filePath || this.defaultFileAuthPath(), withRefresh);
      await this.writeIndex({ ...derived, source: "file" });
      return;
    }
    this.ensureKeychain();
    await this.saveToKeychain(JSON.stringify(withRefresh));
    await this.writeIndex({ ...derived, source: "keychain" });
  }

  async delete(): Promise<void> {
    const filePath = this.fileAuthPath();
    if (this.useFileStorage() || filePath) {
      await rm(filePath || this.defaultFileAuthPath(), { force: true });
      await this.writeIndex({ active: false, source: "file" } as CodexAuthMetadata & { active: boolean });
      return;
    }
    if (this.keytar) await this.deleteFromKeychain();
    await this.writeIndex({ active: false, source: "keychain" } as CodexAuthMetadata & { active: boolean });
  }

  async status(): Promise<CodexAuthStatus> {
    const auth = await this.load();
    if (!auth?.tokens?.access_token) {
      const index = await this.readIndex();
      return { active: false, storage: this.storageKind(), ...index };
    }
    const metadata = metadataFromAuth(auth);
    const expiresAt = jwtExpiresAt(auth.tokens.access_token);
    return {
      active: true,
      storage: this.storageKind(),
      ...metadata,
      hasAccessToken: Boolean(auth.tokens.access_token),
      hasRefreshToken: Boolean(auth.tokens.refresh_token),
      ...(expiresAt ? { expiresAt: expiresAt.toISOString() } : {}),
    };
  }

  private useFileStorage(): boolean {
    return process.env.QUBIT_CODEX_AUTH_STORAGE === "file";
  }

  private storageKind(): "keychain" | "file" {
    return this.useFileStorage() || Boolean(this.fileAuthPath()) ? "file" : "keychain";
  }

  private fileAuthPath(): string {
    return process.env.QUBIT_CODEX_AUTH_FILE || "";
  }

  private defaultFileAuthPath(): string {
    const codexHome = process.env.CODEX_HOME;
    return codexHome ? join(codexHome, "auth.json") : join(this.dataDir, "codex-auth.json");
  }

  private indexPath(): string {
    return join(this.dataDir, INDEX_FILE);
  }

  private async loadFromFile(filePath: string): Promise<CodexAuthJson | null> {
    try {
      return this.parseAuth(await readFile(filePath, "utf8"));
    } catch {
      return null;
    }
  }

  private async saveToFile(filePath: string, auth: CodexAuthJson): Promise<void> {
    await mkdir(dirname(filePath), { recursive: true });
    await writeFile(filePath, `${JSON.stringify(auth, null, 2)}\n`, { mode: 0o600 });
  }

  private parseAuth(secret: string): CodexAuthJson | null {
    try {
      const parsed = JSON.parse(secret);
      return parsed && typeof parsed === "object" ? parsed : null;
    } catch {
      return null;
    }
  }

  private async loadFromKeychain(): Promise<string | null> {
    const secret = await this.keytar!.getPassword(this.keychainService, this.keychainAccount);
    if (!secret) return null;
    const parsed = this.parseKeychainChunkManifest(secret);
    if (!parsed) return secret;

    const parts: string[] = [];
    for (let index = 0; index < parsed.chunks; index += 1) {
      const part = await this.keytar!.getPassword(this.keychainService, this.chunkAccount(index));
      if (typeof part !== "string") return null;
      parts.push(part);
    }
    return parts.join("");
  }

  private async saveToKeychain(secret: string): Promise<void> {
    await this.deleteFromKeychain();
    if (secret.length <= KEYCHAIN_CHUNK_SIZE) {
      await this.keytar!.setPassword(this.keychainService, this.keychainAccount, secret);
      return;
    }

    const chunks = Math.ceil(secret.length / KEYCHAIN_CHUNK_SIZE);
    for (let index = 0; index < chunks; index += 1) {
      await this.keytar!.setPassword(this.keychainService, this.chunkAccount(index), secret.slice(index * KEYCHAIN_CHUNK_SIZE, (index + 1) * KEYCHAIN_CHUNK_SIZE));
    }
    const manifest: KeychainChunkManifest = { qubitCodexAuthChunks: true, version: 1, chunks };
    await this.keytar!.setPassword(this.keychainService, this.keychainAccount, JSON.stringify(manifest));
  }

  private async deleteFromKeychain(): Promise<void> {
    await this.keytar!.deletePassword(this.keychainService, this.keychainAccount);
    for (let index = 0; index < MAX_KEYCHAIN_CHUNKS_TO_CLEAN; index += 1) {
      await this.keytar!.deletePassword(this.keychainService, this.chunkAccount(index));
    }
  }

  private parseKeychainChunkManifest(secret: string): KeychainChunkManifest | null {
    try {
      const parsed = JSON.parse(secret);
      if (parsed?.qubitCodexAuthChunks !== true || parsed.version !== 1) return null;
      if (!Number.isInteger(parsed.chunks) || parsed.chunks < 1 || parsed.chunks > MAX_KEYCHAIN_CHUNKS_TO_CLEAN) return null;
      return parsed;
    } catch {
      return null;
    }
  }

  private chunkAccount(index: number): string {
    return `${this.keychainAccount}${KEYCHAIN_CHUNK_PREFIX}${index}`;
  }

  private async writeIndex(metadata: CodexAuthMetadata & { active?: boolean }): Promise<void> {
    await mkdir(this.dataDir, { recursive: true });
    const safe = {
      version: 1,
      active: metadata.active ?? true,
      ...(metadata.accountEmail ? { accountEmail: metadata.accountEmail } : {}),
      ...(metadata.accountId ? { accountId: metadata.accountId } : {}),
      ...(metadata.planType ? { planType: metadata.planType } : {}),
      updatedAt: metadata.updatedAt || new Date().toISOString(),
      source: metadata.source || this.storageKind(),
    };
    await writeFile(this.indexPath(), `${JSON.stringify(safe, null, 2)}\n`, { mode: 0o600 });
  }

  private async readIndex(): Promise<Record<string, unknown>> {
    try {
      const parsed = JSON.parse(await readFile(this.indexPath(), "utf8"));
      if (!parsed || typeof parsed !== "object") return {};
      const { accountEmail, accountId, planType, updatedAt, source } = parsed;
      return { accountEmail, accountId, planType, updatedAt, source };
    } catch {
      return {};
    }
  }

  private ensureKeychain(): void {
    if (this.keytar) return;
    throw new Error("Codex OAuth token storage requires OS keychain support. Set QUBIT_CODEX_AUTH_STORAGE=file only if you explicitly accept plaintext auth-file storage.");
  }
}
