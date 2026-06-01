import * as fs from 'node:fs'
import * as path from 'node:path'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'

const PLAN_MODE_MESSAGE =
  'You are in planning mode. File modification is not allowed. Please describe your implementation plan instead. Do not try to edit the code or make changes. Do not use bash to skip this warning.'

export interface CreateFileOptions {
  createParentDirs?: boolean
  overwrite?: boolean
  executable?: boolean
  operationMode?: 'plan' | 'execute'
  cwd?: string
}

export interface CreateFileResult {
  success: boolean
  created: boolean
  sizeBytes: number
  message: string
  path?: string
}

export async function createTextFile(
  filePath: string,
  content: string = '',
  options: CreateFileOptions = {}
): Promise<CreateFileResult> {
  const { createParentDirs = true, overwrite = false, executable = false, operationMode } = options

  if (operationMode === 'plan') {
    return { success: false, created: false, sizeBytes: 0, message: PLAN_MODE_MESSAGE }
  }

  try {
    const workspaceCwd = cwdOrDefault(options.cwd)
    const resolved = await resolveRestrictedToolPath(filePath, { cwd: workspaceCwd, mode: 'file' })
    const targetPath = resolved.fsPath

    const existingStats = await fs.promises.stat(targetPath).catch((error: NodeJS.ErrnoException) => {
      if (error.code === 'ENOENT') return null
      throw error
    })

    if (existingStats && existingStats.isDirectory()) {
      return {
        success: false,
        created: false,
        sizeBytes: 0,
        path: resolved.displayPath,
        message: `Cannot create file at ${resolved.displayPath}: a directory already exists at that path.`,
      }
    }

    if (existingStats && !overwrite) {
      return {
        success: false,
        created: false,
        sizeBytes: 0,
        path: resolved.displayPath,
        message: `File already exists at ${resolved.displayPath}. Use overwrite option to replace it.`,
      }
    }

    const parentDir = path.dirname(targetPath)
    if (createParentDirs) {
      await fs.promises.mkdir(parentDir, { recursive: true })
    } else {
      const parentStats = await fs.promises.stat(parentDir).catch((error: NodeJS.ErrnoException) => {
        if (error.code === 'ENOENT') return null
        throw error
      })
      if (!parentStats?.isDirectory()) {
        return {
          success: false,
          created: false,
          sizeBytes: 0,
          path: resolved.displayPath,
          message: `Parent directory does not exist: ${parentDir}`,
        }
      }
    }

    await fs.promises.writeFile(targetPath, content, 'utf8')

    if (executable && process.platform !== 'win32') {
      await fs.promises.chmod(targetPath, 0o755)
    }

    const stats = await fs.promises.stat(targetPath)
    return {
      success: true,
      created: !existingStats,
      sizeBytes: stats.size,
      path: resolved.displayPath,
      message: existingStats
        ? `File overwritten successfully at ${resolved.displayPath}`
        : `File created successfully at ${resolved.displayPath}`,
    }
  } catch (error) {
    return {
      success: false,
      created: false,
      sizeBytes: 0,
      message: `Error creating file: ${error instanceof Error ? error.message : String(error)}`,
    }
  }
}

export const createFileTool = defineTool({
  name: 'createFile',
  description: 'Create a text file with optional parent directory creation, overwrite support, and executable mode.',
  inputSchema: {
    type: 'object',
    properties: {
      path: { type: 'string', description: 'File path to create.' },
      content: { type: 'string', description: 'Initial content to write. Defaults to empty string.' },
      cwd: { type: 'string', description: 'Workspace directory used for path resolution and restriction.' },
      createParentDirs: { type: 'boolean', description: 'Create parent directories as needed. Defaults to true.' },
      overwrite: { type: 'boolean', description: 'Overwrite an existing file. Defaults to false.' },
      executable: { type: 'boolean', description: 'Make the file executable on POSIX systems. Defaults to false.' },
      operationMode: { type: 'string', enum: ['plan', 'execute'] },
    },
    required: ['path'],
    additionalProperties: false,
  },
  permission: { mode: 'ask' },
  async execute(args: CreateFileOptions & { path?: string; content?: string }) {
    if (!args.path) return { ok: false, error: 'path is required' }
    try {
      return { ok: true, data: await createTextFile(args.path, args.content ?? '', args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default createTextFile
