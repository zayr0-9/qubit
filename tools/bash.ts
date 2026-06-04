import { defineTool } from '@hyper-labs/hyper-router'
import { getWSLCommandArgs, isWindows } from '../utils/wslBridge.js'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { cwdBlockEnabledFromContext, restrictToCwd, type ToolAccessPolicyOptions } from '../utils/toolAccessPolicy.js'
import { runSpawnedCommand, type ShellRunOptions, type ShellRunResult } from './shellShared.js'

const EXIT_1_NO_MATCH_COMMANDS = ['grep', 'egrep', 'fgrep', 'diff', 'cmp', 'awk']

export interface BashOptions extends ShellRunOptions, ToolAccessPolicyOptions {
  description?: string
  cwd?: string
}

function getDefaultSuccessCodes(command: string): number[] {
  const trimmed = command.trim()
  const firstWord = trimmed.split(/\s+/)[0]
  const basename = firstWord.split('/').pop() || firstWord
  return EXIT_1_NO_MATCH_COMMANDS.includes(basename) ? [0, 1] : [0]
}

async function buildBashInvocation(command: string, options: BashOptions = {}): Promise<{ cmd: string; args: string[]; cwd?: string; displayCwd: string; detached?: boolean }> {
  const workspaceCwd = cwdOrDefault(options.cwd)
  const resolvedCwd = await resolveRestrictedToolPath(workspaceCwd, {
    cwd: workspaceCwd,
    mode: 'directory',
    restrictToCwd: restrictToCwd(options),
    workspaceCwd: options.workspaceCwd,
  })

  if (isWindows()) {
    if (resolvedCwd.comparisonKind !== 'posix') {
      throw new Error('bash only runs for WSL/Linux cwd paths on Windows. Use the powershell tool for native Windows cwd paths.')
    }
    const [cmd, args] = await getWSLCommandArgs('bash', ['-lc', command], resolvedCwd.comparisonPath)
    return { cmd, args, displayCwd: resolvedCwd.displayPath }
  }

  return { cmd: 'bash', args: ['-lc', command], cwd: resolvedCwd.fsPath, displayCwd: resolvedCwd.displayPath, detached: true }
}

export async function runBashCommand(command: string, options: BashOptions = {}): Promise<ShellRunResult> {
  if (!command || command.trim() === '') {
    return { success: false, cwd: cwdOrDefault(options.cwd), stdout: '', stderr: '', error: 'command is required' }
  }

  const invocation = await buildBashInvocation(command, options)
  return runSpawnedCommand({
    ...invocation,
    options,
    defaultSuccessCodes: getDefaultSuccessCodes(command),
  })
}

export const bashTool = defineTool({
  name: 'bash',
  description: 'Run a Bash command. On Windows, this requires a WSL/Linux cwd and runs through WSL; use powershell for native Windows paths.',
  inputSchema: {
    type: 'object',
    properties: {
      command: { type: 'string', description: 'Shell command to execute.' },
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
  permission: { mode: 'ask', reason: 'Shell commands can read, write, or execute arbitrary code.' },
  async execute(args: BashOptions & { command?: string }, context) {
    if (!args.command) return { ok: false, error: 'command is required' }
    try {
      return { ok: true, data: await runBashCommand(args.command, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) }) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default runBashCommand
