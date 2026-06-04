import * as assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'
import { formatReadFilesResult, readMultipleTextFiles } from './readFiles.js'
import { cleanupTempDir, createTempDir, writeFileInDir } from './testHelpers.js'

describe('readMultipleTextFiles', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-read-files-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('preserves range metadata and formats range headers', async () => {
    await writeFileInDir(tmpDir, 'ranges-a.txt', 'a1\na2\na3\na4\na5')
    await writeFileInDir(tmpDir, 'ranges-b.txt', 'b1\nb2\nb3\nb4\nb5')

    const files = await readMultipleTextFiles(['ranges-a.txt', 'ranges-b.txt'], {
      cwd: tmpDir,
      ranges: [{ startLine: 1, endLine: 2 }, { startLine: 4, endLine: 5 }],
    })

    assert.deepEqual(files[0].ranges, [
      { startLine: 1, endLine: 2, lineCount: 2 },
      { startLine: 4, endLine: 5, lineCount: 2 },
    ])
    assert.equal(files[0].content, 'a1\na2\n\na4\na5')

    const formatted = formatReadFilesResult(files)
    assert.match(formatted, /^ranges-a\.txt \(ranges 1-2, 4-5\)\na1/m)
    assert.match(formatted, /ranges-b\.txt \(ranges 1-2, 4-5\)\nb1/)
  })

  it('formats start and end line selections in headers', async () => {
    await writeFileInDir(tmpDir, 'lines.txt', 'line1\nline2\nline3')

    const files = await readMultipleTextFiles(['lines.txt'], { cwd: tmpDir, startLine: 2, endLine: 3 })

    assert.equal(files[0].startLine, 2)
    assert.equal(files[0].endLine, 3)
    assert.equal(formatReadFilesResult(files), 'lines.txt (lines 2-3)\nline2\nline3')
  })
})
