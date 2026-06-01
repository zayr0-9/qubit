import * as assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'
import { multiEdit, type MultiEditResult } from './editFile.js'
import { cleanupTempDir, createTempDir, readFileInDir, writeFileInDir } from './testHelpers.js'

describe('multiEdit', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-multi-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('edits two files', async () => {
    await writeFileInDir(tmpDir, 'a.txt', 'hello old a')
    await writeFileInDir(tmpDir, 'b.txt', 'hello old b')
    const result: MultiEditResult = await multiEdit(
      [
        { path: 'a.txt', operation: 'replace', searchPattern: 'old a', replacement: 'new a' },
        { path: 'b.txt', operation: 'replace', searchPattern: 'old b', replacement: 'new b' },
      ],
      { cwd: tmpDir }
    )
    assert.equal(result.success, true)
    assert.equal(result.applied, 2)
    assert.equal(result.failed, 0)
    assert.equal(result.stoppedEarly, false)
    const contentA = await readFileInDir(tmpDir, 'a.txt')
    const contentB = await readFileInDir(tmpDir, 'b.txt')
    assert.equal(contentA, 'hello new a')
    assert.equal(contentB, 'hello new b')
  })

  it('stops on first error by default (stopOnError=true)', async () => {
    await writeFileInDir(tmpDir, 'stop-a.txt', 'original a')
    await writeFileInDir(tmpDir, 'stop-b.txt', 'original b')
    const result = await multiEdit(
      [
        { path: 'stop-a.txt', operation: 'replace', searchPattern: 'nonexistent', replacement: 'fail' },
        { path: 'stop-b.txt', operation: 'replace', searchPattern: 'original b', replacement: 'changed b' },
      ],
      { cwd: tmpDir }
    )
    assert.equal(result.success, false)
    assert.equal(result.stoppedEarly, true)
    assert.equal(result.failed, 1)
    // Second file should NOT have been modified
    const contentB = await readFileInDir(tmpDir, 'stop-b.txt')
    assert.equal(contentB, 'original b')
  })

  it('continues on error with stopOnError=false', async () => {
    await writeFileInDir(tmpDir, 'cont-a.txt', 'original a')
    await writeFileInDir(tmpDir, 'cont-b.txt', 'original b')
    const result = await multiEdit(
      [
        { path: 'cont-a.txt', operation: 'replace', searchPattern: 'nonexistent', replacement: 'fail' },
        { path: 'cont-b.txt', operation: 'replace', searchPattern: 'original b', replacement: 'changed b' },
      ],
      { cwd: tmpDir, stopOnError: false }
    )
    assert.equal(result.success, false) // partial failure
    assert.equal(result.applied, 1)
    assert.equal(result.failed, 1)
    assert.equal(result.stoppedEarly, false)
    // Second file should have been modified
    const contentB = await readFileInDir(tmpDir, 'cont-b.txt')
    assert.equal(contentB, 'changed b')
  })

  it('mix of replace and append in multiEdit', async () => {
    await writeFileInDir(tmpDir, 'mix.txt', 'first line')
    const result = await multiEdit(
      [
        { path: 'mix.txt', operation: 'replace', searchPattern: 'first', replacement: 'replaced' },
        { path: 'mix.txt', operation: 'append', content: '\nappended' },
      ],
      { cwd: tmpDir }
    )
    assert.equal(result.success, true)
    assert.equal(result.applied, 2)
    const content = await readFileInDir(tmpDir, 'mix.txt')
    assert.equal(content, 'replaced line\nappended')
  })

  it('plan mode blocks all edits', async () => {
    await writeFileInDir(tmpDir, 'plan-multi.txt', 'original')
    const result = await multiEdit(
      [{ path: 'plan-multi.txt', operation: 'replace', searchPattern: 'original', replacement: 'modified' }],
      { cwd: tmpDir, operationMode: 'plan' }
    )
    assert.equal(result.success, false)
    assert.ok(result.message.includes('planning mode'))
    const content = await readFileInDir(tmpDir, 'plan-multi.txt')
    assert.equal(content, 'original')
  })

  it('empty edits array fails', async () => {
    const result = await multiEdit([], { cwd: tmpDir })
    assert.equal(result.success, false)
    assert.ok(result.message.includes('non-empty array'))
  })

  it('missing path in item fails that item', async () => {
    const result = await multiEdit(
      [{ path: '', operation: 'replace', searchPattern: 'x', replacement: 'y' } as any],
      { cwd: tmpDir }
    )
    assert.equal(result.success, false)
    assert.ok(result.results[0].message.includes('path is required'))
  })

  it('missing operation in item fails that item', async () => {
    const result = await multiEdit(
      [{ path: 'some.txt', operation: undefined as any, searchPattern: 'x', replacement: 'y' }],
      { cwd: tmpDir }
    )
    assert.equal(result.success, false)
    assert.ok(result.results[0].message.includes('operation is required'))
  })
})
