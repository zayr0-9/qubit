import * as crypto from 'node:crypto'
import * as fs from 'node:fs'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { cwdBlockEnabledFromContext, restrictToCwd, type ToolAccessPolicyOptions } from '../utils/toolAccessPolicy.js'

export interface LineRange {
  startLine: number
  endLine: number
}

export interface ReadFileOptions extends ToolAccessPolicyOptions {
  maxBytes?: number
  startLine?: number
  endLine?: number
  ranges?: LineRange[]
  includeHash?: boolean
  cwd?: string
}

export interface FileMetadata {
  lineEnding: '\n' | '\r\n' | 'mixed'
  hasBOM: boolean
  encoding: BufferEncoding
  lastModified: Date
  inode?: number
}

export interface ReadFileResult {
  content: string
  truncated: boolean
  sizeBytes: number
  contentHash?: string
  fileHash?: string
  metadata: FileMetadata
  startLine?: number
  endLine?: number
  totalLines?: number
  ranges?: Array<{
    startLine: number
    endLine: number
    lineCount: number
  }>
}

function isLikelyBinary(buf: Buffer): boolean {
  const len = Math.min(buf.length, 4096)
  if (len === 0) return false

  let suspicious = 0
  for (let i = 0; i < len; i += 1) {
    const byte = buf[i]
    if (byte === 0) return true
    if (byte < 7 || (byte > 13 && byte < 32)) suspicious += 1
  }
  return suspicious / len > 0.3
}

function calculateHash(content: string): string {
  return crypto.createHash('sha256').update(content, 'utf8').digest('hex')
}

function detectLineEnding(content: string): '\n' | '\r\n' | 'mixed' {
  const hasCRLF = content.includes('\r\n')
  const lfOnlyCount = (content.match(/(?<!\r)\n/g) || []).length

  if (hasCRLF && lfOnlyCount > 0) return 'mixed'
  if (hasCRLF) return '\r\n'
  return '\n'
}

function hasBOM(buf: Buffer): boolean {
  return buf.length >= 3 && buf[0] === 0xef && buf[1] === 0xbb && buf[2] === 0xbf
}

function validateLineRangeValues(options: ReadFileOptions): void {
  const validateLineNumber = (name: string, value: number) => {
    if (!Number.isInteger(value) || value < 1) throw new Error(`${name} must be an integer >= 1`)
  }

  if (options.startLine !== undefined) validateLineNumber('startLine', options.startLine)
  if (options.endLine !== undefined) validateLineNumber('endLine', options.endLine)

  if (options.ranges) {
    for (let i = 0; i < options.ranges.length; i += 1) {
      validateLineNumber(`ranges[${i}].startLine`, options.ranges[i].startLine)
      validateLineNumber(`ranges[${i}].endLine`, options.ranges[i].endLine)
    }
  }
}

interface StreamedLineSelectionResult {
  content: string
  totalLines?: number
  startLine?: number
  endLine?: number
  ranges?: Array<{ startLine: number; endLine: number; lineCount: number }>
  lineEndingStyle: '\n' | '\r\n' | 'mixed'
  fileHash?: string
}

async function readProbeBytes(filePath: string, maxBytes: number): Promise<Buffer> {
  const bytesToRead = Math.max(0, maxBytes)
  if (bytesToRead === 0) return Buffer.alloc(0)

  const fd = await fs.promises.open(filePath, 'r')
  try {
    const buf = Buffer.allocUnsafe(bytesToRead)
    const { bytesRead } = await fd.read(buf, 0, bytesToRead, 0)
    return bytesRead === bytesToRead ? buf : buf.subarray(0, bytesRead)
  } finally {
    await fd.close()
  }
}

