import { describe, expect, it } from 'vitest'
import { formatDateTime } from './format'

describe('formatDateTime', () => {
  it('renders ISO timestamps as readable local date and time', () => {
    const raw = '2026-08-16T16:02:03.757679Z'
    expect(formatDateTime(raw)).not.toBe(raw)
    expect(formatDateTime(raw)).toMatch(/2026/)
  })

  it('keeps missing and invalid values safe', () => {
    expect(formatDateTime()).toBe('—')
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })
})
