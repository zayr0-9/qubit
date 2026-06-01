import * as assert from 'node:assert/strict'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import {
  appendToFile,
  editFile,
  editFileSearchReplace,
  type EditFileResult,
} from './editFile.js'
import { cleanupTempDir, createTempDir, fileExists, readFileInDir, writeFileInDir } from './testHelpers.js'

describe('editFileSearchReplace', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-edit-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('replaces exact match', async () => {
    await writeFileInDir(tmpDir, 'exact.txt', 'hello old world')
    const result = await editFileSearchReplace('exact.txt', 'old', 'new', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.equal(result.replacements, 1)
    assert.equal(result.matchStrategy, 'exact')
    const content = await readFileInDir(tmpDir, 'exact.txt')
    assert.equal(content, 'hello new world')
  })

  it('replaces first occurrence only with firstOnly=true', async () => {
    await writeFileInDir(tmpDir, 'first-only.txt', 'foo bar foo baz foo')
    const result = await editFileSearchReplace('first-only.txt', 'foo', 'qux', { cwd: tmpDir }, true)
    assert.equal(result.success, true)
    assert.equal(result.replacements, 1)
    const content = await readFileInDir(tmpDir, 'first-only.txt')
    assert.equal(content, 'qux bar foo baz foo')
  })

  it('replaces all occurrences with firstOnly=false', async () => {
    await writeFileInDir(tmpDir, 'all-replace.txt', 'aaa bbb aaa ccc aaa')
    const result = await editFileSearchReplace('all-replace.txt', 'aaa', 'zzz', { cwd: tmpDir }, false)
    assert.equal(result.success, true)
    assert.equal(result.replacements, 3)
    const content = await readFileInDir(tmpDir, 'all-replace.txt')
    assert.equal(content, 'zzz bbb zzz ccc zzz')
  })

  it('matches with line-ending normalization (CRLF file, LF pattern)', async () => {
    const filePath = path.join(tmpDir, 'crlf.txt')
    await fs.promises.writeFile(filePath, 'line1\r\nline2\r\nline3', 'utf8')
    const result = await editFileSearchReplace('crlf.txt', 'line2', 'replaced', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.ok(result.matchStrategy === 'line_ending_normalized' || result.matchStrategy === 'exact')
  })

  it('matches with whitespace normalization', async () => {
    await writeFileInDir(tmpDir, 'ws-norm.txt', 'function   hello(  )  {')
    const result = await editFileSearchReplace('ws-norm.txt', 'function hello( ) {', 'function greet() {', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.ok(result.matchStrategy === 'whitespace_normalized' || result.matchStrategy === 'exact')
  })

  it('finds fuzzy match when enabled', async () => {
    await writeFileInDir(tmpDir, 'fuzzy.txt', 'function computeTotal(items) {')
    const result = await editFileSearchReplace('fuzzy.txt', 'function computeTotal(item) {', 'function calcTotal(items) {', {
      cwd: tmpDir,
      enableFuzzyMatching: true,
      fuzzyThreshold: 0.6,
    })
    assert.equal(result.success, true)
    assert.equal(result.matchStrategy, 'fuzzy')
  })

  it('returns failure when pattern not found', async () => {
    await writeFileInDir(tmpDir, 'not-found.txt', 'hello world')
    const result = await editFileSearchReplace('not-found.txt', 'nonexistent', 'replacement', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.equal(result.replacements, 0)
    assert.ok(result.attemptedStrategies && result.attemptedStrategies.length > 0)
  })

  it('plan mode is enforced through editFile dispatch (not editFileSearchReplace directly)', async () => {
    // editFileSearchReplace does not check operationMode; editFile() does.
    // Verify editFileSearchReplace ignores plan mode (it's the raw search-replace):
    await writeFileInDir(tmpDir, 'plan-edit.txt', 'original')
    const result = await editFileSearchReplace('plan-edit.txt', 'original', 'modified', {
      cwd: tmpDir,
      operationMode: 'plan',
    })
    // editFileSearchReplace does NOT enforce plan mode — it proceeds
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'plan-edit.txt')
    assert.equal(content, 'modified')
  })

  it('creates backup when createBackup=true', async () => {
    await writeFileInDir(tmpDir, 'backup.txt', 'original content')
    const result = await editFileSearchReplace('backup.txt', 'original', 'modified', {
      cwd: tmpDir,
      createBackup: true,
    })
    assert.equal(result.success, true)
    assert.ok(result.backup, 'backup path should be set')
    // Backup file should exist
    const backupExists = await fileExists(result.backup!)
    assert.equal(backupExists, true)
    // Backup should contain original content
    const backupContent = await fs.promises.readFile(result.backup!, 'utf8')
    assert.equal(backupContent, 'original content')
    // Main file should have new content
    const mainContent = await readFileInDir(tmpDir, 'backup.txt')
    assert.ok(mainContent.includes('modified'))
  })

  it('replaces multiline content', async () => {
    const original = 'line one\nline two\nline three'
    await writeFileInDir(tmpDir, 'multiline.txt', original)
    const result = await editFileSearchReplace(
      'multiline.txt',
      'line one\nline two',
      'replaced one\nreplaced two',
      { cwd: tmpDir }
    )
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'multiline.txt')
    assert.equal(content, 'replaced one\nreplaced two\nline three')
  })

  it('replaces content removing all text (replace with empty)', async () => {
    await writeFileInDir(tmpDir, 'to-empty.txt', 'all content here')
    const result = await editFileSearchReplace('to-empty.txt', 'all content here', '', { cwd: tmpDir })
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'to-empty.txt')
    assert.equal(content, '')
  })

  it('handles approx line hints for narrowing search scope', async () => {
    const lines = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`)
    await writeFileInDir(tmpDir, 'large.txt', lines.join('\n'))
    const result = await editFileSearchReplace('large.txt', 'line 25', 'REPLACED', {
      cwd: tmpDir,
      approxStartLine: 23,
      approxEndLine: 27,
    })
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'large.txt')
    assert.ok(content.includes('REPLACED'))
  })

  it('no-op when search pattern equals replacement', async () => {
    await writeFileInDir(tmpDir, 'noop.txt', 'same text')
    const result = await editFileSearchReplace('noop.txt', 'same text', 'same text', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.equal(result.replacements, 0)
  })

  it('file not found returns error', async () => {
    const result = await editFileSearchReplace('nonexistent.txt', 'old', 'new', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.toLowerCase().includes('error') || result.message.toLowerCase().includes('not found'))
  })
})

describe('appendToFile', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-append-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('appends content to existing file', async () => {
    await writeFileInDir(tmpDir, 'append.txt', 'first')
    const result = await appendToFile('append.txt', '\nsecond', { cwd: tmpDir })
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'append.txt')
    assert.equal(content, 'first\nsecond')
  })

  it('appends to new file (creates if missing)', async () => {
    const result = await appendToFile('new-append.txt', 'hello', { cwd: tmpDir })
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'new-append.txt')
    assert.equal(content, 'hello')
  })
})

describe('editFile (dispatch)', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-dispatch-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('replace operation dispatches to editFileSearchReplace', async () => {
    await writeFileInDir(tmpDir, 'dispatch-replace.txt', 'hello old world')
    const result = await editFile('dispatch-replace.txt', 'replace', {
      searchPattern: 'old',
      replacement: 'new',
      cwd: tmpDir,
    })
    assert.equal(result.success, true)
    assert.equal(result.replacements, 1)
    const content = await readFileInDir(tmpDir, 'dispatch-replace.txt')
    assert.equal(content, 'hello new world')
  })

  it('replace_first operation dispatches correctly', async () => {
    await writeFileInDir(tmpDir, 'dispatch-first.txt', 'foo foo foo')
    const result = await editFile('dispatch-first.txt', 'replace_first', {
      searchPattern: 'foo',
      replacement: 'bar',
      cwd: tmpDir,
    })
    assert.equal(result.success, true)
    assert.equal(result.replacements, 1)
    const content = await readFileInDir(tmpDir, 'dispatch-first.txt')
    assert.equal(content, 'bar foo foo')
  })

  it('append operation dispatches to appendToFile', async () => {
    await writeFileInDir(tmpDir, 'dispatch-append.txt', 'first')
    const result = await editFile('dispatch-append.txt', 'append', {
      content: '\nsecond',
      cwd: tmpDir,
    })
    assert.equal(result.success, true)
    const content = await readFileInDir(tmpDir, 'dispatch-append.txt')
    assert.equal(content, 'first\nsecond')
  })

  it('plan mode blocks all operations', async () => {
    await writeFileInDir(tmpDir, 'dispatch-plan.txt', 'original')
    const result = await editFile('dispatch-plan.txt', 'replace', {
      searchPattern: 'original',
      replacement: 'modified',
      cwd: tmpDir,
      operationMode: 'plan',
    })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('planning mode'))
  })

  it('replace without searchPattern returns error', async () => {
    const result = await editFile('some.txt', 'replace', { replacement: 'x', cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('searchPattern'))
  })

  it('replace without replacement returns error', async () => {
    const result = await editFile('some.txt', 'replace', { searchPattern: 'x', cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('replacement'))
  })

  it('append without content returns error', async () => {
    const result = await editFile('some.txt', 'append', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('content'))
  })

  it('unknown operation returns error', async () => {
    const result = await editFile('some.txt', 'unknown_op' as 'replace', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('Unknown operation'))
  })
})
