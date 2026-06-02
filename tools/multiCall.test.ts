import * as assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'
import { type AnyToolDefinition } from '@hyper-labs/hyper-router'
import { editFileTool } from './editFile.js'
import { createMultiCallTool, multiCall, setMultiCallLifecycleEmitter, setMultiCallPermissionRequester } from './multiCall.js'
import { readFileTool } from './readFile.js'
import { cleanupTempDir, createTempDir, readFileInDir, writeFileInDir } from './testHelpers.js'

const tools: AnyToolDefinition[] = [readFileTool, editFileTool]

describe('multiCall', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-multicall-')
  })

  after(async () => {
    setMultiCallPermissionRequester(undefined)
    setMultiCallLifecycleEmitter(undefined)
    await cleanupTempDir(tmpDir)
  })

  it('executes read-only calls sequentially', async () => {
    await writeFileInDir(tmpDir, 'a.txt', 'alpha')
    await writeFileInDir(tmpDir, 'b.txt', 'beta')

    const result = await multiCall(
      [
        { tool: 'readFile', args: { path: 'a.txt', cwd: tmpDir } },
        { tool: 'readFile', args: { path: 'b.txt', cwd: tmpDir } },
      ],
      {},
      tools
    )

    assert.equal(result.success, true)
    assert.equal(result.completed, 2)
    assert.equal(result.failed, 0)
    assert.equal((result.results[0].data as any).content, 'alpha')
    assert.equal((result.results[1].data as any).content, 'beta')
  })

  it('requests permission for gated nested tools and continues after allow', async () => {
    await writeFileInDir(tmpDir, 'edit.txt', 'old')
    const requests: string[] = []
    setMultiCallPermissionRequester(async request => {
      requests.push(request.toolName)
      return { type: 'allow' }
    })

    const result = await multiCall(
      [
        { tool: 'editFile', args: { path: 'edit.txt', cwd: tmpDir, operation: 'replace', searchPattern: 'old', replacement: 'new' } },
        { tool: 'readFile', args: { path: 'edit.txt', cwd: tmpDir } },
      ],
      {},
      tools,
      { sessionId: 'sess', step: 1 }
    )

    assert.equal(result.success, true)
    assert.deepEqual(requests, ['editFile'])
    assert.equal(result.results[0].permission, 'allowed')
    assert.equal(await readFileInDir(tmpDir, 'edit.txt'), 'new')
    assert.equal((result.results[1].data as any).content, 'new')
  })

  it('emits synthetic lifecycle events for nested tools', async () => {
    await writeFileInDir(tmpDir, 'lifecycle.txt', 'old')
    setMultiCallPermissionRequester(async () => ({ type: 'allow' }))
    const events: any[] = []
    setMultiCallLifecycleEmitter(event => { events.push(event) })

    const result = await multiCall(
      [
        { tool: 'editFile', args: { path: 'lifecycle.txt', cwd: tmpDir, operation: 'replace_first', searchPattern: 'old', replacement: 'new' } },
        { tool: 'readFile', args: { path: 'lifecycle.txt', cwd: tmpDir } },
      ],
      {},
      tools,
      { sessionId: 'sess', step: 3, runId: 'run_1' }
    )

    assert.equal(result.success, true)
    assert.equal(events.length, 4)
    assert.equal(events[0].type, 'start')
    assert.equal(events[0].toolName, 'editFile')
    assert.equal(events[0].step, 4)
    assert.equal(events[1].type, 'finish')
    assert.equal(events[1].toolName, 'editFile')
    assert.equal(events[1].status, 'completed')
    assert.equal(events[1].result.data.success, true)
    assert.equal(events[2].toolName, 'readFile')
    assert.equal(events[2].step, 5)
    assert.equal(events[3].status, 'completed')
    setMultiCallLifecycleEmitter(undefined)
  })

  it('stops when permission is denied by default', async () => {
    await writeFileInDir(tmpDir, 'deny.txt', 'old')
    setMultiCallPermissionRequester(async () => ({ type: 'deny', reason: 'not now' }))

    const result = await multiCall(
      [
        { tool: 'editFile', args: { path: 'deny.txt', cwd: tmpDir, operation: 'replace', searchPattern: 'old', replacement: 'new' } },
        { tool: 'readFile', args: { path: 'deny.txt', cwd: tmpDir } },
      ],
      {},
      tools,
      { sessionId: 'sess', step: 1 }
    )

    assert.equal(result.success, false)
    assert.equal(result.stoppedEarly, true)
    assert.equal(result.failed, 1)
    assert.match(result.results[0].error || '', /not now/)
    assert.equal(await readFileInDir(tmpDir, 'deny.txt'), 'old')
  })

  it('continues on errors when stopOnError=false', async () => {
    await writeFileInDir(tmpDir, 'continue.txt', 'ok')

    const result = await multiCall(
      [
        { tool: 'missingTool', args: {} },
        { tool: 'readFile', args: { path: 'continue.txt', cwd: tmpDir } },
      ],
      { stopOnError: false },
      tools
    )

    assert.equal(result.success, false)
    assert.equal(result.completed, 1)
    assert.equal(result.failed, 1)
    assert.equal(result.stoppedEarly, false)
    assert.equal((result.results[1].data as any).content, 'ok')
  })

  it('rejects nested multiCall', async () => {
    const result = await multiCall(
      [{ tool: 'multiCall', args: { calls: [] } }],
      {},
      tools
    )

    assert.equal(result.success, false)
    assert.match(result.results[0].error || '', /Nested multiCall/)
  })

  it('tool wrapper validates calls argument', async () => {
    const tool = createMultiCallTool(tools)
    const result = await tool.execute({}, { sessionId: 'sess', step: 1 })
    assert.equal(result.ok, false)
    assert.match(result.error || '', /calls is required/)
  })
})
