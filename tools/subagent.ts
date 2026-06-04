import { defineTool, type AgentContext } from '@hyper-labs/hyper-router'

export type SubagentExecutionMode = 'linear' | 'parallel'

export interface SubagentTask {
  name?: string
  prompt: string
}

export interface SubagentToolArgs {
  executionMode?: SubagentExecutionMode
  stopOnError?: boolean
  tasks?: SubagentTask[]
}

export interface NormalizedSubagentToolArgs {
  executionMode: SubagentExecutionMode
  stopOnError: boolean
  tasks: SubagentTask[]
}

export type SubagentExecutor = (args: NormalizedSubagentToolArgs, context: AgentContext) => Promise<unknown>

const MAX_SUBAGENT_TASKS = 8
const MAX_SUBAGENT_PROMPT_CHARS = 24000
const MAX_SUBAGENT_NAME_CHARS = 120

let subagentExecutor: SubagentExecutor | undefined

export function setSubagentExecutor(executor: SubagentExecutor | undefined): void {
  subagentExecutor = executor
}

function normalizeExecutionMode(value: unknown): SubagentExecutionMode {
  if (value === undefined || value === null || value === '') return 'parallel'
  if (value === 'linear' || value === 'parallel') return value
  throw new Error('executionMode must be "linear" or "parallel"')
}

export function normalizeSubagentArgs(args: SubagentToolArgs): NormalizedSubagentToolArgs {
  const executionMode = normalizeExecutionMode(args.executionMode)
  if (!Array.isArray(args.tasks) || args.tasks.length === 0) {
    throw new Error('tasks must be a non-empty array')
  }
  if (args.tasks.length > MAX_SUBAGENT_TASKS) {
    throw new Error(`tasks is limited to ${MAX_SUBAGENT_TASKS} item(s)`) 
  }

  const tasks = args.tasks.map((task, index) => {
    const prompt = String(task?.prompt || '').trim()
    if (!prompt) throw new Error(`tasks[${index}].prompt is required`)
    if (prompt.length > MAX_SUBAGENT_PROMPT_CHARS) {
      throw new Error(`tasks[${index}].prompt is limited to ${MAX_SUBAGENT_PROMPT_CHARS} characters`)
    }
    const name = String(task?.name || '').trim()
    return {
      ...(name ? { name: name.slice(0, MAX_SUBAGENT_NAME_CHARS) } : {}),
      prompt,
    }
  })

  return {
    executionMode,
    stopOnError: args.stopOnError ?? executionMode === 'linear',
    tasks,
  }
}

export const subagentTool = defineTool({
  name: 'subagent',
  description: 'Delegate one or more tasks to hidden Qubit subagents. Use linear for edit/concurrency-sensitive work and parallel for independent investigation or planning.',
  inputSchema: {
    type: 'object',
    properties: {
      executionMode: { type: 'string', enum: ['linear', 'parallel'], description: 'How to execute tasks. Defaults to parallel.' },
      stopOnError: { type: 'boolean', description: 'For linear mode, stop after the first failed task. Defaults true for linear and false for parallel.' },
      tasks: {
        type: 'array',
        minItems: 1,
        maxItems: MAX_SUBAGENT_TASKS,
        items: {
          type: 'object',
          properties: {
            name: { type: 'string', description: 'Optional short task name.' },
            prompt: { type: 'string', description: 'Delegated task prompt.' },
          },
          required: ['prompt'],
          additionalProperties: false,
        },
      },
    },
    required: ['tasks'],
    additionalProperties: false,
  },
  permission: { mode: 'ask' },
  async execute(args: SubagentToolArgs, context: AgentContext) {
    try {
      const normalized = normalizeSubagentArgs(args || {})
      if (!subagentExecutor) {
        return { ok: false, error: 'subagent executor is not configured' }
      }
      return { ok: true, data: await subagentExecutor(normalized, context) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})
