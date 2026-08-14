import { describe, expect, it } from 'vitest'
import { commandArguments, emptyTaskDraft, validateTaskDraft } from './task-editor'

describe('task version editor', () => {
  it('keeps command arguments separate from shell parsing', () => {
    expect(commandArguments('python\nscript.py\n--name\nhello world')).toEqual(['python', 'script.py', '--name', 'hello world'])
  })

  it('validates required fields and JSON policies', () => {
    expect(Object.keys(validateTaskDraft(emptyTaskDraft))).toEqual(['name', 'command', 'pool'])
    expect(validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo\nhello', pool: 'default' })).toEqual({})
  })
})
