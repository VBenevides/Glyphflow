import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run, type Runner } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { hasPermission } from './permissions'
import { describeError } from './errors'
import { downloadArtifact } from './enrollment-page'
import { RunIDCell } from './run-pages'

export function runnerIsStale(lastHeartbeat?: string, now = Date.now(), thresholdMs = 60_000) {
  return Boolean(lastHeartbeat && now - Date.parse(lastHeartbeat) > thresholdMs)
}

function hasActiveRunners(data?: Page<Runner>) {
  return Boolean(data?.items.some((runner) => ['online', 'busy', 'draining', 'starting'].includes((runner.observedState ?? '').toLowerCase())))
}

export function RunnerInventoryPage() {
  const { permissions } = useAuth(); const navigate = useNavigate(); const [params] = useSearchParams(); const archived = params.get('archived') === 'true'; const [search, setSearch] = useState(''); const [page, setPage] = useState(1)
  const debouncedSearch = useDebouncedValue(search)
  const query = useQuery({ queryKey: ['runners', archived, debouncedSearch, page], queryFn: ({ signal }) => api.get<Page<Runner>>('/api/v1/runners', { search: debouncedSearch || undefined, archived, page }, signal), refetchInterval: (current) => !archived && hasActiveRunners(current.state.data as Page<Runner> | undefined) ? 15_000 : false })
  const manage = hasPermission(permissions, 'runners.manage')
  return <main className="gf-content"><PageHeader title="Runners and pools" description="Capacity, sessions, capabilities, and lifecycle state." action={manage && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => navigate('/runners/pools')}>Manage pools</Button><Button onClick={() => navigate('/runners/enroll')}>Enroll runner</Button></div>} /><nav className="gf-account-tabs" aria-label="Runner sections"><Link className={!archived ? 'is-active' : ''} to="/runners">Runners</Link><Link className={archived ? 'is-active' : ''} to="/runners?archived=true">Archived Runners</Link></nav><div className="gf-filter-bar"><label>Search<Input value={search} onChange={(event) => { setSearch(event.target.value); setPage(1) }} placeholder="Name or pool" /></label></div><QueryState query={query} empty={archived ? 'No archived runners.' : 'Enroll a runner to execute tasks.'}>{(data) => data.items.length ? <><DataTable caption={archived ? 'Archived runners' : 'Runners'} rows={data.items} columns={[{ key: 'name', label: 'Runner', render: (runner) => <Link to={`/runners/${runner.id}`}>{runner.name}</Link> }, { key: 'pool', label: 'Pool' }, { key: 'desiredState', label: 'Desired', render: (runner) => <StatusPill status={runner.desiredState ?? '—'} /> }, { key: 'observedState', label: 'Observed', render: (runner) => <StatusPill status={runner.observedState ?? '—'} /> }, { key: 'capacity', label: 'Capacity', render: (runner) => `${runner.activeCount ?? 0}/${runner.capacity ?? 0}` }, { key: 'heartbeatAt', label: 'Heartbeat', render: (runner) => runnerIsStale(runner.heartbeatAt) ? <span className="gf-stale-warning">Stale</span> : runner.heartbeatAt ?? '—' }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title={archived ? 'No archived runners' : 'No runners'}>{archived ? 'Archived runners cannot be recovered.' : 'Enroll a runner to execute tasks.'}</EmptyState>}</QueryState></main>
}

export function RunnerDetailPage() {
  const { runnerId = '' } = useParams()
  const navigate = useNavigate()
  const { permissions } = useAuth()
  const [binaryBusy, setBinaryBusy] = useState(false)
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
  const [runPage, setRunPage] = useState(1)
  const query = useQuery({ queryKey: ['runner', runnerId], queryFn: ({ signal }) => api.get<Runner>(`/api/v1/runners/${encodeURIComponent(runnerId)}`, undefined, signal), enabled: Boolean(runnerId) })
  const currentRuns = useQuery({ queryKey: ['runner-runs', runnerId, runPage], queryFn: ({ signal }) => api.get<Page<Run>>('/api/v1/runs', { runner: runnerId, state: 'ACTIVE', page: runPage, limit: 100 }, signal), enabled: Boolean(runnerId), refetchInterval: 5_000 })
  const manage = hasPermission(permissions, 'runners.manage')
  const action = (state: string) => api.post(`/api/v1/runners/${encodeURIComponent(runnerId)}/${state}`).then(() => { void query.refetch() })
  const updateCapacity = async (runner: Runner) => { const value = Number(capacityDraft || runner.capacity || 0); if (!Number.isInteger(value) || value < 1) { setCapacityError('Capacity must be at least 1.'); return }; setCapacityBusy(true); setCapacityError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { capacity: value }); setCapacityDraft(String(value)); await query.refetch() } catch (cause) { setCapacityError(describeError(cause).message) } finally { setCapacityBusy(false) } }
  const updateNATSEndpoint = async (runner: Runner) => { const value = (natsEndpointDraft ?? runner.natsEndpoint ?? '').trim(); setNatsEndpointBusy(true); setNatsEndpointError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { nats_endpoint: value }); setNatsEndpointDraft(value); await query.refetch() } catch (cause) { setNatsEndpointError(describeError(cause).message) } finally { setNatsEndpointBusy(false) } }
  const updateControlPlaneURL = async (runner: Runner) => { const value = (controlPlaneURLDraft ?? runner.controlPlaneUrl ?? '').trim().replace(/\/$/, ''); setControlPlaneURLBusy(true); setControlPlaneURLError(''); try { await api.put(`/api/v1/runners/${encodeURIComponent(runner.id)}`, { control_plane_url: value }); setControlPlaneURLDraft(value); await query.refetch() } catch (cause) { setControlPlaneURLError(describeError(cause).message) } finally { setControlPlaneURLBusy(false) } }
  const generateBinary = async (runner: Runner) => { setBinaryBusy(true); setBinaryError(''); try { const platform = runner.platform || 'linux'; const architecture = runner.architecture || 'amd64'; const result = await api.post<{ artifact: string; filename?: string }>('/api/v1/runners/enrollments', { runner_id: runner.id, pool_id: runner.poolId || runner.pool, platform, architecture, capacity: runner.capacity ?? 10, control_plane_url: runner.controlPlaneUrl ?? '', embedded_nats_endpoint: runner.natsEndpoint ?? '' }); downloadArtifact(result.artifact, result.filename ?? `${runner.id}-glyphflow-runner-${platform}-${architecture}${platform === 'windows' ? '.exe' : ''}`) } catch (cause) { setBinaryError(describeError(cause).message) } finally { setBinaryBusy(false) } }
  return (
    <main className="gf-content">
      <QueryState query={query}>
        {(runner) => (
          <>
            <PageHeader title={runner.name} description={`Pool ${runner.pool ?? '—'} · ${runner.observedState ?? '—'}`} />
            {runner.isArchived && <p className="gf-form-error" role="alert">This runner is archived permanently and cannot be recovered.</p>}
            <section className="gf-metric-grid">
              <div className="gf-metric"><span>Desired state</span><strong><StatusPill status={runner.desiredState ?? '—'} /></strong></div>
              <div className="gf-metric"><span>Observed state</span><strong><StatusPill status={runner.observedState ?? '—'} /></strong></div>
              <div className="gf-metric"><span>Capacity</span><strong>{runner.activeCount ?? 0}/{runner.capacity ?? 0}</strong><small>{runner.currentCapacity && runner.currentCapacity !== runner.capacity ? `Heartbeat current: ${runner.currentCapacity}` : runnerIsStale(runner.heartbeatAt) ? 'Heartbeat stale' : 'Heartbeat current'}</small></div>
            </section>
            <section className="gf-card-panel">
              <h2>Capacity</h2>
              {manage && !runner.isArchived ? <div className="gf-dialog-actions"><label>Tasks<Input type="number" min={1} value={capacityDraft || String(runner.capacity ?? 10)} onChange={(event) => setCapacityDraft(event.target.value)} /></label><Button busy={capacityBusy} onClick={() => updateCapacity(runner)}>Update capacity</Button></div> : <p className="gf-muted">Configured capacity: {runner.capacity ?? '—'}</p>}
              {capacityError && <p className="gf-form-error" role="alert">{capacityError}</p>}
            </section>
            <section className="gf-card-panel">
              <h2>Control plane endpoint</h2>
              {manage && !runner.isArchived ? <div className="gf-runner-endpoint-form"><div className="gf-runner-endpoint-control"><label htmlFor="runner-control-plane-url">Endpoint</label><Input id="runner-control-plane-url" value={controlPlaneURLDraft ?? runner.controlPlaneUrl ?? ''} placeholder="http://localhost:8080" onChange={(event) => setControlPlaneURLDraft(event.target.value)} /><Button busy={controlPlaneURLBusy} onClick={() => updateControlPlaneURL(runner)}>Save endpoint</Button></div><small><code>GLYPHFLOW_CONTROL_PLANE_URL</code> on the runner machine overrides this value.</small></div> : <><p className="gf-muted">Configured endpoint: {runner.controlPlaneUrl || 'server default'}</p><p className="gf-muted"><code>GLYPHFLOW_CONTROL_PLANE_URL</code> on the runner machine overrides this value.</p></>}
              {controlPlaneURLError && <p className="gf-form-error" role="alert">{controlPlaneURLError}</p>}
            </section>
            <section className="gf-card-panel">
              <h2>NATS endpoint</h2>
              {manage && !runner.isArchived ? <div className="gf-runner-endpoint-form"><div className="gf-runner-endpoint-control"><label htmlFor="runner-nats-endpoint">Endpoint</label><Input id="runner-nats-endpoint" value={natsEndpointDraft ?? runner.natsEndpoint ?? ''} placeholder="nats://localhost:4222" onChange={(event) => setNatsEndpointDraft(event.target.value)} /><Button busy={natsEndpointBusy} onClick={() => updateNATSEndpoint(runner)}>Save endpoint</Button></div><small><code>GLYPHFLOW_NATS_ENDPOINT</code> on the runner machine overrides this value.</small></div> : <><p className="gf-muted">Configured endpoint: {runner.natsEndpoint || 'server default'}</p><p className="gf-muted"><code>GLYPHFLOW_NATS_ENDPOINT</code> on the runner machine overrides this value.</p></>}
              {natsEndpointError && <p className="gf-form-error" role="alert">{natsEndpointError}</p>}
            </section>
            <section className="gf-card-panel">
              <h2>Lifecycle</h2>
              <div className="gf-dialog-actions">
                {manage && !runner.isArchived && <>
                  <Button busy={binaryBusy} title="Create and download a new one-use runner binary" onClick={() => generateBinary(runner)}>Generate Binary</Button>
                  <DangerousAction label="Drain" variant="secondary" onConfirm={() => action('drain')} onConflict={() => query.refetch()} />
                  <DangerousAction label="Revoke" onConfirm={() => action('revoke')} onConflict={() => query.refetch()} />
                  <DangerousAction label="Archive" title="Archive runner permanently" warning="Archiving cancels this runner's active work and permanently removes its ability to connect. Archived runners cannot be recovered." confirmLabel="Archive" onConfirm={() => api.delete(`/api/v1/runners/${encodeURIComponent(runnerId)}`).then(() => navigate('/runners'))} />
                </>}
              </div>
              {binaryError && <p className="gf-form-error" role="alert">{binaryError}</p>}
            </section>
            <section className="gf-card-panel">
              <h2>Current runs</h2>
              <QueryState query={currentRuns} empty="No current runs.">
                {(data) => data.items.length ? <><DataTable caption="Current runs" rows={data.items} columns={[{ key: 'id', label: 'Run', render: (run) => <RunIDCell id={run.id} compact /> }, { key: 'task', label: 'Task', render: (run) => run.taskName ?? run.taskId ?? '—' }, { key: 'state', label: 'State', render: (run) => <StatusPill status={run.state} /> }, { key: 'attempt', label: 'Attempt' }, { key: 'scheduled', label: 'Scheduled', render: (run) => run.scheduledFor ?? '—' }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setRunPage} /></> : null}
              </QueryState>
            </section>
          </>
        )}
      </QueryState>
    </main>
  )
}
