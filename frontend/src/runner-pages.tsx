import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, Archive, CircleOff, HardDrive, ListChecks, Server, WifiOff } from 'lucide-react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run, type Runner, type RunnerMetric, type RunnerMetricHistory } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, FilterInput, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill, Tabs, TabsContent, TabsList, TabsTrigger } from './components'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'
import { describeError } from './errors'
import { downloadArtifact, type WorkerUI, workerUIOptions } from './enrollment-page'
import { RunIDCell } from './run-pages'
import { RunnerPoolsPage } from './runner-pools-page'
import { formatDateTime } from './format'

export function runnerIsStale(lastHeartbeat?: string, now = Date.now(), thresholdMs = 60_000) {
  return Boolean(lastHeartbeat && now - Date.parse(lastHeartbeat) > thresholdMs)
}

export function runnerIsRevoked(runner: Pick<Runner, 'desiredState' | 'observedState'>) {
  return [runner.observedState, runner.desiredState].some((state) => state?.toUpperCase() === 'REVOKED')
}

export type RunnerMetricRange = '1h' | '6h' | '24h' | '7d' | '30d'

const runnerMetricRangeHours: Record<RunnerMetricRange, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 24 * 7, '30d': 24 * 30 }

export function runnerMetricWindow(range: RunnerMetricRange, now = Date.now()) {
  const to = new Date(now)
  const from = new Date(now - runnerMetricRangeHours[range] * 60 * 60 * 1000)
  return { from: from.toISOString(), to: to.toISOString() }
}

export function formatMetricPercent(value?: number) {
  return value === undefined ? '—' : `${value.toFixed(1)}%`
}

function metricPoints(items: RunnerMetric[], field: 'cpuPercent' | 'memoryPercent') {
  if (!items.length) return ''
  const width = 720
  const height = 180
  const maxIndex = Math.max(1, items.length - 1)
  return items.map((item, index) => `${(index / maxIndex) * width},${height - (item[field] / 100) * height}`).join(' ')
}

function RunnerMetricChart({ label, items, field }: { label: string; items: RunnerMetric[]; field: 'cpuPercent' | 'memoryPercent' }) {
  if (!items.length) return <p className="gf-muted">No {label.toLowerCase()} samples in this range.</p>
  const latest = items[items.length - 1]
  return <div className="gf-runner-metric-chart"><div className="gf-runner-metric-chart-heading"><strong>{label}</strong><span>{formatMetricPercent(latest[field])} current</span></div><svg viewBox="0 0 720 180" role="img" aria-label={`${label} history`} preserveAspectRatio="none"><polyline points={metricPoints(items, field)} fill="none" stroke="currentColor" strokeWidth="3" vectorEffect="non-scaling-stroke" /><line x1="0" y1="0" x2="0" y2="180" stroke="currentColor" opacity="0.2" /><line x1="0" y1="180" x2="720" y2="180" stroke="currentColor" opacity="0.2" /></svg></div>
}

function hasActiveRunners(data?: Page<Runner>) {
  return Boolean(data?.items.some((runner) => ['online', 'busy', 'draining', 'starting'].includes((runner.observedState ?? '').toLowerCase())))
}

