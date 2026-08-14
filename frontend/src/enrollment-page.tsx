import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type Runner } from './api'
import { Button, Input, PageHeader, StatusPill } from './components'
import { describeError } from './errors'

export function enrollmentPayload(runnerId: string, platform: string, architecture: string) {
  return { runner_id: runnerId.trim(), platform: platform.trim(), architecture: architecture.trim() }
}

function downloadArtifact(value: string, name: string) {
  const bytes = Uint8Array.from(atob(value), (character) => character.charCodeAt(0)); const blob = new Blob([bytes], { type: 'application/octet-stream' }); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = name; anchor.click(); URL.revokeObjectURL(url)
}

export function EnrollmentPage() {
  const navigate = useNavigate(); const [params] = useSearchParams(); const [runnerId, setRunnerId] = useState(params.get('runner') ?? ''); const [platform, setPlatform] = useState('linux'); const [architecture, setArchitecture] = useState('amd64'); const [artifact, setArtifact] = useState<{ value: string; expiresAt?: string; name: string } | null>(null); const [downloaded, setDownloaded] = useState(false); const [enrolled, setEnrolled] = useState(false); const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const formRevision = useRef(0)
  const resetEnrollment = () => { formRevision.current += 1; setArtifact(null); setDownloaded(false); setEnrolled(false); setError('') }
  useEffect(() => {
    if (!artifact || !downloaded) return
    let active = true
    let poller: number | undefined
    let redirectTimer: number | undefined
    const check = async () => {
      try {
        const runner = await api.get<Runner>(`/api/v1/runners/${encodeURIComponent(runnerId)}`)
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
  }, [artifact, downloaded, navigate, runnerId])
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); const revision = formRevision.current; const request = enrollmentPayload(runnerId, platform, architecture); try { const result = await api.request<{ artifact: string; expires_at?: string; filename?: string }>('/api/v1/runners/enrollments', { method: 'POST', headers: { 'Cache-Control': 'no-store' }, body: JSON.stringify(request) }); if (revision === formRevision.current) setArtifact({ value: result.artifact, expiresAt: result.expires_at, name: result.filename ?? `${request.runner_id}-glyphflow-runner-${request.platform}-${request.architecture}${request.platform === 'windows' ? '.exe' : ''}` }) } catch (cause) { if (revision === formRevision.current) setError(describeError(cause).message) } finally { setBusy(false) } }
  return <main className="gf-content"><PageHeader title="Enroll runner" description="Download a one-use bootstrap binary for the target machine." /><form className="gf-editor-form" onSubmit={submit}><label>Runner ID<Input value={runnerId} onChange={(event) => { setRunnerId(event.target.value); resetEnrollment() }} required /></label><div className="gf-form-grid"><label>Platform<select className="gf-input" value={platform} onChange={(event) => { setPlatform(event.target.value); resetEnrollment() }}><option>linux</option><option>windows</option></select></label><label>Architecture<select className="gf-input" value={architecture} onChange={(event) => { setArchitecture(event.target.value); resetEnrollment() }}><option>amd64</option></select></label></div><p className="gf-muted">The binary contains a one-use credential. On first start it enrolls and saves its connection locally.</p>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate('/runners')}>Cancel</Button><Button type="submit" busy={busy} disabled={Boolean(artifact)}>Create enrollment</Button></div></form>{artifact && <><section className="gf-card-panel"><h2>Runner binary ready</h2><p className="gf-form-error">Run once before {artifact.expiresAt ?? 'expiry'}; the embedded credential is one-use.</p><Button onClick={() => { downloadArtifact(artifact.value, artifact.name); setDownloaded(true) }}>Download runner</Button></section>{downloaded && <section className="gf-card-panel"><h2>Enrollment status</h2><StatusPill status={enrolled ? 'Runner Enrolled' : 'Waiting Enrollment'} /><p className="gf-muted">{enrolled ? 'Runner connected. Returning to runners in 5 seconds.' : 'Start the downloaded runner to complete enrollment.'}</p></section>}</>}</main>
}