async function readLineSelectionFromFile(
  filePath: string,
  options: ReadFileOptions,
  includeHash: boolean
): Promise<StreamedLineSelectionResult> {
  const hasRanges = !!options.ranges && options.ranges.length > 0
  const requestedRanges =
    options.ranges?.map(range => {
      const startLine = Math.max(1, range.startLine)
      const endLine = range.endLine
      if (endLine < startLine) throw new Error(`Range endLine ${endLine} cannot be less than startLine ${startLine}`)
      return { startLine, endLine, lines: [] as string[] }
    }) || []

  const singleStartLine = options.startLine !== undefined ? Math.max(1, options.startLine) : 1
  const singleEndLine = options.endLine ?? Number.POSITIVE_INFINITY
  if (!hasRanges && Number.isFinite(singleEndLine) && singleEndLine < singleStartLine) {
    throw new Error(`endLine ${singleEndLine} cannot be less than startLine ${singleStartLine}`)
  }

  const maxRequestedEndLine = hasRanges
    ? Math.max(...requestedRanges.map(range => range.endLine))
    : Number.isFinite(singleEndLine)
      ? singleEndLine
      : Number.POSITIVE_INFINITY

  let lineNumber = 0
  let carry = ''
  let endedWithLineBreak = false
  let reachedEOF = false
  let stoppedEarly = false
  let sawCRLF = false
  let sawLFOnly = false

  const selectedLines: string[] = []
  const wholeFileHasher = includeHash ? crypto.createHash('sha256') : null

  const processLine = (line: string) => {
    lineNumber += 1
    if (hasRanges) {
      for (const range of requestedRanges) {
        if (lineNumber >= range.startLine && lineNumber <= range.endLine) range.lines.push(line)
      }
    } else if (lineNumber >= singleStartLine && lineNumber <= singleEndLine) {
      selectedLines.push(line)
    }
  }

  const shouldStopAfterLine = () => Number.isFinite(maxRequestedEndLine) && lineNumber >= maxRequestedEndLine

  await new Promise<void>((resolve, reject) => {
    const stream = fs.createReadStream(filePath, { encoding: 'utf8' })
    let settled = false

    const finish = () => {
      if (settled) return
      settled = true
      resolve()
    }

    const fail = (error: unknown) => {
      if (settled) return
      settled = true
      reject(error)
    }

    stream.on('data', rawChunk => {
      const chunk = String(rawChunk)
      if (wholeFileHasher && !stoppedEarly) wholeFileHasher.update(chunk, 'utf8')

      const working = carry + chunk
      carry = ''

      let startIdx = 0
      for (let i = 0; i < working.length; i += 1) {
        if (working[i] !== '\n') continue

        let line = working.slice(startIdx, i)
        if (line.endsWith('\r')) {
          line = line.slice(0, -1)
          sawCRLF = true
        } else {
          sawLFOnly = true
        }

        processLine(line)
        endedWithLineBreak = true

        if (shouldStopAfterLine()) {
          stoppedEarly = true
          stream.destroy()
          return
        }

        startIdx = i + 1
      }

      carry = working.slice(startIdx)
      if (carry.length > 0) endedWithLineBreak = false
    })

    stream.on('end', () => {
      if (!stoppedEarly) {
        if (carry.length > 0) {
          processLine(carry)
          endedWithLineBreak = false
        } else if (endedWithLineBreak) {
          processLine('')
        } else if (lineNumber === 0) {
          processLine('')
        }
        reachedEOF = true
      }
      finish()
    })

    stream.on('close', () => {
      if (stoppedEarly) finish()
    })

    stream.on('error', fail)
  })

  const lineEndingStyle: '\n' | '\r\n' | 'mixed' = sawCRLF && sawLFOnly ? 'mixed' : sawCRLF ? '\r\n' : '\n'
  const joinLineEnding = lineEndingStyle === 'mixed' || lineEndingStyle === '\r\n' ? '\r\n' : '\n'
  const totalLines = reachedEOF ? lineNumber : undefined
  const fileHash = includeHash && reachedEOF && wholeFileHasher ? wholeFileHasher.digest('hex') : undefined

  if (hasRanges) {
    const selectedParts: string[] = []
    const rangesInfo: Array<{ startLine: number; endLine: number; lineCount: number }> = []

    for (const range of requestedRanges) {
      if (totalLines !== undefined && range.startLine > totalLines) {
        throw new Error(`Range startLine ${range.startLine} exceeds total lines ${totalLines} in file`)
      }

      const effectiveEndLine = totalLines !== undefined ? Math.min(totalLines, range.endLine) : range.endLine
      if (effectiveEndLine < range.startLine) {
        throw new Error(`Range endLine ${effectiveEndLine} cannot be less than startLine ${range.startLine}`)
      }

      selectedParts.push(range.lines.join(joinLineEnding))
      if (requestedRanges.length > 1) selectedParts.push('')
      rangesInfo.push({ startLine: range.startLine, endLine: effectiveEndLine, lineCount: range.lines.length })
    }

    if (selectedParts[selectedParts.length - 1] === '') selectedParts.pop()
    return { content: selectedParts.join(joinLineEnding), totalLines, ranges: rangesInfo, lineEndingStyle, fileHash }
  }

  if (totalLines !== undefined && singleStartLine > totalLines) {
    throw new Error(`startLine ${singleStartLine} exceeds total lines ${totalLines} in file`)
  }

  const effectiveEndLine =
    Number.isFinite(singleEndLine) && totalLines !== undefined
      ? Math.min(totalLines, singleEndLine)
      : Number.isFinite(singleEndLine)
        ? singleEndLine
        : totalLines !== undefined
          ? totalLines
          : lineNumber

  if (effectiveEndLine < singleStartLine) {
    throw new Error(`endLine ${effectiveEndLine} cannot be less than startLine ${singleStartLine}`)
  }

  return {
    content: selectedLines.join(joinLineEnding),
    startLine: singleStartLine,
    endLine: effectiveEndLine,
    totalLines,
    lineEndingStyle,
    fileHash,
  }
}

