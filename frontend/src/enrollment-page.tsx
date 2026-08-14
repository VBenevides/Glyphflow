import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from './api'
import { Button, Input, PageHeader } from './components'
import { describeError } from './errors'

export function enrollmentPayload(runnerId: string, platform: string, architecture: string) {
  return { runner_id: runnerId.trim(), platform: platform.trim(), architecture: architecture.trim() }
}

function downloadArtifact(value: string, name: string) {
  const blob = new Blob([value], { type: 'application/octet-stream' }); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = name; anchor.click(); URL.revokeObjectURL(url)
}

export function EnrollmentPage() {
  const navigate = useNavigate(); const [params] = useSearchParams(); const [runnerId, setRunnerId] = useState(params.get('runner') ?? ''); const [platform, setPlatform] = useState('linux'); const [architecture, setArchitecture] = useState('amd64'); const [artifact, setArtifact] = useState<{ value: string; expiresAt?: string; name: string } | null>(null); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); try { const result = await api.request<{ artifact: string; expires_at?: string; filename?: string }>('/api/v1/runners/enrollments', { method: 'POST', headers: { 'Cache-Control': 'no-store' }, body: JSON.stringify(enrollmentPayload(runnerId, platform, architecture)) }); setArtifact({ value: result.artifact, expiresAt: result.expires_at, name: result.filename ?? 'glyphflow-runner-enrollment.bin' }) } catch (cause) { setError(describeError(cause).message) } finally { setBusy(false) } }
  return <main className="gf-content"><PageHeader title="Enroll runner" description="Enrollment is one-use and expires. Generate it only on the target machine." /><form className="gf-editor-form" onSubmit={submit}><label>Runner ID<Input value={runnerId} onChange={(event) => setRunnerId(event.target.value)} required /></label><div className="gf-form-grid"><label>Platform<select className="gf-input" value={platform} onChange={(event) => setPlatform(event.target.value)}><option>linux</option><option>windows</option><option>darwin</option></select></label><label>Architecture<select className="gf-input" value={architecture} onChange={(event) => setArchitecture(event.target.value)}><option>amd64</option><option>arm64</option></select></label></div><p className="gf-muted">The artifact contains a one-use credential. It is not saved in browser storage and must be handled as sensitive data.</p>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate('/runners')}>Cancel</Button><Button type="submit" busy={busy}>Create enrollment</Button></div></form>{artifact && <section className="gf-card-panel"><h2>Enrollment ready</h2><p className="gf-form-error">Download once before {artifact.expiresAt ?? 'expiry'}; a second use is rejected.</p><Button onClick={() => downloadArtifact(artifact.value, artifact.name)}>Download artifact</Button></section>}</main>
}
