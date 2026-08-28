import { describe, expect, it } from 'vitest'
import { commandArgumentLabels, commandArguments, emptyTaskDraft, environmentObject, environmentRows, finalCommand, keyValueObject, keyValueRows, resolveGlobalVariableReferences, secretObject, secretRows, taskDraftFromRecord, validateTaskDraft } from './task-editor'

describe('task version editor', () => {
  it('keeps command arguments separate from shell parsing', () => {
    expect(commandArguments('python\nscript.py\n--name\nhello world')).toEqual(['python', 'script.py', '--name', 'hello world'])
  })

  it('labels only non-empty command lines and resolves global values', () => {
    expect(commandArgumentLabels('python\n\n$ENV:CACHE_PATH')).toEqual(['Arg 1:', '', 'Arg 2:'])
    expect(resolveGlobalVariableReferences('$ENV:CACHE_PATH/bin', [{ id: '1', name: 'CACHE_PATH', value: '/tmp/cache' }])).toBe('/tmp/cache/bin')
    expect(finalCommand('python\n$ENV:CACHE_PATH\nhello world', [{ id: '1', name: 'CACHE_PATH', value: '/tmp/cache' }])).toBe('"python" "/tmp/cache" "hello world"')
  })

	it('validates required fields and JSON policies', () => {
    expect(Object.keys(validateTaskDraft(emptyTaskDraft))).toEqual(['name', 'command', 'pool'])
    expect(validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo\nhello', pool: 'default' })).toEqual({})
	})

	it('loads an edit baseline from the task record', () => {
	const draft = taskDraftFromRecord({ id: 'task-1', name: 'Nightly', pool: 'default', durationSeconds: 90, command: ['python', 'script.py'], environment: { CACHE_PATH: '/tmp/cache' }, maxAttempts: 3 })
		expect(draft.name).toBe('Nightly')
		expect(draft.pool).toBe('default')
	expect(draft.durationSeconds).toBe('90')
		expect(draft.command).toBe('python\nscript.py')
		expect(draft.environment).toEqual([{ name: 'CACHE_PATH', value: '/tmp/cache' }])
		expect(draft.maxAttempts).toBe('3')
	})

	it('maps environment rows and rejects invalid or duplicate names', () => {
		expect(environmentRows({ ZED: '2', APP_HOME: '/app' })).toEqual([{ name: 'APP_HOME', value: '/app' }, { name: 'ZED', value: '2' }])
		expect(environmentObject([{ name: ' APP_HOME ', value: '/app' }, { name: '', value: 'ignored' }])).toEqual({ APP_HOME: '/app' })
		const errors = validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo', pool: 'default', environment: [{ name: 'BAD-NAME', value: 'x' }, { name: 'BAD-NAME', value: 'y' }] })
		expect(errors['environment.0.name']).toContain('valid name')
		expect(errors['environment.1.name']).toContain('unique')
		expect(validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo', pool: 'default', environment: [{ name: '$ENV:NAME', value: '$ENV:VALUE' }] })).toEqual({})
	})

	it('maps selector and secret rows without raw JSON fields', () => {
		expect(keyValueRows({ os: 'linux', labels: { gpu: true } })).toEqual([{ name: 'labels', value: '{"gpu":true}' }, { name: 'os', value: 'linux' }])
		expect(keyValueObject([{ name: ' os ', value: 'linux' }, { name: 'count', value: '2' }], true)).toEqual({ os: 'linux', count: 2 })
		expect(secretRows({ TOKEN: 'secret-1' })).toEqual([{ name: 'TOKEN', secretId: 'secret-1' }])
		expect(secretObject([{ name: ' TOKEN ', secretId: ' secret-1 ' }])).toEqual({ TOKEN: 'secret-1' })
		const errors = validateTaskDraft({ ...emptyTaskDraft, name: 'Nightly', command: 'echo', pool: 'default', secrets: [{ name: 'TOKEN', secretId: '' }, { name: 'TOKEN', secretId: 'secret-1' }] })
		expect(errors['secrets.0.secretId']).toContain('Select')
		expect(errors['secrets.1.name']).toContain('unique')
	})
})
