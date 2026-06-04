import assert from 'node:assert/strict'
import test from 'node:test'
import { normalizeSubagentArgs, setSubagentExecutor, subagentTool } from './subagent.js'

test('normalizeSubagentArgs validates required tasks and prompts', () => {
  assert.throws(() => normalizeSubagentArgs({}), /tasks must be a non-empty array/)
  assert.throws(() => normalizeSubagentArgs({ tasks: [] }), /tasks must be a non-empty array/)
  assert.throws(() => normalizeSubagentArgs({ tasks: [{ prompt: '' }] }), /prompt is required/)
})

test('normalizeSubagentArgs defaults parallel and supports linear stopOnError', () => {
  assert.deepEqual(normalizeSubagentArgs({ tasks: [{ name: 'one', prompt: 'Do it' }] }), {
    executionMode: 'parallel',
    stopOnError: false,
    tasks: [{ name: 'one', prompt: 'Do it' }],
  })
  assert.deepEqual(normalizeSubagentArgs({ executionMode: 'linear', tasks: [{ prompt: 'A' }, { prompt: 'B' }] }), {
    executionMode: 'linear',
    stopOnError: true,
    tasks: [{ prompt: 'A' }, { prompt: 'B' }],
  })
})

test('subagentTool returns clear error without executor', async () => {
  setSubagentExecutor(undefined)
  const result = await subagentTool.execute({ tasks: [{ prompt: 'test' }] }, { sessionId: 'sess', step: 1 })
  assert.equal(result.ok, false)
  assert.match(String(result.error), /executor is not configured/)
})

test('subagentTool calls injected executor with normalized args', async () => {
  const calls: unknown[] = []
  setSubagentExecutor(async (args, context) => {
    calls.push({ args, context })
    return { success: true, completed: args.tasks.length }
  })

  const result = await subagentTool.execute({ executionMode: 'linear', tasks: [{ prompt: 'one' }] }, { sessionId: 'sess', runId: 'run', step: 2 })
  assert.equal(result.ok, true)
  assert.deepEqual(result.data, { success: true, completed: 1 })
  assert.equal(calls.length, 1)
  assert.deepEqual((calls[0] as any).args, { executionMode: 'linear', stopOnError: true, tasks: [{ prompt: 'one' }] })
  assert.equal((calls[0] as any).context.runId, 'run')

  setSubagentExecutor(undefined)
})
