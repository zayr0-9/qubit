import { defineTool, type AgentContext, type AnyToolDefinition, type ToolPermissionDecision } from '@hyper-labs/hyper-router'

export interface MultiCallItem {
  tool: string
  args?: Record<string, unknown>
}

export interface MultiCallOptions {
  stopOnError?: boolean
}

export interface MultiCallItemResult {
  index: number
  tool: string
  ok: boolean
  permission?: 'allowed' | 'denied' | 'not_required'
  data?: unknown
  output?: unknown
  error?: string
}

export interface MultiCallResult {
  success: boolean
  message: string
  results: MultiCallItemResult[]
  completed: number
  failed: number
  stoppedEarly: boolean
}

export interface MultiCallPermissionRequest {
  id: string
  sessionId: string
  step: number
  toolCallId: string
  toolName: string
  args: unknown
  description?: string
  inputSchema?: unknown
  metadata?: Record<string, unknown>
}

export interface MultiCallLifecycleEventBase {
  sessionId: string
  step: number
  toolCallId: string
  toolName: string
  args: unknown
  metadata?: Record<string, unknown>
}

export interface MultiCallLifecycleStartEvent extends MultiCallLifecycleEventBase {
  type: 'start'
  status: 'running'
  startedAt: string
}

export interface MultiCallLifecycleFinishEvent extends MultiCallLifecycleEventBase {
  type: 'finish'
  status: 'completed' | 'failed' | 'denied' | 'unknown_tool'
  result: { ok: boolean; data?: unknown; output?: unknown; error?: string }
  startedAt?: string
  finishedAt: string
  durationMs?: number
}

export type MultiCallLifecycleEvent = MultiCallLifecycleStartEvent | MultiCallLifecycleFinishEvent
export type MultiCallPermissionRequester = (request: MultiCallPermissionRequest) => Promise<ToolPermissionDecision>
export type MultiCallLifecycleEmitter = (event: MultiCallLifecycleEvent) => Promise<void> | void

let permissionRequester: MultiCallPermissionRequester | undefined
let lifecycleEmitter: MultiCallLifecycleEmitter | undefined

export function setMultiCallPermissionRequester(requester: MultiCallPermissionRequester | undefined): void {
  permissionRequester = requester
}

export function setMultiCallLifecycleEmitter(emitter: MultiCallLifecycleEmitter | undefined): void {
  lifecycleEmitter = emitter
}

