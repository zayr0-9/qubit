import * as fs from 'node:fs'
import * as path from 'node:path'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'

const PLAN_MODE_MESSAGE =
  'You are in planning mode. File modification is not allowed. Please describe your implementation plan instead. Do not try to edit the code or make changes. Do not use bash to skip this warning.'

export interface DeleteFileOptions {
  allowedExtensions?: string[]
  operationMode?: 'plan' | 'execute'
  cwd?: string
}

export interface DeleteFileResult {
  success: boolean
  deleted: boolean
  message: string
  path?: string
}

function extensionAllowed(filePath: string, allowedExtensions?: string[]): boolean {
  if (!allowedExtensions || allowedExtensions.length === 0) return true
  const ext = path.extname(filePath).toLowerCase()
  return allowedExtensions.some(allowed => allowed.toLowerCase() === ext)
}

function assertNotSensitivePath(displayPath: string): void {
  const normalized = displayPath.replace(/\\/g, '/').toLowerCase()
  const segments = normalized.split('/').filter(Boolean)

  if (normalized === '/etc/passwd' || normalized === '/etc/shadow') {
    throw new Error(`Cannot delete sensitive system file: ${displayPath}`)
  }

  if (segments.includes('.git') || segments.includes('node_modules')) {
    throw new Error(`Cannot delete sensitive project path: ${displayPath}`)
  }

  if (segments[0] === 'proc' || segments[0] === 'sys' || segments[0] === 'dev') {
    throw new Error(`Cannot delete sensitive system path: ${displayPath}`)
  }
}

export async function deleteTextFile(filePath: string, options: DeleteFileOptions = {}): Promise<DeleteFileResult> {
  if (options.operationMode === 'plan') {
    return { success: false, deleted: false, message: PLAN_MODE_MESSAGE }
  }

  try {
    const workspaceCwd = cwdOrDefault(options.cwd)
    const resolved = await resolveRestrictedToolPath(filePath, { cwd: workspaceCwd, mode: 'file' })

    if (!extensionAllowed(resolved.displayPath, options.allowedExtensions)) {
      const ext = path.extname(resolved.displayPath).toLowerCase()
      return {
        success: false,
        deleted: false,
        path: resolved.displayPath,
        message: `File extension '${ext}' not allowed. Allowed extensions: ${options.allowedExtensions?.join(', ')}`,
      }
    }

    assertNotSensitivePath(resolved.displayPath)

    const stats = await fs.promises.stat(resolved.fsPath).catch((error: NodeJS.ErrnoException) => {
      if (error.code === 'ENOENT') return null
      throw error
    })

    if (!stats) {
      return { success: false, deleted: false, path: resolved.displayPath, message: `File not found: ${resolved.displayPath}` }
    }

    if (!stats.isFile()) {
      return { success: false, deleted: false, path: resolved.displayPath, message: `'${resolved.displayPath}' is not a file` }
    }

    await fs.promises.unlink(resolved.fsPath)
    return { success: true, deleted: true, path: resolved.displayPath, message: `Deleted file: ${resolved.displayPath}` }
  } catch (error) {
    return {
      success: false,
      deleted: false,
      message: `Failed to delete file: ${error instanceof Error ? error.message : String(error)}`,
    }
  }
}

export const deleteFileTool = defineTool({
  name: 'deleteFile',
  description: 'Delete a file within the workspace, optionally restricted to allowed file extensions.',
  inputSchema: {
    type: 'object',
    properties: {
      path: { type: 'string', description: 'File path to delete.' },
      cwd: { type: 'string', description: 'Workspace directory used for path resolution and restriction.' },
      allowedExtensions: { type: 'array', items: { type: 'string' }, description: 'Optional allowed extensions such as .txt or .json.' },
      operationMode: { type: 'string', enum: ['plan', 'execute'] },
    },
    required: ['path'],
    additionalProperties: false,
  },
  permission: { mode: 'ask' },
  async execute(args: DeleteFileOptions & { path?: string }) {
    if (!args.path) return { ok: false, error: 'path is required' }
    try {
      return { ok: true, data: await deleteTextFile(args.path, args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default deleteTextFile
