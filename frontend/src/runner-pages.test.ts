import { describe, expect, it } from 'vitest'
import { runnerIsStale } from './runner-pages'

describe('runner health', () => {
  it('flags stale heartbeats and accepts current ones', () => {
    const now = Date.parse('2026-01-01T00:00:00Z')
    expect(runnerIsStale('2025-12-31T23:58:00Z', now)).toBe(true)
    expect(runnerIsStale('2025-12-31T23:59:30Z', now)).toBe(false)
    expect(runnerIsStale(undefined, now)).toBe(false)
  })
})