function defaultContext(): AgentContext {
  return { sessionId: 'multiCall', step: 1 }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function makePermissionId(context: AgentContext, index: number, toolName: string): string {
  const runPart = context.runId || context.sessionId || 'run'
  return `permission-multiCall-${runPart}-${context.step}-${index}-${toolName}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function makeNestedToolCallId(context: AgentContext, index: number, toolName: string): string {
  const runPart = context.runId || context.sessionId || 'run'
  return `multiCall-${runPart}-${context.step}-${index}-${toolName}`
}

function toolResultData(result: { data?: unknown; output?: unknown }): unknown {
  return result.data !== undefined ? result.data : result.output
}

function failureResult(index: number, tool: string, error: string, permission?: MultiCallItemResult['permission']): MultiCallItemResult {
  return { index, tool, ok: false, error, ...(permission ? { permission } : {}) }
}

function lifecycleStatusForResult(result: { ok: boolean; data?: unknown; output?: unknown; error?: string }): MultiCallLifecycleFinishEvent['status'] {
  if (!result.ok) {
    const error = String(result.error || '').toLowerCase()
    if (error.includes('permission denied')) return 'denied'
    if (error.includes('unknown tool')) return 'unknown_tool'
    return 'failed'
  }
  const payload = result.data !== undefined ? result.data : result.output
  if (payload && typeof payload === 'object' && !Array.isArray(payload) && (payload as Record<string, unknown>).success === false) {
    return 'failed'
  }
  return 'completed'
}

async function emitLifecycle(event: MultiCallLifecycleEvent): Promise<void> {
  await lifecycleEmitter?.(event)
}

async function ensureNestedToolPermission(
  tool: AnyToolDefinition,
  args: unknown,
  context: AgentContext,
  index: number
): Promise<{ allowed: true; permission: MultiCallItemResult['permission'] } | { allowed: false; error: string; permission: MultiCallItemResult['permission'] }> {
  const mode = tool.permission?.mode ?? 'ask'
  if (mode === 'always') return { allowed: true, permission: 'not_required' }
  if (mode === 'never') return { allowed: false, error: tool.permission?.reason || `Tool is disabled: ${tool.name}`, permission: 'denied' }

  if (!permissionRequester) {
    return { allowed: false, error: `Tool requires permission but no permission requester is configured: ${tool.name}`, permission: 'denied' }
  }

  const decision = await permissionRequester({
    id: makePermissionId(context, index, tool.name),
    sessionId: context.sessionId,
    step: context.step,
    toolCallId: makeNestedToolCallId(context, index, tool.name),
    toolName: tool.name,
    args,
    description: tool.description,
    inputSchema: tool.inputSchema,
    metadata: { ...(tool.permission?.metadata || {}), parentTool: 'multiCall', callIndex: index },
  })

  if (decision.type === 'allow') return { allowed: true, permission: 'allowed' }
  return { allowed: false, error: decision.reason || `Permission denied for tool: ${tool.name}`, permission: 'denied' }
}

export async function multiCall(
  calls: MultiCallItem[],
  options: MultiCallOptions = {},
  tools: AnyToolDefinition[],
  context: AgentContext = defaultContext()
): Promise<MultiCallResult> {
  if (!Array.isArray(calls) || calls.length === 0) {
    return { success: false, message: 'calls must be a non-empty array', results: [], completed: 0, failed: 0, stoppedEarly: false }
  }

  const toolMap = new Map(tools.map(tool => [tool.name, tool]))
  const stopOnError = options.stopOnError ?? true
  const results: MultiCallItemResult[] = []

  for (const [index, call] of calls.entries()) {
    context.signal?.throwIfAborted()

    const toolName = typeof call?.tool === 'string' ? call.tool : ''
    const nestedToolCallId = toolName ? makeNestedToolCallId(context, index, toolName) : makeNestedToolCallId(context, index, 'tool')
    const nestedStep = context.step + index + 1
    const nestedMetadata = { parentTool: 'multiCall', parentStep: context.step, callIndex: index }
    let startedAt = ''

    if (!toolName) {
      results.push(failureResult(index, '', 'tool is required for each multiCall item'))
    } else if (toolName === 'multiCall') {
      results.push(failureResult(index, toolName, 'Nested multiCall is not supported'))
    } else {
      const tool = toolMap.get(toolName)
      const args = call.args ?? {}
      if (!tool) {
        results.push(failureResult(index, toolName, `Unknown tool: ${toolName}`))
      } else if (!isPlainObject(args)) {
        results.push(failureResult(index, toolName, 'args must be an object when provided'))
      } else {
        const permission = await ensureNestedToolPermission(tool, args, context, index)
        if (!permission.allowed) {
          results.push(failureResult(index, toolName, permission.error, permission.permission))
        } else {
          try {
            startedAt = new Date().toISOString()
            await emitLifecycle({
              type: 'start',
              sessionId: context.sessionId,
              step: nestedStep,
              toolCallId: nestedToolCallId,
              toolName,
              status: 'running',
              args,
              startedAt,
              metadata: nestedMetadata,
            })

            const startMs = Date.now()
            const result = await tool.execute(args, { ...context, step: nestedStep })
            const resultAny = result as { ok: boolean; data?: unknown; output?: unknown; error?: string }
            context.signal?.throwIfAborted()
            const itemResult: MultiCallItemResult = {
              index,
              tool: toolName,
              ok: Boolean(resultAny.ok),
              permission: permission.permission,
              ...(resultAny.data !== undefined ? { data: resultAny.data } : {}),
              ...(resultAny.output !== undefined ? { output: resultAny.output } : {}),
              ...(resultAny.error ? { error: resultAny.error } : {}),
            }
            results.push(itemResult)
            await emitLifecycle({
              type: 'finish',
              sessionId: context.sessionId,
              step: nestedStep,
              toolCallId: nestedToolCallId,
              toolName,
              status: lifecycleStatusForResult(resultAny),
              args,
              result: resultAny,
              startedAt,
              finishedAt: new Date().toISOString(),
              durationMs: Math.max(0, Date.now() - startMs),
              metadata: nestedMetadata,
            })
          } catch (error) {
            const errorResult = { ok: false, error: error instanceof Error ? error.message : String(error) }
            results.push(failureResult(index, toolName, errorResult.error, permission.permission))
            if (startedAt) {
              await emitLifecycle({
                type: 'finish',
                sessionId: context.sessionId,
                step: nestedStep,
                toolCallId: nestedToolCallId,
                toolName,
                status: 'failed',
                args,
                result: errorResult,
                startedAt,
                finishedAt: new Date().toISOString(),
                metadata: nestedMetadata,
              })
            }
          }
        }
      }
    }

    const latest = results[results.length - 1]
    if (!latest.ok && stopOnError) {
      const completed = results.filter(entry => entry.ok).length
      const failed = results.length - completed
      return {
        success: false,
        message: `Multi-call stopped after failure at item ${index + 1}${latest.tool ? ` (${latest.tool})` : ''}: ${latest.error || 'unknown error'}`,
        results,
        completed,
        failed,
        stoppedEarly: index < calls.length - 1,
      }
    }
  }

  const completed = results.filter(entry => entry.ok).length
  const failed = results.length - completed
  return {
    success: failed === 0,
    message: failed === 0 ? `Successfully processed ${results.length} multiCall item(s).` : `Processed ${results.length} multiCall item(s) with ${failed} failure(s).`,
    results,
    completed,
    failed,
    stoppedEarly: false,
  }
}

export function createMultiCallTool(tools: AnyToolDefinition[]): AnyToolDefinition {
  return defineTool({
    name: 'multiCall',
    description: 'Execute multiple Qubit tool calls sequentially in one tool invocation. Each call is { tool, args }; non-read-only nested tools request permission individually.',
    inputSchema: {
      type: 'object',
      properties: {
        calls: {
          type: 'array',
          items: {
            type: 'object',
            properties: {
              tool: { type: 'string' },
              args: { type: 'object' },
            },
            required: ['tool'],
            additionalProperties: false,
          },
        },
        stopOnError: { type: 'boolean' },
      },
      required: ['calls'],
      additionalProperties: false,
    },
    permission: { mode: 'always' },
    async execute(args: MultiCallOptions & { calls?: MultiCallItem[] }, context: AgentContext) {
      if (!args.calls) return { ok: false, error: 'calls is required' }
      try {
        return { ok: true, data: await multiCall(args.calls, args, tools, context) }
      } catch (error) {
        return { ok: false, error: error instanceof Error ? error.message : String(error) }
      }
    },
  })
}

export default multiCall
