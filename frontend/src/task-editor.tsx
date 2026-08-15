import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, ApiError, type GlobalVariable, type Page, type Runner, type RunnerPool, type Task } from './api'
import { Button, Input, PageHeader } from './components'
import { describeError, FieldError } from './errors'
import { useUnsavedChanges } from './unsaved'

export type EnvironmentEntry = { name: string; value: string }
export type KeyValueEntry = { name: string; value: string }
export type TaskDraft = { name: string; command: string; workingDirectory: string; pool: string; pinnedRunner: string; selectors: KeyValueEntry[]; environment: EnvironmentEntry[]; secretReferences: KeyValueEntry[]; timeoutSeconds: string; maxOutputBytes: string; maxAttempts: string; ambiguityPolicy: string }

export const emptyTaskDraft: TaskDraft = { name: '', command: '', workingDirectory: '', pool: '', pinnedRunner: '', selectors: [], environment: [], secretReferences: [], timeoutSeconds: '300', maxOutputBytes: '1048576', maxAttempts: '1', ambiguityPolicy: 'REQUIRE_MANUAL_RECONCILIATION' }

export function environmentRows(value: Record<string, string> | undefined): EnvironmentEntry[] {
  return Object.entries(value ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([name, entryValue]) => ({ name, value: String(entryValue) }))
}

export function environmentObject(rows: EnvironmentEntry[]): Record<string, string> {
  return Object.fromEntries(rows.map(({ name, value }) => [name.trim(), value]).filter(([name]) => name))
}

export function keyValueRows(value: Record<string, unknown> | undefined): KeyValueEntry[] {
  return Object.entries(value ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([name, entryValue]) => ({ name, value: typeof entryValue === 'string' ? entryValue : JSON.stringify(entryValue) }))
}

export function keyValueObject(rows: KeyValueEntry[], parseValues = false): Record<string, unknown> {
  return Object.fromEntries(rows.map(({ name, value }) => {
    const key = name.trim()
    if (!key) return [key, value]
    if (!parseValues) return [key, value]
    try { return [key, JSON.parse(value)] } catch { return [key, value] }
  }).filter(([name]) => name))
}

export function taskDraftFromRecord(task: Task): TaskDraft {
  return { ...emptyTaskDraft, name: task.name, command: (task.command ?? []).join('\n'), workingDirectory: task.workingDirectory ?? '', pool: task.pool ?? '', pinnedRunner: task.pinnedRunner ?? '', selectors: keyValueRows(task.placementSelectors), environment: environmentRows(task.environment), secretReferences: keyValueRows(task.secretReferences), timeoutSeconds: task.timeoutSeconds ? String(task.timeoutSeconds) : emptyTaskDraft.timeoutSeconds, maxOutputBytes: task.maxOutputBytes ? String(task.maxOutputBytes) : emptyTaskDraft.maxOutputBytes, maxAttempts: task.maxAttempts ? String(task.maxAttempts) : emptyTaskDraft.maxAttempts, ambiguityPolicy: task.ambiguityPolicy ?? emptyTaskDraft.ambiguityPolicy }
}

export function commandArguments(value: string): string[] {
  return value.split('\n').map((line) => line.trim()).filter(Boolean)
}

export function validateTaskDraft(draft: TaskDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = 'Name is required.'
  if (!commandArguments(draft.command).length) errors.command = 'Add at least one command argument.'
  if (!draft.pool.trim()) errors.pool = 'Runner pool is required.'
  for (const [field, rows] of [['selectors', draft.selectors], ['secretReferences', draft.secretReferences] ] as const) {
    const names = new Set<string>()
    rows.forEach((entry, index) => {
      const name = entry.name.trim()
      if (!name) errors[`${field}.${index}.name`] = 'Name is required.'
      if (name && names.has(name)) errors[`${field}.${index}.name`] = 'Names must be unique.'
      if (name) names.add(name)
    })
  }
  const names = new Set<string>()
  draft.environment.forEach((entry, index) => {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(entry.name.trim())) errors[`environment.${index}.name`] = 'Use letters, numbers, and underscores.'
    const name = entry.name.trim()
    if (name && names.has(name)) errors[`environment.${index}.name`] = 'Variable names must be unique.'
    if (name) names.add(name)
  })
  for (const field of ['timeoutSeconds', 'maxOutputBytes', 'maxAttempts'] as const) if (!Number.isInteger(Number(draft[field])) || Number(draft[field]) <= 0) errors[field] = 'Enter a positive whole number.'
  return errors
}

function payload(draft: TaskDraft) {
  return { name: draft.name.trim(), command: commandArguments(draft.command), working_directory: draft.workingDirectory, runner_pool: draft.pool.trim(), pinned_runner: draft.pinnedRunner.trim(), placement_selectors: keyValueObject(draft.selectors, true), environment: environmentObject(draft.environment), secret_references: keyValueObject(draft.secretReferences), timeout_seconds: Number(draft.timeoutSeconds), max_output_bytes: Number(draft.maxOutputBytes), max_attempts: Number(draft.maxAttempts), ambiguity_policy: draft.ambiguityPolicy }
}

