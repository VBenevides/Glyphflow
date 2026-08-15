import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Task, type TaskVersion } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, MetricCard, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { hasPermission } from './permissions'

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

export function TaskDetailPage() {
  const { taskId = '' } = useParams()
  const { permissions } = useAuth()
  const navigate = useNavigate()
  const query = useQuery({ queryKey: ['task', taskId], queryFn: ({ signal }) => api.get<Task>(`/api/v1/tasks/${encodeURIComponent(taskId)}`, undefined, signal), enabled: Boolean(taskId) })
  const links = useMemo(() => taskDetailLinks(taskId), [taskId])
  const versionsQuery = useQuery({ queryKey: ['task-versions', taskId], queryFn: ({ signal }) => api.get<TaskVersion[]>(links.versions, undefined, signal), enabled: Boolean(taskId) })
  const canManage = hasPermission(permissions, 'tasks.manage')
  return <main className="gf-content"><QueryState query={query}>{(task) => <><PageHeader title={task.name} description="Immutable task versions and execution policy." action={canManage && <Button onClick={() => navigate(`/tasks/${taskId}/edit`)}>Edit version</Button>} /><div className="gf-metric-grid"><MetricCard label="Active version" value={task.activeVersion ?? '—'} /><MetricCard label="Runner pool" value={task.pool ?? '—'} /><MetricCard label="Timeout" value={task.timeoutSeconds ? `${task.timeoutSeconds}s` : '—'} /></div><section className="gf-card-panel"><h2>Related records</h2><nav className="gf-related-links"><Link to={links.schedules}>Schedules</Link><Link to={links.runs}>Runs</Link><Link to={links.audit}>Audit events</Link></nav></section><section className="gf-card-panel"><h2>Version history</h2><QueryState query={versionsQuery} empty="No published versions.">{(versions) => <DataTable caption="Task versions" rows={versions} columns={[{ key: 'version', label: 'Version', render: (version) => `v${version.version}${version.version === task.activeVersion ? ' (active)' : ''}` }, { key: 'command', label: 'Command', render: (version) => version.command?.join(' ') ?? '—' }, { key: 'pool', label: 'Runner pool' }, { key: 'createdAt', label: 'Published', render: (version) => version.createdAt ? new Date(version.createdAt).toLocaleString() : '—' }]} />}</QueryState></section>{canManage && <DangerousAction label="Delete task" warning="Permanently deletes this task and its versions. Existing execution history may block deletion." onConfirm={() => api.delete(`/api/v1/tasks/${encodeURIComponent(taskId)}`).then(() => navigate('/tasks'))} />}</>}</QueryState></main>
}
