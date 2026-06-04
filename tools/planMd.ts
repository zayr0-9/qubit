import * as fs from 'node:fs'
import * as path from 'node:path'
import { defineTool, type AgentContext } from '@hyper-labs/hyper-router'
import { resolveProjectQubitSubdirectory } from '../utils/qubitProject.js'

const PLAN_DIR_NAME = 'plans'
const PLAN_FILE_EXTENSION = '.md'
const MAX_LIST_RESULTS = 50
const MAX_ID_ATTEMPTS = 12

const ID_DICTIONARY = [
  'amber', 'atlas', 'bridge', 'cedar', 'delta', 'ember', 'forge', 'garden',
  'harbor', 'island', 'juniper', 'keystone', 'lantern', 'meadow', 'north',
  'orbit', 'prairie', 'quartz', 'river', 'summit', 'trail', 'violet',
]

export type PlanAction = 'create' | 'list' | 'read' | 'edit' | 'display' | 'clarify'

const DEFAULT_MANUAL_OPTION_LABEL = 'None of the above — tell Qubit what to do instead'

export interface PlanClarificationOption {
  id?: string
  label: string
  description?: string
  manual?: boolean
}

export interface PlanClarificationQuestion {
  id?: string
  question: string
  description?: string
  options?: PlanClarificationOption[]
  manualLabel?: string
}

export interface PlanClarificationAnswer {
  questionId: string
  question: string
  selectedOptionId?: string
  selectedOptionLabel?: string
  manual?: boolean
  answer: string
}

export interface PlanClarificationResult {
  clarified: boolean
  cancelled?: boolean
  questions: number
  answers: PlanClarificationAnswer[]
}

export interface PlanClarificationRequest {
  id: string
  sessionId?: string
  runId?: string
  step?: number
  toolCallId?: string
  questions: PlanClarificationQuestion[]
}

export type PlanClarificationRequester = (request: PlanClarificationRequest) => Promise<{ cancelled?: boolean; answers?: PlanClarificationAnswer[] }>

export interface PlanToolArgs {
  action: PlanAction
  name?: string
  content?: string
  search?: string
  replacement?: string
  cwd?: string
  questions?: PlanClarificationQuestion[]
}

export interface PlanInfo {
  name: string
  modifiedAt: string
  title?: string
}

export interface ReadPlanResult {
  exists: boolean
  name: string
  content: string | null
}

export interface CreatePlanResult {
  name: string
  created: boolean
  content: string
}

export interface EditPlanResult {
  success: boolean
  message: string
  name: string
  content: string | null
}

export interface DisplayPlanResult {
  displayed: boolean
  exists: boolean
  name: string
  path?: string
  message: string
}

export type PlanViewEvent = {
  name: string
  path: string
  cwd?: string
  content: string
  sessionId?: string
  runId?: string
  step?: number
}

let planViewEmitter: ((event: PlanViewEvent) => void | Promise<void>) | null = null
let planClarificationRequester: PlanClarificationRequester | null = null

export function setPlanViewEmitter(emitter: ((event: PlanViewEvent) => void | Promise<void>) | null): void {
  planViewEmitter = emitter
}

export function setPlanClarificationRequester(requester: PlanClarificationRequester | null): void {
  planClarificationRequester = requester
}

function normalizeName(rawName: string): string {
  const trimmed = rawName.trim().toLowerCase().replace(/\.md$/i, '').replace(/\s+/g, '-')
  if (!trimmed || /[^a-z0-9-]/.test(trimmed) || trimmed.startsWith('-') || trimmed.endsWith('-')) {
    throw new Error('Plan name must be lowercase alphanumeric with dashes (e.g., "feature-rollout")')
  }
  return trimmed
}

function randomDictionaryWord(): string {
  return ID_DICTIONARY[Math.floor(Math.random() * ID_DICTIONARY.length)]
}

function generateCandidateName(): string {
  return `${randomDictionaryWord()}-${randomDictionaryWord()}-${randomDictionaryWord()}`
}

function planFilePath(planDir: string, name: string): string {
  return path.join(planDir, `${normalizeName(name)}${PLAN_FILE_EXTENSION}`)
}

async function getPlanStorageDirectory(cwd?: string): Promise<string> {
  return resolveProjectQubitSubdirectory(PLAN_DIR_NAME, cwd)
}

async function ensureStorageDirectory(cwd?: string): Promise<string> {
  const dir = await getPlanStorageDirectory(cwd)
  await fs.promises.mkdir(dir, { recursive: true })
  return dir
}

async function existingPlanNames(dir: string): Promise<string[]> {
  try {
    const entries = await fs.promises.readdir(dir)
    return entries
      .filter(entry => entry.endsWith(PLAN_FILE_EXTENSION))
      .map(entry => entry.slice(0, -PLAN_FILE_EXTENSION.length))
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []
    throw error
  }
}

