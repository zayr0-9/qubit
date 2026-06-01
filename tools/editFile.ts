import * as crypto from 'node:crypto'
import * as fs from 'node:fs'
import { defineTool } from '@hyper-labs/hyper-router'
import { resolveRestrictedToolPath } from '../utils/pathSafety.js'
import { cwdOrDefault } from '../utils/toolWorkspace.js'
import { readTextFile, type FileMetadata } from './readFile.js'

const FULL_FILE_READ_MAX_BYTES = Number.MAX_SAFE_INTEGER
const DEFAULT_LINE_HINT_WINDOW = 100
const PLAN_MODE_MESSAGE =
  'You are in planning mode. File modification is not allowed. Please describe your implementation plan instead. Do not try to edit the code or make changes. Do not use bash to skip this warning.'

export type EditOperation = 'replace' | 'replace_first' | 'append'
export type MatchStrategy = 'exact' | 'line_ending_normalized' | 'whitespace_normalized' | 'fuzzy'

export interface EditFileOptions {
  createBackup?: boolean
  encoding?: BufferEncoding
  enableFuzzyMatching?: boolean
  fuzzyThreshold?: number
  preserveIndentation?: boolean
  interpretEscapeSequences?: boolean
  interpretSearchEscapes?: boolean
  interpretReplacementEscapes?: boolean
  operationMode?: 'plan' | 'execute'
  validateContent?: boolean
  expectedHash?: string
  expectedMetadata?: FileMetadata
  cwd?: string
  approxStartLine?: number
  approxEndLine?: number
  lineHintWindow?: number
  skipLastModifiedValidation?: boolean
}

export interface FileValidationResult {
  valid: boolean
  reason?: string
  expectedHash?: string
  actualHash?: string
  expectedModified?: Date
  actualModified?: Date
}

export interface MatchResult {
  found: boolean
  startIndex: number
  endIndex: number
  matchedText: string
  strategy: MatchStrategy
  similarity?: number
}

export interface EditFileLineInfo {
  oldStartLine: number
  oldEndLine: number
  oldLineCount: number
  newStartLine: number
  newEndLine: number
  newLineCount: number
  scope: 'single' | 'first_of_many' | 'append'
}

export interface EditFileResult {
  success: boolean
  sizeBytes: number
  replacements: number
  message: string
  backup?: string
  matchStrategy?: MatchStrategy
  attemptedStrategies?: string[]
  validation?: FileValidationResult
  lineInfo?: EditFileLineInfo
}

export interface MultiEditItem {
  path: string
  operation: EditOperation
  searchPattern?: string
  replacement?: string
  content?: string
  approxStartLine?: number
  approxEndLine?: number
  expectedHash?: string
  expectedMetadata?: FileMetadata
}

export interface MultiEditOptions extends EditFileOptions {
  stopOnError?: boolean
}

export interface MultiEditItemResult extends EditFileResult {
  path: string
  operation?: string
  index: number
}

export interface MultiEditResult {
  success: boolean
  message: string
  results: MultiEditItemResult[]
  applied: number
  failed: number
  stoppedEarly: boolean
}

interface ResolvedEditTarget {
  fsPath: string
  displayPath: string
  cwd: string
}

function calculateHash(content: string): string {
  return crypto.createHash('sha256').update(content, 'utf8').digest('hex')
}

function estimateTextSizeBytes(content: string, encoding: BufferEncoding): number {
  return Buffer.byteLength(content, encoding)
}

function resolveEscapeHandling(options: EditFileOptions) {
  const hasLegacyFlag = typeof options.interpretEscapeSequences === 'boolean'
  const legacyValue = options.interpretEscapeSequences ?? true
  return {
    interpretSearchEscapes: options.interpretSearchEscapes ?? (hasLegacyFlag ? legacyValue : true),
    interpretReplacementEscapes: options.interpretReplacementEscapes ?? (hasLegacyFlag ? legacyValue : false),
  }
}

async function resolveEditTarget(filePath: string, cwd?: string): Promise<ResolvedEditTarget> {
  const workspaceCwd = cwdOrDefault(cwd)
  const resolved = await resolveRestrictedToolPath(filePath, { cwd: workspaceCwd, mode: 'file' })
  return { fsPath: resolved.fsPath, displayPath: resolved.displayPath, cwd: workspaceCwd }
}

