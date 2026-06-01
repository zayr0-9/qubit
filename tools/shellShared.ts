import { spawn } from 'node:child_process'
import type { ChildProcessWithoutNullStreams } from 'node:child_process'
import { isWindows } from '../utils/wslBridge.js'

export const DEFAULT_SHELL_MAX_OUTPUT_CHARS = 20000

export interface ShellRunResult {
  success: boolean
  cwd: string
  stdout: string
  stderr: string
  error?: string
}

export interface ShellRunOptions {
  env?: NodeJS.ProcessEnv
  input?: string
  timeoutMs?: number
  maxOutputChars?: number
  successCodes?: number[]
}

export function clampMaxOutput(max?: number): number {
  if (max === undefined || max === null || Number.isNaN(max) || max <= 0) return DEFAULT_SHELL_MAX_OUTPUT_CHARS
  return Math.min(200000, Math.floor(max))
}

export function filterShellStderr(stderr: string): string {
  return stderr
    .split('\n')
    .filter(line => !line.includes('screen size is bogus'))
    .join('\n')
}

export function killProcessTree(child: ChildProcessWithoutNullStreams): void {
  if (!child.pid) return

  if (isWindows()) {
    const killer = spawn('taskkill.exe', ['/pid', String(child.pid), '/T', '/F'], {
      stdio: 'ignore',
      windowsHide: true,
    })
    killer.on('error', () => child.kill('SIGKILL'))
    return
  }

  try {
    process.kill(-child.pid, 'SIGTERM')
  } catch {
    child.kill('SIGTERM')
  }
}

export function runSpawnedCommand(params: {
  cmd: string
  args: string[]
  cwd?: string
  displayCwd: string
  options?: ShellRunOptions
  detached?: boolean
  windowsHide?: boolean
  defaultSuccessCodes?: number[]
}): Promise<ShellRunResult> {
  const maxOutputChars = clampMaxOutput(params.options?.maxOutputChars)
  let stdout = ''
  let stderr = ''
  let remaining = maxOutputChars
  let truncated = false
  let timedOut = false
  let timeoutHandle: NodeJS.Timeout | null = null

  const append = (target: 'stdout' | 'stderr', chunk: Buffer) => {
    if (remaining <= 0) {
      truncated = true
      return
    }

    const text = chunk.toString('utf8')
    const toTake = Math.min(remaining, text.length)
    if (toTake < text.length) truncated = true
    if (target === 'stdout') stdout += text.slice(0, toTake)
    else stderr += text.slice(0, toTake)
    remaining -= toTake
  }

  return new Promise(resolve => {
    const child = spawn(params.cmd, params.args, {
      cwd: params.cwd,
      env: {
        ...process.env,
        COLUMNS: '120',
        LINES: '24',
        ...(params.options?.env || {}),
      },
      stdio: ['pipe', 'pipe', 'pipe'],
      detached: params.detached,
      windowsHide: params.windowsHide ?? true,
    })

    if (params.options?.input) child.stdin.end(params.options.input)
    else child.stdin.end()

    if (params.options?.timeoutMs && params.options.timeoutMs > 0) {
      timeoutHandle = setTimeout(() => {
        timedOut = true
        killProcessTree(child)
        setTimeout(() => child.kill('SIGKILL'), 300)
      }, params.options.timeoutMs)
    }

    child.stdout.on('data', chunk => append('stdout', chunk))
    child.stderr.on('data', chunk => append('stderr', chunk))

    const finalize = (result: ShellRunResult) => {
      if (timeoutHandle) clearTimeout(timeoutHandle)
      resolve(result)
    }

    child.on('error', error => {
      const filteredStderr = filterShellStderr(stderr)
      finalize({
        success: false,
        cwd: params.displayCwd,
        stdout,
        stderr: truncated ? `${filteredStderr}\n[Output truncated at ${maxOutputChars} characters]` : filteredStderr,
        error: error instanceof Error ? error.message : String(error),
      })
    })

    child.on('close', code => {
      const successCodes = new Set(params.options?.successCodes ?? params.defaultSuccessCodes ?? [0])
      const filteredStderr = filterShellStderr(stderr)
      finalize({
        success: !timedOut && code !== null && successCodes.has(code),
        cwd: params.displayCwd,
        stdout,
        stderr: truncated ? `${filteredStderr}\n[Output truncated at ${maxOutputChars} characters]` : filteredStderr,
        error: timedOut ? `Command timed out after ${params.options?.timeoutMs}ms` : undefined,
      })
    })
  })
}
