import { describe, expect, it } from 'vitest'
import { formatMetricPercent, runnerIsRevoked, runnerIsStale, runnerMetricWindow } from './runner-pages'

describe('runner health', () => {
  it('flags stale heartbeats and accepts current ones', () => {
    const now = Date.parse('2026-01-01T00:00:00Z')
    expect(runnerIsStale('2025-12-31T23:58:00Z', now)).toBe(true)
    expect(runnerIsStale('2025-12-31T23:59:30Z', now)).toBe(false)
    expect(runnerIsStale(undefined, now)).toBe(false)
  })

  it('recognizes revoked desired or observed state', () => {
    expect(runnerIsRevoked({ desiredState: 'DISABLED', observedState: 'REVOKED' })).toBe(true)
    expect(runnerIsRevoked({ desiredState: 'REVOKED', observedState: 'OFFLINE' })).toBe(true)
    expect(runnerIsRevoked({ desiredState: 'ENABLED', observedState: 'ONLINE' })).toBe(false)
  })
})

describe('runner resource metrics', () => {
  it('formats current percentages and selects the requested graph range', () => {
    expect(formatMetricPercent(12.345)).toBe('12.3%')
    expect(formatMetricPercent(undefined)).toBe('—')
    expect(runnerMetricWindow('1h', Date.parse('2026-01-01T01:00:00Z'))).toEqual({ from: '2026-01-01T00:00:00.000Z', to: '2026-01-01T01:00:00.000Z' })
    expect(runnerMetricWindow('7d', Date.parse('2026-01-08T00:00:00Z')).from).toBe('2026-01-01T00:00:00.000Z')
  })
})
