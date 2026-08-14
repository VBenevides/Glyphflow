import { describe, expect, it } from 'vitest'
import { runQuery, runStatusLabel } from './run-pages'

describe('run inventory', () => {
  it('builds server-side filters and keeps terminal labels distinct', () => {
    const filters = { task: 't1', runner: '', state: 'UNKNOWN', trigger: '', from: '', to: '' }
    expect(runQuery(filters, 3)).toMatchObject({ task: 't1', state: 'UNKNOWN', page: 3 })
    expect(runStatusLabel('unknown')).toBe('UNKNOWN')
    expect(runStatusLabel('TIMED_OUT')).toBe('TIMED_OUT')
  })
})
