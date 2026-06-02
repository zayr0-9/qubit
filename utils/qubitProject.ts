import * as path from 'node:path'
import { resolveToolPath } from './pathSafety.js'
import { cwdOrDefault } from './toolWorkspace.js'

export const QUBIT_DIR_NAME = '.qubit'

function assertSafeInternalSegment(segment: string): string {
  const value = String(segment || '').trim()
  if (!value || value === '.' || value === '..' || value.includes('/') || value.includes('\\')) {
    throw new Error(`Invalid Qubit internal path segment: ${segment}`)
  }
  return value
}

export async function getProjectQubitDirectory(cwd?: string): Promise<string> {
  const workspaceCwd = cwdOrDefault(cwd)
  const resolved = await resolveToolPath(QUBIT_DIR_NAME, { cwd: workspaceCwd, mode: 'directory' })
  return resolved.fsPath
}

export async function resolveProjectQubitPath(segments: string[] = [], cwd?: string): Promise<string> {
  const safeSegments = segments.map(assertSafeInternalSegment)
  const workspaceCwd = cwdOrDefault(cwd)
  const resolved = await resolveToolPath(path.join(QUBIT_DIR_NAME, ...safeSegments), {
    cwd: workspaceCwd,
    mode: safeSegments.length > 0 ? 'file' : 'directory',
  })
  return resolved.fsPath
}

export async function resolveProjectQubitSubdirectory(name: string, cwd?: string): Promise<string> {
  return resolveProjectQubitPath([name], cwd)
}