export async function readTextFile(inputPath: string, options: ReadFileOptions = {}): Promise<ReadFileResult> {
  const maxBytes = options.maxBytes && options.maxBytes > 0 ? options.maxBytes : 200 * 1024
  const includeHash = options.includeHash === true

  validateLineRangeValues(options)

  const resolved = await resolveRestrictedToolPath(inputPath, {
    cwd: cwdOrDefault(options.cwd),
    mode: 'file',
    restrictToCwd: restrictToCwd(options),
    workspaceCwd: options.workspaceCwd,
  })
  const abs = resolved.fsPath

  let stats: fs.Stats
  try {
    stats = await fs.promises.stat(abs)
  } catch {
    throw new Error(`File '${inputPath}' does not exist or is not accessible`)
  }

  if (!stats.isFile()) throw new Error(`'${inputPath}' is not a file`)

  const sizeBytes = stats.size
  const needsLineAccess =
    options.startLine !== undefined || options.endLine !== undefined || (options.ranges && options.ranges.length > 0)

  if (needsLineAccess) {
    const probeBuf = await readProbeBytes(abs, Math.min(sizeBytes, 4096))
    if (isLikelyBinary(probeBuf)) throw new Error('Binary file detected; reading binary is not supported by this tool')

    const lineSelection = await readLineSelectionFromFile(abs, options, includeHash)
    return {
      content: lineSelection.content,
      truncated: false,
      sizeBytes,
      contentHash: includeHash ? calculateHash(lineSelection.content) : undefined,
      fileHash: lineSelection.fileHash,
      metadata: {
        lineEnding: lineSelection.lineEndingStyle,
        hasBOM: hasBOM(probeBuf),
        encoding: 'utf8',
        lastModified: stats.mtime,
        inode: stats.ino,
      },
      startLine: lineSelection.startLine,
      endLine: lineSelection.endLine,
      totalLines: lineSelection.totalLines,
      ranges: lineSelection.ranges,
    }
  }

  const toRead = Math.min(sizeBytes, maxBytes)
  let buf: Buffer
  if (toRead === sizeBytes) {
    buf = await fs.promises.readFile(abs)
  } else {
    const fd = await fs.promises.open(abs, 'r')
    try {
      buf = Buffer.allocUnsafe(toRead)
      await fd.read(buf, 0, toRead, 0)
    } finally {
      await fd.close()
    }
  }

  if (isLikelyBinary(buf)) throw new Error('Binary file detected; reading binary is not supported by this tool')

  const content = buf.toString('utf8')
  const contentHash = includeHash ? calculateHash(content) : undefined

  return {
    content,
    truncated: sizeBytes > maxBytes,
    sizeBytes,
    contentHash,
    fileHash: contentHash,
    metadata: {
      lineEnding: detectLineEnding(content),
      hasBOM: hasBOM(buf),
      encoding: 'utf8',
      lastModified: stats.mtime,
      inode: stats.ino,
    },
  }
}

