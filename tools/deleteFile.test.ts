import * as assert from 'node:assert/strict'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { deleteTextFile, type DeleteFileResult } from './deleteFile.js'
import { cleanupTempDir, createTempDir, fileExists, writeFileInDir } from './testHelpers.js'

describe('deleteTextFile', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-delete-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('deletes an existing file', async () => {
    await writeFileInDir(tmpDir, 'to-delete.txt', 'content')
    assert.equal(await fileExists(path.join(tmpDir, 'to-delete.txt')), true)

    const result = await deleteTextFile('to-delete.txt', { cwd: tmpDir })
    assert.equal(result.success, true)
    assert.equal(result.deleted, true)
    assert.equal(await fileExists(path.join(tmpDir, 'to-delete.txt')), false)
  })

  it('returns failure for file not found', async () => {
    const result = await deleteTextFile('nonexistent.txt', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.equal(result.deleted, false)
    assert.ok(result.message.includes('not found') || result.message.includes('does not exist'))
  })

  it('rejects directories', async () => {
    const dirPath = path.join(tmpDir, 'a-dir')
    await fs.promises.mkdir(dirPath, { recursive: true })
    const result = await deleteTextFile('a-dir', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.equal(result.deleted, false)
    assert.ok(result.message.includes('not a file'))
  })

  it('blocks files with disallowed extension', async () => {
    await writeFileInDir(tmpDir, 'blocked.log', 'log content')
    const result = await deleteTextFile('blocked.log', { cwd: tmpDir, allowedExtensions: ['.txt'] })
    assert.equal(result.success, false)
    assert.equal(result.deleted, false)
    assert.ok(result.message.includes('not allowed') || result.message.includes('extension'))
    // File should still exist
    assert.equal(await fileExists(path.join(tmpDir, 'blocked.log')), true)
  })

  it('allows files with permitted extension', async () => {
    await writeFileInDir(tmpDir, 'allowed.txt', 'txt content')
    const result = await deleteTextFile('allowed.txt', { cwd: tmpDir, allowedExtensions: ['.txt'] })
    assert.equal(result.success, true)
    assert.equal(result.deleted, true)
  })

  it('rejects sensitive .git paths', async () => {
    const gitDir = path.join(tmpDir, '.git')
    await fs.promises.mkdir(gitDir, { recursive: true })
    await writeFileInDir(tmpDir, '.git/config', 'gitconfig')
    const result = await deleteTextFile('.git/config', { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('sensitive') || result.message.includes('denied'))
  })

  it('plan mode blocks deletion', async () => {
    await writeFileInDir(tmpDir, 'plan-delete.txt', 'content')
    const result = await deleteTextFile('plan-delete.txt', { cwd: tmpDir, operationMode: 'plan' })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('planning mode'))
    // File should still exist
    assert.equal(await fileExists(path.join(tmpDir, 'plan-delete.txt')), true)
  })
})
