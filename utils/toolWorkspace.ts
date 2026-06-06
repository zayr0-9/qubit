import * as path from 'node:path'
import { detectPathType, isWslUncPath, isWindows, toWslPath } from './wslBridge.js'

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

function resolveRelativeCwd(cwd: string, baseCwd: string): string {
  const baseType = detectPathType(baseCwd)

  if (baseType === 'windows' && !isWslUncPath(baseCwd)) {
    return path.win32.resolve(baseCwd, cwd)
  }

  const normalizedBase = toWslPath(baseCwd)
  const normalizedCwd = toWslPath(cwd)
  if (normalizedBase.startsWith('/')) {
    return path.posix.resolve(normalizedBase, normalizedCwd)
  }

  if (isWindows()) return path.win32.resolve(baseCwd, cwd)
  return path.resolve(baseCwd, cwd)
}

export function cwdOrDefault(cwd?: string, baseCwd?: string): string {
  const trimmed = String(cwd || '').trim()
  if (!trimmed) return getDefaultToolCwd()
  if (detectPathType(trimmed) !== 'relative') return trimmed
  return resolveRelativeCwd(trimmed, baseCwd || getDefaultToolCwd())
}
