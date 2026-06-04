import { defineTool } from '@hyper-labs/hyper-router'
import { isWindows } from '../utils/wslBridge.js'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { cwdBlockEnabledFromContext, restrictToCwd, type ToolAccessPolicyOptions } from '../utils/toolAccessPolicy.js'
import { runSpawnedCommand, type ShellRunOptions, type ShellRunResult } from './shellShared.js'

export interface PowerShellOptions extends ShellRunOptions, ToolAccessPolicyOptions {
  description?: string
  cwd?: string
}

async function buildPowerShellInvocation(command: string, options: PowerShellOptions = {}): Promise<{ cmd: string; args: string[]; cwd: string; displayCwd: string; detached?: boolean }> {
  const workspaceCwd = cwdOrDefault(options.cwd)
  const resolvedCwd = await resolveRestrictedToolPath(workspaceCwd, {
    cwd: workspaceCwd,
    mode: 'directory',
    restrictToCwd: restrictToCwd(options),
    workspaceCwd: options.workspaceCwd,
  })
  return {
    cmd: isWindows() ? 'powershell.exe' : 'pwsh',
    args: ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-Command', command],
    cwd: resolvedCwd.fsPath,
    displayCwd: resolvedCwd.displayPath,
    detached: !isWindows(),
  }
}

export async function runPowerShellCommand(command: string, options: PowerShellOptions = {}): Promise<ShellRunResult> {
  if (!command || command.trim() === '') {
    return { success: false, cwd: cwdOrDefault(options.cwd), stdout: '', stderr: '', error: 'command is required' }
  }

  const invocation = await buildPowerShellInvocation(command, options)
  return runSpawnedCommand({ ...invocation, options, defaultSuccessCodes: [0] })
}

export const powershellTool = defineTool({
  name: 'powershell',
  description: 'Run a PowerShell command. On Windows, WSL/Linux cwd paths are converted to UNC for native PowerShell; on non-Windows this uses pwsh.',
  inputSchema: {
    type: 'object',
    properties: {
      command: { type: 'string', description: 'PowerShell command to execute.' },
      description: { type: 'string', description: 'Brief human-readable explanation of why this command is being run.' },
      cwd: { type: 'string', description: 'Working directory for the command.' },
      env: { type: 'object', additionalProperties: { type: 'string' } },
      input: { type: 'string' },
      timeoutMs: { type: 'number' },
      maxOutputChars: { type: 'number' },
      successCodes: { type: 'array', items: { type: 'number' } },
    },
    required: ['command'],
    additionalProperties: false,
  },
  permission: { mode: 'ask', reason: 'PowerShell commands can read, write, or execute arbitrary code.' },
  async execute(args: PowerShellOptions & { command?: string }, context) {
    if (!args.command) return { ok: false, error: 'command is required' }
    try {
      return { ok: true, data: await runPowerShellCommand(args.command, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) }) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default runPowerShellCommand
