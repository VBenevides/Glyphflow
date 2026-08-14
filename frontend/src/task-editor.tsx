import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, ApiError } from './api'
import { Button, Input, PageHeader } from './components'
import { describeError, FieldError } from './errors'
import { useUnsavedChanges } from './unsaved'

export type TaskDraft = { name: string; command: string; workingDirectory: string; pool: string; pinnedRunner: string; selectors: string; environment: string; secretReferences: string; timeoutSeconds: string; maxOutputBytes: string; maxAttempts: string; ambiguityPolicy: string }

export const emptyTaskDraft: TaskDraft = { name: '', command: '', workingDirectory: '', pool: '', pinnedRunner: '', selectors: '{}', environment: '{}', secretReferences: '{}', timeoutSeconds: '300', maxOutputBytes: '1048576', maxAttempts: '1', ambiguityPolicy: 'REQUIRE_MANUAL_RECONCILIATION' }

export function commandArguments(value: string): string[] {
  return value.split('\n').map((line) => line.trim()).filter(Boolean)
}

export function validateTaskDraft(draft: TaskDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = 'Name is required.'
  if (!commandArguments(draft.command).length) errors.command = 'Add at least one command argument.'
  if (!draft.pool.trim()) errors.pool = 'Runner pool is required.'
  for (const field of ['selectors', 'environment', 'secretReferences'] as const) { try { JSON.parse(draft[field]) } catch { errors[field] = 'Enter valid JSON.' } }
  for (const field of ['timeoutSeconds', 'maxOutputBytes', 'maxAttempts'] as const) if (!Number.isInteger(Number(draft[field])) || Number(draft[field]) <= 0) errors[field] = 'Enter a positive whole number.'
  return errors
}

function payload(draft: TaskDraft) {
  return { name: draft.name.trim(), command: commandArguments(draft.command), working_directory: draft.workingDirectory, runner_pool: draft.pool.trim(), pinned_runner: draft.pinnedRunner.trim() || undefined, placement_selectors: JSON.parse(draft.selectors), environment: JSON.parse(draft.environment), secret_references: JSON.parse(draft.secretReferences), timeout_seconds: Number(draft.timeoutSeconds), max_output_bytes: Number(draft.maxOutputBytes), max_attempts: Number(draft.maxAttempts), ambiguity_policy: draft.ambiguityPolicy }
}

export function TaskEditorPage() {
  const { taskId } = useParams()
  const navigate = useNavigate()
  const { permissions } = useAuth()
  const [draft, setDraft] = useState<TaskDraft>(emptyTaskDraft)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [review, setReview] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [busy, setBusy] = useState(false)
  useUnsavedChanges(JSON.stringify(draft) !== JSON.stringify(emptyTaskDraft))
  const update = (field: keyof TaskDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }))
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const nextErrors = validateTaskDraft(draft)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length || !review) { setReview(true); return }
    setBusy(true); setSaveError('')
    try { await api.post(taskId ? `/api/v1/tasks/${encodeURIComponent(taskId)}/versions` : '/api/v1/tasks', payload(draft)); navigate(taskId ? `/tasks/${encodeURIComponent(taskId)}` : '/tasks') } catch (cause) { const error = describeError(cause); setSaveError(`${error.message}${error.correlationId ? ` (${error.correlationId})` : ''}`); if (cause instanceof ApiError && cause.status === 422) setErrors(error.fields) } finally { setBusy(false) }
  }
  if (!permissions.includes('tasks.manage')) return <main className="gf-content"><h1>Access denied</h1></main>
  return <main className="gf-content"><PageHeader title={taskId ? 'Edit task version' : 'Create task'} description="Versions are immutable after review and publication." /><form className="gf-editor-form" onSubmit={submit}><label>Name<Input value={draft.name} onChange={(event) => update('name', event.target.value)} aria-invalid={Boolean(errors.name)} />{<FieldError message={errors.name} />}</label><label>Command arguments <small>one argument per line; no shell parsing</small><textarea className="gf-input gf-textarea" value={draft.command} onChange={(event) => update('command', event.target.value)} aria-invalid={Boolean(errors.command)} />{<FieldError message={errors.command} />}</label><div className="gf-form-grid"><label>Runner pool<Input value={draft.pool} onChange={(event) => update('pool', event.target.value)} />{<FieldError message={errors.pool} />}</label><label>Pinned runner (optional)<Input value={draft.pinnedRunner} onChange={(event) => update('pinnedRunner', event.target.value)} /></label><label>Timeout seconds<Input type="number" min="1" value={draft.timeoutSeconds} onChange={(event) => update('timeoutSeconds', event.target.value)} />{<FieldError message={errors.timeoutSeconds} />}</label><label>Output limit bytes<Input type="number" min="1" value={draft.maxOutputBytes} onChange={(event) => update('maxOutputBytes', event.target.value)} />{<FieldError message={errors.maxOutputBytes} />}</label><label>Maximum attempts<Input type="number" min="1" value={draft.maxAttempts} onChange={(event) => update('maxAttempts', event.target.value)} />{<FieldError message={errors.maxAttempts} />}</label><label>Ambiguity policy<select className="gf-input" value={draft.ambiguityPolicy} onChange={(event) => update('ambiguityPolicy', event.target.value)}><option>REQUIRE_MANUAL_RECONCILIATION</option><option>RETRY</option><option>MARK_FAILED</option></select></label></div><label>Placement selectors JSON<textarea className="gf-input gf-textarea" value={draft.selectors} onChange={(event) => update('selectors', event.target.value)} />{<FieldError message={errors.selectors} />}</label><label>Environment JSON<textarea className="gf-input gf-textarea" value={draft.environment} onChange={(event) => update('environment', event.target.value)} />{<FieldError message={errors.environment} />}</label><label>Secret references JSON<textarea className="gf-input gf-textarea" value={draft.secretReferences} onChange={(event) => update('secretReferences', event.target.value)} />{<FieldError message={errors.secretReferences} />}</label>{review && <section className="gf-review-panel"><h2>Review new immutable version</h2><p><strong>{draft.name || 'Unnamed task'}</strong> will run <code>{commandArguments(draft.command).join(' ') || '—'}</code> with the selected limits and policies.</p><p className="gf-muted">Resolved secret values are never sent to or displayed by the browser.</p></section>}{saveError && <p className="gf-form-error" role="alert">{saveError}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate(-1)}>Cancel</Button><Button type="submit" busy={busy}>{review ? 'Publish version' : 'Review version'}</Button></div></form></main>
}
