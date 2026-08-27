import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, ApiError, type GlobalVariable, type Page, type Resource, type Runner, type RunnerPool, type Task } from './api'
import { Button, Dialog, InfoTooltip, Input, PageHeader } from './components'
import { describeError, FieldError } from './errors'
import { useUnsavedChanges } from './unsaved'
import { GlobalVariableInput } from './global-variable-input'

export type EnvironmentEntry = { name: string; value: string }
export type KeyValueEntry = { name: string; value: string }
export type TaskDraft = { name: string; command: string; workingDirectory: string; pool: string; pinnedRunner: string; selectors: KeyValueEntry[]; environment: EnvironmentEntry[]; resources: string[]; timeoutSeconds: string; maxAttempts: string; ambiguityPolicy: string }

export const emptyTaskDraft: TaskDraft = { name: '', command: '', workingDirectory: '', pool: '', pinnedRunner: '', selectors: [], environment: [], resources: [], timeoutSeconds: '300', maxAttempts: '1', ambiguityPolicy: 'REQUIRE_MANUAL_RECONCILIATION' }

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
  return { ...emptyTaskDraft, name: task.name, command: (task.command ?? []).join('\n'), workingDirectory: task.workingDirectory ?? '', pool: task.pool ?? '', pinnedRunner: task.pinnedRunner ?? '', selectors: keyValueRows(task.placementSelectors), environment: environmentRows(task.environment), resources: [...(task.resources ?? [])], timeoutSeconds: task.timeoutSeconds ? String(task.timeoutSeconds) : emptyTaskDraft.timeoutSeconds, maxAttempts: task.maxAttempts ? String(task.maxAttempts) : emptyTaskDraft.maxAttempts, ambiguityPolicy: task.ambiguityPolicy ?? emptyTaskDraft.ambiguityPolicy }
}

export function commandArguments(value: string): string[] {
  return value.split('\n').map((line) => line.trim()).filter(Boolean)
}

export function commandArgumentLabels(value: string): string[] {
  let argument = 0
  return value.split('\n').map((line) => line.trim() ? `Arg ${++argument}:` : '')
}

export function resolveGlobalVariableReferences(value: string, variables: GlobalVariable[]): string {
  const values = new Map(variables.map((item) => [item.name, item.value]))
  return value.replace(/\$ENV:([A-Z_][A-Z0-9_]*)/g, (reference, name: string) => values.get(name) ?? reference)
}

export function finalCommand(value: string, variables: GlobalVariable[]): string {
  return commandArguments(value).map((argument) => JSON.stringify(resolveGlobalVariableReferences(argument, variables))).join(' ')
}

function CommandArgumentField({ value, variables, error, onChange }: { value: string; variables: GlobalVariable[]; error?: string; onChange: (value: string) => void }) {
  const labels = commandArgumentLabels(value)
  const resolvedCommand = finalCommand(value, variables)
  const [scrollTop, setScrollTop] = useState(0)
  return <div className="gf-command-field"><label htmlFor="task-command">Command arguments <small>one argument per line; use $ENV:VARIABLE_NAME for global values</small></label><div className="gf-command-editor"><div className="gf-command-gutter" aria-hidden="true"><div style={{ transform: `translateY(-${scrollTop}px)` }}>{labels.map((label, index) => <span key={index}>{label}</span>)}</div></div><GlobalVariableInput id="task-command" multiline value={value} variables={variables} onChange={onChange} onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)} aria-invalid={Boolean(error)} /></div><FieldError message={error} />{value.includes('$ENV:') && <><label htmlFor="task-final-command">Final Command</label><textarea id="task-final-command" className="gf-input gf-textarea" value={resolvedCommand} readOnly rows={4} /></>}</div>
}

