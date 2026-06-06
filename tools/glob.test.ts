import * as assert from 'node:assert/strict'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { globSearch } from './glob.js'
import { cleanupTempDir, createTempDir, writeFileInDir } from './testHelpers.js'
import { getDefaultToolCwd, setDefaultToolCwd } from '../utils/toolWorkspace.js'

describe('globSearch cwd handling', () => {
  let previousCwd: string
  let workspaceDir: string
  let outsideDir: string

  before(async () => {
    previousCwd = getDefaultToolCwd()
    workspaceDir = await createTempDir('qubit-glob-workspace-')
    outsideDir = await createTempDir('qubit-glob-outside-')
    setDefaultToolCwd(workspaceDir)
    await writeFileInDir(workspaceDir, 'one.txt', 'one')
    await writeFileInDir(workspaceDir, 'two.md', 'two')
    await writeFileInDir(workspaceDir, 'nested/three.txt', 'three')
  })

  after(async () => {
    setDefaultToolCwd(previousCwd)
    await cleanupTempDir(workspaceDir)
    await cleanupTempDir(outsideDir)
  })

  it('resolves dot cwd to the default workspace instead of filesystem root', async () => {
    const result = await globSearch('*.txt', { cwd: '.', workspaceCwd: workspaceDir })

    assert.equal(result.success, true)
    assert.equal(result.cwd, workspaceDir)
    assert.deepEqual(result.matches, ['one.txt'])
  })

  it('resolves relative cwd under the default workspace', async () => {
    const result = await globSearch('*.txt', { cwd: './nested', workspaceCwd: workspaceDir })

    assert.equal(result.success, true)
    assert.equal(result.cwd, path.join(workspaceDir, 'nested'))
    assert.deepEqual(result.matches, ['three.txt'])
  })

  it('blocks cwd outside the workspace when cwd blocking is enabled', async () => {
    const result = await globSearch('*.txt', { cwd: outsideDir, workspaceCwd: workspaceDir })

    assert.equal(result.success, false)
    assert.match(result.error || '', /outside the workspace|Access denied/)
  })
})
