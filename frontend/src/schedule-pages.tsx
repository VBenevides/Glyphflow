import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Schedule } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, FieldLabel, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { describeError, FieldError } from './errors'
import { useUnsavedChanges } from './unsaved'
import { TaskPicker } from './task-picker'

export type ScheduleDraft = { taskId: string; name: string; scheduleType: 'cron' | 'interval'; expression: string; timezone: string; misfirePolicy: string; catchupLimit: string; deadlineSeconds: string; concurrencyPolicy: string; maxConcurrentRuns: string }
export const emptyScheduleDraft: ScheduleDraft = { taskId: '', name: '', scheduleType: 'cron', expression: '0 * * * *', timezone: '0', misfirePolicy: 'SKIP_ALL', catchupLimit: '0', deadlineSeconds: '0', concurrencyPolicy: 'QUEUE', maxConcurrentRuns: '0' }

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
  type: 'How the schedule repeats.\nCron: calendar-based five-field expression.\nInterval: fixed duration such as 5m or 1h.',
  cronExpression: 'Cron fields, in order: minute (0-59), hour (0-23), day (1-31), month (1-12), weekday (0-6; Sunday=0).\nUse * for any value, */n for steps, and comma/range lists.\nExample: */5 * * * * = every 5 minutes.',
  intervalExpression: 'Fixed duration between runs.\nValues use Go duration syntax, such as 30s, 5m, or 1h.',
  misfire: 'What happens when occurrences are missed.\nSKIP_ALL: discard missed occurrences.\nRUN_LATEST: run only the latest missed occurrence.\nRUN_ALL: run every missed occurrence.\nRUN_UP_TO_N: run at most Catch-up limit occurrences.\nFAIL_AND_ALERT: mark the missed schedule as failed.',
  catchup: 'Maximum missed occurrences replayed by RUN_UP_TO_N.\nValues: 0 or a positive whole number.\n0 = no explicit catch-up limit.',
  deadline: 'Maximum delay allowed after the scheduled time for a run to start.\nValues: 0 or a positive number of seconds.\n0 = no start deadline.',
  concurrency: 'How overlapping runs are handled.\nQUEUE: wait for active runs to finish.\nSKIP: ignore a trigger while a run is active.\nREPLACE: replace the active run with the new run.\nALLOW: permit overlap up to Max concurrent runs.',
  maxConcurrent: 'Maximum active runs for this schedule.\nValues: 1 or greater when Concurrency is ALLOW.\nIgnored for QUEUE, SKIP, and REPLACE.',
}

export function scheduleDraftFromRecord(schedule: Schedule): ScheduleDraft {
  return { ...emptyScheduleDraft, taskId: schedule.taskId, name: schedule.name, scheduleType: schedule.scheduleType ?? 'cron', expression: schedule.expression ?? emptyScheduleDraft.expression, timezone: utcOffsetFromTimezone(schedule.timezone ?? emptyScheduleDraft.timezone), misfirePolicy: schedule.misfirePolicy ?? emptyScheduleDraft.misfirePolicy, catchupLimit: String(schedule.catchupLimit ?? 0), deadlineSeconds: String(schedule.deadlineSeconds ?? 0), concurrencyPolicy: schedule.concurrencyPolicy ?? emptyScheduleDraft.concurrencyPolicy, maxConcurrentRuns: String(schedule.maxConcurrentRuns ?? 0) }
}

export function validateScheduleDraft(draft: ScheduleDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.taskId.trim()) errors.taskId = 'Task is required.'
  if (!draft.name.trim()) errors.name = 'Name is required.'
  if (!draft.expression.trim()) errors.expression = 'Expression is required.'
  const offset = Number(draft.timezone)
  const legacyTimezone = draft.timezone.includes('/') || draft.timezone === 'UTC'
  if ((!Number.isInteger(offset) || offset < -23 || offset > 23) && !legacyTimezone) errors.timezone = 'UTC offset must be a whole number from -23 to +23.'
  if (!Number.isInteger(Number(draft.catchupLimit)) || Number(draft.catchupLimit) < 0) errors.catchupLimit = 'Use zero or a positive whole number.'
  if (!Number.isInteger(Number(draft.deadlineSeconds)) || Number(draft.deadlineSeconds) < 0) errors.deadlineSeconds = 'Use zero or a positive whole number.'
  if (!Number.isInteger(Number(draft.maxConcurrentRuns)) || Number(draft.maxConcurrentRuns) < 0) errors.maxConcurrentRuns = 'Use zero or a positive whole number.'
  return errors
}

