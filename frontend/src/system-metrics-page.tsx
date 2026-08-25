import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, HardDrive, Inbox, Server, Timer } from 'lucide-react'
import { api, type SystemMetrics } from './api'
import { DataTable, EmptyState, MetricCard, PageHeader, StatusPill, type MetricTone } from './components'
import { formatDateTime } from './format'
import { QueryState } from './query'

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
      <MetricCard label="Disk free" value={`${data.signals.disk.freePercent.toFixed(1)}%`} detail={formatBytes(data.signals.disk.freeBytes)} icon={HardDrive} tone={signalTone(data, 'disk_free_percent')} />
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

export function SystemMetricsPage() {
  const query = useQuery<SystemMetrics>({
    queryKey: ['system-metrics'],
    queryFn: ({ signal }) => api.get<SystemMetrics>('/api/v1/admin/system/metrics', undefined, signal),
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
  return <main className="gf-content"><PageHeader title="System Metrics" description="Operational readiness, queue pressure, storage, and recovery signals." meta="Refreshes every 15 seconds" /><QueryState query={query} empty="System metrics are not available.">{(data) => <SystemMetricsView data={data} />}</QueryState></main>
}
