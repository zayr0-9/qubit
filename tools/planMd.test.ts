import * as assert from 'node:assert/strict'
import * as path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { cleanupTempDir, createTempDir } from './testHelpers.js'
import { createPlan, editPlan, listPlans, normalizePlanClarificationQuestions, readPlan, runPlanTool, setPlanClarificationRequester, setPlanViewEmitter } from './planMd.js'

describe('planMd', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-plan-')
  })

  after(async () => {
    setPlanViewEmitter(null)
    setPlanClarificationRequester(null)
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

  it('display emits UI-only plan event and returns bounded result', async () => {
    await createPlan('# Display Me\n\nhello', 'display-me', tmpDir)
    let event: any = null
    setPlanViewEmitter((next) => { event = next })
    const result = await runPlanTool(
      { action: 'display', name: 'display-me', cwd: tmpDir },
      { sessionId: 'sess_1', runId: 'run_1', step: 3 },
    ) as any
    assert.equal(result.displayed, true)
    assert.equal(result.name, 'display-me')
    assert.equal(result.modelContent, 'Plan "display-me" was displayed to the user in the chat view. Do not repeat the plan unless the user asks.')
    assert.equal(event.name, 'display-me')
    assert.equal(event.content, '# Display Me\n\nhello')
    assert.equal(event.sessionId, 'sess_1')
    assert.equal(event.runId, 'run_1')
    assert.equal(event.step, 3)
    assert.equal(path.basename(path.dirname(event.path)), 'plans')
  })

  it('clarify appends manual option and returns user answers', async () => {
    let request: any = null
    setPlanClarificationRequester(async (next) => {
      request = next
      return {
        answers: [{
          questionId: 'scope',
          question: 'Which scope?',
          selectedOptionId: 'ui',
          selectedOptionLabel: 'Go UI',
          manual: false,
          answer: 'Go UI',
        }],
      }
    })

    const result = await runPlanTool({
      action: 'clarify',
      questions: [{ id: 'scope', question: 'Which scope?', options: [{ id: 'ui', label: 'Go UI' }] }],
    }, { sessionId: 'sess_1', runId: 'run_1', step: 2 }) as any

    assert.equal(result.clarified, true)
    assert.equal(result.questions, 1)
    assert.equal(result.answers[0].answer, 'Go UI')
    assert.equal(request.sessionId, 'sess_1')
    assert.equal(request.runId, 'run_1')
    assert.equal(request.step, 2)
    assert.equal(request.questions[0].options.at(-1).manual, true)
    assert.match(request.questions[0].options.at(-1).label, /None of the above/)
  })

  it('validates clarify questions', () => {
    assert.throws(() => normalizePlanClarificationQuestions([]), /non-empty array/)
    assert.throws(() => normalizePlanClarificationQuestions([{ question: '' }]), /question is required/)
  })
})