import { useState, type ReactNode } from 'react'
import { ApiError } from './api'
import { Button, Dialog, Input } from './components'

export function dangerousWarning(action: string): string {
	const descriptions: Record<string, string> = {
		drain: 'Stops this runner from receiving new work while existing work can finish.',
		revoke: 'Marks this runner revoked so it can no longer receive work.',
		enable: 'Enables this runner to receive work.',
		disable: 'Disables this runner from receiving work.',
		reset: 'Returns this runner to the enabled state.',
		cancel: 'Stops this run and prevents further execution.',
		unlink: 'Removes this sign-in method from your account.',
		delete: 'Permanently removes this resource or role.',
	}
	const description = descriptions[action.toLowerCase()]
	if (description) return description
	if (/retry|reconcile/i.test(action)) return 'This may repeat external side effects. Confirm that the task is safe to run again.'
	return `Confirm ${action.toLowerCase()}? This change can affect active work.`
}

export function dangerousActionError(cause: unknown, onConflict?: () => void): string {
	if (cause instanceof ApiError && cause.status === 409 && onConflict) {
		onConflict()
		return 'This resource changed. Reload it before trying again.'
	}
	return cause instanceof Error ? cause.message : 'Action failed'
}

export function DangerousAction({ label, title = label, warning = dangerousWarning(label), confirmLabel = label, cancelLabel = 'Cancel', reasonRequired = false, variant = 'danger', onConfirm, onConflict, renderTrigger }: Readonly<{ label: string; title?: string; warning?: string; confirmLabel?: string; cancelLabel?: string; reasonRequired?: boolean; variant?: 'danger' | 'secondary'; onConfirm: (reason?: string) => void | Promise<void>; onConflict?: () => void; renderTrigger?: (open: () => void) => ReactNode }>) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const confirm = async () => {
    if (reasonRequired && !reason.trim()) { setError('A reason is required.'); return }
    setBusy(true); setError('')
	    try { await onConfirm(reason.trim() || undefined); setOpen(false); setReason('') } catch (cause) {
	      setError(dangerousActionError(cause, onConflict))
    } finally { setBusy(false) }
  }
  return <>{renderTrigger ? renderTrigger(() => setOpen(true)) : <Button variant={variant} title={warning} onClick={() => setOpen(true)}>{label}</Button>}<Dialog open={open} title={title} onClose={() => !busy && setOpen(false)}><div className="gf-danger-dialog"><p>{warning}</p>{reasonRequired && <><label htmlFor="danger-reason">Reason</label><Input id="danger-reason" value={reason} onChange={(event) => setReason(event.target.value)} /></>}{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button variant="secondary" disabled={busy} onClick={() => setOpen(false)}>{cancelLabel}</Button><Button variant="danger" busy={busy} onClick={confirm}>{confirmLabel}</Button></div></div></Dialog></>
}
