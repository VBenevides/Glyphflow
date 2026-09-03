import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, HardDrive, ListChecks, Server } from 'lucide-react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run } from './api'
import { Button, DataTable, Dialog, EmptyState, FilterInput, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill } from './components'
import { QueryRefresh, QueryState, queryClient } from './query'
import { hasPermission } from './permissions'
import { isTerminalRunState, LiveLogPanel } from './run-logs'
import { DangerousAction } from './actions'
import { TaskPicker } from './task-picker'
import { formatDateTime, normalizeUtcDateTimeFilter } from './format'

export function runQuery(filters: { task: string; runner: string; state: string; trigger: string; from: string; to: string }, page: number, limit?: number) {
  return { task: filters.task || undefined, runner: filters.runner || undefined, state: filters.state || undefined, trigger: filters.trigger || undefined, from: normalizeUtcDateTimeFilter(filters.from) || undefined, to: normalizeUtcDateTimeFilter(filters.to) || undefined, page, ...(limit ? { limit } : {}) }
}

export function runStatusLabel(state: string) {
  return ['UNKNOWN', 'FAILED', 'TIMED_OUT'].includes(state.toUpperCase()) ? state.toUpperCase() : state
}

function memoryLabel(bytes?: number) {
  return bytes ? `${(bytes / 1024 / 1024).toFixed(2)} MB` : '—'
}

