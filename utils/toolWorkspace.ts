let defaultToolCwd = process.env.QUBIT_WORKSPACE_CWD || process.cwd()

export function getDefaultToolCwd(): string {
  return defaultToolCwd
}

export function setDefaultToolCwd(cwd: string): void {
  const trimmed = String(cwd || '').trim()
  if (!trimmed) throw new Error('Default tool cwd must be a non-empty string')
  defaultToolCwd = trimmed
  process.env.QUBIT_WORKSPACE_CWD = trimmed
}

export function cwdOrDefault(cwd?: string): string {
  const trimmed = String(cwd || '').trim()
  return trimmed || getDefaultToolCwd()
}
