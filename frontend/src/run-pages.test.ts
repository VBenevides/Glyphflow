import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { eligibleRunActions, hasActiveRuns, runQuery, runStatusLabel, RunTimeline } from './run-pages'
import { isTerminalRunState } from './run-logs'

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
    expect(isTerminalRunState('SUCCEEDED')).toBe(true)
    expect(isTerminalRunState('RUNNING')).toBe(false)
  })

  it('renders the detail timeline payload', () => {
    const markup = renderToStaticMarkup(createElement(RunTimeline, { run: {
      id: 'run-1', state: 'RUNNING', attempts: [{ id: 'attempt-1', attemptNumber: 1, state: 'RUNNING', runnerId: 'runner-1', runnerSessionId: 'session-1', fencingToken: 4, dispatchedAt: '2026-08-14T12:00:00Z' }],
      events: [{ eventId: 'event-1', attemptId: 'attempt-1', eventKind: 'started', stateSequence: 2, reportedAt: '2026-08-14T12:00:01Z', payload: { pid: 7 } }],
      sessions: [{ id: 'session-1', runnerId: 'runner-1', bootId: 'boot-1', connectedAt: '2026-08-14T11:59:00Z' }],
      leases: [{ id: 'lease-1', resourceId: 'resource-1', state: 'ACTIVE', fencingToken: 5, expiresAt: '2026-08-14T12:05:00Z' }],
      cancellation: { state: 'REQUESTED', reason: 'operator stop', requestedAt: '2026-08-14T12:00:02Z' },
      logGaps: [{ stream: 'stdout', fromSequence: 3, toSequence: 4 }],
    } }))
    for (const value of ['Attempt timeline', 'runner-1', 'started', 'session-1', 'ACTIVE', 'operator stop', 'stdout', '3–4']) expect(markup).toContain(value)
  })
})
