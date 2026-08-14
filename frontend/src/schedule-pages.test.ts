import { describe, expect, it } from 'vitest'
import { emptyScheduleDraft, previewPayload, scheduleDraftFromRecord, validateScheduleDraft } from './schedule-pages'

describe('schedule pages', () => {
  it('validates policy fields and sends explicit timezone preview data', () => {
    expect(Object.keys(validateScheduleDraft(emptyScheduleDraft))).toEqual(['taskId', 'name'])
    const draft = { ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: 'America/Sao_Paulo' }
    expect(validateScheduleDraft(draft)).toEqual({})
		 expect(previewPayload(draft)).toMatchObject({ task_id: 'task-1', schedule_type: 'cron', timezone: 'America/Sao_Paulo' })
	})

	it('loads an edit baseline from the schedule record', () => {
		const draft = scheduleDraftFromRecord({ id: 'schedule-1', name: 'Hourly', taskId: 'task-1', timezone: 'America/Sao_Paulo', expression: '*/5 * * * *', scheduleType: 'cron', catchupLimit: 2 })
		expect(draft.taskId).toBe('task-1')
		expect(draft.timezone).toBe('America/Sao_Paulo')
		expect(draft.expression).toBe('*/5 * * * *')
		expect(draft.catchupLimit).toBe('2')
	})
})
