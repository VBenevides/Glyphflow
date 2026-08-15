import { useQueries, type UseQueryResult } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Run, type Runner, type Schedule } from './api'
import { EmptyState, ErrorState, LoadingState, MetricCard, PageHeader, StatusPill } from './components'
import { hasPermission } from './permissions'

type Widget = { key: string; label: string; permission: string; path: string; kind: 'metric' | 'list' }
export const DASHBOARD_WIDGETS: Widget[] = [
  { key: 'runs', label: 'Active runs', permission: 'runs.read', path: '/api/v1/runs?state=active', kind: 'metric' },
  { key: 'schedules', label: 'Due schedules', permission: 'tasks.read', path: '/api/v1/schedules?due=true', kind: 'list' },
  { key: 'runners', label: 'Offline runners', permission: 'runners.read', path: '/api/v1/runners?state=offline', kind: 'list' },
  { key: 'audit', label: 'Recent audit events', permission: 'audit.read', path: '/api/v1/audit?recent=true', kind: 'list' },
]

export function permittedWidgets(permissions: string[]) {
  return DASHBOARD_WIDGETS.filter((widget) => hasPermission(permissions, widget.permission))
}

function WidgetState({ widget, result }: { widget: Widget; result: UseQueryResult<unknown> }) {
  if (result.isPending) return <section className="gf-dashboard-widget"><h2>{widget.label}</h2><LoadingState label="Loading" /></section>
  if (result.isError && result.data === undefined) return <section className="gf-dashboard-widget"><h2>{widget.label}</h2><ErrorState message={result.error instanceof Error ? result.error.message : 'Widget failed'} onRetry={() => result.refetch()} /></section>
  const value = result.data as Page<Run | Schedule | Runner> | undefined
  const items = Array.isArray(value) ? value : value?.items ?? []
  if (widget.kind === 'metric') return <MetricCard label={widget.label} value={value?.total ?? items.length} detail={result.isFetching ? 'Refreshing…' : undefined} />
  return <section className="gf-dashboard-widget"><h2>{widget.label}</h2>{items.length ? <ul className="gf-dashboard-list">{items.slice(0, 5).map((item, index) => <li key={String((item as { id?: string }).id ?? index)}><span>{String((item as { name?: string; id?: string }).name ?? (item as { id?: string }).id ?? 'Record')}</span>{'state' in item && <StatusPill status={String(item.state)} />}</li>)}</ul> : <EmptyState title="None" />}</section>
}

export function DashboardPage() {
  const { permissions } = useAuth()
  const widgets = permittedWidgets(permissions)
  const results = useQueries({ queries: widgets.map((widget) => ({ queryKey: ['dashboard', widget.key], queryFn: ({ signal }: { signal: AbortSignal }) => api.get(widget.path, undefined, signal), staleTime: 15_000, enabled: true })) })
  return <main className="gf-content"><PageHeader title="Overview" description="A live view of the scheduler resources you can access." /><div className="gf-dashboard-grid">{widgets.map((widget, index) => <WidgetState key={widget.key} widget={widget} result={results[index]} />)}</div>{!widgets.length && <EmptyState title="No dashboard widgets available">Ask an administrator for a read permission.</EmptyState>}<p className="gf-dashboard-links"><Link to="/runs">Review runs</Link><Link to="/tasks">Manage tasks</Link></p></main>
}
