import * as fs from 'node:fs'
import * as path from 'node:path'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveProjectQubitSubdirectory } from '../utils/qubitProject.js'

const TODO_DIR_NAME = 'todos'
const TODO_FILE_EXTENSION = '.md'
const MAX_LIST_RESULTS = 5
const MAX_ID_ATTEMPTS = 12

const ID_DICTIONARY = [
  'ember', 'atlas', 'sage', 'haven', 'lumen', 'quill', 'cinder', 'aurora',
  'drift', 'marble', 'pioneer', 'fern', 'opal', 'orbit', 'spark', 'basil',
  'cascade', 'north', 'horizon', 'goku', 'vegeta', 'piccolo', 'gohan',
  'freeza', 'cell', 'bulma', 'trunks', 'broly', 'gurren', 'lagann',
]

export type TodoAction = 'create' | 'list' | 'read' | 'edit'

export interface TodoEdit {
  search: string
  replacement: string
}

export interface TodoToolArgs {
  action: TodoAction
  name?: string
  content?: string
  search?: string
  replacement?: string
  edits?: TodoEdit[]
  cwd?: string
}

export interface TodoListInfo {
  id: string
  modifiedAt: string
}

export interface ReadTodoResult {
  exists: boolean
  content: string | null
}

export interface CreateTodoResult {
  id: string
  created: boolean
  success: boolean
  message: string
  content: string | null
}

export interface TodoEditResultItem {
  search: string
  replacement: string
  matched: number
  success: boolean
  message: string
}

export interface EditTodoResult {
  success: boolean
  message: string
  content: string | null
  results?: TodoEditResultItem[]
}

function normalizeId(rawId: string): string {
  const trimmed = rawId.trim().toLowerCase().replace(/\s+/g, '-')
  if (!trimmed || /[^a-z0-9-]/.test(trimmed) || trimmed.startsWith('-') || trimmed.endsWith('-')) {
    throw new Error('Todo name must be lowercase alphanumeric with dashes (e.g., "my-project-tasks")')
  }
  return trimmed
}

function randomDictionaryWord(): string {
  return ID_DICTIONARY[Math.floor(Math.random() * ID_DICTIONARY.length)]
}

function generateCandidateId(): string {
  return `${randomDictionaryWord()}-${randomDictionaryWord()}-${randomDictionaryWord()}`
}

function todoFilePath(todoDir: string, name: string): string {
  return path.join(todoDir, `${normalizeId(name)}${TODO_FILE_EXTENSION}`)
}

export async function getTodoStorageDirectory(cwd?: string): Promise<string> {
  return resolveProjectQubitSubdirectory(TODO_DIR_NAME, cwd)
}

async function ensureStorageDirectory(cwd?: string): Promise<string> {
  const dir = await getTodoStorageDirectory(cwd)
  await fs.promises.mkdir(dir, { recursive: true })
  return dir
}

async function existingTodoIds(dir: string): Promise<string[]> {
  try {
    const entries = await fs.promises.readdir(dir)
    return entries
      .filter(entry => entry.endsWith(TODO_FILE_EXTENSION))
      .map(entry => entry.slice(0, -TODO_FILE_EXTENSION.length))
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []
    throw error
  }
}

async function generateTodoId(dir: string): Promise<string> {
  const existing = new Set(await existingTodoIds(dir))
  for (let attempt = 0; attempt < MAX_ID_ATTEMPTS; attempt += 1) {
    const candidate = generateCandidateId()
    if (!existing.has(candidate)) return candidate
  }
  throw new Error('Unable to generate a unique todo id after multiple attempts')
}

export async function listTodoLists(cwd?: string): Promise<TodoListInfo[]> {
  const dir = await getTodoStorageDirectory(cwd)
  try {
    const entries = await fs.promises.readdir(dir)
    const todoFiles = entries.filter(entry => entry.endsWith(TODO_FILE_EXTENSION))
    const filesWithStats = await Promise.all(
      todoFiles.map(async entry => {
        const filePath = path.join(dir, entry)
        const stats = await fs.promises.stat(filePath)
        return {
          id: entry.slice(0, -TODO_FILE_EXTENSION.length),
          modifiedAt: stats.mtime.toISOString(),
          mtime: stats.mtime.getTime(),
        }
      })
    )

    return filesWithStats
      .sort((a, b) => b.mtime - a.mtime)
      .slice(0, MAX_LIST_RESULTS)
      .map(({ id, modifiedAt }) => ({ id, modifiedAt }))
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []
    throw error
  }
}

export async function readTodoList(name: string, cwd?: string): Promise<ReadTodoResult> {
  const dir = await getTodoStorageDirectory(cwd)
  const filePath = todoFilePath(dir, name)
  try {
    const content = await fs.promises.readFile(filePath, 'utf8')
    return { exists: true, content }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return { exists: false, content: null }
    throw error
  }
}

