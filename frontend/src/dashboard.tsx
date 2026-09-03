import { useQueries, useQuery, type UseQueryResult } from '@tanstack/react-query'
import { useEffect, useState, type ComponentType } from 'react'
import { Link } from 'react-router-dom'
import { Activity, CalendarClock, ServerOff } from 'lucide-react'
import { useAuth } from './auth'
import { api, type Page, type Run, type Runner, type Schedule, type ScheduleProjection } from './api'
import { Button, Dialog, EmptyState, ErrorState, LoadingState, MetricCard, PageHeader, StatusPill } from './components'
import { hasPermission } from './permissions'
import { QueryRefresh } from './query'
import { projectionIsStale } from './schedule-gantt'

type Widget = { key: string; label: string; permission: string; path: string; kind: 'metric' | 'list'; description?: string; tone?: 'default' | 'success' | 'warning' | 'danger' | 'info'; icon?: ComponentType<{ className?: string; size?: string | number }> }
export const DASHBOARD_WIDGETS: Widget[] = [
  { key: 'runs', label: 'Active runs', permission: 'runs.read', path: '/api/v1/runs?state=active', kind: 'metric', tone: 'info', icon: Activity },
  { key: 'schedules', label: 'Due schedules', permission: 'tasks.read', path: '/api/v1/schedules?due=true', kind: 'metric', tone: 'warning', icon: CalendarClock },
  { key: 'runners', label: 'Offline runners', permission: 'runners.read', path: '/api/v1/runners?state=offline', kind: 'metric', tone: 'danger', icon: ServerOff },
  { key: 'audit', label: 'Recent audit events', permission: 'audit.read', path: '/api/v1/audit?recent=true', kind: 'list' },
  { key: 'secrets', label: 'Secrets requiring attention', permission: 'secrets.read|secrets.manage', path: '/api/v1/admin/secrets/attention', kind: 'list', description: 'Integrity failure means the stored value could not be authenticated or decrypted. It does not necessarily mean the external credential has expired or been revoked.' },
]

export function permittedWidgets(permissions: string[]) {
  return DASHBOARD_WIDGETS.filter((widget) => hasPermission(permissions, widget.permission))
}

export function projectionDismissalKey(calculatedAt: string) {
  return `glyphflow:schedule-projection-dismissed:${calculatedAt}`
}

function WidgetState({ widget, result }: Readonly<{ widget: Widget; result: UseQueryResult<unknown> }>) {
  if (result.isPending) return <section className="gf-dashboard-widget"><h2>{widget.label}</h2><LoadingState label="Loading" /></section>
  if (result.isError && result.data === undefined) return <section className="gf-dashboard-widget"><h2>{widget.label}</h2><ErrorState message={result.error instanceof Error ? result.error.message : 'Widget failed'} onRetry={() => result.refetch()} /></section>
  const value = result.data as Page<Run | Schedule | Runner> | undefined
  const items = Array.isArray(value) ? value : value?.items ?? []
  if (widget.kind === 'metric') return <MetricCard label={widget.label} value={value?.total ?? items.length} icon={widget.icon ?? Activity} tone={widget.tone} detail={result.isFetching ? 'Refreshing…' : undefined} />
  return <section className="gf-dashboard-widget"><h2>{widget.label}</h2>{widget.description && <p className="gf-muted">{widget.description}</p>}{items.length ? <ul className="gf-dashboard-list">{items.slice(0, 5).map((item, index) => { const record = item as { id?: string; name?: string; action?: string; description?: string; status?: string; state?: string; result?: string }; return <li key={String(record.id ?? index)}><span><strong>{record.action ?? record.name ?? record.id ?? 'Record'}</strong>{record.description && <><br /><small>{record.description}</small></>}</span>{(record.status ?? record.state ?? record.result) && <StatusPill status={String(record.status ?? record.state ?? record.result)} />}</li> })}</ul> : <EmptyState title="None" />}</section>
}

