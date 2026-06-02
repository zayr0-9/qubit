import { createHash, randomUUID } from "node:crypto";
import { mkdirSync } from "node:fs";
import { dirname } from "node:path";

import Database from "better-sqlite3";
import type { Message } from "@hyper-labs/hyper-router";
import type {
  CommitRunRecord,
  CommitRunResult,
  RunRecord,
  SessionMetadata,
  SessionState,
  StorageAdapter,
} from "@hyper-labs/hyper-router/storage/types";
import type { ToolCall } from "@hyper-labs/hyper-router";

interface QubitSqliteStorageOptions {
  filePath: string;
}

interface MessageRow {
  id: string;
  session_id: string;
  ordinal: number;
  role: Message["role"];
  content: string;
  date: string;
  reasoning_content: string | null;
  name: string | null;
  tool_call_id: string | null;
  tool_calls_json: string | null;
  fingerprint: string;
}

interface LegacySessionRow {
  session_id: string;
  messages_json: string | null;
  run_status: string | null;
  metadata_json: string | null;
}

interface SessionRow {
  current_revision: number;
  metadata_json: string | null;
}

interface CountRow {
  count: number;
}

interface RevisionRow {
  current_revision: number;
}

type SqliteDatabase = Database.Database;
type SqliteTransaction<TArgs extends unknown[]> = ((...args: TArgs) => unknown) & {
  default: (...args: TArgs) => unknown;
};

export class QubitSqliteStorage implements StorageAdapter {
  private readonly db: SqliteDatabase;

  constructor(options: QubitSqliteStorageOptions) {
    mkdirSync(dirname(options.filePath), { recursive: true });
    this.db = new Database(options.filePath);
    this.configureDatabase();
    this.ensureSchema();
    this.migrateLegacySessions();
  }

  async loadMessages(sessionId: string): Promise<Message[]> {
    const rows = this.db
      .prepare("SELECT * FROM qubit_messages WHERE session_id = ? ORDER BY ordinal ASC")
      .all(sessionId) as MessageRow[];
    return rows.map((row) => this.deserializeMessage(row));
  }

  async saveMessages(sessionId: string, messages: Message[]): Promise<void> {
    const tx = this.db.transaction((incoming: Message[]) => {
      this.ensureSession(sessionId);
      const existing = this.loadMessageRows(sessionId);
      const transcript = incoming.filter((message) => message.role !== "system");

      if (this.messagesEqual(existing, transcript)) {
        this.updateSessionRevision(sessionId, existing.length);
        return;
      }

      if (!this.isPrefix(existing, transcript)) {
        throw new Error(`Refusing to overwrite divergent transcript for session ${sessionId}`);
      }

      this.insertMessages(sessionId, transcript.slice(existing.length), existing.length);
      this.updateSessionRevision(sessionId, transcript.length);
    }) as SqliteTransaction<[Message[]]>;

    tx.default(messages);
  }

  async saveRun(record: RunRecord): Promise<void> {
    const tx = this.db.transaction(() => {
      this.ensureSession(record.sessionId);
      this.db
        .prepare(
          `UPDATE qubit_sessions
           SET run_status = ?, updated_at = CURRENT_TIMESTAMP
           WHERE session_id = ?`,
        )
        .run(record.status, record.sessionId);
      this.db
        .prepare(
          `INSERT INTO qubit_runs (run_id, session_id, status, started_at, finished_at)
           VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
           ON CONFLICT(run_id) DO UPDATE SET
             status = excluded.status,
             finished_at = excluded.finished_at`,
        )
        .run(record.runId ?? `legacy-${record.sessionId}`, record.sessionId, record.status);
    }) as SqliteTransaction<[]>;

    tx.default();
  }

  async getSessionMetadata(sessionId: string): Promise<SessionMetadata | null> {
    const row = this.db
      .prepare("SELECT metadata_json FROM qubit_sessions WHERE session_id = ?")
      .get(sessionId) as Pick<SessionRow, "metadata_json"> | undefined;

    return row?.metadata_json ? (JSON.parse(row.metadata_json) as SessionMetadata) : null;
  }

  async setSessionMetadata(sessionId: string, metadata: SessionMetadata): Promise<void> {
    const tx = this.db.transaction(() => {
      this.ensureSession(sessionId);
      this.db
        .prepare(
          `UPDATE qubit_sessions
           SET metadata_json = ?, updated_at = CURRENT_TIMESTAMP
           WHERE session_id = ?`,
        )
        .run(JSON.stringify(metadata), sessionId);
    }) as SqliteTransaction<[]>;

    tx.default();
  }

  async getSessionState(sessionId: string): Promise<SessionState> {
    const row = this.db
      .prepare("SELECT current_revision, metadata_json FROM qubit_sessions WHERE session_id = ?")
      .get(sessionId) as SessionRow | undefined;
    const messageCount = this.getMessageCount(sessionId);

    return {
      revision: row?.current_revision ?? messageCount,
      messageCount,
      metadata: row?.metadata_json ? (JSON.parse(row.metadata_json) as SessionMetadata) : null,
    };
  }

