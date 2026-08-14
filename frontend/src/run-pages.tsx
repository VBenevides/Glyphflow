import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run } from './api'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'
import { LiveLogPanel } from './run-logs'

export function runQuery(filters: { task: string; runner: string; state: string; trigger: string; from: string; to: string }, page: number) {
  return { task: filters.task || undefined, runner: filters.runner || undefined, state: filters.state || undefined, trigger: filters.trigger || undefined, from: filters.from || undefined, to: filters.to || undefined, page }
}

export function runStatusLabel(state: string) {
  return ['UNKNOWN', 'FAILED', 'TIMED_OUT'].includes(state.toUpperCase()) ? state.toUpperCase() : state
}

export function RunInventoryPage() {
  const navigate = useNavigate(); const { permissions } = useAuth(); const [filters, setFilters] = useState({ task: '', runner: '', state: '', trigger: '', from: '', to: '' }); const [page, setPage] = useState(1)
  const active = ['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(filters.state.toUpperCase())
  const query = useQuery({ queryKey: ['runs', filters, page], queryFn: ({ signal }) => api.get<Page<Run>>('/api/v1/runs', runQuery(filters, page), signal), refetchInterval: active ? 5_000 : false })
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content"><PageHeader title="Runs" description="Inspect attempts, state transitions, and external effects." action={hasPermission(permissions, 'runs.execute') && <Button onClick={() => navigate('/runs/execute')}>Start manual run</Button>} /><div className="gf-filter-bar"><label>Task<Input value={filters.task} onChange={(event) => update('task', event.target.value)} /></label><label>Runner<Input value={filters.runner} onChange={(event) => update('runner', event.target.value)} /></label><label>State<select className="gf-input" value={filters.state} onChange={(event) => update('state', event.target.value)}><option value="">All</option>{['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'TIMED_OUT', 'CANCELLED', 'UNKNOWN'].map((state) => <option key={state}>{state}</option>)}</select></label><label>Trigger<select className="gf-input" value={filters.trigger} onChange={(event) => update('trigger', event.target.value)}><option value="">All</option><option>SCHEDULE</option><option>MANUAL</option><option>RETRY</option></select></label><label>From<input className="gf-input" type="datetime-local" value={filters.from} onChange={(event) => update('from', event.target.value)} /></label><label>To<input className="gf-input" type="datetime-local" value={filters.to} onChange={(event) => update('to', event.target.value)} /></label></div><QueryState query={query} empty="No runs match these filters.">{(data) => data.items.length ? <><DataTable caption="Runs" rows={data.items} columns={[{ key: 'id', label: 'Run', render: (run) => <Link to={`/runs/${run.id}`}>{run.id}</Link> }, { key: 'taskName', label: 'Task', render: (run) => run.taskName ?? run.taskId ?? '—' }, { key: 'trigger', label: 'Trigger' }, { key: 'state', label: 'State', render: (run) => <StatusPill status={runStatusLabel(run.state)} /> }, { key: 'attempt', label: 'Attempt' }, { key: 'runner', label: 'Runner' }, { key: 'scheduledFor', label: 'Scheduled' }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No matching runs">Try a wider time range or remove a filter.</EmptyState>}</QueryState></main>
}

export function RunDetailPage() {
  const { runId = '' } = useParams(); const { permissions } = useAuth(); const query = useQuery({ queryKey: ['run', runId], queryFn: ({ signal }) => api.get<Run>(`/api/v1/runs/${encodeURIComponent(runId)}`, undefined, signal), enabled: Boolean(runId) }); const canCancel = hasPermission(permissions, 'runs.cancel'); const canRetry = hasPermission(permissions, 'runs.retry')
  return <main className="gf-content"><QueryState query={query}>{(run) => <><PageHeader title={`Run ${run.id}`} description={`Task ${run.taskName ?? run.taskId ?? '—'} · ${run.trigger ?? '—'}`} /><section className="gf-metric-grid"><div className="gf-metric"><span>State</span><strong><StatusPill status={runStatusLabel(run.state)} /></strong></div><div className="gf-metric"><span>Attempt</span><strong>{run.attempt ?? '—'}</strong></div><div className="gf-metric"><span>Runner</span><strong>{run.runner ?? '—'}</strong></div></section><section className="gf-card-panel"><h2>Immutable references</h2><div className="gf-related-links"><Link to={`/tasks/${run.taskId ?? ''}`}>Task version</Link><Link to={`/schedules?run=${encodeURIComponent(run.id)}`}>Schedule version</Link><Link to={`/audit?target=${encodeURIComponent(run.id)}`}>Audit events</Link></div></section><section className="gf-card-panel"><h2>Actions</h2><div className="gf-dialog-actions">{canCancel && <Button variant="danger" disabled={!['WAITING', 'RUNNING', 'RETRY_WAIT', 'CANCELLING'].includes(run.state)}>Cancel</Button>}{canRetry && <Button variant="secondary" disabled={!['FAILED', 'TIMED_OUT', 'UNKNOWN'].includes(run.state)}>Retry</Button>}</div></section><section className="gf-card-panel"><h2>Attempts, events, and leases</h2><p className="gf-muted">Immutable task/schedule links and attempt-specific runner, session, state-event, and lease details are retained by the API.</p></section><LiveLogPanel runId={run.id} stream="stdout" /><LiveLogPanel runId={run.id} stream="stderr" /></>}</QueryState></main>
}