function KeyValueEditor({ legend, rows, errors, onChange, onAdd, onRemove, valueLabel }: { legend: string; rows: KeyValueEntry[]; errors: Record<string, string>; onChange: (index: number, field: keyof KeyValueEntry, value: string) => void; onAdd: () => void; onRemove: (index: number) => void; valueLabel: string }) {
  const field = legend === 'Placement selectors' ? 'selectors' : 'secretReferences'
  return <fieldset><legend>{legend}</legend><table className="gf-table"><caption className="gf-sr-only">{legend}</caption><thead><tr><th scope="col">Name</th><th scope="col">{valueLabel}</th><th scope="col"><span className="gf-sr-only">Actions</span></th></tr></thead><tbody>{rows.map((entry, index) => <tr key={index}><td><Input aria-label={`${legend} name ${index + 1}`} value={entry.name} onChange={(event) => onChange(index, 'name', event.target.value)} aria-invalid={Boolean(errors[`${field}.${index}.name`])} />{<FieldError message={errors[`${field}.${index}.name`]} />}</td><td><Input aria-label={`${legend} value ${index + 1}`} value={entry.value} onChange={(event) => onChange(index, 'value', event.target.value)} /></td><td><Button type="button" variant="ghost" onClick={() => onRemove(index)}>Remove</Button></td></tr>)}</tbody></table><Button type="button" variant="secondary" onClick={onAdd}>Add row</Button></fieldset>
}