async function generatePlanName(dir: string): Promise<string> {
  const existing = new Set(await existingPlanNames(dir))
  for (let attempt = 0; attempt < MAX_ID_ATTEMPTS; attempt += 1) {
    const candidate = generateCandidateName()
    if (!existing.has(candidate)) return candidate
  }
  throw new Error('Unable to generate a unique plan name after multiple attempts')
}
function firstMarkdownHeading(content: string): string | undefined {
  for (const line of content.split('\n')) {
    const match = line.match(/^#\s+(.+)$/)
    if (match?.[1]) return match[1].trim()
  }
  return undefined
}

function normalizeQuestionId(raw: string | undefined, index: number): string {
  const normalized = String(raw || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return normalized || `question-${index + 1}`
}

function normalizeOptionId(raw: string | undefined, label: string, index: number): string {
  const normalized = String(raw || label || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return normalized || `option-${index + 1}`
}

export function normalizePlanClarificationQuestions(questions: PlanClarificationQuestion[] | undefined): PlanClarificationQuestion[] {
  if (!Array.isArray(questions) || questions.length === 0) {
    throw new Error('questions must be a non-empty array for clarify action')
  }

  return questions.map((question, questionIndex) => {
    const text = String(question?.question || '').trim()
    if (!text) throw new Error(`questions[${questionIndex}].question is required`)

    const options = Array.isArray(question.options) ? question.options : []
    const normalizedOptions: PlanClarificationOption[] = options.map((option, optionIndex) => {
      const label = String(option?.label || '').trim()
      if (!label) throw new Error(`questions[${questionIndex}].options[${optionIndex}].label is required`)
      return {
        id: normalizeOptionId(option.id, label, optionIndex),
        label,
        ...(option.description ? { description: String(option.description) } : {}),
      }
    })

    const manualLabel = String(question.manualLabel || DEFAULT_MANUAL_OPTION_LABEL).trim() || DEFAULT_MANUAL_OPTION_LABEL
    normalizedOptions.push({
      id: 'manual',
      label: manualLabel,
      manual: true,
    })

    return {
      id: normalizeQuestionId(question.id, questionIndex),
      question: text,
      ...(question.description ? { description: String(question.description) } : {}),
      options: normalizedOptions,
    }
  })
}

export async function listPlans(cwd?: string): Promise<PlanInfo[]> {
  const dir = await getPlanStorageDirectory(cwd)
  try {
    const entries = await fs.promises.readdir(dir)
    const planFiles = entries.filter(entry => entry.endsWith(PLAN_FILE_EXTENSION))
    const filesWithStats = await Promise.all(
      planFiles.map(async entry => {
        const filePath = path.join(dir, entry)
        const [stats, content] = await Promise.all([
          fs.promises.stat(filePath),
          fs.promises.readFile(filePath, 'utf8').catch(() => ''),
        ])
        return {
          name: entry.slice(0, -PLAN_FILE_EXTENSION.length),
          modifiedAt: stats.mtime.toISOString(),
          mtime: stats.mtime.getTime(),
          title: firstMarkdownHeading(content),
        }
      })
    )

    return filesWithStats
      .sort((a, b) => b.mtime - a.mtime)
      .slice(0, MAX_LIST_RESULTS)
      .map(({ name, modifiedAt, title }) => ({ name, modifiedAt, ...(title ? { title } : {}) }))
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []
    throw error
  }
}

export async function readPlan(name: string, cwd?: string): Promise<ReadPlanResult> {
  const normalized = normalizeName(name)
  const dir = await getPlanStorageDirectory(cwd)
  const filePath = planFilePath(dir, normalized)
  try {
    const content = await fs.promises.readFile(filePath, 'utf8')
    return { exists: true, name: normalized, content }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return { exists: false, name: normalized, content: null }
    throw error
  }
}

export async function createPlan(content: string, name?: string, cwd?: string): Promise<CreatePlanResult> {
  const dir = await ensureStorageDirectory(cwd)
  const planName = name ? normalizeName(name) : await generatePlanName(dir)
  const filePath = path.join(dir, `${planName}${PLAN_FILE_EXTENSION}`)
  await fs.promises.writeFile(filePath, content, { encoding: 'utf8', flag: 'wx' })
  return { name: planName, created: true, content }
}

export async function editPlan(
  name: string,
  search: string,
  replacement: string,
  cwd?: string
): Promise<EditPlanResult> {
  const normalized = normalizeName(name)
  const dir = await getPlanStorageDirectory(cwd)
  const filePath = path.join(dir, `${normalized}${PLAN_FILE_EXTENSION}`)

  try {
    const content = await fs.promises.readFile(filePath, 'utf8')
    const lines = content.split('\n')
    let matchCount = 0
    const newLines = lines.map(line => {
      if (!line.includes(search)) return line
      matchCount += 1
      return replacement
    })

    if (matchCount === 0) {
      return { success: false, message: `No lines found containing "${search}"`, name: normalized, content }
    }

    const newContent = newLines.join('\n')
    await fs.promises.writeFile(filePath, newContent, 'utf8')
    return { success: true, message: `Updated ${matchCount} line(s)`, name: normalized, content: newContent }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return { success: false, message: `Plan "${normalized}" does not exist`, name: normalized, content: null }
    }
    throw error
  }
}

export async function displayPlan(name: string, cwd?: string, context?: AgentContext): Promise<DisplayPlanResult> {
  const normalized = normalizeName(name)
  const dir = await getPlanStorageDirectory(cwd)
  const filePath = path.join(dir, `${normalized}${PLAN_FILE_EXTENSION}`)
  try {
    const content = await fs.promises.readFile(filePath, 'utf8')
    await planViewEmitter?.({ name: normalized, path: filePath, cwd, content, sessionId: context?.sessionId, runId: context?.runId, step: context?.step })
    return { displayed: true, exists: true, name: normalized, path: filePath, message: `Displayed plan "${normalized}" in the chat view.` }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return { displayed: false, exists: false, name: normalized, message: `Plan "${normalized}" does not exist` }
    }
    throw error
  }
}

export async function clarifyPlan(questions: PlanClarificationQuestion[] | undefined, context?: AgentContext): Promise<PlanClarificationResult> {
  const normalizedQuestions = normalizePlanClarificationQuestions(questions)
  if (!planClarificationRequester) {
    throw new Error('Plan clarification requester is not configured')
  }
  const requestId = `plan-clarify-${context?.runId || context?.sessionId || 'run'}-${context?.step || 0}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const response = await planClarificationRequester({
    id: requestId,
    sessionId: context?.sessionId,
    runId: context?.runId,
    step: context?.step,
    questions: normalizedQuestions,
  })
  if (response.cancelled) {
    return { clarified: false, cancelled: true, questions: normalizedQuestions.length, answers: [] }
  }
  return { clarified: true, questions: normalizedQuestions.length, answers: response.answers || [] }
}


export async function runPlanTool(args: PlanToolArgs, context?: AgentContext) {
  switch (args.action as string) {
    case 'create':
      return await createPlan(args.content ?? '', args.name, args.cwd)
    case 'list':
      return await listPlans(args.cwd)
    case 'read':
      if (!args.name) throw new Error('name is required for read action')
      return await readPlan(args.name, args.cwd)
    case 'edit':
      if (!args.name) throw new Error('name is required for edit action')
      if (args.search === undefined || args.search === '') throw new Error('search is required for edit action')
      if (args.replacement === undefined) throw new Error('replacement is required for edit action')
      return await editPlan(args.name, args.search, args.replacement, args.cwd)
    case 'display':
      if (!args.name) throw new Error('name is required for display action')
      return await displayPlan(args.name, args.cwd, context)
    case 'clarify':
      return await clarifyPlan(args.questions, context)
    default: {
      throw new Error(`Unsupported plan action: ${String(args.action)}`)
    }
  }
}

export const planMdTool = defineTool({
  name: 'planMd',
  description: 'Create, list, read, edit, or display Markdown plans stored in the project .qubit/plans directory.',
  inputSchema: {
    type: 'object',
    properties: {
      action: { type: 'string', enum: ['create', 'list', 'read', 'edit', 'display', 'clarify'] },
      name: { type: 'string', description: 'Plan name. Required for read/edit/display; optional for create.' },
      content: { type: 'string', description: 'Markdown content for create action.' },
      search: { type: 'string', description: 'Line substring to find for edit action.' },
      replacement: { type: 'string', description: 'Full replacement line for edit action.' },
      cwd: { type: 'string', description: 'Workspace directory whose .qubit/plans directory stores plan files.' },
      questions: {
        type: 'array',
        description: 'Clarification questions for the user before final plan output. Used with action=clarify.',
        items: {
          type: 'object',
          properties: {
            id: { type: 'string' },
            question: { type: 'string' },
            description: { type: 'string' },
            manualLabel: { type: 'string' },
            options: {
              type: 'array',
              items: {
                type: 'object',
                properties: {
                  id: { type: 'string' },
                  label: { type: 'string' },
                  description: { type: 'string' },
                },
                required: ['label'],
                additionalProperties: false,
              },
            },
          },
          required: ['question'],
          additionalProperties: false,
        },
      }
    },
    required: ['action'],
    additionalProperties: false,
  },
  permission: { mode: 'always' },
  async execute(args: PlanToolArgs, context: AgentContext) {
    try {
      return { ok: true, data: await runPlanTool(args, context) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})