export function validateTaskDraft(draft: TaskDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = 'Name is required.'
  if (!commandArguments(draft.command).length) errors.command = 'Add at least one command argument.'
  if (!draft.pool.trim()) errors.pool = 'Runner pool is required.'
  for (const [field, rows] of [['selectors', draft.selectors]] as const) {
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
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(entry.name.trim()) && !/^\$ENV:[A-Z_][A-Z0-9_]*$/.test(entry.name.trim())) errors[`environment.${index}.name`] = 'Use a valid name or $ENV:VARIABLE_NAME.'
    const name = entry.name.trim()
    if (name && names.has(name)) errors[`environment.${index}.name`] = 'Variable names must be unique.'
    if (name) names.add(name)
  })
  const resources = new Set<string>()
  draft.resources.forEach((resource, index) => {
    const id = resource.trim()
    if (!id) errors[`resources.${index}`] = 'Select a resource.'
    if (id && resources.has(id)) errors[`resources.${index}`] = 'Resources must be unique.'
    if (id) resources.add(id)
  })
  for (const field of ['timeoutSeconds', 'maxAttempts'] as const) if (!Number.isInteger(Number(draft[field])) || Number(draft[field]) <= 0) errors[field] = 'Enter a positive whole number.'
  return errors
}

function payload(draft: TaskDraft) {
  return { name: draft.name.trim(), command: commandArguments(draft.command), working_directory: draft.workingDirectory, runner_pool: draft.pool.trim(), pinned_runner: draft.pinnedRunner.trim(), placement_selectors: keyValueObject(draft.selectors, true), environment: environmentObject(draft.environment), resources: draft.resources.map((resource) => resource.trim()).filter(Boolean), timeout_seconds: Number(draft.timeoutSeconds), max_attempts: Number(draft.maxAttempts), ambiguity_policy: draft.ambiguityPolicy }
}

function KeyValueEditor({ legend, info, rows, errors, onChange, onAdd, onRemove, valueLabel }: { legend: string; info?: string; rows: KeyValueEntry[]; errors: Record<string, string>; onChange: (index: number, field: keyof KeyValueEntry, value: string) => void; onAdd: () => void; onRemove: (index: number) => void; valueLabel: string }) {
  return <fieldset><legend>{legend} {info && <InfoTooltip text={info} />}</legend><div className="gf-table-wrap"><table className="gf-table"><caption className="gf-sr-only">{legend}</caption><thead><tr><th scope="col">Name</th><th scope="col">{valueLabel}</th><th scope="col"><span className="gf-sr-only">Actions</span></th></tr></thead><tbody>{rows.map((entry, index) => <tr key={index}><td><Input aria-label={`${legend} name ${index + 1}`} value={entry.name} onChange={(event) => onChange(index, 'name', event.target.value)} aria-invalid={Boolean(errors[`selectors.${index}.name`])} />{<FieldError message={errors[`selectors.${index}.name`]} />}</td><td><Input aria-label={`${legend} value ${index + 1}`} value={entry.value} onChange={(event) => onChange(index, 'value', event.target.value)} /></td><td><Button type="button" variant="ghost" onClick={() => onRemove(index)}>Remove</Button></td></tr>)}</tbody></table></div><Button type="button" variant="secondary" onClick={onAdd}>Add row</Button></fieldset>
}

