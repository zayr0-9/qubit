import { spawn } from 'node:child_process'
import { defineTool } from '@hyper-labs/hyper-router'
import { isWindows, toWslPath } from '../utils/wslBridge.js'
import { resolveToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'

const DEFAULT_MAX_OUTPUT_CHARS = 40000
const MAX_RESULT_LINES = 5000
const MAX_LINE_LENGTH = 1000
const DEFAULT_TIMEOUT_MS = 30000

export interface RipgrepOptions {
  caseSensitive?: boolean
  lineNumbers?: boolean
  count?: boolean
  filesWithMatches?: boolean
  maxCount?: number
  glob?: string
  hidden?: boolean
  noIgnore?: boolean
  contextLines?: number
  maxOutputChars?: number
  timeoutMs?: number
  cwd?: string
}

export interface RipgrepMatch {
  file: string
  lineNumber?: number
  line?: string
  matchCount?: number
}

export interface RipgrepResult {
  success: boolean
  matches: RipgrepMatch[]
  error?: string
  command?: string
  searchPath?: string
}

let windowsNativeRgPathPromise: Promise<string | null> | null = null

async function detectWindowsNativeRgPath(): Promise<string | null> {
  if (!isWindows()) return null

  if (!windowsNativeRgPathPromise) {
    windowsNativeRgPathPromise = new Promise(resolve => {
      const child = spawn('where.exe', ['rg.exe'], { stdio: ['ignore', 'pipe', 'ignore'], windowsHide: true })
      let stdout = ''
      child.stdout.on('data', data => {
        stdout += data.toString('utf8')
      })
      child.on('error', () => resolve(null))
      child.on('close', code => {
        if (code !== 0) return resolve(null)
        const firstPath = stdout
          .split(/\r?\n/)
          .map(line => line.trim())
          .find(Boolean)
        resolve(firstPath || null)
      })
    })
  }

  return windowsNativeRgPathPromise
}

function clampPositiveInteger(value: number | undefined, fallback: number, max: number): number {
  if (value === undefined || value === null || Number.isNaN(value) || value <= 0) return fallback
  return Math.min(max, Math.floor(value))
}

function parseRipgrepOutput(output: string, flags: { count?: boolean; filesWithMatches?: boolean }): RipgrepMatch[] {
  const matches: RipgrepMatch[] = []
  if (!output.trim()) return matches

  if (flags.count) {
    for (const line of output.trim().split('\n')) {
      const match = line.trim().match(/^(.*?):(\d+)$/)
      if (!match) continue
      matches.push({ file: match[1], matchCount: Number.parseInt(match[2], 10) || 0 })
    }
    return matches
  }

  if (flags.filesWithMatches) {
    for (const line of output.trim().split('\n')) {
      const trimmed = line.trim()
      if (trimmed) matches.push({ file: trimmed })
    }
    return matches
  }

  for (const line of output.trim().split('\n')) {
    if (!line.trim()) continue
    try {
      const data = JSON.parse(line)
      if (data.type !== 'match' || !data.data) continue
      const filePath = data.data.path
      const matchLines = data.data.lines
      const file = typeof filePath === 'string' ? filePath : filePath?.text || ''
      if (matchLines?.text !== undefined) {
        matches.push({ file, lineNumber: data.data.line_number, line: matchLines.text })
      }
    } catch {
      const match = line.match(/^(.+?):(\d+):(.*)$/)
      if (match) matches.push({ file: match[1], lineNumber: Number.parseInt(match[2], 10), line: match[3] })
    }
  }

  return matches
}

async function buildRipgrepCommand(searchPath: string, args: string[]): Promise<{ cmd: string; args: string[]; displayCommand: string }> {
  const windowsRgPath = await detectWindowsNativeRgPath()
  const resolvedPath = await resolveToolPath(searchPath || '.', { mode: 'directory' })

  if (isWindows() && resolvedPath.comparisonKind === 'posix') {
    const wslSearchPath = toWslPath(resolvedPath.comparisonPath)
    return {
      cmd: 'wsl.exe',
      args: ['-e', 'rg', ...args, wslSearchPath],
      displayCommand: `wsl.exe -e rg ${args.join(' ')} ${wslSearchPath}`,
    }
  }

  const nativeSearchPath = resolvedPath.fsPath
  if (isWindows() && windowsRgPath) {
    return {
      cmd: windowsRgPath,
      args: [...args, nativeSearchPath],
      displayCommand: `${windowsRgPath} ${args.join(' ')} ${nativeSearchPath}`,
    }
  }

  return {
    cmd: 'rg',
    args: [...args, nativeSearchPath],
    displayCommand: `rg ${args.join(' ')} ${nativeSearchPath}`,
  }
}

export async function ripgrepSearch(pattern: string, searchPath = '.', options: RipgrepOptions = {}): Promise<RipgrepResult> {
  if (!pattern || pattern.trim() === '') {
    return { success: false, matches: [], error: 'Pattern cannot be empty' }
  }

  const maxOutputChars = clampPositiveInteger(options.maxOutputChars, DEFAULT_MAX_OUTPUT_CHARS, 500000)
  const timeoutMs = clampPositiveInteger(options.timeoutMs, DEFAULT_TIMEOUT_MS, 120000)
  const args: string[] = [pattern]

  args.push(options.caseSensitive ? '-s' : '-i')
  if (options.lineNumbers !== false && !options.count && !options.filesWithMatches) args.push('-n')
  if (options.count) args.push('-c')
  if (options.filesWithMatches) args.push('-l')
  if (options.maxCount !== undefined) args.push('-m', String(Math.max(1, Math.floor(options.maxCount))))
  if (options.glob) args.push('-g', options.glob)
  if (options.hidden) args.push('--hidden')
  if (options.noIgnore) args.push('--no-ignore')
  if (options.contextLines !== undefined) args.push('-C', String(Math.max(0, Math.floor(options.contextLines))))
  if (!options.count && !options.filesWithMatches) args.push('--json')

  const effectiveSearchPath = searchPath || cwdOrDefault(options.cwd)
  const command = await buildRipgrepCommand(effectiveSearchPath, args)

  return new Promise(resolve => {
    const child = spawn(command.cmd, command.args, { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true })
    let stdout = ''
    let stderr = ''
    let timedOut = false
    const timeout = setTimeout(() => {
      timedOut = true
      child.kill('SIGTERM')
      setTimeout(() => child.kill('SIGKILL'), 300)
    }, timeoutMs)

    child.stdout.on('data', data => {
      stdout += data.toString('utf8')
    })
    child.stderr.on('data', data => {
      stderr += data.toString('utf8')
    })
    child.on('error', error => {
      clearTimeout(timeout)
      resolve({
        success: false,
        matches: [],
        error: `Failed to execute ripgrep: ${error.message}. Make sure ripgrep (rg) is installed and in your PATH.`,
        command: command.displayCommand,
      })
    })
    child.on('close', code => {
      clearTimeout(timeout)
      if (timedOut) {
        resolve({ success: false, matches: [], error: `ripgrep timed out after ${timeoutMs}ms`, command: command.displayCommand })
        return
      }
      if (code === 2 && stderr) {
        resolve({ success: false, matches: [], error: `ripgrep error: ${stderr.trim()}`, command: command.displayCommand })
        return
      }

      try {
        const matches = parseRipgrepOutput(stdout, options)
        if (matches.length > MAX_RESULT_LINES) {
          resolve({
            success: false,
            matches: [],
            error: `Search returned too many matches (${matches.length} matches, limit is ${MAX_RESULT_LINES}). Please narrow your search.`,
            command: command.displayCommand,
          })
          return
        }

        let totalChars = 0
        for (const match of matches) {
          if (match.line) totalChars += match.line.length
        }
        if (totalChars > maxOutputChars) {
          resolve({
            success: false,
            matches: [],
            error: `Search output too large (${totalChars} characters, limit is ${maxOutputChars}). Please narrow your search.`,
            command: command.displayCommand,
          })
          return
        }

        for (const match of matches) {
          if (match.line && match.line.length > MAX_LINE_LENGTH) {
            match.line = `${match.line.slice(0, MAX_LINE_LENGTH)}... [truncated]`
          }
        }

        resolve({ success: true, matches, command: command.displayCommand, searchPath: effectiveSearchPath })
      } catch (error) {
        resolve({
          success: false,
          matches: [],
          error: `Failed to parse ripgrep output: ${error instanceof Error ? error.message : String(error)}`,
          command: command.displayCommand,
        })
      }
    })
  })
}

export const ripgrepTool = defineTool({
  name: 'ripgrep',
  description: 'Search files using ripgrep (rg), with Windows/WSL path support and output limits.',
  inputSchema: {
    type: 'object',
    properties: {
      pattern: { type: 'string', description: 'Search pattern (regex or literal string).' },
      searchPath: { type: 'string', description: 'Directory or file path to search.' },
      cwd: { type: 'string', description: 'Alias used as search path when searchPath is omitted.' },
      caseSensitive: { type: 'boolean' },
      lineNumbers: { type: 'boolean' },
      count: { type: 'boolean' },
      filesWithMatches: { type: 'boolean' },
      maxCount: { type: 'number' },
      glob: { type: 'string' },
      hidden: { type: 'boolean' },
      noIgnore: { type: 'boolean' },
      contextLines: { type: 'number' },
      maxOutputChars: { type: 'number' },
      timeoutMs: { type: 'number' },
    },
    required: ['pattern'],
    additionalProperties: false,
  },
  permission: { mode: 'always' },
  async execute(args: RipgrepOptions & { pattern?: string; searchPath?: string }) {
    if (!args.pattern) return { ok: false, error: 'pattern is required' }
    try {
      return { ok: true, data: await ripgrepSearch(args.pattern, args.searchPath || cwdOrDefault(args.cwd), args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default ripgrepSearch
