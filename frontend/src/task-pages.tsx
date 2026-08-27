import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Archive, CircleOff, ListChecks, Server, Timer } from 'lucide-react'
import { useAuth } from './auth'
import { api, type Page, type Task, type TaskVersion } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, FilterInput, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill } from './components'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'
import { formatDateTime } from './format'
import { TaskEditorPage } from './task-editor'
import { ManualRunPage } from './run-pages'

export function taskQuery(search: string, state: string, page: number, limit?: number, archived = false) {
  return { search: search || undefined, state: state || undefined, page, ...(limit ? { limit } : {}), ...(archived ? { archived: true } : {}) }
}

export function taskStateMatches(task: Pick<Task, 'enabled'>, state: string) {
  return !state || (state === 'enabled' ? task.enabled !== false : task.enabled === false)
}

export function taskNameLabel(name: string, maxLength = 30) {
  return name.length > maxLength ? `${name.slice(0, maxLength)}…` : name
}

export function TaskInventoryPage() {
  const { permissions } = useAuth()
  const [params] = useSearchParams()
  const archived = params.get('archived') === 'true'
  const [editorOpen, setEditorOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [state, setState] = useState('')
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const query = useQuery({ queryKey: ['tasks', archived, search, state, page, limit], queryFn: ({ signal }) => api.get<Page<Task>>('/api/v1/tasks', taskQuery(search, state, page, limit, archived), signal) })
  const optionsQuery = useQuery({ queryKey: ['task-filter-options', archived], queryFn: ({ signal }) => api.get<Page<Task>>('/api/v1/tasks', { all: true, archived }, signal) })
  const taskOptions = optionsQuery.data?.items ?? query.data?.items ?? []
  const summaryQuery = useQuery({ queryKey: ['task-summary'], queryFn: async ({ signal }) => { const [all, disabled, archivedTasks] = await Promise.all([api.get<Page<Task>>('/api/v1/tasks', { page: 1, limit: 1 }, signal), api.get<Page<Task>>('/api/v1/tasks', { page: 1, limit: 1, state: 'disabled' }, signal), api.get<Page<Task>>('/api/v1/tasks', { page: 1, limit: 1, archived: true }, signal)]); return { total: all.total ?? 0, disabled: disabled.total ?? 0, archived: archivedTasks.total ?? 0 } }, refetchInterval: 5_000 })
  const refresh = async () => { await Promise.all([query.refetch(), summaryQuery.refetch()]) }
  const canManage = hasPermission(permissions, 'tasks.manage')
  return <main className="gf-content"><PageHeader title="Tasks" description="Versioned commands and their schedules." refresh={<QueryRefresh query={query} />} /><nav className="gf-account-tabs" aria-label="Task status"><Link className={!archived ? 'is-active' : ''} to="/tasks">Tasks</Link><Link className={archived ? 'is-active' : ''} to="/tasks?archived=true">Archived Tasks</Link></nav><div className="gf-metric-grid"><MetricCard label="Total tasks" value={summaryQuery.data?.total ?? '—'} detail="All configured tasks" icon={ListChecks} tone="info" /><MetricCard label="Disabled tasks" value={summaryQuery.data?.disabled ?? '—'} detail="Tasks not accepting work" icon={CircleOff} tone="warning" /><MetricCard label="Archived tasks" value={summaryQuery.data?.archived ?? '—'} detail="Archived task history" icon={Archive} tone="default" /></div><div className="gf-filter-bar"><FilterInput label="Search" options={taskOptions.flatMap((task) => [task.name, task.id])} value={search} onChange={(value) => { setSearch(value); setPage(1) }} placeholder="Name or ID" /><label>State <select className="gf-input" value={state} onChange={(event) => { setState(event.target.value); setPage(1) }}><option value="">All</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></label></div>{canManage && <div className="gf-table-toolbar"><Button onClick={() => setEditorOpen(true)}>Create task</Button></div>}<QueryState query={query} empty={archived ? 'No archived tasks.' : 'Create a task version to schedule work.'}>{(data) => { const tasks = data.items.filter((task) => taskStateMatches(task, state)); return tasks.length ? <><DataTable caption={archived ? 'Archived tasks' : 'Tasks'} rows={tasks} columns={[{ key: 'name', label: 'Task Name', render: (task) => <Link to={`/tasks/${encodeURIComponent(task.id)}`} title={task.name}>{taskNameLabel(task.name)}</Link> }, { key: 'id', label: 'Task ID', render: (task) => <Identifier id={task.id} copyLabel="Copy task ID" /> }, { key: 'enabled', label: 'State', render: (task) => <StatusPill status={task.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'activeVersion', label: 'Version' }, { key: 'pool', label: 'Runner pool' }, { key: 'latestRun', label: 'Latest run', render: (task) => task.latestRun ? <StatusPill status={task.latestRun.state} /> : '—' }]} /><Pagination page={data.page} pages={data.pages ?? (data.total ? Math.ceil(data.total / data.limit) : 1)} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title={archived ? 'No archived tasks' : 'No matching tasks'}>{state ? `No ${state} tasks match this filter.` : 'Create a task version to schedule work.'}</EmptyState> }}</QueryState>{editorOpen && <TaskEditorPage inDialog onClose={() => setEditorOpen(false)} onSaved={async () => { setEditorOpen(false); await refresh() }} />}</main>
}

export function taskDetailLinks(taskId: string) {
  return { schedules: `/schedules?task=${encodeURIComponent(taskId)}`, runs: `/runs?task=${encodeURIComponent(taskId)}`, audit: `/audit?target=${encodeURIComponent(taskId)}`, versions: `/api/v1/tasks/${encodeURIComponent(taskId)}/versions` }
}

export type TaskVersionDiff = { id: string; field: string; previous: string; current: string }

export function taskVersionDiff(previous: TaskVersion, current: TaskVersion): TaskVersionDiff[] {
  const rows: TaskVersionDiff[] = [
    { id: 'command', field: 'Command', previous: previous.command?.join(' ') ?? '—', current: current.command?.join(' ') ?? '—' },
    { id: 'pool', field: 'Runner pool', previous: previous.pool ?? '—', current: current.pool ?? '—' },
    { id: 'pinned-runner', field: 'Pinned runner', previous: previous.pinnedRunner || 'Any', current: current.pinnedRunner || 'Any' },
    { id: 'working-directory', field: 'Working directory', previous: previous.workingDirectory ?? '—', current: current.workingDirectory ?? '—' },
    { id: 'resources', field: 'Resources', previous: [...(previous.resources ?? [])].sort().join(', ') || 'None', current: [...(current.resources ?? [])].sort().join(', ') || 'None' },
    { id: 'duration', field: 'Task Duration in Seconds', previous: previous.durationSeconds === undefined ? '—' : `${previous.durationSeconds}s`, current: current.durationSeconds === undefined ? '—' : `${current.durationSeconds}s` },
    { id: 'maximum-attempts', field: 'Maximum attempts', previous: previous.maxAttempts === undefined ? '—' : String(previous.maxAttempts), current: current.maxAttempts === undefined ? '—' : String(current.maxAttempts) },
    { id: 'ambiguity-policy', field: 'Ambiguity policy', previous: previous.ambiguityPolicy ?? '—', current: current.ambiguityPolicy ?? '—' },
  ]
  return rows.filter((row) => row.previous !== row.current)
}

function ManualTaskRunButton({ taskId }: { taskId: string }) {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  return <><Button type="button" onClick={() => setOpen(true)}>Run now</Button>{open && <ManualRunPage inDialog initialTaskId={taskId} onClose={() => setOpen(false)} onStarted={(result) => { setOpen(false); if (result.id) navigate(`/runs/${result.id}`) }} />}</>
}

export function TaskDetailPage() {
  const { taskId = '' } = useParams()
  const { permissions } = useAuth()
  const navigate = useNavigate()
  const [compareVersion, setCompareVersion] = useState<number>()
  const [versionPage, setVersionPage] = useState(1); const [versionLimit, setVersionLimit] = useState(10)
  const [editorOpen, setEditorOpen] = useState(false)
  const query = useQuery({ queryKey: ['task', taskId], queryFn: ({ signal }) => api.get<Task>(`/api/v1/tasks/${encodeURIComponent(taskId)}`, undefined, signal), enabled: Boolean(taskId) })
  const links = useMemo(() => taskDetailLinks(taskId), [taskId])
  const versionsQuery = useQuery({ queryKey: ['task-versions', taskId], queryFn: ({ signal }) => api.get<TaskVersion[]>(links.versions, undefined, signal), enabled: Boolean(taskId) })
  const canManage = hasPermission(permissions, 'tasks.manage')
  return <main className="gf-content"><QueryState query={query}>{(task) => <><PageHeader title={task.name} description="Immutable task versions and execution policy." action={<>{hasPermission(permissions, 'runs.execute') && <ManualTaskRunButton taskId={taskId} />}{canManage && <Button onClick={() => setEditorOpen(true)}>Edit version</Button>}</>} refresh={<QueryRefresh query={query} />} /><div className="gf-metric-grid"><MetricCard label="Active version" value={task.activeVersion ?? '—'} icon={ListChecks} /><MetricCard label="Runner pool" value={task.pool ?? '—'} icon={Server} /><MetricCard label="Task Duration in Seconds" value={task.durationSeconds ? `${task.durationSeconds}s` : '—'} icon={Timer} /></div><section className="gf-card-panel"><h2>Version history</h2><QueryState query={versionsQuery} empty="No published versions.">{(versions) => <><DataTable caption="Task versions" rows={versions.slice((versionPage - 1) * versionLimit, versionPage * versionLimit)} columns={[{ key: 'version', label: 'Version', render: (version) => <span><Button type="button" variant="ghost" disabled={version.version < 2} onClick={() => setCompareVersion(version.version)}>v{version.version}{version.version === task.activeVersion ? ' (active)' : ''}</Button></span> }, { key: 'command', label: 'Command', render: (version) => version.command?.join(' ') ?? '—' }, { key: 'pool', label: 'Runner pool' }, { key: 'createdAt', label: 'Published', className: 'gf-cell-nowrap', render: (version) => <time dateTime={version.createdAt}>{formatDateTime(version.createdAt)}</time> }]} /><Pagination page={versionPage} pages={Math.max(1, Math.ceil(versions.length / versionLimit))} limit={versionLimit} onChange={setVersionPage} onLimitChange={(next) => { setVersionLimit(next); setVersionPage(1) }} />{compareVersion !== undefined && (() => { const current = versions.find((version) => version.version === compareVersion); const previous = versions.find((version) => version.version === compareVersion - 1); if (!current) return null; const changes = previous ? taskVersionDiff(previous, current) : []; return <section className="gf-card-panel"><h3>Changes from v{compareVersion - 1} to v{compareVersion}</h3>{!previous ? <p className="gf-muted">Previous version is not available.</p> : changes.length ? <DataTable caption={`Changes from version ${compareVersion - 1} to ${compareVersion}`} rows={changes} columns={[{ key: 'field', label: 'Field' }, { key: 'previous', label: `v${compareVersion - 1}` }, { key: 'current', label: `v${compareVersion}` }]} /> : <p className="gf-muted">No changes detected.</p>}</section> })()}</>}</QueryState></section><div className="gf-task-archive-action">{canManage && <DangerousAction label="Archive task" confirmLabel="Archive" warning="Marks this task deleted, disables its schedules, cancels pending work, and keeps execution history." onConfirm={() => api.delete(`/api/v1/tasks/${encodeURIComponent(taskId)}`).then(() => navigate('/tasks'))} />}</div>{editorOpen && <TaskEditorPage editTaskId={taskId} inDialog onClose={() => setEditorOpen(false)} onSaved={async () => { setEditorOpen(false); await query.refetch(); await versionsQuery.refetch() }} />}</>}</QueryState></main>
}
