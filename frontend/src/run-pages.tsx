import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run } from './api'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'
import { isTerminalRunState, LiveLogPanel } from './run-logs'
import { DangerousAction } from './actions'
import { queryClient } from './query'
import { TaskPicker } from './task-picker'
import { Copy } from 'lucide-react'

export function runQuery(filters: { task: string; runner: string; state: string; trigger: string; from: string; to: string }, page: number) {
  return { task: filters.task || undefined, runner: filters.runner || undefined, state: filters.state || undefined, trigger: filters.trigger || undefined, from: filters.from || undefined, to: filters.to || undefined, page }
}

export function runStatusLabel(state: string) {
  return ['UNKNOWN', 'FAILED', 'TIMED_OUT'].includes(state.toUpperCase()) ? state.toUpperCase() : state
}

export function eligibleRunActions(state: string) {
  const normalized = state.toUpperCase()
  return { cancel: ['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(normalized), retry: ['FAILED', 'TIMED_OUT'].includes(normalized), reconcile: normalized === 'UNKNOWN' }
}

export function hasActiveRuns(data?: Page<Run>) {
  return Boolean(data?.items.some((run) => ['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(run.state.toUpperCase())))
}

export function canReadRunLogs(permissions: Iterable<string>) {
  return hasPermission(permissions, 'logs.read')
}

export function waitingRunMessage(run: Pick<Run, 'state' | 'placementBlocker'>) {
  return run.state.toUpperCase() === 'WAITING' ? run.placementBlocker ?? '' : ''
}

export function RunIDCell({ id, compact = false }: { id: string; compact?: boolean }) {
  return <div className={`gf-run-id-cell${compact ? ' gf-run-id-cell-compact' : ''}`}><Link className="gf-run-id" to={`/runs/${id}`} title={id}>{id}</Link><Button variant="ghost" className="gf-run-id-copy" aria-label="Copy run ID" onClick={() => { void navigator.clipboard?.writeText(id).catch(() => undefined) }}><Copy size={15} aria-hidden="true" /></Button></div>
}

export function RunInventoryPage() {
  const navigate = useNavigate(); const { permissions } = useAuth(); const [filters, setFilters] = useState({ task: '', runner: '', state: '', trigger: '', from: '', to: '' }); const [page, setPage] = useState(1)
  const active = ['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(filters.state.toUpperCase())
  const query = useQuery({ queryKey: ['runs', filters, page], queryFn: ({ signal }) => api.get<Page<Run>>('/api/v1/runs', runQuery(filters, page), signal), refetchInterval: (current) => hasActiveRuns(current.state.data as Page<Run> | undefined) || active ? 5_000 : false })
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content"><PageHeader title="Runs" description="Inspect attempts, state transitions, and external effects." action={hasPermission(permissions, 'runs.execute') && <Button onClick={() => navigate('/runs/execute')}>Start manual run</Button>} /><div className="gf-filter-bar"><TaskPicker value={filters.task} onChange={(value) => update('task', value)} label="Task" /><label>Runner<Input value={filters.runner} onChange={(event) => update('runner', event.target.value)} /></label><label>State<select className="gf-input" value={filters.state} onChange={(event) => update('state', event.target.value)}><option value="">All</option>{['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'TIMED_OUT', 'CANCELLED', 'UNKNOWN'].map((state) => <option key={state}>{state}</option>)}</select></label><label>Trigger<select className="gf-input" value={filters.trigger} onChange={(event) => update('trigger', event.target.value)}><option value="">All</option><option>SCHEDULE</option><option>MANUAL</option><option>RETRY</option></select></label><label>From<input className="gf-input" type="datetime-local" value={filters.from} onChange={(event) => update('from', event.target.value)} /></label><label>To<input className="gf-input" type="datetime-local" value={filters.to} onChange={(event) => update('to', event.target.value)} /></label></div><QueryState query={query} empty="No runs match these filters.">{(data) => data.items.length ? <><DataTable caption="Runs" rows={data.items} columns={[{ key: 'id', label: 'Run', render: (run) => <RunIDCell id={run.id} /> }, { key: 'taskName', label: 'Task', render: (run) => run.taskName ?? run.taskId ?? '—' }, { key: 'trigger', label: 'Trigger' }, { key: 'state', label: 'State', render: (run) => <StatusPill status={runStatusLabel(run.state)} /> }, { key: 'exitCode', label: 'Exit Code', render: (run) => run.exitCode ?? '—' }, { key: 'attempt', label: 'Attempt' }, { key: 'runner', label: 'Runner' }, { key: 'scheduledFor', label: 'Scheduled' }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No matching runs">Try a wider time range or remove a filter.</EmptyState>}</QueryState></main>
}

export function RunDetailPage() {
  const { runId = '' } = useParams(); const { permissions } = useAuth(); const query = useQuery({ queryKey: ['run', runId], queryFn: ({ signal }) => api.get<Run>(`/api/v1/runs/${encodeURIComponent(runId)}`, undefined, signal), enabled: Boolean(runId), refetchInterval: (current) => ['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(String(current.state.data?.state ?? '').toUpperCase()) ? 5_000 : false }); const canCancel = hasPermission(permissions, 'runs.cancel'); const canRetry = hasPermission(permissions, 'runs.retry'); const canReadLogs = canReadRunLogs(permissions)
  return <main className="gf-content"><QueryState query={query}>{(run) => <><PageHeader title={`Run ${run.id}`} description={`Task ${run.taskName ?? run.taskId ?? '—'} · ${run.trigger ?? '—'}`} />{waitingRunMessage(run) && <p className="gf-form-error" role="status">{waitingRunMessage(run)}</p>}<section className="gf-metric-grid"><div className="gf-metric"><span>State</span><strong><StatusPill status={runStatusLabel(run.state)} /></strong></div><div className="gf-metric"><span>Attempt</span><strong>{run.attempt ?? '—'}</strong></div><div className="gf-metric"><span>Runner</span><strong>{run.runner ?? '—'}</strong></div><div className="gf-metric"><span>Exit Code</span><strong>{run.exitCode ?? '—'}</strong></div><div className="gf-metric"><span>Exit Code Meaning</span><strong>{run.exitCodeMeaning ?? '—'}</strong></div></section><section className="gf-card-panel"><h2>Immutable references</h2><div className="gf-related-links"><Link to={`/tasks/${run.taskId ?? ''}`}>Task version</Link><Link to={`/schedules?run=${encodeURIComponent(run.id)}`}>Schedule version</Link><Link to={`/audit?target=${encodeURIComponent(run.id)}`}>Audit events</Link></div></section><RunActionPanel run={run} canCancel={canCancel} canRetry={canRetry} /><RunTimeline run={run} />{canReadLogs ? <><LiveLogPanel runId={run.id} stream="stdout" terminal={isTerminalRunState(run.state)} /><LiveLogPanel runId={run.id} stream="stderr" terminal={isTerminalRunState(run.state)} /></> : <section className="gf-card-panel"><h2>Logs</h2><p className="gf-muted">Log access is not granted for this account.</p></section>}</>}</QueryState></main>
}

function timelineValue(value: unknown) {
  if (value === undefined || value === null || value === '') return '—'
  return typeof value === 'object' ? JSON.stringify(value) : String(value)
}

export function RunTimeline({ run }: { run: Run }) {
  const attempts = run.attempts ?? []
  const events = run.events ?? []
  const sessions = run.sessions ?? []
  const leases = run.leases ?? []
  return <section className="gf-card-panel"><h2>Attempt timeline</h2>{attempts.length ? <ol className="gf-dashboard-list">{attempts.map((attempt, index) => <li key={attempt.id ?? index}><span><strong>Attempt {attempt.attemptNumber ?? index + 1}</strong> <StatusPill status={attempt.state} /></span><small>Runner {timelineValue(attempt.runnerId)} · Session {timelineValue(attempt.runnerSessionId)} · Fencing {timelineValue(attempt.fencingToken)}</small><small>Dispatched {timelineValue(attempt.dispatchedAt)} · Started {timelineValue(attempt.startedAt)} · Finished {timelineValue(attempt.finishedAt)}</small></li>)}</ol> : <p className="gf-muted">No attempts returned.</p>}<h3>Events</h3>{events.length ? <ul className="gf-dashboard-list">{events.map((event, index) => <li key={event.eventId ?? event.id ?? index}><span><strong>{timelineValue(event.eventKind)}</strong> · Attempt {timelineValue(event.attemptId)}</span><small>Sequence {timelineValue(event.stateSequence)} · {timelineValue(event.reportedAt)}</small>{event.payload !== undefined && <small>{timelineValue(event.payload)}</small>}</li>)}</ul> : <p className="gf-muted">No state events returned.</p>}<h3>Sessions</h3>{sessions.length ? <ul className="gf-dashboard-list">{sessions.map((session) => <li key={session.id}><span><strong>{session.id}</strong> · Runner {timelineValue(session.runnerId)} · Boot {timelineValue(session.bootId)}</span><small>Connected {timelineValue(session.connectedAt)} · Heartbeat {timelineValue(session.lastHeartbeatAt)} · Disconnected {timelineValue(session.disconnectedAt)}</small></li>)}</ul> : <p className="gf-muted">No runner sessions returned.</p>}<h3>Leases</h3>{leases.length ? <ul className="gf-dashboard-list">{leases.map((lease) => <li key={lease.id}><span><strong>{timelineValue(lease.resourceName ?? lease.resourceId)}</strong> · <StatusPill status={lease.state} /></span><small>Fencing {timelineValue(lease.fencingToken)} · Acquired {timelineValue(lease.acquiredAt)} · Expires {timelineValue(lease.expiresAt)} · Released {timelineValue(lease.releasedAt)}</small></li>)}</ul> : <p className="gf-muted">No resource leases returned.</p>}<h3>Cancellation</h3>{run.cancellation ? <p>{timelineValue(run.cancellation.state)} · {timelineValue(run.cancellation.reason)} · Requested {timelineValue(run.cancellation.requestedAt)} · Acknowledged {timelineValue(run.cancellation.acknowledgedAt)}</p> : <p className="gf-muted">No cancellation requested.</p>}<h3>Log gaps</h3>{run.logGaps?.length ? <ul className="gf-dashboard-list">{run.logGaps.map((gap, index) => <li key={`${gap.stream}-${gap.fromSequence}-${gap.toSequence}-${index}`}><strong>{gap.stream}</strong>: sequences {gap.fromSequence}–{gap.toSequence}</li>)}</ul> : <p className="gf-muted">No log gaps detected.</p>}</section>
}

function RunActionPanel({ run, canCancel, canRetry }: { run: Run; canCancel: boolean; canRetry: boolean }) {
  const actions = eligibleRunActions(run.state)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['run', run.id] })
  return <section className="gf-card-panel"><h2>Actions</h2><div className="gf-dialog-actions">{canCancel && actions.cancel && <DangerousAction label="Cancel" reasonRequired onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/cancel`, { reason }).then(refresh)} onConflict={refresh} />}{canRetry && actions.retry && <DangerousAction label="Retry" variant="secondary" reasonRequired onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/retry`, { reason }).then(refresh)} onConflict={refresh} />}{canRetry && actions.reconcile && <DangerousAction label="Reconcile unknown" variant="secondary" reasonRequired warning="This can cause an external command to run again. Confirm the side effect is understood." onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/reconcile`, { reason }).then(refresh)} onConflict={refresh} />}</div></section>
}

export function ManualRunPage() {
  const navigate = useNavigate(); const [taskId, setTaskId] = useState(''); const [reason, setReason] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!taskId.trim()) { setError('Task ID is required.'); return }; setBusy(true); setError(''); try { const result = await api.post<{ id?: string }>('/api/v1/runs/execute', { task_id: taskId.trim(), reason: reason.trim() || undefined }); navigate(result.id ? `/runs/${result.id}` : '/runs') } catch (cause) { setError(cause instanceof Error ? cause.message : 'Unable to start run') } finally { setBusy(false) } }
  return <main className="gf-content"><PageHeader title="Start manual run" description="Manual execution uses the active immutable task version." /><form className="gf-editor-form" onSubmit={submit}><TaskPicker value={taskId} onChange={setTaskId} label="Task" required /><label>Reason (optional)<Input value={reason} onChange={(event) => setReason(event.target.value)} /></label>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate('/runs')}>Cancel</Button><Button type="submit" busy={busy}>Start run</Button></div></form></main>
}