export function RunnerInventoryPage({ view = 'runners' }: { view?: 'runners' | 'pools' } = {}) {
  const { permissions } = useAuth(); const navigate = useNavigate(); const [params] = useSearchParams(); const archived = view === 'runners' && params.get('archived') === 'true'; const [search, setSearch] = useState(''); const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const query = useQuery({ queryKey: ['runners', archived, search, page, limit], queryFn: ({ signal }) => api.get<Page<Runner>>('/api/v1/runners', { search: search || undefined, archived, page, limit }, signal), enabled: view === 'runners', refetchInterval: (current) => view === 'runners' && !archived && hasActiveRunners(current.state.data as Page<Runner> | undefined) ? 15_000 : false })
  const optionsQuery = useQuery({ queryKey: ['runner-filter-options', archived], queryFn: ({ signal }) => api.get<Page<Runner>>('/api/v1/runners', { all: true, archived }, signal), enabled: view === 'runners' })
  const runnerOptions = optionsQuery.data?.items ?? query.data?.items ?? []
  const summaryQuery = useQuery({ queryKey: ['runner-summary'], queryFn: async ({ signal }) => { const [all, disabled, offline, archivedRunners] = await Promise.all([api.get<Page<Runner>>('/api/v1/runners', { page: 1, limit: 1 }, signal), api.get<Page<Runner>>('/api/v1/runners', { page: 1, limit: 1, desired_state: 'DISABLED' }, signal), api.get<Page<Runner>>('/api/v1/runners', { page: 1, limit: 1, state: 'OFFLINE' }, signal), api.get<Page<Runner>>('/api/v1/runners', { page: 1, limit: 1, archived: true }, signal)]); return { total: all.total ?? 0, disabled: disabled.total ?? 0, offline: offline.total ?? 0, archived: archivedRunners.total ?? 0 } }, enabled: view === 'runners', refetchInterval: 5_000 })
  const manage = hasPermission(permissions, 'runners.manage')
  const tabs = <div className="gf-runner-navigation"><nav className="gf-account-tabs" aria-label="Runner sections"><Link className={view === 'runners' ? 'is-active' : ''} to="/runners">Runners</Link><Link className={view === 'pools' ? 'is-active' : ''} to="/runners/pools">Pools</Link></nav>{view === 'runners' && <><span className="gf-runner-navigation-arrow" aria-hidden="true">→</span><nav className="gf-account-tabs" aria-label="Runner status"><Link className={!archived ? 'is-active' : ''} to="/runners">Current Runners</Link><Link className={archived ? 'is-active' : ''} to="/runners?archived=true">Archived Runners</Link></nav></>}</div>
  if (view === 'pools') { return <RunnerPoolsPage navigation={tabs} title="Runners and pools" description="Capacity, sessions, capabilities, and lifecycle state." /> }; return <main className="gf-content"><PageHeader title="Runners and pools" description="Capacity, sessions, capabilities, and lifecycle state." refresh={view === 'runners' ? <QueryRefresh query={query} /> : undefined} />{tabs}<div className="gf-metric-grid"><MetricCard label="Number of runners" value={summaryQuery.data?.total ?? '—'} detail="Active runner registrations" icon={Server} tone="info" /><MetricCard label="Disabled runners" value={summaryQuery.data?.disabled ?? '—'} detail="Runners not accepting work" icon={CircleOff} tone="warning" /><MetricCard label="Offline runners" value={summaryQuery.data?.offline ?? '—'} detail="Runners not reporting" icon={WifiOff} tone={summaryQuery.data?.offline && summaryQuery.data.offline > 0 ? 'danger' : 'default'} /><MetricCard label="Archived runners" value={summaryQuery.data?.archived ?? '—'} detail="Permanently archived registrations" icon={Archive} tone="default" /></div><div className="gf-filter-bar"><FilterInput label="Search" options={runnerOptions.flatMap((runner) => [runner.name, runner.id, runner.pool].filter((value): value is string => Boolean(value)))} value={search} onChange={(value) => { setSearch(value); setPage(1) }} placeholder="Name or pool" /></div>{manage && <div className="gf-table-toolbar"><Button onClick={() => navigate('/runners/enroll')}>Enroll runner</Button></div>}<QueryState query={query} empty={archived ? 'No archived runners.' : 'Enroll a runner to execute tasks.'}>{(data) => data.items.length ? <><DataTable caption={archived ? 'Archived runners' : 'Runners'} rows={data.items} columns={[{ key: 'name', label: 'Runner', render: (runner) => <Identifier id={runner.id} name={runner.name} href={`/runners/${runner.id}`} copyLabel="Copy runner ID" /> }, { key: 'pool', label: 'Pool' }, { key: 'desiredState', label: 'Desired', render: (runner) => <StatusPill status={runner.desiredState ?? '—'} /> }, { key: 'observedState', label: 'Observed', render: (runner) => <StatusPill status={runner.observedState ?? '—'} /> }, { key: 'capacity', label: 'Capacity', render: (runner) => `${runner.activeCount ?? 0}/${runner.capacity ?? 0}` }, { key: 'cpu', label: 'CPU', render: (runner) => runnerIsStale(runner.heartbeatAt) ? 'Stale' : formatMetricPercent(runner.currentMetrics?.cpuPercent) }, { key: 'memory', label: 'Memory', render: (runner) => runnerIsStale(runner.heartbeatAt) ? 'Stale' : formatMetricPercent(runner.currentMetrics?.memoryPercent) }, { key: 'heartbeatAt', label: 'Heartbeat', render: (runner) => runnerIsStale(runner.heartbeatAt) ? <span className="gf-stale-warning">Stale</span> : formatDateTime(runner.heartbeatAt) }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title={archived ? 'No archived runners' : 'No runners'}>{archived ? 'Archived runners cannot be recovered.' : 'Enroll a runner to execute tasks.'}</EmptyState>}</QueryState></main>
}

export function RunnerDetailPage() {
  const { runnerId = '' } = useParams()
  const navigate = useNavigate()
  const { permissions } = useAuth()
  const [binaryBusy, setBinaryBusy] = useState(false)
  const [workerUI, setWorkerUI] = useState<WorkerUI>('gui')
  const [binaryError, setBinaryError] = useState('')
  const [capacityDraft, setCapacityDraft] = useState('')
  const [capacityBusy, setCapacityBusy] = useState(false)
  const [capacityError, setCapacityError] = useState('')
  const [natsEndpointDraft, setNatsEndpointDraft] = useState<string>()
  const [natsEndpointBusy, setNatsEndpointBusy] = useState(false)
  const [natsEndpointError, setNatsEndpointError] = useState('')
  const [controlPlaneURLDraft, setControlPlaneURLDraft] = useState<string>()
  const [controlPlaneURLBusy, setControlPlaneURLBusy] = useState(false)
  const [controlPlaneURLError, setControlPlaneURLError] = useState('')
  const [metricRange, setMetricRange] = useState<RunnerMetricRange>('24h')
  const [runPage, setRunPage] = useState(1); const [runLimit, setRunLimit] = useState(10)
  const query = useQuery({ queryKey: ['runner', runnerId], queryFn: ({ signal }) => api.get<Runner>(`/api/v1/runners/${encodeURIComponent(runnerId)}`, undefined, signal), enabled: Boolean(runnerId) })
  const currentRuns = useQuery({ queryKey: ['runner-runs', runnerId, runPage, runLimit], queryFn: ({ signal }) => api.get<Page<Run>>('/api/v1/runs', { runner: runnerId, state: 'ACTIVE', page: runPage, limit: runLimit }, signal), enabled: Boolean(runnerId), refetchInterval: 5_000 })
  const metricsQuery = useQuery({ queryKey: ['runner-metrics', runnerId, metricRange], queryFn: ({ signal }) => api.get<RunnerMetricHistory>(`/api/v1/runners/${encodeURIComponent(runnerId)}/metrics`, { ...runnerMetricWindow(metricRange), limit: 2000 }, signal), enabled: Boolean(runnerId), refetchInterval: 15_000 })
  const manage = hasPermission(permissions, 'runners.manage')
  const action = (state: string) => api.post(`/api/v1/runners/${encodeURIComponent(runnerId)}/${state}`).then(() => { void query.refetch() })
  const updateCapacity = async (runner: Runner) => { const value = Number(capacityDraft || runner.capacity || 0); if (!Number.isInteger(value) || value < 1) { setCapacityError('Capacity must be at least 1.'); return }; setCapacityBusy(true); setCapacityError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { capacity: value }); setCapacityDraft(String(value)); await query.refetch() } catch (cause) { setCapacityError(describeError(cause).message) } finally { setCapacityBusy(false) } }
  const updateNATSEndpoint = async (runner: Runner) => { const value = (natsEndpointDraft ?? runner.natsEndpoint ?? '').trim(); setNatsEndpointBusy(true); setNatsEndpointError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { nats_endpoint: value }); setNatsEndpointDraft(value); await query.refetch() } catch (cause) { setNatsEndpointError(describeError(cause).message) } finally { setNatsEndpointBusy(false) } }
  const updateControlPlaneURL = async (runner: Runner) => { const value = (controlPlaneURLDraft ?? runner.controlPlaneUrl ?? '').trim().replace(/\/$/, ''); setControlPlaneURLBusy(true); setControlPlaneURLError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { control_plane_url: value }); setControlPlaneURLDraft(value); await query.refetch() } catch (cause) { setControlPlaneURLError(describeError(cause).message) } finally { setControlPlaneURLBusy(false) } }
  const generateBinary = async (runner: Runner) => { setBinaryBusy(true); setBinaryError(''); try { const platform = runner.platform || 'linux'; const architecture = runner.architecture || 'amd64'; const result = await api.post<{ artifact: string; filename?: string }>('/api/v1/runners/enrollments', { runner_id: runner.id, pool_id: runner.poolId || runner.pool, platform, architecture, capacity: runner.capacity ?? 10, control_plane_url: runner.controlPlaneUrl ?? '', embedded_nats_endpoint: runner.natsEndpoint ?? '', ui: workerUI }); downloadArtifact(result.artifact, result.filename ?? `${runner.id}-glyphflow-runner-${platform}-${architecture}${workerUI === 'gui' ? '' : `-${workerUI}`}${platform === 'windows' ? '.exe' : ''}`) } catch (cause) { setBinaryError(describeError(cause).message) } finally { setBinaryBusy(false) } }
  return (
    <main className="gf-content">
      <QueryState query={query}>
        {(runner) => (
          <>
            <PageHeader title={runner.name} description={`Pool ${runner.pool ?? '—'} · ${runner.observedState ?? '—'}`} refresh={<QueryRefresh query={query} />} />
            {runner.isArchived && <p className="gf-form-error" role="alert">This runner is archived permanently and cannot be recovered.</p>}
            <section className="gf-metric-grid">
              <MetricCard label="Desired state" value={<StatusPill status={runner.desiredState ?? '—'} />} icon={Server} />
              <MetricCard label="Observed state" value={<StatusPill status={runner.observedState ?? '—'} />} icon={Activity} />
              <MetricCard label="Capacity" value={`${runner.activeCount ?? 0}/${runner.capacity ?? 0}`} detail={runner.currentCapacity && runner.currentCapacity !== runner.capacity ? `Heartbeat current: ${runner.currentCapacity}` : runnerIsStale(runner.heartbeatAt) ? 'Heartbeat stale' : 'Heartbeat current'} icon={ListChecks} />
              <MetricCard label="CPU" value={runnerIsStale(runner.heartbeatAt) ? 'Stale' : formatMetricPercent(runner.currentMetrics?.cpuPercent)} detail={runner.currentMetrics ? `Sampled ${formatDateTime(runner.currentMetrics.sampledAt)}` : 'No samples'} icon={Activity} />
              <MetricCard label="Memory" value={runnerIsStale(runner.heartbeatAt) ? 'Stale' : formatMetricPercent(runner.currentMetrics?.memoryPercent)} detail={runner.currentMetrics ? `Sampled ${formatDateTime(runner.currentMetrics.sampledAt)}` : 'No samples'} icon={HardDrive} />
            </section>
            <section className="gf-card-panel">
              <div className="gf-runner-metrics-header"><h2>Resource history</h2><label>Range<select className="gf-input" aria-label="Resource history range" value={metricRange} onChange={(event) => setMetricRange(event.target.value as RunnerMetricRange)}><option value="1h">Last hour</option><option value="6h">Last 6 hours</option><option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option><option value="30d">Last 30 days</option></select></label></div>
              <QueryState query={metricsQuery} empty="No resource samples in this range.">{(history) => <div className="gf-runner-metrics-charts"><RunnerMetricChart label="CPU" items={history.items} field="cpuPercent" /><RunnerMetricChart label="Memory" items={history.items} field="memoryPercent" /></div>}</QueryState>
            </section>
            <section className="gf-card-panel">
              <h2>Capacity</h2>
              {manage && !runner.isArchived ? <div className="gf-dialog-actions gf-capacity-controls"><label>Tasks<Input type="number" min={1} value={capacityDraft || String(runner.capacity ?? 10)} onChange={(event) => setCapacityDraft(event.target.value)} /></label><Button busy={capacityBusy} onClick={() => updateCapacity(runner)}>Update capacity</Button></div> : <p className="gf-muted">Configured capacity: {runner.capacity ?? '—'}</p>}
              {capacityError && <p className="gf-form-error" role="alert">{capacityError}</p>}
            </section>
            <section className="gf-card-panel gf-binary-configuration">
              <h2>Binary Configuration</h2>
              <Tabs defaultValue="endpoints" className="gf-binary-tabs">
                <TabsList aria-label="Binary configuration tabs"><TabsTrigger value="endpoints">Endpoints</TabsTrigger><TabsTrigger value="generation">Binary Generation</TabsTrigger></TabsList>
                <TabsContent value="endpoints" className="gf-binary-tab-content"><div className="gf-binary-endpoint-grid"><div className="gf-binary-endpoint"><h3>Control plane endpoint</h3>{manage && !runner.isArchived ? <div className="gf-runner-endpoint-form"><div className="gf-runner-endpoint-control"><label htmlFor="runner-control-plane-url">Endpoint</label><Input id="runner-control-plane-url" value={controlPlaneURLDraft ?? runner.controlPlaneUrl ?? ''} placeholder="http://localhost:8080" onChange={(event) => setControlPlaneURLDraft(event.target.value)} /><Button busy={controlPlaneURLBusy} onClick={() => updateControlPlaneURL(runner)}>Save endpoint</Button></div><small><code>GLYPHFLOW_CONTROL_PLANE_URL</code> on the runner machine overrides this value.</small></div> : <><p className="gf-muted">Configured endpoint: {runner.controlPlaneUrl || 'server default'}</p><p className="gf-muted"><code>GLYPHFLOW_CONTROL_PLANE_URL</code> on the runner machine overrides this value.</p></>}{controlPlaneURLError && <p className="gf-form-error" role="alert">{controlPlaneURLError}</p>}</div><div className="gf-binary-endpoint"><h3>NATS endpoint</h3>{manage && !runner.isArchived ? <div className="gf-runner-endpoint-form"><div className="gf-runner-endpoint-control"><label htmlFor="runner-nats-endpoint">Endpoint</label><Input id="runner-nats-endpoint" value={natsEndpointDraft ?? runner.natsEndpoint ?? ''} placeholder="nats://localhost:4222" onChange={(event) => setNatsEndpointDraft(event.target.value)} /><Button busy={natsEndpointBusy} onClick={() => updateNATSEndpoint(runner)}>Save endpoint</Button></div><small><code>GLYPHFLOW_NATS_ENDPOINT</code> on the runner machine overrides this value.</small></div> : <><p className="gf-muted">Configured endpoint: {runner.natsEndpoint || 'server default'}</p><p className="gf-muted"><code>GLYPHFLOW_NATS_ENDPOINT</code> on the runner machine overrides this value.</p></>}{natsEndpointError && <p className="gf-form-error" role="alert">{natsEndpointError}</p>}</div></div></TabsContent>
                <TabsContent value="generation" className="gf-binary-tab-content">{manage && !runner.isArchived ? <div className="gf-binary-generation-controls"><label className="gf-worker-ui-control">Worker UI<select className="gf-input" value={workerUI} onChange={(event) => setWorkerUI(event.target.value as WorkerUI)}>{workerUIOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label><Button busy={binaryBusy} title="Create and download a new one-use runner binary" onClick={() => generateBinary(runner)}>Generate Binary</Button></div> : <p className="gf-muted">Binary generation is unavailable for this runner.</p>}{binaryError && <p className="gf-form-error" role="alert">{binaryError}</p>}</TabsContent>
              </Tabs>
            </section>
            <section className="gf-card-panel">
              <h2>Lifecycle</h2>
              {manage && !runner.isArchived ? <div className="gf-lifecycle-list"><div className="gf-lifecycle-action"><DangerousAction label="Drain" variant="secondary" onConfirm={() => action('drain')} onConflict={() => query.refetch()} /><p>Stops new work while active runs finish.</p></div><div className="gf-lifecycle-action">{runnerIsRevoked(runner) ? <DangerousAction label="Unrevoke" variant="secondary" title="Unrevoke runner" warning="Re-enables this runner so it can connect and receive work." confirmLabel="Unrevoke" onConfirm={() => action('reset')} onConflict={() => query.refetch()} /> : <DangerousAction label="Revoke" onConfirm={() => action('revoke')} onConflict={() => query.refetch()} />}<p>{runnerIsRevoked(runner) ? 'Re-enables this runner to receive work.' : 'Prevents this runner from receiving work.'}</p></div><div className="gf-lifecycle-action"><DangerousAction label="Archive" title="Archive runner permanently" warning="Archiving cancels this runner's active work and permanently removes its ability to connect. Archived runners cannot be recovered." confirmLabel="Archive" onConfirm={() => api.delete(`/api/v1/runners/${encodeURIComponent(runnerId)}`).then(() => navigate('/runners'))} /><p>Permanently removes this runner and cancels active work.</p></div></div> : <p className="gf-muted">Lifecycle actions are unavailable for this runner.</p>}
            </section>
            <section className="gf-card-panel">
              <h2>Current runs</h2>
              <QueryState query={currentRuns} empty="No current runs.">
                {(data) => data.items.length ? <><DataTable caption="Current runs" rows={data.items} columns={[{ key: 'id', label: 'Run', render: (run) => <RunIDCell id={run.id} compact /> }, { key: 'task', label: 'Task', render: (run) => run.taskName ?? run.taskId ?? '—' }, { key: 'state', label: 'State', render: (run) => <StatusPill status={run.state} /> }, { key: 'attempt', label: 'Attempt' }, { key: 'scheduled', label: 'Scheduled', render: (run) => formatDateTime(run.scheduledFor) }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={runLimit} onChange={setRunPage} onLimitChange={(next) => { setRunLimit(next); setRunPage(1) }} /></> : null}
              </QueryState>
            </section>
          </>
        )}
      </QueryState>
    </main>
  )
}
