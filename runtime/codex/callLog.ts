import { appendFile, mkdir } from "node:fs/promises";
import { dirname } from "node:path";
import type { CodexProviderCallLogEvent } from "./types.js";

export class CodexCallLogWriter {
  private readonly filePath: string;

  constructor(filePath: string) {
    this.filePath = filePath;
  }

  async append(event: CodexProviderCallLogEvent): Promise<void> {
    await mkdir(dirname(this.filePath), { recursive: true });
    await appendFile(this.filePath, `${JSON.stringify(sanitizeLogEvent(event))}\n`, "utf8");
  }
}

function sanitizeLogEvent(event: CodexProviderCallLogEvent): CodexProviderCallLogEvent {
  return sanitizeUnknown(event) as CodexProviderCallLogEvent;
}

function sanitizeUnknown(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sanitizeUnknown);
  if (!value || typeof value !== "object") return value;

  const result: Record<string, unknown> = {};
  for (const [key, entry] of Object.entries(value)) {
    result[key] = isSecretKey(key) ? "[redacted]" : sanitizeUnknown(entry);
  }
  return result;
}

function isSecretKey(key: string): boolean {
  const normalized = key.toLowerCase();
  return normalized === "token"
    || normalized.endsWith("_token")
    || normalized.endsWith("-token")
    || normalized.includes("authorization")
    || normalized.includes("api_key")
    || normalized.includes("apikey")
    || normalized.includes("secret")
    || normalized.includes("password")
    || normalized.includes("bearer");
}
