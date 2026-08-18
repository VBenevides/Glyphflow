import { describe, expect, it } from 'vitest'
import { formatDateTime, normalizeUtcDateTimeFilter } from './format'

describe('formatDateTime', () => {
  it('renders ISO timestamps as UTC date and time', () => {
    const raw = '2026-08-16T16:02:03.757679Z'
    expect(formatDateTime(raw)).toBe('2026-08-16 16:02 UTC')
  })

  it('keeps missing and invalid values safe', () => {
    expect(formatDateTime()).toBe('—')
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })

  it('converts the explicit UTC filter format to RFC3339', () => {
    expect(normalizeUtcDateTimeFilter('2026-08-16 16:02 UTC')).toBe('2026-08-16T16:02:00Z')
    expect(normalizeUtcDateTimeFilter('')).toBe('')
  })
})
