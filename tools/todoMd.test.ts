import * as assert from 'node:assert/strict'
import { after, before, describe, it } from 'node:test'
import { cleanupTempDir, createTempDir } from './testHelpers.js'
import { createTodoList, editTodoList, readTodoList, runTodoTool } from './todoMd.js'

describe('todoMd', () => {
  let tmpDir: string

  before(async () => {
    tmpDir = await createTempDir('qubit-todo-')
  })

  after(async () => {
    await cleanupTempDir(tmpDir)
  })

  it('edits multiple todo lines in one call', async () => {
    const created = await createTodoList('- [ ] inspect\n- [ ] implement\n- [ ] validate\n', tmpDir)

    const edited = await editTodoList(created.id, [
      { search: '[ ] inspect', replacement: '- [x] inspect' },
      { search: '[ ] implement', replacement: '- [x] implement' },
    ], tmpDir)

    assert.equal(edited.success, true)
    assert.equal(edited.results?.length, 2)
    assert.match(edited.content ?? '', /\[x\] inspect/)
    assert.match(edited.content ?? '', /\[x\] implement/)

    const read = await readTodoList(created.id, tmpDir)
    assert.equal(read.content, edited.content)
  })

  it('keeps legacy search/replacement edit args working', async () => {
    const created = await createTodoList('- [ ] legacy\n', tmpDir)

    const edited = await runTodoTool({
      action: 'edit',
      name: created.id,
      search: '[ ] legacy',
      replacement: '- [x] legacy',
      cwd: tmpDir,
    }) as any

    assert.equal(edited.success, true)
    assert.match(edited.content ?? '', /\[x\] legacy/)
  })

  it('does not write partial batched edits when any search misses', async () => {
    const created = await createTodoList('- [ ] first\n- [ ] second\n', tmpDir)

    const edited = await editTodoList(created.id, [
      { search: '[ ] first', replacement: '- [x] first' },
      { search: '[ ] missing', replacement: '- [x] missing' },
    ], tmpDir)

    assert.equal(edited.success, false)
    assert.match(edited.message, /no changes written/)

    const read = await readTodoList(created.id, tmpDir)
    assert.equal(read.content, '- [ ] first\n- [ ] second\n')
  })

  it('creates a named todo list for later read and edit calls', async () => {
    const created = await runTodoTool({
      action: 'create',
      name: 'Fix relative cwd',
      content: '- [ ] inspect\n',
      cwd: tmpDir,
    }) as any

    assert.equal(created.id, 'fix-relative-cwd')
    assert.equal(created.created, true)
    assert.equal(created.success, true)

    const edited = await runTodoTool({
      action: 'edit',
      name: 'fix-relative-cwd',
      search: '[ ] inspect',
      replacement: '- [x] inspect',
      cwd: tmpDir,
    }) as any

    assert.equal(edited.success, true)
    assert.match(edited.content ?? '', /\[x\] inspect/)

    const read = await readTodoList('fix-relative-cwd', tmpDir)
    assert.equal(read.content, edited.content)
  })

  it('does not overwrite an existing named todo list', async () => {
    const created = await runTodoTool({
      action: 'create',
      name: 'duplicate-list',
      content: '- [ ] original\n',
      cwd: tmpDir,
    }) as any
    assert.equal(created.success, true)

    const duplicate = await runTodoTool({
      action: 'create',
      name: 'duplicate-list',
      content: '- [ ] replacement\n',
      cwd: tmpDir,
    }) as any

    assert.equal(duplicate.created, false)
    assert.equal(duplicate.success, false)
    assert.match(duplicate.message, /already exists/)

    const read = await readTodoList('duplicate-list', tmpDir)
    assert.equal(read.content, '- [ ] original\n')
  })
})