export function TaskEditorPage() {
  const { taskId } = useParams()
  const navigate = useNavigate()
  const { permissions } = useAuth()
  const [draft, setDraft] = useState<TaskDraft>(emptyTaskDraft)
  const [baseline, setBaseline] = useState<TaskDraft>(emptyTaskDraft)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [saveError, setSaveError] = useState('')
  const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['task-edit', taskId], queryFn: ({ signal }) => api.get<Task>(`/api/v1/tasks/${encodeURIComponent(taskId ?? '')}`, undefined, signal), enabled: Boolean(taskId) })
  const poolsQuery = useQuery({ queryKey: ['runner-pools'], queryFn: ({ signal }) => api.get<Page<RunnerPool>>('/api/v1/runners/pools', { limit: 100 }, signal) })
  const runnersQuery = useQuery({ queryKey: ['runners'], queryFn: ({ signal }) => api.get<Page<Runner>>('/api/v1/runners', { limit: 100 }, signal) })
  const variablesQuery = useQuery({ queryKey: ['global-variables'], queryFn: ({ signal }) => api.get<Page<GlobalVariable>>('/api/v1/global-variables', { limit: 100 }, signal) })
  useEffect(() => {
    if (query.data) {
      const next = taskDraftFromRecord(query.data)
      setDraft(next)
      setBaseline(next)
    } else if (!taskId) {
      setDraft(emptyTaskDraft)
      setBaseline(emptyTaskDraft)
    }
  }, [query.data, taskId])
  useEffect(() => {
    if (taskId || draft.pool || !poolsQuery.data?.items.length) return
    const pool = poolsQuery.data.items.find((item) => item.id === 'default') ?? poolsQuery.data.items[0]
    setDraft((current) => current.pool ? current : { ...current, pool: pool.id })
  }, [draft.pool, poolsQuery.data, taskId])
  useUnsavedChanges(JSON.stringify(draft) !== JSON.stringify(baseline))
  const update = (field: keyof TaskDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }))
  const updateKeyValue = (field: 'selectors' | 'secretReferences', index: number, entryField: keyof KeyValueEntry, value: string) => setDraft((current) => ({ ...current, [field]: current[field].map((entry, entryIndex) => entryIndex === index ? { ...entry, [entryField]: value } : entry) }))
  const addKeyValue = (field: 'selectors' | 'secretReferences') => setDraft((current) => ({ ...current, [field]: [...current[field], { name: '', value: '' }] }))
  const removeKeyValue = (field: 'selectors' | 'secretReferences', index: number) => setDraft((current) => ({ ...current, [field]: current[field].filter((_, entryIndex) => entryIndex !== index) }))
  const updateEnvironment = (index: number, field: keyof EnvironmentEntry, value: string) => setDraft((current) => ({ ...current, environment: current.environment.map((entry, entryIndex) => entryIndex === index ? { ...entry, [field]: value } : entry) }))
  const addEnvironment = () => setDraft((current) => ({ ...current, environment: [...current.environment, { name: '', value: '' }] }))
  const removeEnvironment = (index: number) => setDraft((current) => ({ ...current, environment: current.environment.filter((_, entryIndex) => entryIndex !== index) }))
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const nextErrors = validateTaskDraft(draft)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length) return
    setBusy(true); setSaveError('')
    try { await api.post(taskId ? `/api/v1/tasks/${encodeURIComponent(taskId)}/versions` : '/api/v1/tasks', payload(draft)); navigate(taskId ? `/tasks/${encodeURIComponent(taskId)}` : '/tasks') } catch (cause) { const error = describeError(cause); setSaveError(`${error.message}${error.correlationId ? ` (${error.correlationId})` : ''}`); if (cause instanceof ApiError && cause.status === 422) setErrors(error.fields) } finally { setBusy(false) }
  }
  if (!permissions.includes('tasks.manage')) return <main className="gf-content"><h1>Access denied</h1></main>
  const runners = (runnersQuery.data?.items ?? []).filter((runner) => runner.poolId === draft.pool || runner.pool === draft.pool)
  return <main className="gf-content"><PageHeader title={taskId ? 'Edit task version' : 'Create task'} description="Versions are immutable after publication." /><form className="gf-editor-form" onSubmit={submit}><label>Name<Input value={draft.name} onChange={(event) => update('name', event.target.value)} aria-invalid={Boolean(errors.name)} />{<FieldError message={errors.name} />}</label><label>Command arguments <small>one argument per line; no shell parsing</small><textarea className="gf-input gf-textarea" value={draft.command} onChange={(event) => update('command', event.target.value)} aria-invalid={Boolean(errors.command)} />{<FieldError message={errors.command} />}</label><div className="gf-form-grid"><label>Runner pool<select className="gf-input" value={draft.pool} onChange={(event) => setDraft((current) => ({ ...current, pool: event.target.value, pinnedRunner: '' }))} required disabled={poolsQuery.isPending}><option value="">Select a pool</option>{(poolsQuery.data?.items ?? []).filter((pool) => pool.enabled !== false).map((pool) => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select>{<FieldError message={errors.pool} />}</label>{draft.pool && <label>Runner<select className="gf-input" value={draft.pinnedRunner} onChange={(event) => update('pinnedRunner', event.target.value)} disabled={runnersQuery.isPending}><option value="">Any in Pool</option>{runners.filter((runner) => runner.observedState?.toUpperCase() !== 'REVOKED').map((runner) => <option key={runner.id} value={runner.id}>{runner.name}</option>)}</select></label>}<label>Timeout seconds<Input type="number" min="1" value={draft.timeoutSeconds} onChange={(event) => update('timeoutSeconds', event.target.value)} />{<FieldError message={errors.timeoutSeconds} />}</label><label>Output limit bytes<Input type="number" min="1" value={draft.maxOutputBytes} onChange={(event) => update('maxOutputBytes', event.target.value)} />{<FieldError message={errors.maxOutputBytes} />}</label><label>Maximum attempts<Input type="number" min="1" value={draft.maxAttempts} onChange={(event) => update('maxAttempts', event.target.value)} />{<FieldError message={errors.maxAttempts} />}</label><label>Ambiguity policy<select className="gf-input" value={draft.ambiguityPolicy} onChange={(event) => update('ambiguityPolicy', event.target.value)}><option>REQUIRE_MANUAL_RECONCILIATION</option><option>RETRY</option><option>MARK_FAILED</option></select></label></div><KeyValueEditor legend="Placement selectors" rows={draft.selectors} errors={errors} valueLabel="Selector value" onChange={(index, field, value) => updateKeyValue('selectors', index, field, value)} onAdd={() => addKeyValue('selectors')} onRemove={(index) => removeKeyValue('selectors', index)} /><fieldset><legend>Environment variables</legend><table className="gf-table"><caption className="gf-sr-only">Task environment variables</caption><thead><tr><th scope="col">Variable Name</th><th scope="col">Variable Value</th><th scope="col"><span className="gf-sr-only">Actions</span></th></tr></thead><tbody>{draft.environment.map((entry, index) => <tr key={index}><td><Input list="global-variable-names" aria-label={`Environment variable name ${index + 1}`} value={entry.name} onChange={(event) => updateEnvironment(index, 'name', event.target.value)} aria-invalid={Boolean(errors[`environment.${index}.name`])} />{<FieldError message={errors[`environment.${index}.name`]} />}</td><td><Input aria-label={`Environment variable value ${index + 1}`} value={entry.value} onChange={(event) => updateEnvironment(index, 'value', event.target.value)} /></td><td><Button type="button" variant="ghost" onClick={() => removeEnvironment(index)}>Remove</Button></td></tr>)}</tbody></table><datalist id="global-variable-names">{(variablesQuery.data?.items ?? []).map((item) => <option key={item.id} value={item.name} />)}</datalist><Button type="button" variant="secondary" onClick={addEnvironment}>Add variable</Button></fieldset><KeyValueEditor legend="Secret references" rows={draft.secretReferences} errors={errors} valueLabel="Reference" onChange={(index, field, value) => updateKeyValue('secretReferences', index, field, value)} onAdd={() => addKeyValue('secretReferences')} onRemove={(index) => removeKeyValue('secretReferences', index)} />{saveError && <p className="gf-form-error" role="alert">{saveError}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => navigate(-1)}>Cancel</Button><Button type="submit" busy={busy}>{taskId ? 'Publish version' : 'Create task'}</Button></div></form></main>
}
