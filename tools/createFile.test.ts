import * as assert from 'node:assert/strict'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { createTextFile, type CreateFileResult } from './createFile.js'
import { cleanupTempDir, createTempDir, fileExists, readFileInDir } from './testHelpers.js'

describe('createTextFile', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-create-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('creates a new file with content', async () => {
    const result: CreateFileResult = await createTextFile('hello.txt', 'hello world', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.equal(result.created, true)
    assert.ok(result.sizeBytes > 0)
    const content = await readFileInDir(tmpDir, 'hello.txt')
    assert.equal(content, 'hello world')
  })

  it('creates file in nested directory with createParentDirs=true', async () => {
    const result = await createTextFile('sub/dir/test.txt', 'nested', { cwd: tmpDir, createParentDirs: true })
    assert.equal(result.success, true)
    assert.equal(result.created, true)
    const content = await readFileInDir(tmpDir, 'sub/dir/test.txt')
    assert.equal(content, 'nested')
  })

  it('fails without createParentDirs when parent missing', async () => {
    const result = await createTextFile('missing/parent/file.txt', 'content', { cwd: tmpDir, createParentDirs: false })
    assert.equal(result.success, false)
    assert.equal(result.created, false)
    assert.ok(result.message.includes('Parent directory does not exist'))
  })

  it('fails when file exists and overwrite=false', async () => {
    await createTextFile('existing.txt', 'first', { cwd: tmpDir })
    const result = await createTextFile('existing.txt', 'second', { cwd: tmpDir, overwrite: false })
    assert.equal(result.success, false)
    assert.equal(result.created, false)
    assert.ok(result.message.includes('already exists'))
    // Original content should be preserved
    const content = await readFileInDir(tmpDir, 'existing.txt')
    assert.equal(content, 'first')
  })

  it('overwrites existing file when overwrite=true', async () => {
    await createTextFile('overwrite.txt', 'old', { cwd: tmpDir })
    const result = await createTextFile('overwrite.txt', 'new', { cwd: tmpDir, overwrite: true })
    assert.equal(result.success, true)
    assert.equal(result.created, false) // created is false when overwriting
    const content = await readFileInDir(tmpDir, 'overwrite.txt')
    assert.equal(content, 'new')
  })

  it('fails when path is a directory', async () => {
    const dirPath = path.join(tmpDir, 'a-directory')
    await fs.promises.mkdir(dirPath, { recursive: true })
    const result = await createTextFile('a-directory', 'content', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('directory already exists'))
  })

  it('plan mode blocks creation', async () => {
    const result = await createTextFile('plan.txt', 'content', { cwd: tmpDir, operationMode: 'plan' })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('planning mode'))
    // File should not have been created
    assert.equal(await fileExists(path.join(tmpDir, 'plan.txt')), false)
  })

  it('empty content creates empty file', async () => {
    const result = await createTextFile('empty.txt', '', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.equal(result.created, true)
    assert.equal(result.sizeBytes, 0)
    const content = await readFileInDir(tmpDir, 'empty.txt')
    assert.equal(content, '')
  })

  it('path escape with .. is blocked', async () => {
    const result = await createTextFile('../outside.txt', 'escape', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.toLowerCase().includes('outside') || result.message.toLowerCase().includes('denied'))
  })
})


describe('createTextFile cwd block policy', () => {
  let workspaceDir: string
  let outsideDir: string

  before(async () => {
    workspaceDir = await createTempDir('qubit-create-workspace-')
    outsideDir = await createTempDir('qubit-create-outside-')
  })

  after(async () => {
    await cleanupTempDir(workspaceDir)
    await cleanupTempDir(outsideDir)
  })

  it('blocks creating outside cwd by default and allows when cwd block is disabled', async () => {
    const outsidePath = path.join(outsideDir, 'outside-created.txt')
    const blocked = await createTextFile(outsidePath, 'blocked', { cwd: workspaceDir })
    assert.equal(blocked.success, false)
    assert.match(blocked.message, /outside the workspace|Access denied/)

    const allowed = await createTextFile(outsidePath, 'allowed', { cwd: workspaceDir, cwdBlockEnabled: false })
    assert.equal(allowed.success, true)
    assert.equal(await fs.promises.readFile(outsidePath, 'utf8'), 'allowed')
  })
})
