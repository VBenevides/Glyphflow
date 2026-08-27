import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { CalendarClock, CircleOff } from 'lucide-react'
import { useAuth } from './auth'
import { api, type GlobalVariable, type Page, type Schedule } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, Dialog, DropdownMenuItem, EmptyState, FieldLabel, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill, TableActions } from './components'
import { QueryRefresh, QueryState } from './query'
import { describeError, FieldError } from './errors'
import { useUnsavedChanges } from './unsaved'
import { TaskPicker } from './task-picker'
import { GlobalVariableInput } from './global-variable-input'
import { formatDateTime } from './format'
import { ScheduleGanttPage } from './schedule-gantt'

export type ScheduleDraft = { taskId: string; name: string; expression: string; timezone: string; misfirePolicy: string; catchupLimit: string; deadlineSeconds: string; concurrencyPolicy: string; maxConcurrentRuns: string }
export const emptyScheduleDraft: ScheduleDraft = { taskId: '', name: '', expression: '0 * * * *', timezone: '0', misfirePolicy: 'SKIP_ALL', catchupLimit: '0', deadlineSeconds: '0', concurrencyPolicy: 'QUEUE', maxConcurrentRuns: '0' }
const globalTimezoneReference = /^\$ENV:[A-Z_][A-Z0-9_]*$/

export function utcOffsetFromTimezone(value: string) {
  if (value === 'UTC') return '0'
  const match = /^UTC([+-])(\d{1,2})(?::00)?$/.exec(value)
  return match ? `${match[1] === '-' ? '-' : ''}${Number(match[2])}` : value
}

export function timezoneFromUTCOffset(value: string) {
  const offset = Number(value)
  if (!Number.isInteger(offset) || offset < -23 || offset > 23) return value.trim()
  const sign = offset < 0 ? '-' : '+'
  return `UTC${sign}${String(Math.abs(offset)).padStart(2, '0')}:00`
}

const scheduleInfo = {
  task: 'Select the task to run.\nValues: every available task, shown as Name (ID).\nThe task active version is used.',
  name: 'Human-readable schedule name.\nValue: required and unique, case-insensitive.\nExample: Every 5 minutes.',
  timezone: 'UTC offset used for calendar schedules.\nValues: whole hours from -23 to +23.\n0 = UTC; -3 = UTC-03:00; +2 = UTC+02:00.',
  cronExpression: 'Five-field cron: minute, hour, day of month, month, weekday (0-6; Sunday=0).\nSupports numbers, *, ranges, lists, and steps such as 1/2 and */5. If both day fields are restricted, either match triggers.\nExample: */5 * * * * = every 5 minutes.',
  misfire: 'What happens when occurrences are missed.\nSKIP_ALL: discard missed occurrences.\nRUN_LATEST: run only the latest missed occurrence.\nRUN_ALL: run every missed occurrence.\nRUN_UP_TO_N: run at most Catch-up limit occurrences.\nFAIL_AND_ALERT: mark the missed schedule as failed.',
  catchup: 'Maximum missed occurrences replayed by RUN_UP_TO_N.\nValues: 0 or a positive whole number.\n0 = no explicit catch-up limit.',
  deadline: 'Maximum delay allowed after the scheduled time for a run to start.\nValues: 0 or a positive number of seconds.\n0 = no start deadline.',
  concurrency: 'How overlapping runs are handled.\nQUEUE: wait for active runs to finish.\nSKIP: ignore a trigger while a run is active.\nREPLACE: replace the active run with the new run.\nALLOW: permit overlap up to Max concurrent runs.',
  maxConcurrent: 'Maximum active runs for this schedule.\nValues: 1 or greater when Concurrency is ALLOW.\nIgnored for QUEUE, SKIP, and REPLACE.',
}

export function scheduleDraftFromRecord(schedule: Schedule): ScheduleDraft {
  return { ...emptyScheduleDraft, taskId: schedule.taskId, name: schedule.name, expression: schedule.expression ?? emptyScheduleDraft.expression, timezone: utcOffsetFromTimezone(schedule.timezone ?? emptyScheduleDraft.timezone), misfirePolicy: schedule.misfirePolicy ?? emptyScheduleDraft.misfirePolicy, catchupLimit: String(schedule.catchupLimit ?? 0), deadlineSeconds: String(schedule.deadlineSeconds ?? 0), concurrencyPolicy: schedule.concurrencyPolicy ?? emptyScheduleDraft.concurrencyPolicy, maxConcurrentRuns: String(schedule.maxConcurrentRuns ?? 0) }
}

