export type RuntimeMessageLike = {
  role?: string;
  content?: unknown;
};

export function overlayActiveRunUserMessages<T extends RuntimeMessageLike>(
  persistedMessages: T[],
  activeInputs: string[],
): RuntimeMessageLike[] {
  const messages: RuntimeMessageLike[] = Array.isArray(persistedMessages) ? [...persistedMessages] : [];
  for (const input of activeInputs) {
    const content = typeof input === "string" ? input.trim() : "";
    if (!content) continue;
    if (lastPersistedUserContent(messages) === content) continue;
    messages.push({ role: "user", content });
  }
  return messages;
}

function lastPersistedUserContent(messages: RuntimeMessageLike[]): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (!message || message.role === "system") continue;
    if (message.role !== "user") return "";
    return textContentFromMessage(message).trim();
  }
  return "";
}

function textContentFromMessage(message: RuntimeMessageLike): string {
  const content = message?.content;
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .map((part) => {
        if (!part || typeof part !== "object") return "";
        const record = part as Record<string, unknown>;
        if (typeof record.text === "string") return record.text;
        if (typeof record.content === "string") return record.content;
        return "";
      })
      .filter(Boolean)
      .join("\n");
  }
  return String(content ?? "");
}
