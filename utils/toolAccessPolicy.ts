import { cwdOrDefault } from './toolWorkspace.js'

export interface ToolAccessPolicyOptions {
  cwdBlockEnabled?: boolean
  restrictToCwd?: boolean
  workspaceCwd?: string
}

const cwdBlockByRunId = new Map<string, boolean>()

export function setRunCwdBlockEnabled(runId: string | undefined, enabled: boolean): void {
  if (!runId) return
  cwdBlockByRunId.set(runId, enabled)
}

export function clearRunCwdBlockEnabled(runId: string | undefined): void {
  if (!runId) return
  cwdBlockByRunId.delete(runId)
}

export function cwdBlockEnabledFromContext(context?: { runId?: string }): boolean {
  if (context?.runId && cwdBlockByRunId.has(context.runId)) return cwdBlockByRunId.get(context.runId) !== false
  return true
}

export function restrictToCwd(options?: ToolAccessPolicyOptions): boolean {
  if (options?.restrictToCwd !== undefined) return options.restrictToCwd
  return options?.cwdBlockEnabled !== false
}

export function defaultWorkspaceCwd(): string {
  return cwdOrDefault(undefined)
}
