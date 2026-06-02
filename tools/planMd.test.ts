import * as assert from 'node:assert/strict'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { cleanupTempDir, createTempDir } from './testHelpers.js'
import { createPlan, editPlan, listPlans, readPlan, runPlanTool, setPlanViewEmitter } from './planMd.js'

describe('planMd', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-plan-')
  })

  after(async () => {
    setPlanViewEmitter(null)
    await cleanupTempDir(tmpDir)
  })

  it('creates, lists, reads, and edits plans in project .qubit/plans', async () => {
    const created = await createPlan('# Test Plan\n\n- [ ] first\n', 'Test Plan', tmpDir)
    assert.equal(created.name, 'test-plan')
    assert.equal(created.created, true)

    const listed = await listPlans(tmpDir)
    assert.equal(listed.length, 1)
    assert.equal(listed[0].name, 'test-plan')
    assert.equal(listed[0].title, 'Test Plan')

    const read = await readPlan('test-plan', tmpDir)
    assert.equal(read.exists, true)
    assert.match(read.content ?? '', /first/)

    const edited = await editPlan('test-plan', '[ ] first', '[x] first', tmpDir)
    assert.equal(edited.success, true)
    assert.match(edited.content ?? '', /\[x\] first/)
  })

  it('view emits UI-only plan event and returns bounded result', async () => {
    await createPlan('# View Me\n\nhello', 'view-me', tmpDir)
    let event: any = null
    setPlanViewEmitter((next) => { event = next })

    const result = await runPlanTool({ action: 'view', name: 'view-me', cwd: tmpDir }) as any
    assert.equal(result.viewed, true)
    assert.equal(result.name, 'view-me')
    assert.equal(event.name, 'view-me')
    assert.equal(event.content, '# View Me\n\nhello')
    assert.equal(path.basename(path.dirname(event.path)), 'plans')
  })
})
