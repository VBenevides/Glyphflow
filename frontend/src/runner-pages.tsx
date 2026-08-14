import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Runner } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { hasPermission } from './permissions'

export function runnerIsStale(lastHeartbeat?: string, now = Date.now(), thresholdMs = 60_000) {
  return Boolean(lastHeartbeat && now - Date.parse(lastHeartbeat) > thresholdMs)
}

function hasActiveRunners(data?: Page<Runner>) {
  return Boolean(data?.items.some((runner) => ['online', 'busy', 'draining', 'starting'].includes((runner.observedState ?? '').toLowerCase())))
}

export function RunnerInventoryPage() {
  const { permissions } = useAuth(); const navigate = useNavigate(); const [search, setSearch] = useState(''); const [page, setPage] = useState(1)
  const debouncedSearch = useDebouncedValue(search)
  const query = useQuery({ queryKey: ['runners', debouncedSearch, page], queryFn: ({ signal }) => api.get<Page<Runner>>('/api/v1/runners', { search: debouncedSearch || undefined, page }, signal), refetchInterval: (current) => hasActiveRunners(current.state.data as Page<Runner> | undefined) ? 15_000 : false })
  const manage = hasPermission(permissions, 'runners.manage')
  return <main className="gf-content"><PageHeader title="Runners and pools" description="Capacity, sessions, capabilities, and lifecycle state." /><div className="gf-filter-bar"><label>Search<Input value={search} onChange={(event) => { setSearch(event.target.value); setPage(1) }} placeholder="Name or pool" /></label></div><QueryState query={query} empty="Enroll a runner to execute tasks.">{(data) => data.items.length ? <><DataTable caption="Runners" rows={data.items} columns={[{ key: 'name', label: 'Runner', render: (runner) => <Link to={`/runners/${runner.id}`}>{runner.name}</Link> }, { key: 'pool', label: 'Pool' }, { key: 'desiredState', label: 'Desired', render: (runner) => <StatusPill status={runner.desiredState ?? '—'} /> }, { key: 'observedState', label: 'Observed', render: (runner) => <StatusPill status={runner.observedState ?? '—'} /> }, { key: 'capacity', label: 'Capacity', render: (runner) => `${runner.activeCount ?? 0}/${runner.capacity ?? 0}` }, { key: 'heartbeatAt', label: 'Heartbeat', render: (runner) => runnerIsStale(runner.heartbeatAt) ? <span className="gf-stale-warning">Stale</span> : runner.heartbeatAt ?? '—' }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No runners">Enroll a runner to execute tasks.</EmptyState>}</QueryState>{manage && <Button onClick={() => navigate('/runners/enroll')}>Enroll runner</Button>}</main>
}

export function RunnerDetailPage() {
  const { runnerId = '' } = useParams(); const navigate = useNavigate(); const { permissions } = useAuth(); const query = useQuery({ queryKey: ['runner', runnerId], queryFn: ({ signal }) => api.get<Runner>(`/api/v1/runners/${encodeURIComponent(runnerId)}`, undefined, signal), enabled: Boolean(runnerId) }); const manage = hasPermission(permissions, 'runners.manage')
  const action = (state: string) => api.post(`/api/v1/runners/${encodeURIComponent(runnerId)}/${state}`).then(() => { void query.refetch() })
  return <main className="gf-content"><QueryState query={query}>{(runner) => <><PageHeader title={runner.name} description={`Pool ${runner.pool ?? '—'} · ${runner.observedState ?? '—'}`} /><section className="gf-metric-grid"><div className="gf-metric"><span>Desired state</span><strong><StatusPill status={runner.desiredState ?? '—'} /></strong></div><div className="gf-metric"><span>Observed state</span><strong><StatusPill status={runner.observedState ?? '—'} /></strong></div><div className="gf-metric"><span>Capacity</span><strong>{runner.activeCount ?? 0}/{runner.capacity ?? 0}</strong><small>{runnerIsStale(runner.heartbeatAt) ? 'Heartbeat stale' : 'Heartbeat current'}</small></div></section><section className="gf-card-panel"><h2>Lifecycle</h2><div className="gf-dialog-actions">{manage && <><DangerousAction label="Drain" variant="secondary" onConfirm={() => action('drain')} onConflict={() => query.refetch()} /><DangerousAction label="Revoke" onConfirm={() => action('revoke')} onConflict={() => query.refetch()} /><DangerousAction label="Delete" warning="Permanently deletes this runner and its enrollment, session, and key data. Existing execution history may block deletion." onConfirm={() => api.delete(`/api/v1/runners/${encodeURIComponent(runnerId)}`).then(() => navigate('/runners'))} /></>}</div></section><section className="gf-card-panel"><h2>Sessions, capabilities, and attempts</h2><p className="gf-muted">Session boot IDs, capabilities, key state, and recent attempts are loaded from the runner API.</p></section></>}</QueryState></main>
}
