import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Activity, AlertTriangle, HardDrive, Inbox, Server, Timer } from 'lucide-react'
import { api, type DeadLetter, type DeadLetterPage, type SystemMetrics } from './api'
import { DataTable, EmptyState, Identifier, Input, MetricCard, PageHeader, StatusPill, Button, type MetricTone } from './components'
import { formatDateTime } from './format'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'
import { useAuth } from './auth'

export function alertTone(severity?: string): MetricTone {
  if (severity === 'critical') return 'danger'
  if (severity === 'warning') return 'warning'
  return 'success'
}

export function signalTone(data: SystemMetrics, code: string): MetricTone {
  return alertTone(data.alerts.find((alert) => alert.code === code)?.severity)
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '—'
  if (value < 1024) return `${value} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let amount = value
  let unit = 'B'
  for (const next of units) {
    amount /= 1024
    unit = next
    if (amount < 1024 || next === units[units.length - 1]) break
  }
  return `${amount.toFixed(amount >= 10 ? 0 : 1)} ${unit}`
}

export function SystemMetricsView({ data }: { data: SystemMetrics }) {
  const alertRows = data.alerts.map((alert) => ({ ...alert, id: alert.code }))
  const metricRows = Object.entries(data.metrics).sort(([left], [right]) => left.localeCompare(right)).map(([name, value]) => ({ id: name, name, value }))
  return <>
    <div className="gf-metric-grid">
      <MetricCard label="Readiness" value={<StatusPill status={data.ready ? 'Ready' : 'Not ready'} />} detail={data.ready ? 'All required components are healthy' : 'One or more required components are unhealthy'} icon={Server} tone={data.ready ? 'success' : 'danger'} />
      <MetricCard label="Queue lag" value={`${data.signals.queueLagSeconds}s`} detail="Oldest pending operational message" icon={Timer} tone={signalTone(data, 'queue_lag')} />
      <MetricCard label="Open dead letters" value={data.signals.deadLetters.open} detail={`Oldest age: ${data.signals.deadLetters.oldestAgeSeconds}s`} icon={Inbox} tone={signalTone(data, 'dead_letters_open')} />
      <MetricCard label="Database free" value={data.signals.disk.state === 'UNAVAILABLE' ? 'Unavailable' : `${data.signals.disk.freePercent.toFixed(1)}%`} detail={data.signals.disk.state === 'UNAVAILABLE' ? (data.signals.disk.code ?? 'Capacity signal unavailable') : formatBytes(data.signals.disk.freeBytes)} icon={HardDrive} tone={signalTone(data, data.signals.disk.state === 'UNAVAILABLE' ? 'storage_unavailable' : 'disk_free_percent')} />
      <MetricCard label="Stuck runs" value={data.signals.stuckRuns} detail="Runs beyond the operational threshold" icon={Activity} tone={signalTone(data, 'stuck_runs')} />
    </div>
    <section className="gf-card-panel">
      <div className="gf-section-heading"><h2>Operational alerts</h2><AlertTriangle size={18} aria-hidden="true" /></div>
      {alertRows.length ? <DataTable caption="Operational alerts" rows={alertRows} columns={[{ key: 'code', label: 'Signal' }, { key: 'severity', label: 'Severity', render: (alert) => <StatusPill status={alert.severity} /> }, { key: 'status', label: 'Status', render: (alert) => <StatusPill status={alert.status} /> }, { key: 'value', label: 'Value' }, { key: 'threshold', label: 'Threshold' }]} /> : <EmptyState title="No active alerts">All monitored thresholds are within range.</EmptyState>}
    </section>
    <section className="gf-card-panel">
      <div className="gf-section-heading"><h2>Low-cardinality counters</h2><small>Sampled {formatDateTime(data.generatedAt)}</small></div>
      {metricRows.length ? <DataTable caption="Low-cardinality counters" rows={metricRows} columns={[{ key: 'name', label: 'Metric' }, { key: 'value', label: 'Value' }]} /> : <EmptyState title="No counters reported">The control plane has not reported counters yet.</EmptyState>}
    </section>
  </>
}

function DeadLetterRecovery({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [reasons, setReasons] = useState<Record<string, string>>({})
  const query = useQuery<DeadLetterPage>({ queryKey: ['dead-letters', page], queryFn: ({ signal }) => api.get<DeadLetterPage>('/api/v1/admin/dead-letters', { state: 'OPEN', page, limit: 20 }, signal), staleTime: 10_000, refetchInterval: 15_000 })
  const action = useMutation({ mutationFn: ({ id, state, reason }: { id: string; state: 'retry' | 'reconcile'; reason: string }) => api.post(`/api/v1/admin/dead-letters/${encodeURIComponent(id)}/${state}`, state === 'reconcile' ? { state: 'DISCARDED', reason } : { reason }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['dead-letters'] }) })
  return <section className="gf-card-panel"><div className="gf-section-heading"><h2>Dead-letter recovery</h2><small>Payloads remain hidden</small></div>{action.isError && <p role="alert">Recovery action failed. The record was not acknowledged as recovered.</p>}<QueryState query={query} empty="No open dead letters.">{(data) => <DeadLetterTable data={data} canManage={canManage} page={page} setPage={setPage} reasons={reasons} setReasons={setReasons} action={action} />}</QueryState></section>
}

function deadLetterDiagnostic(error?: string) {
  if (!error) return '—'
  return error.length > 160 ? `${error.slice(0, 160)}…` : error
}

function DeadLetterTable({ data, canManage, page, setPage, reasons, setReasons, action }: { data: DeadLetterPage; canManage: boolean; page: number; setPage: (page: number) => void; reasons: Record<string, string>; setReasons: (reasons: Record<string, string>) => void; action: { isPending: boolean; mutate: (input: { id: string; state: 'retry' | 'reconcile'; reason: string }) => void } }) {
  const rows = data.items.map((item) => ({ ...item, id: item.id }))
  return <>
    {rows.length ? <DataTable<DeadLetter> caption="Dead-letter records" rows={rows} columns={[{ key: 'id', label: 'Record', render: (item) => <Identifier id={item.id} copyLabel="Copy dead-letter ID" /> }, { key: 'runnerId', label: 'Runner', render: (item) => <Identifier id={item.runnerId} copyLabel="Copy runner ID" /> }, { key: 'subject', label: 'Subject' }, { key: 'messageId', label: 'Message', render: (item) => <Identifier id={item.messageId} copyLabel="Copy message ID" /> }, { key: 'attempts', label: 'Attempts' }, { key: 'state', label: 'State', render: (item) => <StatusPill status={item.state} /> }, { key: 'error', label: 'Diagnostic', render: (item) => <span title={item.error}>{deadLetterDiagnostic(item.error)}</span> }, ...(canManage ? [{ key: 'actions', label: 'Actions', render: (item: DeadLetter) => <div className="gf-inline-actions"><Input aria-label={`Reason for ${item.id}`} value={reasons[item.id] ?? ''} maxLength={512} placeholder="Operator reason" onChange={(event) => setReasons({ ...reasons, [item.id]: event.target.value })} /><Button busy={action.isPending} disabled={!reasons[item.id]?.trim()} onClick={() => action.mutate({ id: item.id, state: 'retry', reason: reasons[item.id].trim() })}>Retry</Button><Button variant="danger" busy={action.isPending} disabled={!reasons[item.id]?.trim()} onClick={() => action.mutate({ id: item.id, state: 'reconcile', reason: reasons[item.id].trim() })}>Discard</Button></div> }] : [])]} /> : <EmptyState title="No open dead letters">The queue has no records requiring operator action.</EmptyState>}
    {data.pages && data.pages > 1 && <nav className="gf-pagination" aria-label="Dead-letter pagination"><Button variant="secondary" disabled={page <= 1} onClick={() => setPage(page - 1)}>Previous</Button><span>Page {page} of {data.pages}</span><Button variant="secondary" disabled={page >= data.pages} onClick={() => setPage(page + 1)}>Next</Button></nav>}
  </>
}

export function SystemMetricsPage() {
  const { permissions } = useAuth()
  const query = useQuery<SystemMetrics>({
    queryKey: ['system-metrics'],
    queryFn: ({ signal }) => api.get<SystemMetrics>('/api/v1/admin/system/metrics', undefined, signal),
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
  return <main className="gf-content"><PageHeader title="System Metrics" description="Operational readiness, queue pressure, storage, and recovery signals." meta="Refreshes every 15 seconds" refresh={<QueryRefresh query={query} />} /><QueryState query={query} empty="System metrics are not available.">{(data) => <><SystemMetricsView data={data} />{hasPermission(permissions, 'system.deadletter.read') && <DeadLetterRecovery canManage={hasPermission(permissions, 'system.deadletter.manage')} />}</>}</QueryState></main>
}
