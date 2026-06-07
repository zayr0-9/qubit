const SECRET_KEY_PATTERN = /api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|authorization|bearer|client[_-]?secret|password|secret|private[_-]?key/i

export function redactMcpSecrets(value: unknown): unknown {
  if (typeof value === 'string') return redactSecretText(value)
  if (Array.isArray(value)) return value.map(item => redactMcpSecrets(item))
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    out[key] = SECRET_KEY_PATTERN.test(key) ? '[redacted]' : redactMcpSecrets(item)
  }
  return out
}

export function redactSecretText(text: string): string {
  return String(text || '')
    .replace(/Bearer\s+[A-Za-z0-9._~+\-/]+=*/gi, 'Bearer [redacted]')
    .replace(/(access_token|refresh_token|id_token|api_key|apikey|token|password|secret)=([^\s&]+)/gi, '$1=[redacted]')
}

export function previewMcpValue(value: unknown, maxChars = 1200): string {
  const redacted = redactMcpSecrets(value)
  const text = typeof redacted === 'string' ? redacted : JSON.stringify(redacted, null, 2)
  return text.length > maxChars ? `${text.slice(0, maxChars)}…` : text
}

export function maskSecret(value: string): string {
  const text = String(value || '')
  if (text.length <= 8) return '••••'
  return `${text.slice(0, 4)}…${text.slice(-4)}`
}
