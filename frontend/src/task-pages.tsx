import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Task, type TaskVersion } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, MetricCard, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { hasPermission } from './permissions'
import { formatDateTime } from './format'

export function taskQuery(search: string, state: string, page: number) {
  return { search: search || undefined, state: state || undefined, page }
}

export function TaskInventoryPage() {
  const { permissions } = useAuth()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [state, setState] = useState('')
  const [page, setPage] = useState(1)
  const debouncedSearch = useDebouncedValue(search)
  const query = useQuery({ queryKey: ['tasks', debouncedSearch, state, page], queryFn: ({ signal }) => api.get<Page<Task>>('/api/v1/tasks', taskQuery(debouncedSearch, state, page), signal) })
  const canManage = hasPermission(permissions, 'tasks.manage')
  return <main className="gf-content"><PageHeader title="Tasks" description="Versioned commands and their schedules." action={canManage && <Button onClick={() => navigate('/tasks/new')}>Create task</Button>} /><div className="gf-filter-bar"><label>Search <Input value={search} onChange={(event) => { setSearch(event.target.value); setPage(1) }} placeholder="Name or ID" /></label><label>State <select className="gf-input" value={state} onChange={(event) => { setState(event.target.value); setPage(1) }}><option value="">All</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></label></div><QueryState query={query} empty="Create a task version to schedule work.">{(data) => data.items.length ? <><DataTable caption="Tasks" rows={data.items} columns={[{ key: 'name', label: 'Task', render: (task) => <Link to={`/tasks/${task.id}`}>{task.name}</Link> }, { key: 'id', label: 'Task ID', render: (task) => <code>{task.id}</code> }, { key: 'enabled', label: 'State', render: (task) => <StatusPill status={task.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'activeVersion', label: 'Version' }, { key: 'pool', label: 'Runner pool' }, { key: 'latestRun', label: 'Latest run', render: (task) => task.latestRun ? <StatusPill status={task.latestRun.state} /> : '—' }]} /><Pagination page={data.page} pages={data.pages ?? (data.total ? Math.ceil(data.total / data.limit) : 1)} onChange={setPage} /></> : <EmptyState title="No tasks">Create a task version to schedule work.</EmptyState>}</QueryState></main>
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
    { id: 'timeout', field: 'Timeout', previous: previous.timeoutSeconds === undefined ? '—' : `${previous.timeoutSeconds}s`, current: current.timeoutSeconds === undefined ? '—' : `${current.timeoutSeconds}s` },
    { id: 'output-limit', field: 'Output limit', previous: previous.maxOutputBytes === undefined ? '—' : String(previous.maxOutputBytes), current: current.maxOutputBytes === undefined ? '—' : String(current.maxOutputBytes) },
    { id: 'maximum-attempts', field: 'Maximum attempts', previous: previous.maxAttempts === undefined ? '—' : String(previous.maxAttempts), current: current.maxAttempts === undefined ? '—' : String(current.maxAttempts) },
    { id: 'ambiguity-policy', field: 'Ambiguity policy', previous: previous.ambiguityPolicy ?? '—', current: current.ambiguityPolicy ?? '—' },
  ]
  return rows.filter((row) => row.previous !== row.current)
}

function ManualTaskRunButton({ taskId }: { taskId: string }) {
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const run = async () => {
    setBusy(true)
    setError('')
    try {
      const result = await api.post<{ id?: string }>('/api/v1/runs/execute', { task_id: taskId })
      navigate(result.id ? `/runs/${result.id}` : '/runs')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to start run')
    } finally {
      setBusy(false)
    }
  }
  return <span>{error && <span className="gf-form-error" role="alert">{error}</span>}<Button type="button" onClick={() => { void run() }} busy={busy}>Run now</Button></span>
}

export function TaskDetailPage() {
  const { taskId = '' } = useParams()
  const { permissions } = useAuth()
  const navigate = useNavigate()
  const [compareVersion, setCompareVersion] = useState<number>()
  const query = useQuery({ queryKey: ['task', taskId], queryFn: ({ signal }) => api.get<Task>(`/api/v1/tasks/${encodeURIComponent(taskId)}`, undefined, signal), enabled: Boolean(taskId) })
  const links = useMemo(() => taskDetailLinks(taskId), [taskId])
  const versionsQuery = useQuery({ queryKey: ['task-versions', taskId], queryFn: ({ signal }) => api.get<TaskVersion[]>(links.versions, undefined, signal), enabled: Boolean(taskId) })
  const canManage = hasPermission(permissions, 'tasks.manage')
  return <main className="gf-content"><QueryState query={query}>{(task) => <><PageHeader title={task.name} description="Immutable task versions and execution policy." action={<>{hasPermission(permissions, 'runs.execute') && <ManualTaskRunButton taskId={taskId} />}{canManage && <Button onClick={() => navigate(`/tasks/${taskId}/edit`)}>Edit version</Button>}</>} /><div className="gf-metric-grid"><MetricCard label="Active version" value={task.activeVersion ?? '—'} /><MetricCard label="Runner pool" value={task.pool ?? '—'} /><MetricCard label="Timeout" value={task.timeoutSeconds ? `${task.timeoutSeconds}s` : '—'} /></div><section className="gf-card-panel"><h2>Related records</h2><nav className="gf-related-links"><Link to={links.schedules}>Schedules</Link><Link to={links.runs}>Runs</Link><Link to={links.audit}>Audit events</Link></nav></section><section className="gf-card-panel"><h2>Version history</h2><QueryState query={versionsQuery} empty="No published versions.">{(versions) => <><DataTable caption="Task versions" rows={versions} columns={[{ key: 'version', label: 'Version', render: (version) => <span><Button type="button" variant="ghost" disabled={version.version < 2} onClick={() => setCompareVersion(version.version)}>v{version.version}{version.version === task.activeVersion ? ' (active)' : ''}</Button></span> }, { key: 'command', label: 'Command', render: (version) => version.command?.join(' ') ?? '—' }, { key: 'pool', label: 'Runner pool' }, { key: 'createdAt', label: 'Published', className: 'gf-cell-nowrap', render: (version) => <time dateTime={version.createdAt}>{formatDateTime(version.createdAt)}</time> }]} />{compareVersion !== undefined && (() => { const current = versions.find((version) => version.version === compareVersion); const previous = versions.find((version) => version.version === compareVersion - 1); if (!current) return null; const changes = previous ? taskVersionDiff(previous, current) : []; return <section className="gf-card-panel"><h3>Changes from v{compareVersion - 1} to v{compareVersion}</h3>{!previous ? <p className="gf-muted">Previous version is not available.</p> : changes.length ? <DataTable caption={`Changes from version ${compareVersion - 1} to ${compareVersion}`} rows={changes} columns={[{ key: 'field', label: 'Field' }, { key: 'previous', label: `v${compareVersion - 1}` }, { key: 'current', label: `v${compareVersion}` }]} /> : <p className="gf-muted">No changes detected.</p>}</section> })()}</>}</QueryState></section>{canManage && <DangerousAction label="Archive task" confirmLabel="Archive" warning="Marks this task deleted, disables its schedules, cancels pending work, and keeps execution history." onConfirm={() => api.delete(`/api/v1/tasks/${encodeURIComponent(taskId)}`).then(() => navigate('/tasks'))} />}</>}</QueryState></main>
}