async function readFullTextFileForEdit(filePath: string, cwd: string) {
  const fileData = await readTextFile(filePath, { cwd, maxBytes: FULL_FILE_READ_MAX_BYTES, includeHash: false })
  if (fileData.truncated) throw new Error(`Refusing to edit '${filePath}' because the file read was truncated.`)
  return fileData
}

function shouldValidateAgainstExpectations(options: EditFileOptions, validateContent: boolean): boolean {
  if (!validateContent) return false
  if (options.skipLastModifiedValidation) return options.expectedMetadata?.inode !== undefined
  return Boolean(options.expectedMetadata?.lastModified || options.expectedMetadata?.inode !== undefined)
}

async function validateFileContent(
  absolutePath: string,
  content: string,
  options: EditFileOptions
): Promise<FileValidationResult> {
  const expectedHash = options.expectedHash
  const actualHash = expectedHash ? calculateHash(content) : undefined
  const expectedModified = options.expectedMetadata?.lastModified
  const expectedInode = options.expectedMetadata?.inode

  if (!expectedModified && expectedInode === undefined) {
    return {
      valid: true,
      expectedHash,
      actualHash,
      reason: expectedHash && actualHash !== expectedHash ? 'Content hash mismatch ignored' : undefined,
    }
  }

  const stats = await fs.promises.stat(absolutePath)
  if (expectedInode !== undefined && stats.ino !== expectedInode) {
    return { valid: false, reason: 'File identity changed (inode mismatch) - file may have been replaced' }
  }

  if (expectedModified) {
    const expectedTime = new Date(expectedModified).getTime()
    const actualTime = stats.mtime.getTime()
    if (actualTime > expectedTime) {
      return {
        valid: false,
        reason: 'File has been modified since it was read',
        expectedModified,
        actualModified: stats.mtime,
      }
    }
  }

  return { valid: true, expectedHash, actualHash }
}

async function createBackupIfNeeded(
  absolutePath: string,
  originalContent: string,
  options: EditFileOptions,
  encoding: BufferEncoding
): Promise<string | undefined> {
  if (!options.createBackup) return undefined
  const backupPath = `${absolutePath}.backup.${Date.now()}`
  await fs.promises.writeFile(backupPath, originalContent, encoding)
  return backupPath
}

function countDisplayLines(text: string): number {
  if (text.length === 0) return 0
  let lines = 1
  for (let i = 0; i < text.length; i += 1) if (text.charCodeAt(i) === 10) lines += 1
  if (text.endsWith('\n')) lines -= 1
  return Math.max(lines, 0)
}

function lineNumberAtIndex(content: string, index: number): number {
  const clampedIndex = Math.min(Math.max(index, 0), content.length)
  let line = 1
  for (let i = 0; i < clampedIndex; i += 1) if (content.charCodeAt(i) === 10) line += 1
  return line
}

function endLineFromStart(startLine: number, lineCount: number): number {
  return lineCount > 0 ? startLine + lineCount - 1 : startLine
}

function buildLineInfo(
  originalContent: string,
  startIndex: number,
  oldText: string,
  newText: string,
  scope: EditFileLineInfo['scope']
): EditFileLineInfo {
  const oldStartLine = lineNumberAtIndex(originalContent, startIndex)
  const oldLineCount = countDisplayLines(oldText)
  const newLineCount = countDisplayLines(newText)
  return {
    oldStartLine,
    oldEndLine: endLineFromStart(oldStartLine, oldLineCount),
    oldLineCount,
    newStartLine: oldStartLine,
    newEndLine: endLineFromStart(oldStartLine, newLineCount),
    newLineCount,
    scope,
  }
}

