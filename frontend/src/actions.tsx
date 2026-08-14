import { useState, type ReactNode } from 'react'
import { ApiError } from './api'
import { Button, Dialog, Input } from './components'

export function dangerousWarning(action: string): string {
  if (/retry|reconcile/i.test(action)) return 'This may repeat external side effects. Confirm that the task is safe to run again.'
  return `Confirm ${action.toLowerCase()}? This change can affect active work.`
}

export function DangerousAction({ label, title = label, warning = dangerousWarning(label), confirmLabel = label, reasonRequired = false, variant = 'danger', onConfirm, onConflict }: { label: string; title?: string; warning?: string; confirmLabel?: string; reasonRequired?: boolean; variant?: 'danger' | 'secondary'; onConfirm: (reason?: string) => void | Promise<void>; onConflict?: () => void }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const confirm = async () => {
    if (reasonRequired && !reason.trim()) { setError('A reason is required.'); return }
    setBusy(true); setError('')
    try { await onConfirm(reason.trim() || undefined); setOpen(false); setReason('') } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) { onConflict?.(); setError('This resource changed. Reload it before trying again.') } else setError(cause instanceof Error ? cause.message : 'Action failed')
    } finally { setBusy(false) }
  }
  return <><Button variant={variant} onClick={() => setOpen(true)}>{label}</Button><Dialog open={open} title={title} onClose={() => !busy && setOpen(false)}><div className="gf-danger-dialog"><p>{warning}</p>{reasonRequired && <><label htmlFor="danger-reason">Reason</label><Input id="danger-reason" value={reason} onChange={(event) => setReason(event.target.value)} /></>}{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button variant="secondary" disabled={busy} onClick={() => setOpen(false)}>Cancel</Button><Button variant="danger" busy={busy} onClick={confirm}>{confirmLabel}</Button></div></div></Dialog></>
}

export function PermissionAction({ allowed, children }: { allowed: boolean; children: ReactNode }) {
  return allowed ? <>{children}</> : null
}
