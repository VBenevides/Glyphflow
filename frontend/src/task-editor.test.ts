import { describe, expect, it } from 'vitest'
import { commandArguments, emptyTaskDraft, environmentObject, environmentRows, taskDraftFromRecord, validateTaskDraft } from './task-editor'

describe('task version editor', () => {
  it('keeps command arguments separate from shell parsing', () => {
    expect(commandArguments('python\nscript.py\n--name\nhello world')).toEqual(['python', 'script.py', '--name', 'hello world'])
  })

	it('validates required fields and JSON policies', () => {
    expect(Object.keys(validateTaskDraft(emptyTaskDraft))).toEqual(['name', 'command', 'pool'])
    expect(validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo\nhello', pool: 'default' })).toEqual({})
	})

	it('loads an edit baseline from the task record', () => {
		const draft = taskDraftFromRecord({ id: 'task-1', name: 'Nightly', pool: 'default', timeoutSeconds: 90, command: ['python', 'script.py'], environment: { CACHE_PATH: '/tmp/cache' }, maxAttempts: 3 })
		expect(draft.name).toBe('Nightly')
		expect(draft.pool).toBe('default')
		expect(draft.timeoutSeconds).toBe('90')
		expect(draft.command).toBe('python\nscript.py')
		expect(draft.environment).toEqual([{ name: 'CACHE_PATH', value: '/tmp/cache' }])
		expect(draft.maxAttempts).toBe('3')
	})

	it('maps environment rows and rejects invalid or duplicate names', () => {
		expect(environmentRows({ ZED: '2', APP_HOME: '/app' })).toEqual([{ name: 'APP_HOME', value: '/app' }, { name: 'ZED', value: '2' }])
		expect(environmentObject([{ name: ' APP_HOME ', value: '/app' }, { name: '', value: 'ignored' }])).toEqual({ APP_HOME: '/app' })
		const errors = validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo', pool: 'default', environment: [{ name: 'BAD-NAME', value: 'x' }, { name: 'BAD-NAME', value: 'y' }] })
		expect(errors['environment.0.name']).toContain('letters')
		expect(errors['environment.1.name']).toContain('unique')
	})
})
