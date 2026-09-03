import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type Page, type Runner, type RunnerPool } from './api'
import { Button, Input, PageHeader, StatusPill } from './components'
import { describeError } from './errors'
import { formatDateTime } from './format'

export const workerUIOptions = [
  { value: 'gui', label: 'GUI - Default UI' },
  { value: 'tui', label: 'TUI - Lower Memory Usage' },
  { value: 'headless', label: 'Headless - Lowest Memory Usage' },
] as const
export type WorkerUI = typeof workerUIOptions[number]['value']

export function enrollmentPayload(runnerName: string, platform: string, architecture: string, poolId?: string, capacity = 10, embeddedNatsEndpoint = '', controlPlaneURL = '', ui: WorkerUI = 'gui') {
  return { runner_name: runnerName.trim(), platform: platform.trim(), architecture: architecture.trim(), capacity, ui, ...(poolId ? { pool_id: poolId.trim() } : {}), ...(embeddedNatsEndpoint.trim() ? { embedded_nats_endpoint: embeddedNatsEndpoint.trim() } : {}), ...(controlPlaneURL.trim() ? { control_plane_url: controlPlaneURL.trim().replace(/\/$/, '') } : {}) }
}

export function downloadArtifact(value: string, name: string) {
  const bytes = Uint8Array.from(atob(value), (character) => character.codePointAt(0) ?? 0); const blob = new Blob([bytes], { type: 'application/octet-stream' }); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = name; anchor.click(); URL.revokeObjectURL(url)
}