function escapeRegExp(string: string): string {
  return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function interpretEscapeSequences(str: string, enable: boolean = true): string {
  if (!enable) return str
  const BACKSLASH_PLACEHOLDER = '\u0000LITERAL_BACKSLASH\u0000'
  return str
    .replace(/\\\\/g, BACKSLASH_PLACEHOLDER)
    .replace(/\\n/g, '\n')
    .replace(/\\r/g, '\r')
    .replace(/\\t/g, '\t')
    .replace(/\\'/g, "'")
    .replace(/\\"/g, '"')
    .replace(new RegExp(BACKSLASH_PLACEHOLDER, 'g'), '\\\\')
}

function normalizeLineEndings(str: string): string {
  return str.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

function normalizeWhitespaceLine(line: string): string {
  return line.trim().replace(/\s+/g, ' ')
}

function normalizeWhitespace(str: string): string {
  return str.split('\n').map(normalizeWhitespaceLine).join('\n')
}

function mapNormalizedIndexToOriginal(original: string, normalizedIndex: number): number {
  let originalIndex = 0
  let normalizedCount = 0
  while (normalizedCount < normalizedIndex && originalIndex < original.length) {
    if (original[originalIndex] === '\r' && original[originalIndex + 1] === '\n') {
      originalIndex += 2
      normalizedCount += 1
    } else {
      originalIndex += 1
      normalizedCount += 1
    }
  }
  return originalIndex
}

function findOriginalTextForNormalizedMatch(
  content: string,
  pattern: string
): { found: boolean; startIndex: number; endIndex: number; matchedText: string } {
  const normalizedContent = normalizeLineEndings(content)
  const normalizedPattern = normalizeLineEndings(pattern)
  const patternLines = normalizedPattern.split('\n').map(normalizeWhitespaceLine)
  const contentLines = normalizedContent.split('\n')
  const contentLineStarts: number[] = []
  let offset = 0

  for (let i = 0; i < contentLines.length; i += 1) {
    contentLineStarts.push(offset)
    offset += contentLines[i].length + (i < contentLines.length - 1 ? 1 : 0)
  }

  for (let i = 0; i <= contentLines.length - patternLines.length; i += 1) {
    let matches = true
    for (let j = 0; j < patternLines.length; j += 1) {
      if (normalizeWhitespaceLine(contentLines[i + j]) !== patternLines[j]) {
        matches = false
        break
      }
    }

    if (matches) {
      const normalizedStart = contentLineStarts[i]
      const lastLineIndex = i + patternLines.length - 1
      const normalizedEnd = contentLineStarts[lastLineIndex] + contentLines[lastLineIndex].length
      const startIndex = mapNormalizedIndexToOriginal(content, normalizedStart)
      const endIndex = mapNormalizedIndexToOriginal(content, normalizedEnd)
      return { found: true, startIndex, endIndex, matchedText: content.substring(startIndex, endIndex) }
    }
  }

  return { found: false, startIndex: -1, endIndex: -1, matchedText: '' }
}

function levenshteinDistance(a: string, b: string): number {
  const matrix: number[][] = []
  for (let i = 0; i <= b.length; i += 1) matrix[i] = [i]
  for (let j = 0; j <= a.length; j += 1) matrix[0][j] = j

  for (let i = 1; i <= b.length; i += 1) {
    for (let j = 1; j <= a.length; j += 1) {
      matrix[i][j] = b.charAt(i - 1) === a.charAt(j - 1)
        ? matrix[i - 1][j - 1]
        : Math.min(matrix[i - 1][j - 1] + 1, matrix[i][j - 1] + 1, matrix[i - 1][j] + 1)
    }
  }

  return matrix[b.length][a.length]
}

function calculateSimilarity(a: string, b: string): number {
  if (a === b) return 1
  if (a.length === 0 || b.length === 0) return 0
  return 1 - levenshteinDistance(a, b) / Math.max(a.length, b.length)
}

function findFuzzyMatch(
  content: string,
  pattern: string,
  threshold: number
): { found: boolean; startIndex: number; endIndex: number; similarity: number; matchedText: string } {
  const normalizedPattern = normalizeWhitespace(pattern)
  const patternLines = normalizedPattern.split('\n').length
  const contentLines = content.split('\n')
  let best = { found: false, startIndex: -1, endIndex: -1, similarity: 0, matchedText: '' }

  for (let i = 0; i <= contentLines.length - patternLines; i += 1) {
    const candidateText = contentLines.slice(i, i + patternLines).join('\n')
    const similarity = calculateSimilarity(normalizedPattern, normalizeWhitespace(candidateText))
    if (similarity > best.similarity && similarity >= threshold) {
      const linesBefore = contentLines.slice(0, i).join('\n')
      const startIndex = linesBefore.length + (i > 0 ? 1 : 0)
      best = { found: true, startIndex, endIndex: startIndex + candidateText.length, similarity, matchedText: candidateText }
    }
  }

  return best
}

function findMatchWithStrategies(
  content: string,
  pattern: string,
  enableFuzzy: boolean,
  fuzzyThreshold: number
): MatchResult & { attemptedStrategies: string[] } {
  const attemptedStrategies: string[] = ['exact']
  const exactIndex = content.indexOf(pattern)
  if (exactIndex !== -1) {
    return { found: true, startIndex: exactIndex, endIndex: exactIndex + pattern.length, matchedText: pattern, strategy: 'exact', attemptedStrategies }
  }

  attemptedStrategies.push('line_ending_normalized')
  const normalizedContent = normalizeLineEndings(content)
  const normalizedPattern = normalizeLineEndings(pattern)
  const lineEndingIndex = normalizedContent.indexOf(normalizedPattern)
  if (lineEndingIndex !== -1) {
    const startIndex = mapNormalizedIndexToOriginal(content, lineEndingIndex)
    const endIndex = mapNormalizedIndexToOriginal(content, lineEndingIndex + normalizedPattern.length)
    return { found: true, startIndex, endIndex, matchedText: content.substring(startIndex, endIndex), strategy: 'line_ending_normalized', attemptedStrategies }
  }

  attemptedStrategies.push('whitespace_normalized')
  if (normalizeWhitespace(normalizedContent).includes(normalizeWhitespace(normalizedPattern))) {
    const match = findOriginalTextForNormalizedMatch(content, pattern)
    if (match.found) return { ...match, strategy: 'whitespace_normalized', attemptedStrategies }
  }

  if (enableFuzzy) {
    attemptedStrategies.push('fuzzy')
    const fuzzy = findFuzzyMatch(content, pattern, fuzzyThreshold)
    if (fuzzy.found) return { ...fuzzy, strategy: 'fuzzy', attemptedStrategies }
  }

  return { found: false, startIndex: -1, endIndex: -1, matchedText: '', strategy: 'exact', attemptedStrategies }
}

function captureIndentation(text: string): string[] {
  return text.split('\n').map(line => line.match(/^(\s*)/)?.[1] || '')
}

function applyIndentation(replacement: string, originalIndentation: string[]): string {
  const replacementLines = replacement.split('\n')
  const baseIndent = originalIndentation[0] || ''
  const replacementBaseIndent = captureIndentation(replacement)[0] || ''

  return replacementLines
    .map(line => {
      const lineIndent = line.match(/^(\s*)/)?.[1] || ''
      const trimmedLine = line.trimStart()
      if (trimmedLine === '') return ''
      const relativeIndent = lineIndent.length <= replacementBaseIndent.length ? '' : lineIndent.slice(replacementBaseIndent.length)
      return baseIndent + relativeIndent + trimmedLine
    })
    .join('\n')
}

function coercePositiveInteger(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) return null
  const rounded = Math.round(value)
  return rounded < 1 ? null : rounded
}

function getLineStartIndex(content: string, lineNumber: number): number {
  if (lineNumber <= 1) return 0
  let currentLine = 1
  for (let i = 0; i < content.length; i += 1) {
    if (content.charCodeAt(i) === 10) {
      currentLine += 1
      if (currentLine === lineNumber) return i + 1
    }
  }
  return content.length
}

function resolveLineHintBounds(content: string, options: EditFileOptions): { startIndex: number; endIndex: number; label: string } | null {
  const normalizedStart = coercePositiveInteger(options.approxStartLine)
  const normalizedEnd = coercePositiveInteger(options.approxEndLine)
  if (!normalizedStart && !normalizedEnd) return null

  const anchorStart = normalizedStart ?? normalizedEnd!
  const anchorEnd = normalizedEnd ?? normalizedStart!
  const windowSize = Math.max(0, coercePositiveInteger(options.lineHintWindow) ?? DEFAULT_LINE_HINT_WINDOW)
  const totalLines = Math.max(1, lineNumberAtIndex(content, content.length))
  const startLine = Math.max(1, Math.min(anchorStart, anchorEnd) - windowSize)
  const endLine = Math.min(totalLines, Math.max(anchorStart, anchorEnd) + windowSize)
  return {
    startIndex: getLineStartIndex(content, startLine),
    endIndex: endLine >= totalLines ? content.length : getLineStartIndex(content, endLine + 1),
    label: `line_hint_window(${startLine}-${endLine})`,
  }
}

function findMatchWithLineHintFallback(
  content: string,
  pattern: string,
  options: EditFileOptions,
  enableFuzzy: boolean,
  fuzzyThreshold: number
): MatchResult & { attemptedStrategies: string[] } {
  const bounds = resolveLineHintBounds(content, options)
  if (!bounds) return findMatchWithStrategies(content, pattern, enableFuzzy, fuzzyThreshold)

  const scoped = content.slice(bounds.startIndex, bounds.endIndex)
  const scopedResult = findMatchWithStrategies(scoped, pattern, enableFuzzy, fuzzyThreshold)
  if (scopedResult.found) {
    return {
      ...scopedResult,
      startIndex: scopedResult.startIndex + bounds.startIndex,
      endIndex: scopedResult.endIndex + bounds.startIndex,
      attemptedStrategies: scopedResult.attemptedStrategies.map(strategy => `${bounds.label}:${strategy}`),
    }
  }

  const fallback = findMatchWithStrategies(content, pattern, enableFuzzy, fuzzyThreshold)
  return {
    ...fallback,
    attemptedStrategies: [
      ...scopedResult.attemptedStrategies.map(strategy => `${bounds.label}:${strategy}`),
      `${bounds.label}:fallback_to_full_file`,
      ...fallback.attemptedStrategies.map(strategy => `full_file:${strategy}`),
    ],
  }
}

export async function editFileSearchReplace(
  filePath: string,
  searchPattern: string,
  replacement: string,
  options: EditFileOptions = {},
  firstOnly = false
): Promise<EditFileResult> {
  const {
    encoding = 'utf8',
    enableFuzzyMatching = true,
    fuzzyThreshold = 0.8,
    preserveIndentation = true,
    validateContent = true,
  } = options
  const { interpretSearchEscapes, interpretReplacementEscapes } = resolveEscapeHandling(options)

  try {
    const target = await resolveEditTarget(filePath, options.cwd)
    const fileData = await readFullTextFileForEdit(filePath, target.cwd)
    const originalContent = fileData.content

    let validation: FileValidationResult | undefined
    if (shouldValidateAgainstExpectations(options, validateContent)) {
      validation = await validateFileContent(target.fsPath, originalContent, options)
      if (!validation.valid) {
        return { success: false, sizeBytes: fileData.sizeBytes, replacements: 0, message: `Validation failed: ${validation.reason}`, validation }
      }
    }

    const processedSearchPattern = interpretEscapeSequences(searchPattern, interpretSearchEscapes)
    const matchResult = firstOnly
      ? findMatchWithLineHintFallback(originalContent, processedSearchPattern, options, enableFuzzyMatching, fuzzyThreshold)
      : findMatchWithStrategies(originalContent, processedSearchPattern, enableFuzzyMatching, fuzzyThreshold)

    if (!matchResult.found) {
      return {
        success: false,
        sizeBytes: fileData.sizeBytes,
        replacements: 0,
        message: `Search pattern not found in file. Attempted strategies: ${matchResult.attemptedStrategies.join(', ')}`,
        attemptedStrategies: matchResult.attemptedStrategies,
      }
    }

    let finalReplacement = replacement
    if (preserveIndentation && matchResult.strategy !== 'exact') {
      finalReplacement = applyIndentation(replacement, captureIndentation(matchResult.matchedText))
    }
    finalReplacement = interpretEscapeSequences(finalReplacement, interpretReplacementEscapes)

    let newContent: string
    let replacements = 0
    let lineInfoScope: EditFileLineInfo['scope'] = 'single'

    if (firstOnly || matchResult.strategy !== 'exact') {
      replacements = finalReplacement === matchResult.matchedText ? 0 : 1
      newContent = replacements > 0
        ? originalContent.slice(0, matchResult.startIndex) + finalReplacement + originalContent.slice(matchResult.endIndex)
        : originalContent
    } else {
      const exactRegex = new RegExp(escapeRegExp(processedSearchPattern), 'g')
      const matches = processedSearchPattern ? originalContent.match(exactRegex) : null
      replacements = matches && processedSearchPattern !== finalReplacement ? matches.length : 0
      if (replacements > 1) lineInfoScope = 'first_of_many'
      newContent = replacements > 0 ? originalContent.replace(exactRegex, () => finalReplacement) : originalContent
    }

    const lineInfo = buildLineInfo(originalContent, matchResult.startIndex, matchResult.matchedText, finalReplacement, lineInfoScope)
    const backup = replacements > 0 ? await createBackupIfNeeded(target.fsPath, originalContent, options, encoding) : undefined
    if (replacements > 0) await fs.promises.writeFile(target.fsPath, newContent, encoding)

    const strategyMessage = matchResult.strategy !== 'exact' ? ` (matched using ${matchResult.strategy} strategy)` : ''
    return {
      success: true,
      sizeBytes: estimateTextSizeBytes(newContent, encoding),
      replacements,
      message: replacements > 0
        ? firstOnly
          ? `Successfully replaced first occurrence${strategyMessage} in ${target.displayPath}`
          : `Successfully replaced ${replacements} occurrence(s)${strategyMessage} in ${target.displayPath}`
        : `No changes needed in ${target.displayPath}`,
      backup,
      matchStrategy: matchResult.strategy,
      attemptedStrategies: matchResult.attemptedStrategies,
      validation,
      lineInfo,
    }
  } catch (error) {
    return { success: false, sizeBytes: 0, replacements: 0, message: `Error editing file: ${error instanceof Error ? error.message : String(error)}` }
  }
}

export async function appendToFile(filePath: string, content: string, options: EditFileOptions = {}): Promise<EditFileResult> {
  const { encoding = 'utf8' } = options

  try {
    const target = await resolveEditTarget(filePath, options.cwd)
    const existingStats = await fs.promises.stat(target.fsPath).catch((error: NodeJS.ErrnoException) => {
      if (error.code === 'ENOENT') return null
      throw error
    })

    if (existingStats && !existingStats.isFile()) throw new Error(`'${target.displayPath}' is not a file`)
    const existingContent = existingStats ? await fs.promises.readFile(target.fsPath, encoding) : ''
    const backup = existingStats ? await createBackupIfNeeded(target.fsPath, existingContent, options, encoding) : undefined

    await fs.promises.appendFile(target.fsPath, content, encoding)
    const newContent = existingContent + content

    return {
      success: true,
      sizeBytes: estimateTextSizeBytes(newContent, encoding),
      replacements: 1,
      message: `Successfully appended content to ${target.displayPath}`,
      backup,
      lineInfo: buildLineInfo(existingContent, existingContent.length, '', content, 'append'),
    }
  } catch (error) {
    return { success: false, sizeBytes: 0, replacements: 0, message: `Error appending to file: ${error instanceof Error ? error.message : String(error)}` }
  }
}

export async function editFile(
  filePath: string,
  operation: EditOperation,
  options: EditFileOptions & { searchPattern?: string; replacement?: string; content?: string } = {}
): Promise<EditFileResult> {
  if (options.operationMode === 'plan') return { success: false, sizeBytes: 0, replacements: 0, message: PLAN_MODE_MESSAGE }

  if (operation === 'replace') {
    if (!options.searchPattern || options.replacement === undefined) {
      return { success: false, sizeBytes: 0, replacements: 0, message: 'searchPattern and replacement are required for replace operation' }
    }
    return editFileSearchReplace(filePath, options.searchPattern, options.replacement, options, false)
  }

  if (operation === 'replace_first') {
    if (!options.searchPattern || options.replacement === undefined) {
      return { success: false, sizeBytes: 0, replacements: 0, message: 'searchPattern and replacement are required for replace_first operation' }
    }
    return editFileSearchReplace(filePath, options.searchPattern, options.replacement, options, true)
  }

  if (operation === 'append') {
    if (options.content === undefined) return { success: false, sizeBytes: 0, replacements: 0, message: 'content is required for append operation' }
    return appendToFile(filePath, options.content, options)
  }

  return { success: false, sizeBytes: 0, replacements: 0, message: `Unknown operation: ${operation}` }
}

export async function multiEdit(edits: MultiEditItem[], options: MultiEditOptions = {}): Promise<MultiEditResult> {
  if (options.operationMode === 'plan') {
    return { success: false, message: PLAN_MODE_MESSAGE, results: [], applied: 0, failed: Array.isArray(edits) ? edits.length : 0, stoppedEarly: false }
  }

  if (!Array.isArray(edits) || edits.length === 0) {
    return { success: false, message: 'edits must be a non-empty array', results: [], applied: 0, failed: 0, stoppedEarly: false }
  }

  const stopOnError = options.stopOnError ?? true
  const results: MultiEditItemResult[] = []

  for (const [index, item] of edits.entries()) {
    const itemPath = typeof item?.path === 'string' ? item.path : ''
    const itemOperation = typeof item?.operation === 'string' ? item.operation : undefined
    const result = itemPath && itemOperation
      ? await editFile(itemPath, itemOperation as EditOperation, {
          ...options,
          searchPattern: item.searchPattern,
          replacement: item.replacement,
          content: item.content,
          approxStartLine: item.approxStartLine,
          approxEndLine: item.approxEndLine,
          expectedHash: item.expectedHash,
          expectedMetadata: item.expectedMetadata,
          skipLastModifiedValidation: true,
        })
      : { success: false, sizeBytes: 0, replacements: 0, message: itemPath ? 'operation is required for each multiEdit item' : 'path is required for each multiEdit item' }

    const itemResult: MultiEditItemResult = { ...result, path: itemPath, operation: itemOperation, index }
    results.push(itemResult)

    if (!itemResult.success && stopOnError) {
      const applied = results.filter(entry => entry.success).length
      const failed = results.length - applied
      return {
        success: false,
        message: `Multi-edit stopped after failure at item ${index + 1}${itemPath ? ` (${itemPath})` : ''}: ${itemResult.message}`,
        results,
        applied,
        failed,
        stoppedEarly: index < edits.length - 1,
      }
    }
  }

  const applied = results.filter(entry => entry.success).length
  const failed = results.length - applied
  return {
    success: failed === 0,
    message: failed === 0 ? `Successfully processed ${results.length} multiEdit item(s).` : `Processed ${results.length} multiEdit item(s) with ${failed} failure(s).`,
    results,
    applied,
    failed,
    stoppedEarly: false,
  }
}

const editOperationSchema = {
  type: 'string',
  enum: ['replace', 'replace_first', 'append'],
}

const editFileOptionProperties = {
  cwd: { type: 'string' },
  createBackup: { type: 'boolean' },
  encoding: { type: 'string' },
  enableFuzzyMatching: { type: 'boolean' },
  fuzzyThreshold: { type: 'number' },
  preserveIndentation: { type: 'boolean' },
  interpretEscapeSequences: { type: 'boolean' },
  interpretSearchEscapes: { type: 'boolean' },
  interpretReplacementEscapes: { type: 'boolean' },
  operationMode: { type: 'string', enum: ['plan', 'execute'] },
  validateContent: { type: 'boolean' },
  expectedHash: { type: 'string' },
  expectedMetadata: { type: 'object' },
  approxStartLine: { type: 'number' },
  approxEndLine: { type: 'number' },
  lineHintWindow: { type: 'number' },
}

export const editFileTool = defineTool({
  name: 'editFile',
  description: 'Edit a file using replace, replace_first, or append operations with layered matching and optional validation.',
  inputSchema: {
    type: 'object',
    properties: {
      path: { type: 'string' },
      operation: editOperationSchema,
      searchPattern: { type: 'string' },
      replacement: { type: 'string' },
      content: { type: 'string' },
      ...editFileOptionProperties,
    },
    required: ['path', 'operation'],
    additionalProperties: false,
  },
  permission: { mode: 'ask' },
  async execute(args: EditFileOptions & { path?: string; operation?: EditOperation; searchPattern?: string; replacement?: string; content?: string }) {
    if (!args.path) return { ok: false, error: 'path is required' }
    if (!args.operation) return { ok: false, error: 'operation is required' }
    try {
      return { ok: true, data: await editFile(args.path, args.operation, args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export const multiEditTool = defineTool({
  name: 'multiEdit',
  description: 'Apply multiple editFile-style operations sequentially across one or more files.',
  inputSchema: {
    type: 'object',
    properties: {
      edits: {
        type: 'array',
        items: {
          type: 'object',
          properties: {
            path: { type: 'string' },
            operation: editOperationSchema,
            searchPattern: { type: 'string' },
            replacement: { type: 'string' },
            content: { type: 'string' },
            approxStartLine: { type: 'number' },
            approxEndLine: { type: 'number' },
            expectedHash: { type: 'string' },
            expectedMetadata: { type: 'object' },
          },
          required: ['path', 'operation'],
          additionalProperties: false,
        },
      },
      stopOnError: { type: 'boolean' },
      ...editFileOptionProperties,
    },
    required: ['edits'],
    additionalProperties: false,
  },
  permission: { mode: 'ask' },
  async execute(args: MultiEditOptions & { edits?: MultiEditItem[] }) {
    if (!args.edits) return { ok: false, error: 'edits is required' }
    try {
      return { ok: true, data: await multiEdit(args.edits, args) }
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error.message : String(error) }
    }
  },
})

export default editFile