export function eligibleRunActions(state: string) {
  const normalized = state.toUpperCase()
  return { cancel: ['WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(normalized), retry: ['FAILED', 'TIMED_OUT'].includes(normalized), reconcile: normalized === 'UNKNOWN' }
}

export function hasActiveRuns(data?: Page<Run>) {
  return Boolean(data?.items.some((run) => ['WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(run.state.toUpperCase())))
}

export function canReadRunLogs(permissions: Iterable<string>) {
  return hasPermission(permissions, 'logs.read')
}

export function waitingRunMessage(run: Pick<Run, 'state' | 'placementBlocker'>) {
  return run.state.toUpperCase() === 'WAITING' ? run.placementBlocker ?? '' : ''
}

export function RunIDCell({ id, compact = false }: { id: string; compact?: boolean }) {
  return <Identifier id={id} href={`/runs/${id}`} className={`gf-run-id-cell${compact ? ' gf-run-id-cell-compact' : ''}`} linkClassName="gf-run-id" copyLabel="Copy run ID" />
}

export function RunInventoryPage() {
  const { permissions } = useAuth(); const [filters, setFilters] = useState({ task: '', runner: '', state: '', trigger: '', from: '', to: '' }); const [page, setPage] = useState(1); const [limit, setLimit] = useState(10); const [manualRunOpen, setManualRunOpen] = useState(false)
  const active = ['WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(filters.state.toUpperCase())
  const query = useQuery({ queryKey: ['runs', filters, page, limit], queryFn: ({ signal }) => api.get<Page<Run>>('/api/v1/runs', runQuery(filters, page, limit), signal), refetchInterval: (current) => hasActiveRuns(current.state.data as Page<Run> | undefined) || active ? 5_000 : false })
  const runOptions = query.data?.items ?? []
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content"><PageHeader title="Runs" description="Inspect attempts, state transitions, and external effects." refresh={<QueryRefresh query={query} />} /><div className="gf-filter-bar"><TaskPicker value={filters.task} onChange={(value) => update('task', value)} label="Task" /><FilterInput label="Runner" options={runOptions.flatMap((run) => run.runner ? [run.runner] : [])} value={filters.runner} onChange={(value) => update('runner', value)} /><label>State<select className="gf-input" value={filters.state} onChange={(event) => update('state', event.target.value)}><option value="">All</option>{['WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'TIMED_OUT', 'CANCELLED', 'UNKNOWN'].map((state) => <option key={state}>{state}</option>)}</select></label><label>Trigger<select className="gf-input" value={filters.trigger} onChange={(event) => update('trigger', event.target.value)}><option value="">All</option><option>SCHEDULE</option><option>MANUAL</option><option>RETRY</option></select></label><div className="gf-filter-datetime"><label>From (UTC)<Input data-utc-datetime value={filters.from} placeholder="YYYY-mm-dd HH:MM UTC" onChange={(event) => update('from', event.target.value)} /></label><label>To (UTC)<Input data-utc-datetime value={filters.to} placeholder="YYYY-mm-dd HH:MM UTC" onChange={(event) => update('to', event.target.value)} /></label></div></div>{hasPermission(permissions, 'runs.execute') && <div className="gf-table-toolbar"><Button onClick={() => setManualRunOpen(true)}>Start manual run</Button></div>}<QueryState query={query} empty="No runs match these filters.">{(data) => data.items.length ? <><DataTable caption="Runs" rows={data.items} columns={[{ key: 'id', label: 'Run', render: (run) => <RunIDCell id={run.id} /> }, { key: 'taskName', label: 'Task', render: (run) => run.taskName ?? run.taskId ?? '—' }, { key: 'trigger', label: 'Trigger' }, { key: 'state', label: 'State', render: (run) => <StatusPill status={runStatusLabel(run.state)} /> }, { key: 'exitCode', label: 'Exit Code', render: (run) => run.exitCode ?? '—' }, { key: 'attempt', label: 'Attempt' }, { key: 'runner', label: 'Runner' }, { key: 'scheduledFor', label: 'Scheduled', render: (run) => formatDateTime(run.scheduledFor) }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No matching runs">Try a wider time range or remove a filter.</EmptyState>}</QueryState>{manualRunOpen && <ManualRunPage inDialog onClose={() => setManualRunOpen(false)} onStarted={async () => { setManualRunOpen(false); await query.refetch() }} />}</main>
}

export function RunDetailPage() {
  const { runId = '' } = useParams(); const { permissions } = useAuth(); const query = useQuery({ queryKey: ['run', runId], queryFn: ({ signal }) => api.get<Run>(`/api/v1/runs/${encodeURIComponent(runId)}`, undefined, signal), enabled: Boolean(runId), refetchInterval: (current) => ['WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(String(current.state.data?.state ?? '').toUpperCase()) ? 5_000 : false }); const canCancel = hasPermission(permissions, 'runs.cancel'); const canRetry = hasPermission(permissions, 'runs.retry'); const canReadLogs = canReadRunLogs(permissions)
  return <main className="gf-content"><QueryState query={query}>{(run) => <><PageHeader title={`Run ${run.id}`} description={`Task ${run.taskName ?? run.taskId ?? '—'} · ${run.trigger ?? '—'}`} refresh={<QueryRefresh query={query} />} />{waitingRunMessage(run) && <p className="gf-form-error" role="status">{waitingRunMessage(run)}</p>}<section className="gf-metric-grid"><MetricCard label="State" value={<StatusPill status={runStatusLabel(run.state)} />} icon={Activity} /><MetricCard label="Attempt" value={run.attempt ?? '—'} icon={ListChecks} /><MetricCard label="Runner" value={run.runner ?? '—'} icon={Server} /><MetricCard label="Exit Code" value={run.exitCode ?? '—'} icon={Activity} /><MetricCard label="Exit Code Meaning" value={run.exitCodeMeaning ?? '—'} icon={Activity} /><MetricCard label="Max memory used" value={memoryLabel(run.maxMemoryUsedBytes)} icon={HardDrive} /><MetricCard label="Average memory used" value={memoryLabel(run.averageMemoryUsedBytes)} icon={HardDrive} /></section>{run.error && <section className="gf-card-panel"><h2>Execution error</h2><p className="gf-form-error" role="alert">{run.error}</p></section>}<section className="gf-card-panel"><h2>Immutable references</h2><div className="gf-related-links">{runImmutableLinks(run).map((link) => <Link key={link.label} to={link.to}>{link.label}</Link>)}<Link to={`/audit?target=${encodeURIComponent(run.id)}`}>Audit events</Link></div></section><RunActionPanel run={run} canCancel={canCancel} canRetry={canRetry} /><RunTimeline run={run} />{canReadLogs ? <><LiveLogPanel runId={run.id} stream="stdout" terminal={isTerminalRunState(run.state)} /><LiveLogPanel runId={run.id} stream="stderr" terminal={isTerminalRunState(run.state)} /></> : <section className="gf-card-panel"><h2>Logs</h2><p className="gf-muted">Log access is not granted for this account.</p></section>}</>}</QueryState></main>
}

export function runImmutableLinks(run: Pick<Run, 'taskId' | 'taskVersionId' | 'scheduleId' | 'scheduleVersionId'>) {
  return [
    run.taskId && run.taskVersionId ? { label: 'Task version', to: `/tasks/${encodeURIComponent(run.taskId)}?version=${encodeURIComponent(run.taskVersionId)}` } : null,
    run.scheduleId && run.scheduleVersionId ? { label: 'Schedule version', to: `/schedules/${encodeURIComponent(run.scheduleId)}/edit?version=${encodeURIComponent(run.scheduleVersionId)}` } : null,
  ].filter((link): link is { label: string; to: string } => link !== null)
}

function timelineValue(value: unknown) {
  if (value === undefined || value === null || value === '') return '—'
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(value)) return formatDateTime(value)
  return typeof value === 'object' ? JSON.stringify(value) ?? '—' : value.toString()
}

export function RunTimeline({ run }: { run: Run }) {
  const attempts = run.attempts ?? []
  const events = run.events ?? []
  const sessions = run.sessions ?? []
  const leases = run.leases ?? []
  return <section className="gf-card-panel"><h2>Attempt timeline</h2>{attempts.length ? <ol className="gf-dashboard-list">{attempts.map((attempt, index) => <li key={attempt.id ?? index}><span><strong>Attempt {attempt.attemptNumber ?? index + 1}</strong> <StatusPill status={attempt.state} /></span><small>Runner <Identifier id={attempt.runnerId} /> · Session <Identifier id={attempt.runnerSessionId} /> · Fencing {timelineValue(attempt.fencingToken)}</small><small>Dispatched {timelineValue(attempt.dispatchedAt)} · Started {timelineValue(attempt.startedAt)} · Finished {timelineValue(attempt.finishedAt)}</small></li>)}</ol> : <p className="gf-muted">No attempts returned.</p>}<h3>Events</h3>{events.length ? <ul className="gf-dashboard-list">{events.map((event, index) => <li key={event.eventId ?? event.id ?? index}><span><strong>{timelineValue(event.eventKind)}</strong> · Attempt <Identifier id={event.attemptId} /></span><small>Sequence {timelineValue(event.stateSequence)} · {timelineValue(event.reportedAt)}</small>{event.payload !== undefined && <small>{timelineValue(event.payload)}</small>}</li>)}</ul> : <p className="gf-muted">No state events returned.</p>}<h3>Sessions</h3>{sessions.length ? <ul className="gf-dashboard-list">{sessions.map((session) => <li key={session.id}><span><strong><Identifier id={session.id} /></strong> · Runner <Identifier id={session.runnerId} /> · Boot <Identifier id={session.bootId} /></span><small>Connected {timelineValue(session.connectedAt)} · Heartbeat {timelineValue(session.lastHeartbeatAt)} · Disconnected {timelineValue(session.disconnectedAt)}</small></li>)}</ul> : <p className="gf-muted">No runner sessions returned.</p>}<h3>Leases</h3>{leases.length ? <ul className="gf-dashboard-list">{leases.map((lease) => <li key={lease.id}><span><strong><Identifier id={lease.resourceId ?? lease.resourceName} name={lease.resourceId ? lease.resourceName : undefined} copyLabel="Copy resource ID" /></strong> · <StatusPill status={lease.state} /></span><small>Fencing {timelineValue(lease.fencingToken)} · Acquired {timelineValue(lease.acquiredAt)} · Expires {timelineValue(lease.expiresAt)} · Released {timelineValue(lease.releasedAt)}</small></li>)}</ul> : <p className="gf-muted">No resource leases returned.</p>}<h3>Cancellation</h3>{run.cancellation ? <p>{timelineValue(run.cancellation.state)} · {timelineValue(run.cancellation.reason)} · Requested {timelineValue(run.cancellation.requestedAt)} · Acknowledged {timelineValue(run.cancellation.acknowledgedAt)}</p> : <p className="gf-muted">No cancellation requested.</p>}<h3>Log gaps</h3>{run.logGaps?.length ? <ul className="gf-dashboard-list">{run.logGaps.map((gap, index) => <li key={`${gap.stream}-${gap.fromSequence}-${gap.toSequence}-${index}`}><strong>{gap.stream}</strong>: sequences {gap.fromSequence}–{gap.toSequence}</li>)}</ul> : <p className="gf-muted">No log gaps detected.</p>}</section>
}

function RunActionPanel({ run, canCancel, canRetry }: { run: Run; canCancel: boolean; canRetry: boolean }) {
  const actions = eligibleRunActions(run.state)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['run', run.id] })
  return <section className="gf-card-panel"><h2>Actions</h2><div className="gf-dialog-actions">{canCancel && actions.cancel && <DangerousAction label="Cancel" title="Cancel task" cancelLabel="Abandon task cancellation" confirmLabel="Confirm task cancellation" reasonRequired onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/cancel`, { reason }).then(refresh)} onConflict={refresh} />}{canRetry && actions.retry && <DangerousAction label="Retry" variant="secondary" reasonRequired onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/retry`, { reason }).then(refresh)} onConflict={refresh} />}{canRetry && actions.reconcile && <DangerousAction label="Reconcile unknown" variant="secondary" reasonRequired warning="This can cause an external command to run again. Confirm the side effect is understood." onConfirm={(reason) => api.post(`/api/v1/runs/${encodeURIComponent(run.id)}/reconcile`, { reason }).then(refresh)} onConflict={refresh} />}</div></section>
}

export type ManualRunProps = { initialTaskId?: string; inDialog?: boolean; onClose?: () => void; onStarted?: (result: { id?: string }) => void | Promise<void> }

export function ManualRunPage({ initialTaskId = '', inDialog = false, onClose, onStarted }: ManualRunProps = {}) {
  const navigate = useNavigate(); const [taskId, setTaskId] = useState(initialTaskId); const [reason, setReason] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!taskId.trim()) { setError('Task ID is required.'); return }; setBusy(true); setError(''); try { const result = await api.post<{ id?: string }>('/api/v1/runs/execute', { task_id: taskId.trim(), reason: reason.trim() || undefined }); if (onStarted) await onStarted(result); else navigate(result.id ? `/runs/${result.id}` : '/runs') } catch (cause) { setError(cause instanceof Error ? cause.message : 'Unable to start run') } finally { setBusy(false) } }
  const close = () => onClose ? onClose() : navigate('/runs')
  const form = <form className="gf-editor-form" onSubmit={submit}><TaskPicker value={taskId} onChange={setTaskId} label="Task" required /><label>Reason (optional)<Input value={reason} onChange={(event) => setReason(event.target.value)} /></label>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" busy={busy}>Start run</Button></div></form>
  return inDialog ? <Dialog open title="Start manual run" onClose={close}>{form}</Dialog> : <main className="gf-content"><PageHeader title="Start manual run" description="Manual execution uses the active immutable task version." />{form}</main>
}
