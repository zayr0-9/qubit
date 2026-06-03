export function assistantReasoningContent(messages: any[]): string | undefined {
  const parts: string[] = [];
  for (const message of messages || []) {
    if (message?.role !== "assistant") continue;
    const reasoning = typeof message.reasoningContent === "string" ? message.reasoningContent : String(message.reasoningContent ?? "");
    if (!reasoning.trim()) continue;
    if (parts.some((part) => part.includes(reasoning))) continue;
    parts.push(reasoning);
  }
  return parts.join("\n\n") || undefined;
}
