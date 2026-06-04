import * as assert from 'node:assert/strict'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { readTextFile, type ReadFileResult } from './readFile.js'
import { cleanupTempDir, createTempDir, writeFileInDir } from './testHelpers.js'

describe('readTextFile', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-read-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('reads full file content', async () => {
    await writeFileInDir(tmpDir, 'full.txt', 'hello world')
    const result = await readTextFile('full.txt', { cwd: tmpDir })
    assert.equal(result.content, 'hello world')
    assert.equal(result.truncated, false)
    assert.ok(result.sizeBytes > 0)
  })

  it('reads with startLine and endLine', async () => {
    const content = 'line1\nline2\nline3\nline4\nline5'
    await writeFileInDir(tmpDir, 'lines.txt', content)
    const result = await readTextFile('lines.txt', { cwd: tmpDir, startLine: 2, endLine: 4 })
    assert.equal(result.content, 'line2\nline3\nline4')
    assert.equal(result.startLine, 2)
    assert.equal(result.endLine, 4)
    // totalLines is undefined when the stream stops early (did not reach EOF)
    assert.equal(result.totalLines, undefined)
  })

  it('reads with ranges', async () => {
    const content = 'line1\nline2\nline3\nline4\nline5'
    await writeFileInDir(tmpDir, 'ranges.txt', content)
    const result = await readTextFile('ranges.txt', {
      cwd: tmpDir,
      ranges: [{ startLine: 1, endLine: 2 }, { startLine: 4, endLine: 5 }],
    })
    assert.ok(result.ranges)
    assert.equal(result.ranges!.length, 2)
    assert.equal(result.ranges![0].startLine, 1)
    assert.equal(result.ranges![0].endLine, 2)
    assert.equal(result.ranges![1].startLine, 4)
    assert.equal(result.ranges![1].endLine, 5)
    // Content should include both ranges
    assert.ok(result.content.includes('line1'))
    assert.ok(result.content.includes('line5'))
  })

  it('returns content hash when includeHash=true', async () => {
    await writeFileInDir(tmpDir, 'hash.txt', 'hashable content')
    const result = await readTextFile('hash.txt', { cwd: tmpDir, includeHash: true })
    assert.ok(result.contentHash, 'contentHash should be present')
    assert.ok(result.fileHash, 'fileHash should be present')
    assert.equal(result.contentHash!.length, 64) // SHA256 hex length
  })

  it('truncates content when maxBytes is smaller than file', async () => {
    const bigContent = 'A'.repeat(1000)
    await writeFileInDir(tmpDir, 'big.txt', bigContent)
    const result = await readTextFile('big.txt', { cwd: tmpDir, maxBytes: 100 })
    assert.equal(result.truncated, true)
    assert.ok(result.sizeBytes > 100)
  })

  it('throws for file not found', async () => {
    await assert.rejects(
      () => readTextFile('nonexistent.txt', { cwd: tmpDir }),
      /does not exist|not accessible/
    )
  })

  it('detects binary files and rejects them', async () => {
    const filePath = path.join(tmpDir, 'binary.bin')
    // Create a file with null bytes
    const buf = Buffer.alloc(100, 0)
    buf[0] = 0x00
    await fs.promises.writeFile(filePath, buf)
    await assert.rejects(
      () => readTextFile('binary.bin', { cwd: tmpDir }),
      /binary|Binary/
    )
  })

  it('returns metadata with line ending info', async () => {
    await writeFileInDir(tmpDir, 'meta.txt', 'line1\nline2\n')
    const result = await readTextFile('meta.txt', { cwd: tmpDir })
    assert.ok(result.metadata)
    assert.ok(['\n', '\r\n', 'mixed'].includes(result.metadata.lineEnding))
    assert.equal(result.metadata.encoding, 'utf8')
    assert.ok(result.metadata.lastModified instanceof Date)
  })

  it('reads empty file', async () => {
    await writeFileInDir(tmpDir, 'empty-read.txt', '')
    const result = await readTextFile('empty-read.txt', { cwd: tmpDir })
    assert.equal(result.content, '')
    assert.equal(result.sizeBytes, 0)
    assert.equal(result.truncated, false)
  })
})


describe('readTextFile cwd block policy', () => {
  let workspaceDir: string
  let outsideDir: string

  before(async () => {
    workspaceDir = await createTempDir('qubit-read-workspace-')
    outsideDir = await createTempDir('qubit-read-outside-')
    await fs.promises.writeFile(path.join(outsideDir, 'outside.txt'), 'outside content', 'utf8')
  })

  after(async () => {
    await cleanupTempDir(workspaceDir)
    await cleanupTempDir(outsideDir)
  })

  it('blocks reading outside cwd by default and allows when cwd block is disabled', async () => {
    const outsidePath = path.join(outsideDir, 'outside.txt')
    await assert.rejects(
      () => readTextFile(outsidePath, { cwd: workspaceDir }),
      /outside the workspace|Access denied/
    )

    const result = await readTextFile(outsidePath, { cwd: workspaceDir, cwdBlockEnabled: false })
    assert.equal(result.content, 'outside content')
  })
})
