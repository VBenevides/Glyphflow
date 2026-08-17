import { describe, expect, it } from 'vitest'
import { emptyScheduleDraft, previewPayload, scheduleDraftFromRecord, timezoneFromUTCOffset, validateScheduleDraft } from './schedule-pages'

describe('schedule pages', () => {
	it('uses a whole-hour UTC offset from -23 to +23', () => {
		expect(emptyScheduleDraft.timezone).toBe('0')
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '-23' }).timezone).toBeUndefined()
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '$ENV:UTC_OFFSET' }).timezone).toBeUndefined()
		expect(validateScheduleDraft({ ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: '24' }).timezone).toContain('-23 to +23')
		expect(timezoneFromUTCOffset('3')).toBe('UTC+03:00')
	})

  it('validates policy fields and sends explicit timezone preview data', () => {
    expect(Object.keys(validateScheduleDraft(emptyScheduleDraft))).toEqual(['taskId', 'name'])
    const draft = { ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: 'America/Sao_Paulo' }
    expect(validateScheduleDraft(draft)).toEqual({})
		expect(previewPayload(draft)).toMatchObject({ task_id: 'task-1', expression: '0 * * * *', timezone: 'America/Sao_Paulo' })
	})

	it('loads an edit baseline from the schedule record', () => {
		const draft = scheduleDraftFromRecord({ id: 'schedule-1', name: 'Hourly', taskId: 'task-1', timezone: 'America/Sao_Paulo', expression: '*/5 * * * *', catchupLimit: 2 })
		expect(draft.taskId).toBe('task-1')
		expect(draft.timezone).toBe('America/Sao_Paulo')
		expect(draft.expression).toBe('*/5 * * * *')
		expect(draft.catchupLimit).toBe('2')
	})
})