export function validateScheduleDraft(draft: ScheduleDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.taskId.trim()) errors.taskId = 'Task is required.'
  if (!draft.name.trim()) errors.name = 'Name is required.'
  if (!draft.expression.trim()) errors.expression = 'Expression is required.'
  const offset = Number(draft.timezone)
  const legacyTimezone = draft.timezone.includes('/') || draft.timezone === 'UTC'
  if ((!Number.isInteger(offset) || offset < -23 || offset > 23) && !legacyTimezone && !globalTimezoneReference.test(draft.timezone.trim())) errors.timezone = 'UTC offset must be a whole number from -23 to +23.'
  if (draft.misfirePolicy === 'RUN_UP_TO_N' && (!Number.isInteger(Number(draft.catchupLimit)) || Number(draft.catchupLimit) < 0)) errors.catchupLimit = 'Use zero or a positive whole number.'
  if (!Number.isInteger(Number(draft.deadlineSeconds)) || Number(draft.deadlineSeconds) < 0) errors.deadlineSeconds = 'Use zero or a positive whole number.'
  if (draft.concurrencyPolicy === 'ALLOW' && (!Number.isInteger(Number(draft.maxConcurrentRuns)) || Number(draft.maxConcurrentRuns) < 1)) errors.maxConcurrentRuns = 'Use one or more concurrent runs.'
  return errors
}

export function previewPayload(draft: ScheduleDraft) {
  return { task_id: draft.taskId.trim(), expression: draft.expression.trim(), timezone: timezoneFromUTCOffset(draft.timezone), starts_at: undefined, ends_at: undefined }
}

