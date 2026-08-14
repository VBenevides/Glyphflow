import { describe, expect, it } from 'vitest'
import { emptyScheduleDraft, previewPayload, validateScheduleDraft } from './schedule-pages'

describe('schedule pages', () => {
  it('validates policy fields and sends explicit timezone preview data', () => {
    expect(Object.keys(validateScheduleDraft(emptyScheduleDraft))).toEqual(['taskId', 'name'])
    const draft = { ...emptyScheduleDraft, taskId: 'task-1', name: 'Hourly', timezone: 'America/Sao_Paulo' }
    expect(validateScheduleDraft(draft)).toEqual({})
    expect(previewPayload(draft)).toMatchObject({ task_id: 'task-1', schedule_type: 'cron', timezone: 'America/Sao_Paulo' })
  })
})
