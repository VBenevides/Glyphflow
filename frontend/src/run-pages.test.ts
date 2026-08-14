import { describe, expect, it } from 'vitest'
import { eligibleRunActions, hasActiveRuns, runQuery, runStatusLabel } from './run-pages'

describe('run inventory', () => {
  it('builds server-side filters and keeps terminal labels distinct', () => {
    const filters = { task: 't1', runner: '', state: 'UNKNOWN', trigger: '', from: '', to: '' }
    expect(runQuery(filters, 3)).toMatchObject({ task: 't1', state: 'UNKNOWN', page: 3 })
    expect(runStatusLabel('unknown')).toBe('UNKNOWN')
    expect(runStatusLabel('TIMED_OUT')).toBe('TIMED_OUT')
    expect(eligibleRunActions('UNKNOWN')).toEqual({ cancel: false, retry: false, reconcile: true })
    expect(eligibleRunActions('RUNNING').cancel).toBe(true)
    expect(hasActiveRuns({ items: [{ id: 'r1', state: 'RUNNING' }], page: 1, limit: 20 })).toBe(true)
    expect(hasActiveRuns({ items: [{ id: 'r2', state: 'SUCCEEDED' }], page: 1, limit: 20 })).toBe(false)
  })
})
