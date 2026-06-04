import { glob } from 'glob'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { cwdBlockEnabledFromContext, restrictToCwd, type ToolAccessPolicyOptions } from '../utils/toolAccessPolicy.js'

const DEFAULT_MAX_MATCHES = 3000
const DEFAULT_TIMEOUT_MS = 5000
const DIRECTORY_DEPTH_LIMIT = 6
const DEFAULT_IGNORE_PATTERNS = [
  '**/node_modules/**',
  '**/.git/**',
  '**/.svn/**',
  '**/.hg/**',
  '**/.idea/**',
  '**/.vscode/**',
  '**/.next/**',
  '**/.nuxt/**',
  '**/.cache/**',
  '**/dist/**',
  '**/build/**',
  '**/coverage/**',
  '**/tmp/**',
  '**/temp/**',
  '**/*.min.js',
]

export interface GlobOptions extends ToolAccessPolicyOptions {
  cwd?: string
  ignore?: string | string[]
  dot?: boolean
  absolute?: boolean
  mark?: boolean
  nosort?: boolean
  nocase?: boolean
  nodir?: boolean
  follow?: boolean
  realpath?: boolean
  stat?: boolean
  withFileTypes?: boolean
  maxMatches?: number
  timeoutMs?: number
}

export interface GlobResult {
  success: boolean
  matches: string[]
  error?: string
  pattern?: string
  cwd?: string
}

function mergeIgnorePatterns(defaults: string[], custom?: string | string[]): string[] {
  if (!custom) return defaults
  const userPatterns = Array.isArray(custom) ? custom : [custom]
  return Array.from(new Set([...defaults, ...userPatterns.filter(Boolean)]))
}

function enforcePatternDepth(pattern: string): string {
  const segments = pattern.split('/').filter(Boolean)
  if (segments.length <= DIRECTORY_DEPTH_LIMIT) return pattern
  return segments.slice(0, DIRECTORY_DEPTH_LIMIT).join('/')
}

export async function globSearch(pattern: string, options: GlobOptions = {}): Promise<GlobResult> {
  if (!pattern || pattern.trim() === '') {
    return { success: false, matches: [], error: 'Pattern cannot be empty' }
  }

  const {
    cwd,
    ignore,
    dot = false,
    absolute = false,
    mark = false,
    nosort = false,
    nocase = false,
    nodir = false,
    follow = false,
    realpath = false,
    stat = false,
    withFileTypes = false,
    maxMatches = DEFAULT_MAX_MATCHES,
    timeoutMs = DEFAULT_TIMEOUT_MS,
  } = options

  try {
    const workspaceCwd = cwdOrDefault(cwd)
    const resolvedCwd = await resolveRestrictedToolPath(workspaceCwd, {
      cwd: workspaceCwd,
      mode: 'directory',
      restrictToCwd: restrictToCwd(options),
      workspaceCwd: options.workspaceCwd,
    })
    const sanitizedPattern = enforcePatternDepth(pattern)
    const ignorePatterns = mergeIgnorePatterns(DEFAULT_IGNORE_PATTERNS, ignore)

    const globOptions: Record<string, unknown> = {
      cwd: resolvedCwd.fsPath,
      ignore: ignorePatterns,
      dot,
      absolute,
      mark,
      nosort,
      nocase,
      nodir,
      follow,
      realpath,
      stat,
      withFileTypes,
      windowsPathsNoEscape: true,
    }

    const results = (await Promise.race([
      glob(sanitizedPattern, globOptions),
      new Promise<never>((_, reject) => {
        setTimeout(() => reject(new Error('Glob search timed out. Narrow the pattern or specify a smaller cwd.')), timeoutMs)
      }),
    ])) as unknown[]

    if (!Array.isArray(results)) throw new Error('Glob search did not return an array of results')

    if (results.length > maxMatches) {
      return {
        success: false,
        matches: [],
        error: `Too many matches (${results.length} > ${maxMatches}). Narrow the pattern or reduce cwd scope.`,
        pattern: sanitizedPattern,
        cwd: resolvedCwd.displayPath,
      }
    }

    const matches = withFileTypes
      ? results.map((dirent: any) => dirent?.fullpath?.() || dirent?.path || String(dirent))
      : (results as string[])

    return { success: true, matches, pattern: sanitizedPattern, cwd: resolvedCwd.displayPath }
  } catch (error) {
    return {
      success: false,
      matches: [],
      error: error instanceof Error ? error.message : 'Glob search failed',
      pattern,
      cwd,
    }
  }
}

export const globTool = defineTool({
  name: 'glob',
  description: 'Search for files using glob patterns with ignore filters and match limits.',
  inputSchema: {
    type: 'object',
    properties: {
      pattern: { type: 'string', description: 'Glob pattern to match files.' },
      cwd: { type: 'string', description: 'Current working directory to search from.' },
      ignore: {
        oneOf: [{ type: 'string' }, { type: 'array', items: { type: 'string' } }],
        description: 'Additional ignore pattern or patterns.',
      },
      dot: { type: 'boolean' },
      absolute: { type: 'boolean' },
      mark: { type: 'boolean' },
      nosort: { type: 'boolean' },
      nocase: { type: 'boolean' },
      nodir: { type: 'boolean' },
      follow: { type: 'boolean' },
      realpath: { type: 'boolean' },
      stat: { type: 'boolean' },
      withFileTypes: { type: 'boolean' },
      maxMatches: { type: 'number' },
      timeoutMs: { type: 'number' },
    },
    required: ['pattern'],
    additionalProperties: false,
  },
  permission: { mode: 'always' },
  async execute(args: GlobOptions & { pattern?: string }, context) {
    if (!args.pattern) return { ok: false, error: 'pattern is required' }
    return { ok: true, data: await globSearch(args.pattern, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) }) }
  },
})

export default globSearch