export async function readFileContinuation(
  inputPath: string,
  afterLine: number,
  numLines: number,
  options: Omit<ReadFileOptions, 'startLine' | 'endLine' | 'ranges'> & { cwd?: string } = {}
): Promise<ReadFileResult> {
  if (afterLine < 0) throw new Error('afterLine must be >= 0 (use 0 to read from beginning)')
  if (numLines < 1) throw new Error('numLines must be >= 1')

  return readTextFile(inputPath, { ...options, startLine: afterLine + 1, endLine: afterLine + numLines })
}

const readFileInputSchema = {
  type: 'object',
  properties: {
    path: { type: 'string', description: 'The file path to read.' },
    cwd: { type: 'string', description: 'Optional workspace directory used for path resolution and restriction.' },
    maxBytes: { type: 'number', description: 'Optional safety limit on bytes to read. Defaults to 200KB.' },
    startLine: { type: 'number', description: 'Optional 1-based start line, inclusive.' },
    endLine: { type: 'number', description: 'Optional 1-based end line, inclusive.' },
    ranges: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          startLine: { type: 'number' },
          endLine: { type: 'number' },
        },
        required: ['startLine', 'endLine'],
        additionalProperties: false,
      },
    },
    includeHash: { type: 'boolean', description: 'Calculate SHA256 content/file hashes.' },
  },
  required: ['path'],
  additionalProperties: false,
}

export const readFileTool = defineTool({
  name: 'readFile',
  description: 'Read the contents of a text/code/config file with optional line ranges, truncation, metadata, and hashes.',
  inputSchema: readFileInputSchema,
  permission: { mode: 'always' },
  async execute(args: ReadFileOptions & { path?: string }, context) {
    if (!args.path) return { ok: false, error: 'path is required' }
    try {
      return { ok: true, data: await readTextFile(args.path, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) }) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export const readFileContinuationTool = defineTool({
  name: 'readFileContinuation',
  description: 'Read the next chunk of a file after a specific line number.',
  inputSchema: {
    type: 'object',
    properties: {
      path: { type: 'string' },
      afterLine: { type: 'number' },
      numLines: { type: 'number' },
      cwd: { type: 'string' },
      maxBytes: { type: 'number' },
      includeHash: { type: 'boolean' },
    },
    required: ['path', 'afterLine', 'numLines'],
    additionalProperties: false,
  },
  permission: { mode: 'always' },
  async execute(args: { path?: string; afterLine?: number; numLines?: number } & Omit<ReadFileOptions, 'startLine' | 'endLine' | 'ranges'>, context) {
    if (!args.path) return { ok: false, error: 'path is required' }
    if (args.afterLine === undefined) return { ok: false, error: 'afterLine is required' }
    if (args.numLines === undefined) return { ok: false, error: 'numLines is required' }
    try {
      return { ok: true, data: await readFileContinuation(args.path, args.afterLine, args.numLines, { ...args, cwdBlockEnabled: cwdBlockEnabledFromContext(context) }) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})
