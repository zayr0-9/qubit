import { exec } from 'node:child_process'
import { promisify } from 'node:util'

const execAsync = promisify(exec)

let defaultDistro: string | null = null

export type PathType = 'linux' | 'windows' | 'relative'

export function isWindows(): boolean {
  return process.platform === 'win32'
}

export function detectPathType(filePath: string): PathType {
  const trimmed = filePath.trim()
  if (!trimmed) return 'relative'

  if (/^[a-zA-Z]:[\\/]/.test(trimmed)) return 'windows'
  if (/^[\\/]{2}/.test(trimmed)) return 'windows'
  if (trimmed.startsWith('/')) return 'linux'

  return 'relative'
}

export function isWslLikePath(filePath: string): boolean {
  return detectPathType(filePath) === 'linux' || isWslUncPath(filePath)
}

export function isWslUncPath(filePath: string): boolean {
  return filePath.replace(/\\/g, '/').toLowerCase().startsWith('//wsl$/')
}

export function toWslPath(rawPath: string): string {
  const trimmed = rawPath.trim()
  if (!trimmed) return trimmed

  let normalized = trimmed.replace(/\\/g, '/')
  let lowerNormalized = normalized.toLowerCase()
  const isUncWSLPath = lowerNormalized.startsWith('//wsl$/')

  if (!isUncWSLPath) {
    normalized = normalized.replace(/\/+/g, '/')
    lowerNormalized = normalized.toLowerCase()
  }

  if (isUncWSLPath) {
    const withoutLeadingSlashes = normalized.replace(/^\/+/g, '')
    const segments = withoutLeadingSlashes.split('/').filter(Boolean)
    if (segments.length >= 2 && segments[0].toLowerCase() === 'wsl$') {
      const remainder = segments.slice(2).join('/')
      return remainder ? `/${remainder}` : '/'
    }
  }

  if (normalized.startsWith('/')) return normalized

  const driveMatch = lowerNormalized.match(/^([a-zA-Z]):\/(.*)$/)
  if (driveMatch) {
    const drive = driveMatch[1].toLowerCase()
    const rest = driveMatch[2]
    return `/mnt/${drive}/${rest}`
  }

  return normalized
}

export async function getDefaultDistro(): Promise<string> {
  if (defaultDistro) return defaultDistro

  try {
    const { stdout: buffer } = await execAsync('wsl.exe --list --verbose', { encoding: 'buffer' })
    const stdout = buffer.toString('utf16le').replace(/^\uFEFF/, '')
    for (const line of stdout.split('\n')) {
      if (!line.trim().startsWith('*')) continue
      const parts = line.trim().split(/\s+/)
      if (parts[1]) {
        defaultDistro = parts[1]
        return defaultDistro
      }
    }

    const { stdout: simpleBuffer } = await execAsync('wsl.exe --list --quiet', { encoding: 'buffer' })
    const simpleOut = simpleBuffer.toString('utf16le').replace(/^\uFEFF/, '')
    const firstDistro = simpleOut.split(/\s+/).find(Boolean)
    if (firstDistro) {
      defaultDistro = firstDistro
      return defaultDistro
    }
  } catch {
    // Fall through to practical default used by most local WSL installs.
  }

  defaultDistro = 'Ubuntu'
  return defaultDistro
}

export async function resolveToWindowsPath(filePath: string): Promise<string> {
  if (!isWindows()) return filePath

  const trimmedPath = filePath.trim()
  if (!trimmedPath.startsWith('/') || /^[a-zA-Z]:/.test(trimmedPath)) return filePath

  const gitBashMatch = trimmedPath.match(/^\/([a-zA-Z])\/(.*)$/)
  if (gitBashMatch) {
    const drive = gitBashMatch[1]
    const rest = gitBashMatch[2]
    return `${drive}:${rest ? '\\' + rest.replace(/\//g, '\\') : '\\'}`
  }

  const distro = await getDefaultDistro()
  const cleanPath = trimmedPath.replace(/\//g, '\\')
  const finalPath = cleanPath.startsWith('\\') ? cleanPath.slice(1) : cleanPath
  return `\\\\wsl$\\${distro}\\${finalPath}`
}

export async function getWSLCommandArgs(
  command: string,
  args: string[] = [],
  cwd?: string
): Promise<[string, string[]]> {
  const distro = await getDefaultDistro()
  const finalArgs = ['-d', distro]

  if (cwd) finalArgs.push('--cd', cwd)
  finalArgs.push('-e', command, ...args)

  return ['wsl.exe', finalArgs]
}
