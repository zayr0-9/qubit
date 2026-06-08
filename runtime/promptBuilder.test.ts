import assert from 'node:assert/strict'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import test from 'node:test'
import { baseInstructions, createPromptBuilder, defaultSubagentPrompt, normalizePromptMode } from './promptBuilder.js'

test('normalizePromptMode maps edit aliases and defaults to plan', () => {
  assert.equal(normalizePromptMode('edit'), 'edit')
  assert.equal(normalizePromptMode('always_allow'), 'edit')
  assert.equal(normalizePromptMode('allow'), 'edit')
  assert.equal(normalizePromptMode('plan'), 'plan')
  assert.equal(normalizePromptMode(undefined), 'plan')
  assert.equal(normalizePromptMode('unknown'), 'plan')
})

test('prompt builder composes parent modes but keeps subagent prompt standalone', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'qubit-prompts-'))
  try {
    await writeFile(join(dir, 'plan.md'), 'Plan addendum', 'utf8')
    await writeFile(join(dir, 'edit.md'), 'Edit addendum', 'utf8')
    await writeFile(join(dir, 'subagent.md'), 'Standalone subagent prompt', 'utf8')

    const builder = await createPromptBuilder(dir)

    assert.equal(builder.instructionsForMode('plan'), `${baseInstructions}\n\nPlan addendum`)
    assert.equal(builder.instructionsForMode('edit'), `${baseInstructions}\n\nEdit addendum`)
    assert.equal(builder.instructionsForSubagent(), 'Standalone subagent prompt')
    assert.equal(builder.instructionsForSubagent().includes(baseInstructions), false)
    assert.equal(builder.instructionsForSubagent().includes('Plan addendum'), false)
    assert.equal(builder.instructionsForSubagent().includes('Edit addendum'), false)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})

test('prompt builder falls back to a standalone default subagent prompt', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'qubit-prompts-'))
  try {
    const builder = await createPromptBuilder(dir)
    assert.equal(builder.instructionsForSubagent(), defaultSubagentPrompt)
    assert.equal(builder.instructionsForSubagent().includes(baseInstructions), false)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})