function ScheduleListPage() {
  const { permissions } = useAuth(); const [page, setPage] = useState(1); const [limit, setLimit] = useState(10); const [task, setTask] = useState(''); const [editor, setEditor] = useState<{ id?: string } | null>(null)
  const query = useQuery({ queryKey: ['schedules', page, limit, task], queryFn: ({ signal }) => api.get<Page<Schedule>>('/api/v1/schedules', { page, limit, task: task || undefined }, signal), refetchInterval: 5_000 })
  const summaryQuery = useQuery({ queryKey: ['schedule-summary'], queryFn: async ({ signal }) => { const [all, disabled] = await Promise.all([api.get<Page<Schedule>>('/api/v1/schedules', { page: 1, limit: 1 }, signal), api.get<Page<Schedule>>('/api/v1/schedules', { page: 1, limit: 1, enabled: false }, signal)]); return { total: all.total ?? 0, disabled: disabled.total ?? 0 } }, refetchInterval: 5_000 })
  const refresh = async () => { await Promise.all([query.refetch(), summaryQuery.refetch()]) }
  return <main className="gf-content"><PageHeader title="Schedules" description="Versioned triggers with explicit UTC offset and misfire policy." refresh={<QueryRefresh query={query} />} /><div className="gf-metric-grid"><MetricCard label="Total schedules" value={summaryQuery.data?.total ?? '—'} detail="All configured schedules" icon={CalendarClock} tone="info" /><MetricCard label="Disabled schedules" value={summaryQuery.data?.disabled ?? '—'} detail="Schedules not currently firing" icon={CircleOff} tone="warning" /></div><div className="gf-filter-bar"><TaskPicker value={task} onChange={(value) => { setTask(value); setPage(1) }} label="Task" /></div>{permissions.includes('tasks.manage') && <div className="gf-table-toolbar"><Button onClick={() => setEditor({})}>Create schedule</Button></div>}<QueryState query={query} empty="Create a schedule to trigger a task.">{(data) => data.items.length ? <><DataTable caption="Schedules" rows={data.items} columns={[{ key: 'name', label: 'Schedule', render: (schedule) => <Identifier id={schedule.id} name={schedule.name} href={`/schedules/${schedule.id}/edit`} copyLabel="Copy schedule ID" /> }, { key: 'taskId', label: 'Task', render: (schedule) => <Identifier id={schedule.taskId} copyLabel="Copy task ID" /> }, { key: 'timezone', label: 'UTC offset', render: (schedule) => utcOffsetFromTimezone(schedule.timezone ?? 'UTC') }, { key: 'nextFireAt', label: 'Next fire', render: (schedule) => formatDateTime(schedule.nextFireAt) }, { key: 'enabled', label: 'State', render: (schedule) => <StatusPill status={schedule.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'actions', label: 'Actions', render: (schedule) => permissions.includes('tasks.manage') && <TableActions label={`Actions for ${schedule.name}`}><DangerousAction label="Delete" warning="Permanently deletes this schedule and its versions. Existing execution history may block deletion." onConfirm={() => api.delete(`/api/v1/schedules/${encodeURIComponent(schedule.id)}`).then(refresh)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No schedules">Create a schedule to trigger a task.</EmptyState>}</QueryState>{editor && <ScheduleEditorPage editScheduleId={editor.id} inDialog onClose={() => setEditor(null)} onSaved={async () => { setEditor(null); await refresh() }} />}</main>
}

function ScheduleNavigation({ gantt }: { gantt: boolean }) {
  return <nav className="gf-schedule-navigation" role="tablist" aria-label="Schedule sections"><Link role="tab" aria-selected={!gantt} className={!gantt ? 'is-active' : ''} to="/schedules">Schedules</Link><Link role="tab" aria-selected={gantt} className={gantt ? 'is-active' : ''} to="/schedules?tab=gantt">Scheduling Gantt</Link></nav>
}

export function ScheduleInventoryPage() {
  const [params] = useSearchParams()
  const gantt = params.get('tab') === 'gantt'
  return <><ScheduleNavigation gantt={gantt} />{gantt ? <ScheduleGanttPage /> : <ScheduleListPage />}</>
}

type ScheduleEditorProps = { editScheduleId?: string; inDialog?: boolean; onClose?: () => void; onSaved?: () => void | Promise<void> }

export function ScheduleEditorPage({ editScheduleId, inDialog = false, onClose, onSaved }: ScheduleEditorProps = {}) {
  const { scheduleId: routeScheduleId } = useParams(); const scheduleId = editScheduleId ?? routeScheduleId; const navigate = useNavigate(); const { permissions } = useAuth(); const [draft, setDraft] = useState<ScheduleDraft>(emptyScheduleDraft); const [baseline, setBaseline] = useState<ScheduleDraft>(emptyScheduleDraft); const [errors, setErrors] = useState<Record<string, string>>({}); const [preview, setPreview] = useState<string[]>([]); const [previewed, setPreviewed] = useState(false); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['schedule-edit', scheduleId], queryFn: ({ signal }) => api.get<Schedule>(`/api/v1/schedules/${encodeURIComponent(scheduleId ?? '')}`, undefined, signal), enabled: Boolean(scheduleId) })
  const variablesQuery = useQuery({ queryKey: ['global-variable-options'], queryFn: ({ signal }) => api.get<Page<GlobalVariable>>('/api/v1/global-variables/options', { limit: 100 }, signal) })
  useEffect(() => {
    if (query.data) {
      const next = scheduleDraftFromRecord(query.data)
      setDraft(next)
      setBaseline(next)
    } else if (!scheduleId) {
      setDraft(emptyScheduleDraft)
      setBaseline(emptyScheduleDraft)
    }
  }, [query.data, scheduleId])
  const update = (field: keyof ScheduleDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }))
  useUnsavedChanges(JSON.stringify(draft) !== JSON.stringify(baseline))
  if (!permissions.includes('tasks.manage')) return <main className="gf-content"><h1>Access denied</h1></main>
  const showPreview = async () => { const next = validateScheduleDraft(draft); setErrors(next); if (Object.keys(next).length) return; setBusy(true); setPreviewed(true); setPreview([]); setError(''); try { const result = await api.post<{ occurrences?: string[] }>('/api/v1/schedules/preview', previewPayload(draft)); setPreview(result.occurrences ?? []) } catch (cause) { const details = describeError(cause); setError(details.message) } finally { setBusy(false) } }
  const save = async (event: FormEvent) => { event.preventDefault(); const next = validateScheduleDraft(draft); setErrors(next); if (Object.keys(next).length) return; setBusy(true); setError(''); try { await api.post(scheduleId ? `/api/v1/schedules/${encodeURIComponent(scheduleId)}` : '/api/v1/schedules', { ...previewPayload(draft), name: draft.name, misfire_policy: draft.misfirePolicy, catchup_limit: draft.misfirePolicy === 'RUN_UP_TO_N' ? Number(draft.catchupLimit) : 0, start_deadline_seconds: Number(draft.deadlineSeconds), concurrency_policy: draft.concurrencyPolicy, max_concurrent_runs: draft.concurrencyPolicy === 'ALLOW' ? Number(draft.maxConcurrentRuns) : 0 }); if (onSaved) await onSaved(); else navigate('/schedules') } catch (cause) { setError(describeError(cause).message) } finally { setBusy(false) } }
  const title = scheduleId ? 'Edit schedule' : 'Create schedule'
  const close = () => onClose ? onClose() : navigate(-1)
  const form = <form className="gf-editor-form" onSubmit={save}>
        <div className="gf-form-grid">
          <TaskPicker id="schedule-task" value={draft.taskId} onChange={(value) => update('taskId', value)} label="Task" info={scheduleInfo.task} error={errors.taskId} required />
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-name" info={scheduleInfo.name}>Name</FieldLabel><Input id="schedule-name" value={draft.name} onChange={(event) => update('name', event.target.value)} /><FieldError message={errors.name} /></div>
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-timezone" info={scheduleInfo.timezone}>UTC offset</FieldLabel><GlobalVariableInput id="schedule-timezone" value={draft.timezone} variables={variablesQuery.data?.items ?? []} onChange={(value) => update('timezone', value)} /><FieldError message={errors.timezone} /></div>
        </div>
        <div className="gf-form-grid">
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-expression" info={scheduleInfo.cronExpression}>Cron expression</FieldLabel><GlobalVariableInput id="schedule-expression" value={draft.expression} variables={variablesQuery.data?.items ?? []} onChange={(value) => update('expression', value)} /><FieldError message={errors.expression} /></div>
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-misfire" info={scheduleInfo.misfire}>Misfire policy</FieldLabel><select id="schedule-misfire" className="gf-input" value={draft.misfirePolicy} onChange={(event) => setDraft((current) => ({ ...current, misfirePolicy: event.target.value, catchupLimit: event.target.value === 'RUN_UP_TO_N' ? current.catchupLimit : '0' }))}><option>SKIP_ALL</option><option>RUN_LATEST</option><option>RUN_ALL</option><option>RUN_UP_TO_N</option><option>FAIL_AND_ALERT</option></select></div>
        </div>
        <div className="gf-form-grid">
          {draft.misfirePolicy === 'RUN_UP_TO_N' && <div className="gf-form-field"><FieldLabel htmlFor="schedule-catchup-limit" info={scheduleInfo.catchup}>Catch-up limit</FieldLabel><Input id="schedule-catchup-limit" type="number" min="0" value={draft.catchupLimit} onChange={(event) => update('catchupLimit', event.target.value)} /><FieldError message={errors.catchupLimit} /></div>}
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-deadline" info={scheduleInfo.deadline}>Start deadline seconds</FieldLabel><Input id="schedule-deadline" type="number" min="0" value={draft.deadlineSeconds} onChange={(event) => update('deadlineSeconds', event.target.value)} /><FieldError message={errors.deadlineSeconds} /></div>
          <div className="gf-form-field"><FieldLabel htmlFor="schedule-concurrency" info={scheduleInfo.concurrency}>Concurrency</FieldLabel><select id="schedule-concurrency" className="gf-input" value={draft.concurrencyPolicy} onChange={(event) => setDraft((current) => ({ ...current, concurrencyPolicy: event.target.value, maxConcurrentRuns: event.target.value === 'ALLOW' ? current.maxConcurrentRuns : '0' }))}><option>QUEUE</option><option>SKIP</option><option>REPLACE</option><option>ALLOW</option></select></div>
          {draft.concurrencyPolicy === 'ALLOW' && <div className="gf-form-field"><FieldLabel htmlFor="schedule-max-concurrent" info={scheduleInfo.maxConcurrent}>Max concurrent runs</FieldLabel><Input id="schedule-max-concurrent" type="number" min="1" value={draft.maxConcurrentRuns} onChange={(event) => update('maxConcurrentRuns', event.target.value)} /><FieldError message={errors.maxConcurrentRuns} /></div>}
        </div>
        {previewed && !error && (preview.length > 0 ? <section className="gf-review-panel"><h2>Next occurrences</h2><ul>{preview.map((occurrence) => <li key={occurrence}>{occurrence}</li>)}</ul></section> : <EmptyState title="No occurrences">No matching occurrences were returned.</EmptyState>)}
        {error && <p className="gf-form-error" role="alert">{error}</p>}
        <div className="gf-dialog-actions"><Button type="button" variant="secondary" busy={busy} onClick={showPreview}>Preview next occurrences</Button><Button type="button" variant="secondary" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" busy={busy}>Save schedule version</Button></div>
      </form>
  return inDialog ? <Dialog open title={title} onClose={close}>{form}</Dialog> : <main className="gf-content"><PageHeader title={title} description="Preview occurrences before activating a new immutable version." />{form}</main>
}
