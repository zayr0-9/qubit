import { defineTool } from '@hyper-labs/hyper-router'
import { relativeDisplayPath, resolveRestrictedToolPath, resolveToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { cwdBlockEnabledFromContext, restrictToCwd } from '../utils/toolAccessPolicy.js'
import { readTextFile, type ReadFileOptions, type ReadFileResult } from './readFile.js'

export interface ReadMultipleOptions extends ReadFileOptions {
  baseDir?: string
}

export interface ReadMultipleResultItem {
  filename: string
  content: string
  totalLines: number
  startLine?: number
  endLine?: number
  ranges?: ReadFileResult['ranges']
  truncated?: boolean
}

export async function readMultipleTextFiles(
  inputPaths: string[],
  options: ReadMultipleOptions = {}
): Promise<ReadMultipleResultItem[]> {
  if (!Array.isArray(inputPaths) || inputPaths.length === 0) throw new Error('No file paths provided')

  const cwdBase = cwdOrDefault(options.cwd)
  await resolveRestrictedToolPath(cwdBase, {
    cwd: cwdBase,
    mode: 'directory',
    restrictToCwd: restrictToCwd(options),
    workspaceCwd: options.workspaceCwd,
  })
  const baseDir = await resolveRestrictedToolPath(options.baseDir || cwdBase, {
    cwd: cwdBase,
    mode: 'directory',
    restrictToCwd: restrictToCwd(options),
    workspaceCwd: options.workspaceCwd,
  })
  const results: ReadMultipleResultItem[] = new Array(inputPaths.length)

  const readOne = async (inputPath: string, index: number): Promise<void> => {
    try {
      const res = await readTextFile(inputPath, {
        maxBytes: options.maxBytes,
        startLine: options.startLine,
        endLine: options.endLine,
        ranges: options.ranges,
        cwd: cwdBase,
        includeHash: false,
        cwdBlockEnabled: options.cwdBlockEnabled,
        restrictToCwd: options.restrictToCwd,
      })

      const resolved = await resolveToolPath(inputPath, { cwd: cwdBase })
      const filename = relativeDisplayPath(baseDir, resolved)
      const totalLines = res.totalLines ?? res.content.split(/\r?\n/).length

      results[index] = {
        filename,
        content: res.content,
        totalLines,
        startLine: res.startLine,
        endLine: res.endLine,
        ranges: res.ranges,
        truncated: res.truncated,
      }
    } catch (error) {
      results[index] = {
        filename: inputPath,
        content: `[Error reading file: ${error instanceof Error ? error.message : String(error)}]`,
        totalLines: 0,
      }
    }
  }

  const concurrency = Math.min(4, inputPaths.length)
  let nextIndex = 0

  await Promise.all(
    Array.from({ length: concurrency }, async () => {
      while (true) {
        const currentIndex = nextIndex
        nextIndex += 1
        if (currentIndex >= inputPaths.length) return
        await readOne(inputPaths[currentIndex], currentIndex)
      }
    })
  )

  return results
}

function formatReadFilesHeader(item: ReadMultipleResultItem): string {
  if (item.ranges && item.ranges.length > 0) {
    const ranges = item.ranges.map(range => `${range.startLine}-${range.endLine}`).join(', ')
    return `${item.filename} (ranges ${ranges})`
  }

  if (item.startLine !== undefined && item.endLine !== undefined) {
    return `${item.filename} (lines ${item.startLine}-${item.endLine})`
  }

  return item.filename
}

export function formatReadFilesResult(items: ReadMultipleResultItem[]): string {
  return items.map(item => `${formatReadFilesHeader(item)}\n${item.content}`).join('\n\n')
}

export const readFilesTool = defineTool({
  name: 'readFiles',
  description: 'Read multiple text/code/config files and return their contents with relative filename headers. Line selections apply to every file in paths.',
  inputSchema: {
    type: 'object',
    properties: {
      paths: { type: 'array', items: { type: 'string' }, description: 'File paths to read.' },
      cwd: { type: 'string', description: 'Optional workspace directory used for path resolution and restriction.' },
      baseDir: { type: 'string', description: 'Optional base directory used to compute relative headers.' },
      maxBytes: { type: 'number', description: 'Optional per-file safety limit. Defaults to 200KB.' },
      startLine: { type: 'number', description: 'Optional 1-based start line applied to every file.' },
      endLine: { type: 'number', description: 'Optional 1-based end line applied to every file.' },
      ranges: {
        type: 'array',
        description: 'Optional line ranges applied to every file in paths.',
        items: {
          type: 'object',
          properties: { startLine: { type: 'number' }, endLine: { type: 'number' } },
          required: ['startLine', 'endLine'],
          additionalProperties: false,
        },
      },
    },
    required: ['paths'],
    additionalProperties: false,
  },
  permission: { mode: 'always' },
  async execute(args: ReadMultipleOptions & { paths?: string[] }, context) {
    if (!args.paths) return { ok: false, error: 'paths is required' }
    try {
      const files = await readMultipleTextFiles(args.paths, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) })
      return { ok: true, data: { files, content: formatReadFilesResult(files) } }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})