export function EnrollmentPage() {
  const navigate = useNavigate(); const [params] = useSearchParams(); const [runnerName, setRunnerName] = useState(params.get('runner') ?? ''); const [embeddedNatsEndpoint, setEmbeddedNatsEndpoint] = useState(''); const [controlPlaneEndpoint, setControlPlaneEndpoint] = useState(''); const [poolId, setPoolId] = useState(params.get('pool') ?? ''); const [platform, setPlatform] = useState('linux'); const [architecture, setArchitecture] = useState('amd64'); const [capacity, setCapacity] = useState(10); const [workerUI, setWorkerUI] = useState<WorkerUI>('gui'); const [artifact, setArtifact] = useState<{ value: string; expiresAt?: string; name: string; runnerId: string } | null>(null); const [downloaded, setDownloaded] = useState(false); const [enrolled, setEnrolled] = useState(false); const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const formRevision = useRef(0)
  const poolsQuery = useQuery({ queryKey: ['runner-pools'], queryFn: ({ signal }) => api.get<Page<RunnerPool>>('/api/v1/runners/pools', { limit: 100 }, signal) })
  useEffect(() => {
    if (poolId || !poolsQuery.data?.items.length) return
    setPoolId(poolsQuery.data.items.find((pool) => pool.id === 'default')?.id ?? poolsQuery.data.items[0].id)
  }, [poolId, poolsQuery.data])
  const resetEnrollment = () => { formRevision.current += 1; setArtifact(null); setDownloaded(false); setEnrolled(false); setError('') }
  useEffect(() => {
    if (!artifact || !downloaded) return
    let active = true
    let poller: number | undefined
    let redirectTimer: number | undefined
    const check = async () => {
      try {
        const runner = await api.get<Runner>(`/api/v1/runners/${encodeURIComponent(artifact.runnerId)}`)
        if (active && runner.observedState?.toUpperCase() === 'ONLINE') {
          setEnrolled(true)
          if (poller !== undefined) window.clearInterval(poller)
          redirectTimer = window.setTimeout(() => { if (active) navigate('/runners') }, 5000)
        }
      } catch {
        // Keep waiting; the enrollment may not have created a readable runner yet.
      }
    }
    void check()
    poller = window.setInterval(() => void check(), 5000)
    return () => { active = false; if (poller !== undefined) window.clearInterval(poller); if (redirectTimer !== undefined) window.clearTimeout(redirectTimer) }
  }, [artifact, downloaded, navigate])
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!poolId) { setError('Select a runner pool.'); return } setBusy(true); setError(''); const revision = formRevision.current; const request = enrollmentPayload(runnerName, platform, architecture, poolId, capacity, embeddedNatsEndpoint, controlPlaneEndpoint, workerUI); try { const result = await api.request<{ artifact: string; expires_at?: string; filename?: string; runner_id: string }>('/api/v1/runners/enrollments', { method: 'POST', headers: { 'Cache-Control': 'no-store' }, body: JSON.stringify(request) }); if (revision === formRevision.current) setArtifact({ value: result.artifact, expiresAt: result.expires_at, runnerId: result.runner_id, name: result.filename ?? `${result.runner_id}-glyphflow-runner-${request.platform}-${request.architecture}${request.ui === 'gui' ? '' : `-${request.ui}`}${request.platform === 'windows' ? '.exe' : ''}` }) } catch (cause) { if (revision === formRevision.current) setError(describeError(cause).message) } finally { setBusy(false) } }
  return <main className="gf-content"><PageHeader title="Enroll runner" description="Download a one-use bootstrap binary for the target machine." /><form className="gf-editor-form" onSubmit={submit}><label>Runner name<Input value={runnerName} onChange={(event) => { setRunnerName(event.target.value); resetEnrollment() }} required /></label><label>Control plane endpoint<Input value={controlPlaneEndpoint} onChange={(event) => { setControlPlaneEndpoint(event.target.value); resetEnrollment() }} placeholder="http://localhost:8080" /><small><code>GLYPHFLOW_CONTROL_PLANE_URL</code> on the target machine overrides this value.</small></label><label>Embedded NATS Endpoint<Input value={embeddedNatsEndpoint} onChange={(event) => { setEmbeddedNatsEndpoint(event.target.value); resetEnrollment() }} placeholder="nats://localhost:4222" /><small><code>GLYPHFLOW_NATS_ENDPOINT</code> on the target machine overrides this value.</small></label><div className="gf-form-grid"><label>Pool<select className="gf-input" value={poolId} onChange={(event) => { setPoolId(event.target.value); resetEnrollment() }} required disabled={poolsQuery.isPending}><option value="">Select a pool</option>{(poolsQuery.data?.items ?? []).filter((pool) => pool.enabled !== false).map((pool) => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select></label><label>Platform<select className="gf-input" value={platform} onChange={(event) => { setPlatform(event.target.value); resetEnrollment() }}><option>linux</option><option>windows</option></select></label><label>Architecture<select className="gf-input" value={architecture} onChange={(event) => { setArchitecture(event.target.value); resetEnrollment() }}><option>amd64</option></select></label><label>Capacity<Input type="number" min={1} step={1} value={capacity} onChange={(event) => { setCapacity(Number(event.target.value)); resetEnrollment() }} required /></label><label className="gf-worker-ui-control">Worker UI<select className="gf-input" value={workerUI} onChange={(event) => { setWorkerUI(event.target.value as WorkerUI); resetEnrollment() }}>{workerUIOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label></div><p className="gf-muted">Capacity defaults to 10. A unique runner ID is generated when you create the enrollment.</p>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate('/runners')}>Cancel</Button><Button type="submit" busy={busy} disabled={Boolean(artifact) || poolsQuery.isPending}>Create enrollment</Button></div></form>{artifact && <><section className="gf-card-panel"><h2>Runner binary ready</h2><p className="gf-form-error">Run once before {artifact.expiresAt ? formatDateTime(artifact.expiresAt) : 'expiry'}; the embedded credential is one-use.</p><Button onClick={() => { downloadArtifact(artifact.value, artifact.name); setDownloaded(true) }}>Download runner</Button></section>{downloaded && <section className="gf-card-panel"><h2>Enrollment status</h2><StatusPill status={enrolled ? 'Runner Enrolled' : 'Waiting Enrollment'} /><p className="gf-muted">{enrolled ? 'Runner connected. Returning to runners in 5 seconds.' : 'Start the downloaded runner to complete enrollment.'}</p></section>}</>}</main>
}