function ResourceEditor({ selected, resources, errors, onChange, onAdd, onRemove }: { selected: string[]; resources: Resource[]; errors: Record<string, string>; onChange: (index: number, value: string) => void; onAdd: () => void; onRemove: (index: number) => void }) {
  return <fieldset><legend>Resources</legend><div className="gf-table-wrap"><table className="gf-table"><caption className="gf-sr-only">Task resources</caption><thead><tr><th scope="col">Resource</th><th scope="col">Type</th><th scope="col"><span className="gf-sr-only">Actions</span></th></tr></thead><tbody>{selected.map((resourceID, index) => { const resource = resources.find((item) => item.id === resourceID); const kind = resource?.kind?.toLowerCase().replace('_', '-'); return <tr key={`${resourceID}-${index}`}><td><select className="gf-input" aria-label={`Task resource ${index + 1}`} value={resourceID} onChange={(event) => onChange(index, event.target.value)} aria-invalid={Boolean(errors[`resources.${index}`])}><option value="">Select a resource</option>{resources.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select><FieldError message={errors[`resources.${index}`]} /></td><td>{kind === 'non-blocking' ? 'Non-blocking' : 'Exclusive'}</td><td><Button type="button" variant="ghost" onClick={() => onRemove(index)}>Remove</Button></td></tr> })}</tbody></table></div>{!resources.length && <p className="gf-muted">No resources are available.</p>}<Button type="button" variant="secondary" onClick={onAdd}>Add resource</Button></fieldset>
}

type TaskEditorProps = { editTaskId?: string; inDialog?: boolean; onClose?: () => void; onSaved?: () => void | Promise<void> }

export function TaskEditorPage({ editTaskId, inDialog = false, onClose, onSaved }: TaskEditorProps = {}) {
  const { taskId: routeTaskId } = useParams()
  const taskId = editTaskId ?? routeTaskId
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
  const variablesQuery = useQuery({ queryKey: ['global-variable-options'], queryFn: ({ signal }) => api.get<Page<GlobalVariable>>('/api/v1/global-variables/options', { limit: 100 }, signal) })
  const resourcesQuery = useQuery({ queryKey: ['task-resource-options'], queryFn: ({ signal }) => api.get<Page<Resource>>('/api/v1/resources', { limit: 100 }, signal) })
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
  const updateKeyValue = (index: number, entryField: keyof KeyValueEntry, value: string) => setDraft((current) => ({ ...current, selectors: current.selectors.map((entry, entryIndex) => entryIndex === index ? { ...entry, [entryField]: value } : entry) }))
  const addKeyValue = () => setDraft((current) => ({ ...current, selectors: [...current.selectors, { name: '', value: '' }] }))
  const removeKeyValue = (index: number) => setDraft((current) => ({ ...current, selectors: current.selectors.filter((_, entryIndex) => entryIndex !== index) }))
  const updateEnvironment = (index: number, field: keyof EnvironmentEntry, value: string) => setDraft((current) => ({ ...current, environment: current.environment.map((entry, entryIndex) => entryIndex === index ? { ...entry, [field]: value } : entry) }))
  const addEnvironment = () => setDraft((current) => ({ ...current, environment: [...current.environment, { name: '', value: '' }] }))
  const removeEnvironment = (index: number) => setDraft((current) => ({ ...current, environment: current.environment.filter((_, entryIndex) => entryIndex !== index) }))
  const updateResource = (index: number, value: string) => setDraft((current) => ({ ...current, resources: current.resources.map((resource, resourceIndex) => resourceIndex === index ? value : resource) }))
  const addResource = () => setDraft((current) => ({ ...current, resources: [...current.resources, ''] }))
  const removeResource = (index: number) => setDraft((current) => ({ ...current, resources: current.resources.filter((_, resourceIndex) => resourceIndex !== index) }))
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const nextErrors = validateTaskDraft(draft)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length) return
    setBusy(true); setSaveError('')
    try { await api.post(taskId ? `/api/v1/tasks/${encodeURIComponent(taskId)}/versions` : '/api/v1/tasks', payload(draft)); if (onSaved) await onSaved(); else navigate(taskId ? `/tasks/${encodeURIComponent(taskId)}` : '/tasks') } catch (cause) { const error = describeError(cause); setSaveError(`${error.message}${error.correlationId ? ` (${error.correlationId})` : ''}`); if (cause instanceof ApiError && cause.status === 422) setErrors(error.fields) } finally { setBusy(false) }
  }
  if (!permissions.includes('tasks.manage')) return <main className="gf-content"><h1>Access denied</h1></main>
  const runners = (runnersQuery.data?.items ?? []).filter((runner) => runner.poolId === draft.pool || runner.pool === draft.pool)
  const title = taskId ? 'Edit task version' : 'Create task'
  const close = () => onClose ? onClose() : navigate(-1)
  const form = <form className="gf-editor-form" onSubmit={submit}><label>Name<Input value={draft.name} onChange={(event) => update('name', event.target.value)} aria-invalid={Boolean(errors.name)} />{<FieldError message={errors.name} />}</label><CommandArgumentField value={draft.command} variables={variablesQuery.data?.items ?? []} error={errors.command} onChange={(value) => update('command', value)} /><label>Working directory<GlobalVariableInput value={draft.workingDirectory} variables={variablesQuery.data?.items ?? []} onChange={(value) => update('workingDirectory', value)} /></label><div className="gf-form-grid"><label>Runner pool<select className="gf-input" value={draft.pool} onChange={(event) => setDraft((current) => ({ ...current, pool: event.target.value, pinnedRunner: '' }))} required disabled={poolsQuery.isPending}><option value="">Select a pool</option>{(poolsQuery.data?.items ?? []).filter((pool) => pool.enabled !== false).map((pool) => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select>{<FieldError message={errors.pool} />}</label>{draft.pool && <label>Runner<select className="gf-input" value={draft.pinnedRunner} onChange={(event) => update('pinnedRunner', event.target.value)} disabled={runnersQuery.isPending}><option value="">Any in Pool</option>{runners.filter((runner) => runner.observedState?.toUpperCase() !== 'REVOKED').map((runner) => <option key={runner.id} value={runner.id}>{runner.name}</option>)}</select></label>}<label>Execution Timeout Seconds<Input type="number" min="1" value={draft.timeoutSeconds} onChange={(event) => update('timeoutSeconds', event.target.value)} />{<FieldError message={errors.timeoutSeconds} />}</label><label>Maximum attempts<Input type="number" min="1" value={draft.maxAttempts} onChange={(event) => update('maxAttempts', event.target.value)} />{<FieldError message={errors.maxAttempts} />}</label><label>Ambiguity policy<select className="gf-input" value={draft.ambiguityPolicy} onChange={(event) => update('ambiguityPolicy', event.target.value)}><option>REQUIRE_MANUAL_RECONCILIATION</option><option>RETRY</option><option>MARK_FAILED</option></select></label></div><fieldset><legend>Environment variables</legend><div className="gf-table-wrap gf-environment-table-wrap"><table className="gf-table"><caption className="gf-sr-only">Task environment variables</caption><thead><tr><th scope="col">Variable Name</th><th scope="col">Variable Value</th><th scope="col"><span className="gf-sr-only">Actions</span></th></tr></thead><tbody>{draft.environment.map((entry, index) => <tr key={index}><td><GlobalVariableInput aria-label={`Environment variable name ${index + 1}`} value={entry.name} variables={variablesQuery.data?.items ?? []} onChange={(value) => updateEnvironment(index, 'name', value)} aria-invalid={Boolean(errors[`environment.${index}.name`])} />{<FieldError message={errors[`environment.${index}.name`]} />}</td><td><GlobalVariableInput aria-label={`Environment variable value ${index + 1}`} value={entry.value} variables={variablesQuery.data?.items ?? []} onChange={(value) => updateEnvironment(index, 'value', value)} /></td><td><Button type="button" variant="ghost" onClick={() => removeEnvironment(index)}>Remove</Button></td></tr>)}</tbody></table></div><Button type="button" variant="secondary" onClick={addEnvironment}>Add variable</Button></fieldset><ResourceEditor selected={draft.resources} resources={resourcesQuery.data?.items ?? []} errors={errors} onChange={updateResource} onAdd={addResource} onRemove={removeResource} /><KeyValueEditor legend="Tags" info="Optional key/value tags that must match runner capabilities. Example: os = linux. Leave empty to allow any runner in the selected pool." rows={draft.selectors} errors={errors} valueLabel="Tag value" onChange={updateKeyValue} onAdd={addKeyValue} onRemove={removeKeyValue} />{saveError && <p className="gf-form-error" role="alert">{saveError}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={close}>Cancel</Button><Button type="submit" busy={busy}>{taskId ? 'Publish version' : 'Create task'}</Button></div></form>
  return inDialog ? <Dialog open title={title} className="gf-task-editor-dialog" onClose={close}>{form}</Dialog> : <main className="gf-content"><PageHeader title={title} description="Versions are immutable after publication." />{form}</main>
}
