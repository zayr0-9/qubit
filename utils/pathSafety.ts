import * as path from 'node:path'
import { cwdOrDefault } from './toolWorkspace.js'
import { detectPathType, isWindows, isWslUncPath, resolveToWindowsPath, toWslPath, type PathType } from './wslBridge.js'

export type ToolPathType = 'windows' | 'wsl' | 'posix' | 'relative'
export type ComparisonKind = 'win32' | 'posix'

export interface ResolvedToolPath {
  inputPath: string
  displayPath: string
  fsPath: string
  pathType: ToolPathType
  comparisonPath: string
  comparisonKind: ComparisonKind
}

export interface ResolveToolPathOptions {
  cwd?: string
  mode?: 'file' | 'directory'
}

function isWindowsPath(inputPath: string): boolean {
  return detectPathType(inputPath) === 'windows' && !isWslUncPath(inputPath)
}

function resolvePosixLike(inputPath: string, cwd?: string): string {
  const normalizedInput = toWslPath(inputPath)
  if (normalizedInput.startsWith('/')) return path.posix.normalize(normalizedInput)

  const normalizedBase = toWslPath(cwdOrDefault(cwd))
  const basePath = normalizedBase.startsWith('/') ? normalizedBase : path.posix.resolve('/', normalizedBase)
  return path.posix.resolve(basePath, normalizedInput)
}

function resolveWindowsLike(inputPath: string, cwd?: string): string {
  const basePath = cwdOrDefault(cwd)
  return path.win32.isAbsolute(inputPath) ? path.win32.normalize(inputPath) : path.win32.resolve(basePath, inputPath)
}

function pathKindFromInput(inputPath: string, cwd?: string): ToolPathType {
  const inputType = detectPathType(inputPath)
  if (inputType === 'linux' || isWslUncPath(inputPath)) return 'wsl'
  if (inputType === 'windows') return 'windows'

  if (cwd) {
    const cwdType = detectPathType(cwd)
    if (cwdType === 'linux' || isWslUncPath(cwd)) return 'wsl'
    if (cwdType === 'windows') return 'windows'
  }

  if (isWindows()) return 'windows'
  return 'posix'
}

export async function resolveToolPath(
  inputPath: string,
  options: ResolveToolPathOptions = {}
): Promise<ResolvedToolPath> {
  if (typeof inputPath !== 'string' || inputPath.trim() === '') {
    throw new Error('Path must be a non-empty string')
  }

  const inputType: PathType = detectPathType(inputPath)
  const pathType = inputType === 'relative' ? 'relative' : pathKindFromInput(inputPath, options.cwd)
  const effectiveKind = pathKindFromInput(inputPath, options.cwd)
  const comparisonKind: ComparisonKind = effectiveKind === 'windows' ? 'win32' : 'posix'

  const comparisonPath = comparisonKind === 'win32'
    ? resolveWindowsLike(inputPath, options.cwd)
    : resolvePosixLike(inputPath, options.cwd)

  let fsPath = comparisonPath
  if (isWindows() && comparisonKind === 'posix') {
    fsPath = await resolveToWindowsPath(comparisonPath)
  }

  return {
    inputPath,
    displayPath: comparisonPath,
    fsPath,
    pathType,
    comparisonPath,
    comparisonKind,
  }
}

export async function resolveWorkspaceRoot(cwd: string): Promise<ResolvedToolPath> {
  return resolveToolPath(cwd)
}

export function assertPathWithinWorkspace(target: ResolvedToolPath, workspace: ResolvedToolPath): void {
  if (target.comparisonKind !== workspace.comparisonKind) {
    throw new Error(
      `Access denied: Path '${target.inputPath}' is not in the same filesystem style as workspace '${workspace.inputPath}'. File operations are restricted to the workspace directory.`
    )
  }

  const pathImpl = target.comparisonKind === 'win32' ? path.win32 : path.posix
  const workspacePath = pathImpl.resolve(workspace.comparisonPath)
  const targetPath = pathImpl.resolve(target.comparisonPath)
  const rel = pathImpl.relative(workspacePath, targetPath)

  if (rel === '') return

  if (rel.startsWith('..') || pathImpl.isAbsolute(rel)) {
    throw new Error(
      `Access denied: Path '${target.inputPath}' resolves to '${target.displayPath}' which is outside the workspace '${workspace.displayPath}'. File operations are restricted to the workspace directory.`
    )
  }
}

export async function resolveRestrictedToolPath(
  inputPath: string,
  options: ResolveToolPathOptions = {}
): Promise<ResolvedToolPath> {
  const resolved = await resolveToolPath(inputPath, options)
  if (options.cwd) {
    const workspace = await resolveWorkspaceRoot(options.cwd)
    assertPathWithinWorkspace(resolved, workspace)
  }
  return resolved
}

export function relativeDisplayPath(from: ResolvedToolPath, to: ResolvedToolPath): string {
  if (from.comparisonKind !== to.comparisonKind) return to.displayPath
  const pathImpl = from.comparisonKind === 'win32' ? path.win32 : path.posix
  return pathImpl.relative(from.comparisonPath, to.comparisonPath).replace(/\\/g, '/')
}
