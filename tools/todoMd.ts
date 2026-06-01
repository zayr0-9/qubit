import * as fs from 'node:fs'
import * as path from 'node:path'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'

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

export interface TodoToolArgs {
  action: TodoAction
  name?: string
  content?: string
  search?: string
  replacement?: string
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
  content: string
}

export interface EditTodoResult {
  success: boolean
  message: string
  content: string | null
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
  const workspaceCwd = cwdOrDefault(cwd)
  const resolved = await resolveRestrictedToolPath(path.join('.qubit', TODO_DIR_NAME), {
    cwd: workspaceCwd,
    mode: 'directory',
  })
  return resolved.fsPath
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

export async function createTodoList(content: string, cwd?: string): Promise<CreateTodoResult> {
  const dir = await ensureStorageDirectory(cwd)
  const id = await generateTodoId(dir)
  const filePath = path.join(dir, `${id}${TODO_FILE_EXTENSION}`)
  await fs.promises.writeFile(filePath, content, 'utf8')
  return { id, created: true, content }
}

export async function editTodoList(
  name: string,
  search: string,
  replacement: string,
  cwd?: string
): Promise<EditTodoResult> {
  const sanitized = normalizeId(name)
  const dir = await getTodoStorageDirectory(cwd)
  const filePath = path.join(dir, `${sanitized}${TODO_FILE_EXTENSION}`)

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
      return { success: false, message: `No lines found containing "${search}"`, content }
    }

    const newContent = newLines.join('\n')
    await fs.promises.writeFile(filePath, newContent, 'utf8')
    return { success: true, message: 'Updated', content: newContent }
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
      return await createTodoList(args.content ?? '', args.cwd)
    case 'list':
      return await listTodoLists(args.cwd)
    case 'read':
      if (!args.name) throw new Error('name is required for read action')
      return await readTodoList(args.name, args.cwd)
    case 'edit':
      if (!args.name) throw new Error('name is required for edit action')
      if (args.search === undefined || args.search === '') throw new Error('search is required for edit action')
      if (args.replacement === undefined) throw new Error('replacement is required for edit action')
      return await editTodoList(args.name, args.search, args.replacement, args.cwd)
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
      name: { type: 'string', description: 'Todo list name. Required for read/edit.' },
      content: { type: 'string', description: 'Markdown content for create action.' },
      search: { type: 'string', description: 'Line substring to find for edit action.' },
      replacement: { type: 'string', description: 'Full replacement line for edit action.' },
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
