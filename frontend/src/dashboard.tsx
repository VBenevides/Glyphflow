import { useQueries, type UseQueryResult } from '@tanstack/react-query'
import type { ComponentType } from 'react'
import { Link } from 'react-router-dom'
import { Activity, CalendarClock, ServerOff } from 'lucide-react'
import { useAuth } from './auth'
import { api, type Page, type Run, type Runner, type Schedule } from './api'
import { EmptyState, ErrorState, LoadingState, MetricCard, PageHeader, StatusPill } from './components'
import { hasPermission } from './permissions'
import { QueryRefresh } from './query'

type Widget = { key: string; label: string; permission: string; path: string; kind: 'metric' | 'list'; tone?: 'default' | 'success' | 'warning' | 'danger' | 'info'; icon?: ComponentType<{ className?: string; size?: string | number }> }
export const DASHBOARD_WIDGETS: Widget[] = [
  { key: 'runs', label: 'Active runs', permission: 'runs.read', path: '/api/v1/runs?state=active', kind: 'metric', tone: 'info', icon: Activity },
  { key: 'schedules', label: 'Due schedules', permission: 'tasks.read', path: '/api/v1/schedules?due=true', kind: 'metric', tone: 'warning', icon: CalendarClock },
  { key: 'runners', label: 'Offline runners', permission: 'runners.read', path: '/api/v1/runners?state=offline', kind: 'metric', tone: 'danger', icon: ServerOff },
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
  if (widget.kind === 'metric') return <MetricCard label={widget.label} value={value?.total ?? items.length} icon={widget.icon} tone={widget.tone} detail={result.isFetching ? 'Refreshing…' : undefined} />
  return <section className="gf-dashboard-widget"><h2>{widget.label}</h2>{items.length ? <ul className="gf-dashboard-list">{items.slice(0, 5).map((item, index) => { const record = item as { id?: string; name?: string; action?: string; description?: string; state?: string; result?: string }; return <li key={String(record.id ?? index)}><span><strong>{record.action ?? record.name ?? record.id ?? 'Record'}</strong>{record.description && <><br /><small>{record.description}</small></>}</span>{(record.state ?? record.result) && <StatusPill status={String(record.state ?? record.result)} />}</li> })}</ul> : <EmptyState title="None" />}</section>
}

export function DashboardPage() {
  const { permissions } = useAuth()
  const widgets = permittedWidgets(permissions)
  const results = useQueries({ queries: widgets.map((widget) => ({ queryKey: ['dashboard', widget.key], queryFn: ({ signal }: { signal: AbortSignal }) => api.get(widget.path, undefined, signal), staleTime: 15_000, enabled: true })) })
  const metricWidgets = widgets.filter((widget) => widget.kind === 'metric')
  const listWidgets = widgets.filter((widget) => widget.kind === 'list')
  const resultFor = (widget: Widget) => results[widgets.indexOf(widget)]
  return <main className="gf-content"><PageHeader title="Overview" description="A live view of the scheduler resources you can access." meta="Live scheduler data" refresh={<QueryRefresh query={results} />} /><div className="gf-metric-grid">{metricWidgets.map((widget) => <WidgetState key={widget.key} widget={widget} result={resultFor(widget)} />)}</div><div className="gf-dashboard-grid">{listWidgets.map((widget) => <WidgetState key={widget.key} widget={widget} result={resultFor(widget)} />)}</div>{!widgets.length && <EmptyState title="No dashboard widgets available">Ask an administrator for a read permission.</EmptyState>}<p className="gf-dashboard-links"><Link to="/runs">Review runs</Link><Link to="/tasks">Manage tasks</Link></p></main>
}
