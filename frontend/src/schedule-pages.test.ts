import { describe, expect, it } from 'vitest'
import { emptyScheduleDraft, previewPayload, scheduleDraftFromRecord, timezoneFromUTCOffset, utcOffsetFromTimezone, validateScheduleDraft } from './schedule-pages'

describe('schedule pages', () => {
	it('uses a whole-hour UTC offset from -23 to +23', () => {
		expect(emptyScheduleDraft.timezone).toBe('0')
		expect(emptyScheduleDraft.deadlineSeconds).toBe('60')
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '-23' }).timezone).toBeUndefined()
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '$ENV:UTC_OFFSET' }).timezone).toBeUndefined()
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '24' }).timezone).toContain('-23 to +23')
		expect(timezoneFromUTCOffset('3')).toBe('UTC+03:00')
		expect(utcOffsetFromTimezone('Europe/Lisbon')).toBe('Europe/Lisbon')
	})

	it('validates policy fields and sends explicit timezone preview data', () => {
		expect(Object.keys(validateScheduleDraft(emptyScheduleDraft))).toEqual(['taskId', 'name'])
		const draft = { ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: 'America/Sao_Paulo' }
		expect(validateScheduleDraft(draft)).toEqual({})
		expect(validateScheduleDraft({ ...draft, deadlineSeconds: '29' }).deadlineSeconds).toContain('30')
		expect(validateScheduleDraft({ ...draft, deadlineSeconds: '30' })).toEqual({})
		expect(previewPayload(draft)).toMatchObject({ task_id: 'task-1', expression: '0 * * * *', timezone: 'America/Sao_Paulo' })
	})

	it('loads an edit baseline from the schedule record', () => {
		const draft = scheduleDraftFromRecord({ id: 'schedule-1', name: 'Hourly', taskId: 'task-1', timezone: 'America/Sao_Paulo', expression: '*/5 * * * *', catchupLimit: 2 })
		expect(draft.taskId).toBe('task-1')
		expect(draft.timezone).toBe('America/Sao_Paulo')
		expect(draft.expression).toBe('*/5 * * * *')
		expect(draft.catchupLimit).toBe('2')
		expect(draft.deadlineSeconds).toBe('60')
		expect(scheduleDraftFromRecord({ id: 'schedule-2', name: 'Fast', taskId: 'task-1', timezone: 'UTC+02:00', expression: '* * * * *', deadlineSeconds: 45 }).deadlineSeconds).toBe('45')
	})
})