  async commitRun(record: CommitRunRecord): Promise<CommitRunResult> {
    const tx = this.db.transaction((): CommitRunResult => {
      this.ensureSession(record.sessionId);
      const currentState = this.getSessionStateSync(record.sessionId);
      const expectedRevision = record.baseRevision ?? record.baseMessageCount;

      if (expectedRevision !== undefined && currentState.revision !== expectedRevision) {
        return {
          sessionId: record.sessionId,
          revision: currentState.revision,
          messageCount: currentState.messageCount,
          conflict: true,
          conflictReason: "revision_mismatch",
        };
      }

      const existing = this.loadMessageRows(record.sessionId);
      if (!this.isPrefix(existing, record.fullMessages)) {
        return {
          sessionId: record.sessionId,
          revision: currentState.revision,
          messageCount: currentState.messageCount,
          conflict: true,
          conflictReason: "transcript_diverged",
        };
      }

      const appendStart = existing.length;
      const appendMessages = record.fullMessages.slice(appendStart);
      this.insertMessages(record.sessionId, appendMessages, appendStart);
      const revision = record.fullMessages.length;
      this.updateSessionRevision(record.sessionId, revision);

      if (record.metadata) {
        this.db
          .prepare("UPDATE qubit_sessions SET metadata_json = ?, updated_at = CURRENT_TIMESTAMP WHERE session_id = ?")
          .run(JSON.stringify({ ...record.metadata, updatedAt: new Date().toISOString() }), record.sessionId);
      }

      this.db
        .prepare(
          `INSERT INTO qubit_runs (
             run_id, session_id, base_revision, base_message_count, status, started_at, finished_at
           ) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
           ON CONFLICT(run_id) DO UPDATE SET
             status = excluded.status,
             base_revision = excluded.base_revision,
             base_message_count = excluded.base_message_count,
             finished_at = excluded.finished_at`,
        )
        .run(
          record.runId,
          record.sessionId,
          record.baseRevision ?? null,
          record.baseMessageCount ?? null,
          record.status,
        );

      return {
        sessionId: record.sessionId,
        revision,
        messageCount: revision,
      };
    }) as SqliteTransaction<[]>;

    return tx.default() as CommitRunResult;
  }

  close(): void {
    this.db.close();
  }

  private configureDatabase(): void {
    this.db.pragma("journal_mode = WAL");
    this.db.pragma("synchronous = NORMAL");
    this.db.pragma("foreign_keys = ON");
    this.db.pragma("busy_timeout = 5000");
  }

  private ensureSchema(): void {
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS qubit_sessions (
        session_id TEXT PRIMARY KEY,
        run_status TEXT,
        metadata_json TEXT,
        current_revision INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      );

      CREATE TABLE IF NOT EXISTS qubit_messages (
        id TEXT PRIMARY KEY,
        session_id TEXT NOT NULL,
        ordinal INTEGER NOT NULL,
        role TEXT NOT NULL,
        content TEXT NOT NULL,
        date TEXT NOT NULL,
        reasoning_content TEXT,
        name TEXT,
        tool_call_id TEXT,
        tool_calls_json TEXT,
        fingerprint TEXT NOT NULL,
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY(session_id) REFERENCES qubit_sessions(session_id) ON DELETE CASCADE,
        UNIQUE(session_id, ordinal),
        UNIQUE(session_id, fingerprint)
      );

      CREATE INDEX IF NOT EXISTS idx_qubit_messages_session_ordinal
        ON qubit_messages(session_id, ordinal);

      CREATE TABLE IF NOT EXISTS qubit_session_forks (
        session_id TEXT PRIMARY KEY,
        parent_session_id TEXT,
        forked_from_message_ordinal INTEGER,
        forked_from_message_id TEXT,
        forked_at TEXT NOT NULL,
        FOREIGN KEY(session_id) REFERENCES qubit_sessions(session_id) ON DELETE CASCADE
      );

      CREATE TABLE IF NOT EXISTS qubit_runs (
        run_id TEXT PRIMARY KEY,
        session_id TEXT NOT NULL,
        base_revision INTEGER,
        base_message_count INTEGER,
        status TEXT NOT NULL,
        prompt_mode TEXT,
        provider TEXT,
        model TEXT,
        started_at TEXT NOT NULL,
        finished_at TEXT,
        FOREIGN KEY(session_id) REFERENCES qubit_sessions(session_id) ON DELETE CASCADE
      );

      CREATE TABLE IF NOT EXISTS qubit_migrations (
        id TEXT PRIMARY KEY,
        applied_at TEXT NOT NULL
      );
    `);
  }

  private migrateLegacySessions(): void {
    const hasLegacySessions = this.db
      .prepare("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'sessions'")
      .get();
    if (!hasLegacySessions) return;

    const migrationId = "legacy-sessions-v1";
    const migrated = this.db
      .prepare("SELECT id FROM qubit_migrations WHERE id = ?")
      .get(migrationId);
    if (migrated) return;

    const rows = this.db
      .prepare("SELECT session_id, messages_json, run_status, metadata_json FROM sessions WHERE messages_json IS NOT NULL")
      .all() as LegacySessionRow[];

    const tx = this.db.transaction(() => {
      for (const row of rows) {
        const existing = this.db
          .prepare("SELECT session_id FROM qubit_sessions WHERE session_id = ?")
          .get(row.session_id);
        if (existing) continue;

        this.ensureSession(row.session_id, row.run_status, row.metadata_json);
        const messages = this.parseLegacyMessages(row.messages_json);
        this.insertMessages(row.session_id, messages, 0);
        this.updateSessionRevision(row.session_id, messages.length);
      }
      this.db
        .prepare("INSERT INTO qubit_migrations (id, applied_at) VALUES (?, CURRENT_TIMESTAMP)")
        .run(migrationId);
    }) as SqliteTransaction<[]>;

    tx.default();
  }

  private ensureSession(sessionId: string, runStatus: string | null = null, metadataJson: string | null = null): void {
    this.db
      .prepare(
        `INSERT INTO qubit_sessions (session_id, run_status, metadata_json)
         VALUES (?, ?, ?)
         ON CONFLICT(session_id) DO NOTHING`,
      )
      .run(sessionId, runStatus, metadataJson);
  }

  private updateSessionRevision(sessionId: string, revision: number): void {
    this.db
      .prepare(
        `UPDATE qubit_sessions
         SET current_revision = ?, updated_at = CURRENT_TIMESTAMP
         WHERE session_id = ?`,
      )
      .run(revision, sessionId);
  }

  private insertMessages(sessionId: string, messages: Message[], startOrdinal: number): void {
    const statement = this.db.prepare(
      `INSERT INTO qubit_messages (
         id, session_id, ordinal, role, content, date, reasoning_content, name,
         tool_call_id, tool_calls_json, fingerprint
       ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    );

    messages
      .filter((message) => message.role !== "system")
      .forEach((message, index) => {
        const ordinal = startOrdinal + index;
        statement.run(
          randomUUID(),
          sessionId,
          ordinal,
          message.role,
          message.content,
          this.serializeDate(message.date),
          message.reasoningContent ?? null,
          message.name ?? null,
          message.toolCallId ?? null,
          message.toolCalls ? JSON.stringify(message.toolCalls) : null,
          this.fingerprintMessage(message, ordinal),
        );
      });
  }

