import { createElement, type ComponentType } from 'react'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { canReadRunLogs, eligibleRunActions, hasActiveRuns, ManualRunPage, type ManualRunProps, runImmutableLinks, runQuery, runStatusLabel, RunIDCell, RunTimeline, waitingRunMessage } from './run-pages'
import { isTerminalRunState } from './run-logs'
import { QueryProvider } from './query'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('run inventory', () => {
  it('builds server-side filters and keeps terminal labels distinct', () => {
    const filters = { task: 't1', runner: '', state: 'UNKNOWN', trigger: '', from: '', to: '' }
    expect(runQuery(filters, 3)).toMatchObject({ task: 't1', state: 'UNKNOWN', page: 3 })
    expect(runStatusLabel('unknown')).toBe('UNKNOWN')
    expect(runStatusLabel('TIMED_OUT')).toBe('TIMED_OUT')
    expect(eligibleRunActions('UNKNOWN')).toEqual({ cancel: false, retry: false, reconcile: true })
    expect(eligibleRunActions('DISPATCHED').cancel).toBe(true)
    expect(eligibleRunActions('RUNNING').cancel).toBe(true)
    expect(hasActiveRuns({ items: [{ id: 'r1', state: 'RUNNING' }], page: 1, limit: 20 })).toBe(true)
    expect(hasActiveRuns({ items: [{ id: 'r-dispatched', state: 'DISPATCHED' }], page: 1, limit: 20 })).toBe(true)
    expect(hasActiveRuns({ items: [{ id: 'r2', state: 'SUCCEEDED' }], page: 1, limit: 20 })).toBe(false)
    expect(canReadRunLogs(['runs.read'])).toBe(false)
    expect(canReadRunLogs(['runs.read', 'logs.read'])).toBe(true)
    expect(isTerminalRunState('SUCCEEDED')).toBe(true)
    expect(isTerminalRunState('RUNNING')).toBe(false)
  })

  it('keeps runner discovery on the bounded runs query', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/run-pages.tsx'), 'utf8')
    expect(source).not.toContain("queryKey: ['run-filter-options']")
    expect(source).not.toContain('{ all: true }')
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
    for (const value of ['Attempt timeline', 'runner-1', 'started', 'session-1', 'ACTIVE', 'operator stop', 'stdout', '3–4', '{&quot;pid&quot;:7}']) expect(markup).toContain(value)
  })

  it('renders a placement blocker for waiting runs', () => {
    expect(waitingRunMessage({ state: 'WAITING', placementBlocker: 'All matching runners are offline.' })).toBe('All matching runners are offline.')
    expect(waitingRunMessage({ state: 'SUCCEEDED', placementBlocker: 'stale' })).toBe('')
  })

  it('keeps compact run IDs hoverable and copyable', () => {
    const markup = renderToStaticMarkup(createElement(MemoryRouter, null, createElement(RunIDCell, { id: 'run-123', compact: true })))
    expect(markup).toContain('gf-run-id-cell-compact')
    expect(markup).toContain('title="run-123"')
    expect(markup).toContain('aria-label="Copy run ID"')
  })

  it('links only available immutable run versions', () => {
    expect(runImmutableLinks({ taskId: 'task/1', taskVersionId: 'task/1-v2', scheduleId: 'schedule/1', scheduleVersionId: 'schedule/1-v3' })).toEqual([
      { label: 'Task version', to: '/tasks/task%2F1?version=task%2F1-v2' },
      { label: 'Schedule version', to: '/schedules/schedule%2F1/edit?version=schedule%2F1-v3' },
    ])
    expect(runImmutableLinks({ taskId: 'task-1' })).toEqual([])
  })

  it('renders manual runs in a focused dialog with a preselected task', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    act(() => root.render(createElement(QueryProvider, null, createElement(MemoryRouter, null, createElement(ManualRunPage as ComponentType<ManualRunProps>, { inDialog: true, initialTaskId: 'task-1' })))) )
    expect(document.body.innerHTML).toContain('role="dialog"')
    expect(document.body.innerHTML).toContain('Start manual run')
    expect(document.body.innerHTML).toContain('value="task-1"')
    act(() => root.unmount())
    host.remove()
  })
})
