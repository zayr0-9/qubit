export interface RuntimeClientTarget {
  id?: string;
  write(line: string): void;
}

export type RuntimeMessageRedactor = (message: unknown) => unknown;

export interface RuntimeWriterOptions {
  redactor?: RuntimeMessageRedactor;
  stdout?: Pick<NodeJS.WriteStream, "write">;
  serverMode?: boolean;
  logger?: (message: string) => void;
}

export function serializeRuntimeMessage(message: unknown, redactor: RuntimeMessageRedactor = (value) => value): string {
  return `${JSON.stringify(redactor(message))}\n`;
}

export function writeTo(
  target: RuntimeClientTarget | null | undefined,
  message: unknown,
  options: RuntimeWriterOptions = {},
): boolean {
  const redactor = options.redactor ?? ((value: unknown) => value);
  const line = serializeRuntimeMessage(message, redactor);
  if (target) {
    target.write(line);
    return true;
  }
  if (!options.serverMode) {
    const stdout = options.stdout ?? process.stdout;
    stdout.write(line);
    return true;
  }
  const type = message && typeof message === "object" && "type" in message ? String((message as { type?: unknown }).type || "unknown") : "unknown";
  options.logger?.(`[runtime-server] dropped untargeted message type=${type}`);
  return false;
}

export function broadcastTo(
  clients: Iterable<RuntimeClientTarget>,
  message: unknown,
  options: RuntimeWriterOptions = {},
): number {
  const redactor = options.redactor ?? ((value: unknown) => value);
  const line = serializeRuntimeMessage(message, redactor);
  let count = 0;
  for (const client of clients) {
    client.write(line);
    count += 1;
  }
  if (count === 0 && !options.serverMode) {
    const stdout = options.stdout ?? process.stdout;
    stdout.write(line);
  }
  return count;
}
