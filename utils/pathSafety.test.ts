import * as assert from 'node:assert/strict'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import {
  assertPathWithinWorkspace,
  resolveRestrictedToolPath,
  resolveToolPath,
  type ResolvedToolPath,
} from './pathSafety.js'
import { cleanupTempDir, createTempDir } from '../tools/testHelpers.js'

describe('resolveToolPath', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-path-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('resolves relative path inside cwd', async () => {
    const result = await resolveToolPath('subdir/file.txt', { cwd: tmpDir })
    assert.ok(result.comparisonPath.includes('subdir'))
    assert.ok(result.comparisonPath.includes('file.txt'))
    assert.equal(result.pathType, 'relative')
  })

  it('rejects empty path', async () => {
    await assert.rejects(
      () => resolveToolPath('', { cwd: tmpDir }),
      /non-empty string/
    )
  })

  it('rejects whitespace-only path', async () => {
    await assert.rejects(
      () => resolveToolPath('   ', { cwd: tmpDir }),
      /non-empty string/
    )
  })
})

describe('resolveRestrictedToolPath', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-restricted-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('allows relative path inside workspace', async () => {
    const result = await resolveRestrictedToolPath('file.txt', { cwd: tmpDir, mode: 'file' })
    assert.ok(result.comparisonPath.includes('file.txt'))
  })

  it('allows absolute path inside workspace', async () => {
    const absPath = path.join(tmpDir, 'inside.txt')
    const result = await resolveRestrictedToolPath(absPath, { cwd: tmpDir, mode: 'file' })
    assert.ok(result.comparisonPath.includes('inside.txt'))
  })

  it('blocks path escape with ..', async () => {
    await assert.rejects(
      () => resolveRestrictedToolPath('../outside.txt', { cwd: tmpDir, mode: 'file' }),
      /outside the workspace|Access denied/
    )
  })

  it('blocks absolute path outside workspace', async () => {
    // Use a path that is clearly outside the temp dir
    const outsidePath = path.resolve(path.join(tmpDir, '..', 'outside-file.txt'))
    await assert.rejects(
      () => resolveRestrictedToolPath(outsidePath, { cwd: tmpDir, mode: 'file' }),
      /outside the workspace|Access denied/
    )
  })

  it('allows absolute path outside workspace when cwd restriction is disabled', async () => {
    const outsidePath = path.resolve(path.join(tmpDir, '..', 'outside-open.txt'))
    const result = await resolveRestrictedToolPath(outsidePath, { cwd: tmpDir, mode: 'file', restrictToCwd: false })
    assert.equal(result.comparisonPath, outsidePath)
  })

  it('can restrict a supplied cwd to a separate default workspace', async () => {
    const defaultWorkspace = path.join(tmpDir, 'default-workspace')
    const outsideWorkspace = path.resolve(path.join(tmpDir, '..', 'outside-workspace'))
    await assert.rejects(
      () => resolveRestrictedToolPath(outsideWorkspace, { cwd: outsideWorkspace, workspaceCwd: defaultWorkspace, mode: 'directory' }),
      /outside the workspace|Access denied/
    )
  })
})

describe('assertPathWithinWorkspace', () => {
  it('allows path inside workspace', () => {
    const target: ResolvedToolPath = {
      inputPath: 'file.txt',
      displayPath: '/workspace/file.txt',
      fsPath: '/workspace/file.txt',
      pathType: 'relative',
      comparisonPath: '/workspace/file.txt',
      comparisonKind: 'posix',
    }
    const workspace: ResolvedToolPath = {
      inputPath: '/workspace',
      displayPath: '/workspace',
      fsPath: '/workspace',
      pathType: 'posix',
      comparisonPath: '/workspace',
      comparisonKind: 'posix',
    }
    // Should not throw
    assertPathWithinWorkspace(target, workspace)
  })

  it('rejects path outside workspace', () => {
    const target: ResolvedToolPath = {
      inputPath: '../outside.txt',
      displayPath: '/outside.txt',
      fsPath: '/outside.txt',
      pathType: 'relative',
      comparisonPath: '/outside.txt',
      comparisonKind: 'posix',
    }
    const workspace: ResolvedToolPath = {
      inputPath: '/workspace',
      displayPath: '/workspace',
      fsPath: '/workspace',
      pathType: 'posix',
      comparisonPath: '/workspace',
      comparisonKind: 'posix',
    }
    assert.throws(
      () => assertPathWithinWorkspace(target, workspace),
      /Access denied|outside the workspace/
    )
  })

  it('rejects mixed comparison kinds', () => {
    const target: ResolvedToolPath = {
      inputPath: '/home/user/file.txt',
      displayPath: '/home/user/file.txt',
      fsPath: '/home/user/file.txt',
      pathType: 'posix',
      comparisonPath: '/home/user/file.txt',
      comparisonKind: 'posix',
    }
    const workspace: ResolvedToolPath = {
      inputPath: 'D:\\workspace',
      displayPath: 'D:\\workspace',
      fsPath: 'D:\\workspace',
      pathType: 'windows',
      comparisonPath: 'D:\\workspace',
      comparisonKind: 'win32',
    }
    assert.throws(
      () => assertPathWithinWorkspace(target, workspace),
      /filesystem style|Access denied/
    )
  })

  it('allows workspace root itself', () => {
    const workspace: ResolvedToolPath = {
      inputPath: '/workspace',
      displayPath: '/workspace',
      fsPath: '/workspace',
      pathType: 'posix',
      comparisonPath: '/workspace',
      comparisonKind: 'posix',
    }
    // Should not throw when target is the workspace itself
    assertPathWithinWorkspace(workspace, workspace)
  })
})