export function previewPayload(draft: ScheduleDraft) {
  return { task_id: draft.taskId.trim(), schedule_type: draft.scheduleType, expression: draft.expression.trim(), timezone: timezoneFromUTCOffset(draft.timezone), starts_at: undefined, ends_at: undefined }
}

export function ScheduleInventoryPage() {
  const { permissions } = useAuth(); const navigate = useNavigate(); const [page, setPage] = useState(1); const [task, setTask] = useState(''); const debouncedTask = useDebouncedValue(task)
  const query = useQuery({ queryKey: ['schedules', page, debouncedTask], queryFn: ({ signal }) => api.get<Page<Schedule>>('/api/v1/schedules', { page, task: debouncedTask || undefined }, signal), refetchInterval: 5_000 })
  return <main className="gf-content"><PageHeader title="Schedules" description="Versioned triggers with explicit UTC offset and misfire policy." action={permissions.includes('tasks.manage') && <Button onClick={() => navigate('/schedules/new')}>Create schedule</Button>} /><div className="gf-filter-bar"><TaskPicker value={task} onChange={(value) => { setTask(value); setPage(1) }} label="Task" /></div><QueryState query={query} empty="Create a schedule to trigger a task.">{(data) => data.items.length ? <><DataTable caption="Schedules" rows={data.items} columns={[{ key: 'name', label: 'Schedule', render: (schedule) => <Link to={`/schedules/${schedule.id}/edit`}>{schedule.name}</Link> }, { key: 'taskId', label: 'Task' }, { key: 'timezone', label: 'UTC offset', render: (schedule) => utcOffsetFromTimezone(schedule.timezone ?? 'UTC') }, { key: 'nextFireAt', label: 'Next fire' }, { key: 'enabled', label: 'State', render: (schedule) => <StatusPill status={schedule.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'actions', label: 'Actions', render: (schedule) => permissions.includes('tasks.manage') && <DangerousAction label="Delete" warning="Permanently deletes this schedule and its versions. Existing execution history may block deletion." onConfirm={() => api.delete(`/api/v1/schedules/${encodeURIComponent(schedule.id)}`).then(() => { void query.refetch() })} /> }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No schedules">Create a schedule to trigger a task.</EmptyState>}</QueryState></main>
}

export function ScheduleEditorPage() {
  const { scheduleId } = useParams(); const navigate = useNavigate(); const { permissions } = useAuth(); const [draft, setDraft] = useState<ScheduleDraft>(emptyScheduleDraft); const [baseline, setBaseline] = useState<ScheduleDraft>(emptyScheduleDraft); const [errors, setErrors] = useState<Record<string, string>>({}); const [preview, setPreview] = useState<string[]>([]); const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['schedule-edit', scheduleId], queryFn: ({ signal }) => api.get<Schedule>(`/api/v1/schedules/${encodeURIComponent(scheduleId ?? '')}`, undefined, signal), enabled: Boolean(scheduleId) })
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
  const showPreview = async () => { const next = validateScheduleDraft(draft); setErrors(next); if (Object.keys(next).length) return; setBusy(true); setError(''); try { const result = await api.post<{ occurrences?: string[] }>('/api/v1/schedules/preview', previewPayload(draft)); setPreview(result.occurrences ?? []) } catch (cause) { const details = describeError(cause); setError(details.message) } finally { setBusy(false) } }
  const save = async (event: FormEvent) => { event.preventDefault(); const next = validateScheduleDraft(draft); setErrors(next); if (Object.keys(next).length) return; setBusy(true); setError(''); try { await api.post(scheduleId ? `/api/v1/schedules/${encodeURIComponent(scheduleId)}` : '/api/v1/schedules', { ...previewPayload(draft), name: draft.name, misfire_policy: draft.misfirePolicy, catchup_limit: Number(draft.catchupLimit), start_deadline_seconds: Number(draft.deadlineSeconds), concurrency_policy: draft.concurrencyPolicy, max_concurrent_runs: Number(draft.maxConcurrentRuns) }); navigate('/schedules') } catch (cause) { setError(describeError(cause).message) } finally { setBusy(false) } }
  return (
    <main className="gf-content">
      <PageHeader title={scheduleId ? 'Edit schedule' : 'Create schedule'} description="Preview occurrences before activating a new immutable version." />
      <form className="gf-editor-form" onSubmit={save}>
        <div className="gf-form-grid">
          <TaskPicker value={draft.taskId} onChange={(value) => update('taskId', value)} label="Task" info={scheduleInfo.task} error={errors.taskId} required />
          <label><FieldLabel info={scheduleInfo.name}>Name</FieldLabel><Input value={draft.name} onChange={(event) => update('name', event.target.value)} /><FieldError message={errors.name} /></label>
          <label><FieldLabel info={scheduleInfo.timezone}>UTC offset</FieldLabel><Input type="number" min="-23" max="23" step="1" value={draft.timezone} onChange={(event) => update('timezone', event.target.value)} /><FieldError message={errors.timezone} /></label>
        </div>
        <div className="gf-form-grid">
          <label><FieldLabel info={scheduleInfo.type}>Type</FieldLabel><select className="gf-input" value={draft.scheduleType} onChange={(event) => update('scheduleType', event.target.value)}><option value="cron">Cron</option><option value="interval">Interval</option></select></label>
          <label><FieldLabel info={draft.scheduleType === 'cron' ? scheduleInfo.cronExpression : scheduleInfo.intervalExpression}>Expression</FieldLabel><Input value={draft.expression} onChange={(event) => update('expression', event.target.value)} /><FieldError message={errors.expression} /></label>
          <label><FieldLabel info={scheduleInfo.misfire}>Misfire policy</FieldLabel><select className="gf-input" value={draft.misfirePolicy} onChange={(event) => update('misfirePolicy', event.target.value)}><option>SKIP_ALL</option><option>RUN_LATEST</option><option>RUN_ALL</option><option>RUN_UP_TO_N</option><option>FAIL_AND_ALERT</option></select></label>
        </div>
        <div className="gf-form-grid">
          <label><FieldLabel info={scheduleInfo.catchup}>Catch-up limit</FieldLabel><Input type="number" min="0" value={draft.catchupLimit} onChange={(event) => update('catchupLimit', event.target.value)} /><FieldError message={errors.catchupLimit} /></label>
          <label><FieldLabel info={scheduleInfo.deadline}>Start deadline seconds</FieldLabel><Input type="number" min="0" value={draft.deadlineSeconds} onChange={(event) => update('deadlineSeconds', event.target.value)} /><FieldError message={errors.deadlineSeconds} /></label>
          <label><FieldLabel info={scheduleInfo.concurrency}>Concurrency</FieldLabel><select className="gf-input" value={draft.concurrencyPolicy} onChange={(event) => update('concurrencyPolicy', event.target.value)}><option>QUEUE</option><option>SKIP</option><option>REPLACE</option><option>ALLOW</option></select></label>
          <label><FieldLabel info={scheduleInfo.maxConcurrent}>Max concurrent runs</FieldLabel><Input type="number" min="0" value={draft.maxConcurrentRuns} onChange={(event) => update('maxConcurrentRuns', event.target.value)} /><FieldError message={errors.maxConcurrentRuns} /></label>
        </div>
        {preview.length > 0 && <section className="gf-review-panel"><h2>Next occurrences</h2><ul>{preview.map((occurrence) => <li key={occurrence}>{occurrence}</li>)}</ul></section>}
        {error && <p className="gf-form-error" role="alert">{error}</p>}
        <div className="gf-dialog-actions"><Button type="button" variant="secondary" busy={busy} onClick={showPreview}>Preview next occurrences</Button><Button type="submit" busy={busy}>Save schedule version</Button></div>
      </form>
    </main>
  )
}