export function DashboardPage() {
  const { permissions } = useAuth()
  const widgets = permittedWidgets(permissions)
  const results = useQueries({ queries: widgets.map((widget) => ({ queryKey: ['dashboard', widget.key], queryFn: ({ signal }: { signal: AbortSignal }) => api.get(widget.path, undefined, signal), staleTime: 15_000, enabled: true })) })
  const projectionQuery = useQuery({ queryKey: ['dashboard-schedule-projection'], queryFn: ({ signal }) => api.get<ScheduleProjection>('/api/v1/schedule-projection', undefined, signal), staleTime: 15_000, refetchInterval: 30_000, enabled: hasPermission(permissions, 'tasks.read') })
  const projection = projectionQuery.data
  const calculatedAt = projection?.calculatedAt
  const [warningKey, setWarningKey] = useState<string>()
  useEffect(() => {
    if (!projection?.available || !calculatedAt || !projection.conflicts?.length) { setWarningKey(undefined); return }
    const key = projectionDismissalKey(calculatedAt)
    try {
      if (typeof window === 'undefined' || window.sessionStorage.getItem(key) !== '1') setWarningKey(key)
    } catch {
      setWarningKey(key)
    }
  }, [calculatedAt, projection?.available, projection?.conflicts?.length])
  const dismissWarning = () => {
    if (warningKey) {
      try { window.sessionStorage.setItem(warningKey, '1') } catch { /* session storage can be unavailable */ }
    }
    setWarningKey(undefined)
  }
  const metricWidgets = widgets.filter((widget) => widget.kind === 'metric')
  const listWidgets = widgets.filter((widget) => widget.kind === 'list')
  const resultFor = (widget: Widget) => results[widgets.indexOf(widget)]
  return <main className="gf-content"><PageHeader title="Overview" description="A live view of the scheduler resources you can access." meta="Live scheduler data" refresh={<QueryRefresh query={[...results, projectionQuery]} />} />{!warningKey && projection?.available && projection.conflicts?.length ? <section className="gf-card-panel gf-overview-conflict-notice" aria-live="polite"><h2>Scheduling conflicts detected</h2><p className="gf-form-error" role="alert">{projection.conflicts.length} exclusive-resource conflict{projection.conflicts.length === 1 ? '' : 's'} were found in the seven-day projection.</p>{projectionIsStale(projection.calculatedAt) && <output className="gf-stale-warning">This report is older than one hour; the last successful snapshot is shown.</output>}<ul>{projection.conflicts.slice(0, 5).map((conflict) => <li key={conflict.id}>{conflict.resourceName} · {conflict.occurrences.length} affected occurrence{conflict.occurrences.length === 1 ? '' : 's'}</li>)}</ul><div className="gf-dialog-actions"><Link className="gf-button gf-button-secondary" to="/schedules?tab=gantt">View Scheduling Gantt</Link></div></section> : null}<div className="gf-metric-grid">{metricWidgets.map((widget) => <WidgetState key={widget.key} widget={widget} result={resultFor(widget)} />)}</div><div className="gf-dashboard-grid">{listWidgets.map((widget) => <WidgetState key={widget.key} widget={widget} result={resultFor(widget)} />)}</div>{!widgets.length && <EmptyState title="No dashboard widgets available">Ask an administrator for a read permission.</EmptyState>}{warningKey && projection && <Dialog open title="Scheduling conflicts detected" onClose={dismissWarning}><p className="gf-form-error" role="alert">{projection.conflicts?.length ?? 0} exclusive-resource conflict{projection.conflicts?.length === 1 ? '' : 's'} were found in the seven-day projection.</p>{projectionIsStale(projection.calculatedAt) && <output className="gf-stale-warning">This report is older than one hour; the last successful snapshot is shown.</output>}<ul>{(projection.conflicts ?? []).slice(0, 5).map((conflict) => <li key={conflict.id}>{conflict.resourceName} · {conflict.occurrences.length} affected occurrence{conflict.occurrences.length === 1 ? '' : 's'}</li>)}</ul><div className="gf-dialog-actions"><Link className="gf-button gf-button-secondary" to="/schedules?tab=gantt" onClick={dismissWarning}>View Scheduling Gantt</Link><Button variant="secondary" onClick={dismissWarning}>Dismiss</Button></div></Dialog>}</main>
}