export async function createTodoList(content: string, cwd?: string, name?: string): Promise<CreateTodoResult> {
  const dir = await ensureStorageDirectory(cwd)
  const id = name ? normalizeId(name) : await generateTodoId(dir)
  const filePath = path.join(dir, `${id}${TODO_FILE_EXTENSION}`)

  try {
    await fs.promises.writeFile(filePath, content, { encoding: 'utf8', flag: 'wx' })
    return { id, created: true, success: true, message: `Created todo list "${id}"`, content }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'EEXIST') {
      return { id, created: false, success: false, message: `Todo list "${id}" already exists`, content: null }
    }
    throw error
  }
}

export async function editTodoList(
  name: string,
  edits: TodoEdit[],
  cwd?: string
): Promise<EditTodoResult> {
  const sanitized = normalizeId(name)
  const dir = await getTodoStorageDirectory(cwd)
  const filePath = path.join(dir, `${sanitized}${TODO_FILE_EXTENSION}`)

  if (!Array.isArray(edits) || edits.length === 0) {
    throw new Error('edits must be a non-empty array for edit action')
  }

  for (const [index, edit] of edits.entries()) {
    if (!edit || edit.search === undefined || edit.search === '') {
      throw new Error(`edits[${index}].search is required for edit action`)
    }
    if (edit.replacement === undefined) {
      throw new Error(`edits[${index}].replacement is required for edit action`)
    }
  }

  try {
    const content = await fs.promises.readFile(filePath, 'utf8')
    let lines = content.split('\n')
    const results: TodoEditResultItem[] = []

    for (const edit of edits) {
      let matchCount = 0
      const nextLines = lines.map(line => {
        if (!line.includes(edit.search)) return line
        matchCount += 1
        return edit.replacement
      })
      results.push({
        search: edit.search,
        replacement: edit.replacement,
        matched: matchCount,
        success: matchCount > 0,
        message: matchCount > 0 ? `Updated ${matchCount} line(s)` : `No lines found containing "${edit.search}"`,
      })
      lines = nextLines
    }

    const failed = results.filter(result => !result.success)
    if (failed.length > 0) {
      return {
        success: false,
        message: `Failed ${failed.length} edit(s); no changes written`,
        content,
        results,
      }
    }

    const totalMatches = results.reduce((sum, result) => sum + result.matched, 0)
    const newContent = lines.join('\n')
    await fs.promises.writeFile(filePath, newContent, 'utf8')
    return {
      success: true,
      message: `Updated ${totalMatches} line(s) across ${results.length} edit(s)`,
      content: newContent,
      results,
    }
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return { success: false, message: `Todo list "${sanitized}" does not exist`, content: null }
    }
    throw error
  }
}

export async function runTodoTool(args: TodoToolArgs) {
  switch (args.action) {
    case 'create':
      return await createTodoList(args.content ?? '', args.cwd, args.name)
    case 'list':
      return await listTodoLists(args.cwd)
    case 'read':
      if (!args.name) throw new Error('name is required for read action')
      return await readTodoList(args.name, args.cwd)
    case 'edit': {
      if (!args.name) throw new Error('name is required for edit action')
      const edits = args.edits ?? (
        args.search !== undefined || args.replacement !== undefined
          ? [{ search: args.search ?? '', replacement: args.replacement ?? '' }]
          : undefined
      )
      if (!edits) throw new Error('edits or search/replacement is required for edit action')
      return await editTodoList(args.name, edits, args.cwd)
    }
    default: {
      const exhaustive: never = args.action
      throw new Error(`Unsupported todo action: ${String(exhaustive)}`)
    }
  }
}

export const todoMdTool = defineTool({
  name: 'todoMd',
  description: 'Manage Markdown todo lists stored in .qubit/todos under the default or supplied workspace.',
  inputSchema: {
    type: 'object',
    properties: {
      action: { type: 'string', enum: ['create', 'list', 'read', 'edit'] },
      name: { type: 'string', description: 'Todo list name. Optional for create; required for read/edit. If omitted on create, a random id is generated and must be used for later read/edit calls.' },
      content: { type: 'string', description: 'Markdown content for create action.' },
      search: { type: 'string', description: 'Line substring to find for single edit action. Prefer edits for multiple changes.' },
      replacement: { type: 'string', description: 'Full replacement line for single edit action. Prefer edits for multiple changes.' },
      edits: {
        type: 'array',
        description: 'Multiple line replacement edits to apply in one call. Each edit finds lines containing search and replaces the full line with replacement. All edits must match or no changes are written.',
        items: {
          type: 'object',
          properties: {
            search: { type: 'string', description: 'Line substring to find.' },
            replacement: { type: 'string', description: 'Full replacement line.' },
          },
          required: ['search', 'replacement'],
          additionalProperties: false,
        },
      },
      cwd: { type: 'string', description: 'Workspace directory whose .qubit/todos directory stores todo files.' },
    },
    required: ['action'],
    additionalProperties: false,
  },
  permission: { mode: 'ask', reason: 'Todo operations can create and edit files under the workspace .qubit/todos directory.' },
  async execute(args: TodoToolArgs) {
    try {
      return { ok: true, data: await runTodoTool(args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})