  private loadMessageRows(sessionId: string): Message[] {
    return (
      this.db.prepare("SELECT * FROM qubit_messages WHERE session_id = ? ORDER BY ordinal ASC").all(sessionId) as MessageRow[]
    ).map((row) => this.deserializeMessage(row));
  }

  private getSessionStateSync(sessionId: string): { revision: number; messageCount: number } {
    const revisionRow = this.db
      .prepare("SELECT current_revision FROM qubit_sessions WHERE session_id = ?")
      .get(sessionId) as RevisionRow | undefined;
    const messageCount = this.getMessageCount(sessionId);
    return {
      revision: revisionRow?.current_revision ?? messageCount,
      messageCount,
    };
  }

  private getMessageCount(sessionId: string): number {
    const row = this.db
      .prepare("SELECT COUNT(*) AS count FROM qubit_messages WHERE session_id = ?")
      .get(sessionId) as CountRow | undefined;
    return row?.count ?? 0;
  }

  private parseLegacyMessages(messagesJson: string | null): Message[] {
    if (!messagesJson) return [];
    const parsed = JSON.parse(messagesJson) as Array<Omit<Message, "date"> & { date: string }>;
    return parsed.map((message) => ({
      ...message,
      date: new Date(message.date),
    }));
  }

  private deserializeMessage(row: MessageRow): Message {
    return {
      role: row.role,
      content: row.content,
      date: new Date(row.date),
      ...(row.reasoning_content ? { reasoningContent: row.reasoning_content } : {}),
      ...(row.name ? { name: row.name } : {}),
      ...(row.tool_call_id ? { toolCallId: row.tool_call_id } : {}),
      ...(row.tool_calls_json ? { toolCalls: JSON.parse(row.tool_calls_json) as ToolCall[] } : {}),
    };
  }

  private messagesEqual(existing: Message[], incoming: Message[]): boolean {
    return existing.length === incoming.length && this.isPrefix(existing, incoming);
  }

  private isPrefix(existing: Message[], incoming: Message[]): boolean {
    if (existing.length > incoming.length) return false;
    return existing.every((message, index) => this.messageIdentity(message) === this.messageIdentity(incoming[index]!));
  }

  private messageIdentity(message: Message): string {
    return JSON.stringify({
      role: message.role,
      content: message.content,
      reasoningContent: message.reasoningContent ?? null,
      name: message.name ?? null,
      date: this.serializeDate(message.date),
      toolCallId: message.toolCallId ?? null,
      toolCalls: message.toolCalls ?? null,
    });
  }

  private fingerprintMessage(message: Message, ordinal: number): string {
    return createHash("sha256")
      .update(`${ordinal}:${this.messageIdentity(message)}`)
      .digest("hex");
  }

  private serializeDate(date: Date | string): string {
    return date instanceof Date ? date.toISOString() : new Date(date).toISOString();
  }
}
