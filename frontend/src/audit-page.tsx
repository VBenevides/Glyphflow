import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type AuditEvent, type Page } from './api'
import { AuditValue, SafeLink } from './safe'
import { Button, DataTable, Dialog, EmptyState, FilterInput, Identifier, MetricCard, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'
import { useAuth } from './auth'
import { formatDateTime } from './format'

export function auditQuery(filters: { actor: string; action: string; target: string; result: string; correlation: string; from: string; to: string; excludeAuditReads?: boolean; excludeRunLogs?: boolean; excludeGet?: boolean }, page: number, limit?: number) {
  return { actor: filters.actor || undefined, action: filters.action || undefined, target: filters.target || undefined, result: filters.result || undefined, correlation_id: filters.correlation || undefined, from: filters.from || undefined, to: filters.to || undefined, exclude_target: filters.excludeAuditReads === false ? undefined : '/api/v1/audit', exclude_run_logs: filters.excludeRunLogs === false ? undefined : true, exclude_method: filters.excludeGet === false ? undefined : 'GET', include_get: filters.excludeGet === false ? true : undefined, page, ...(limit ? { limit } : {}) }
}

type AuditPage = Page<AuditEvent> & { failureCount?: number; writeCount?: number }

function AuditMetadata({ event }: { event: AuditEvent }) {
  const actor = event.actorName ?? event.actor ?? '—'
  return <dl className="gf-audit-meta">
    <div><dt>Timestamp</dt><dd><time dateTime={event.createdAt}>{formatDateTime(event.createdAt)}</time></dd></div>
    <div><dt>Actor</dt><dd>{actor}</dd></div>
    <div><dt>Actor email</dt><dd>{event.actorEmail ?? '—'}</dd></div>
    <div><dt>Description</dt><dd>{event.description ?? '—'}</dd></div>
    <div><dt>Method</dt><dd>{event.action ?? '—'}</dd></div>
    <div><dt>Endpoint</dt><dd>{event.target ?? '—'}</dd></div>
    <div><dt>Result</dt><dd><StatusPill status={event.result ?? '—'} /></dd></div>
    <div><dt>Event ID</dt><dd><Identifier id={event.id} copyLabel="Copy event ID" /></dd></div>
  </dl>
}

function AuditValuePanel({ title, value }: { title: string; value: unknown }) {
  return <section className="gf-audit-value-panel"><h3>{title}</h3><AuditValue value={value} /></section>
}

export function AuditPage() {
  const { permissions, profile } = useAuth()
  const canOpenUsers = hasPermission(permissions, 'users.read|users.manage')
  const [filters, setFilters] = useState({ actor: '', action: '', target: '', result: '', correlation: '', from: '', to: '', excludeAuditReads: true, excludeRunLogs: true, excludeGet: true }); const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const [details, setDetails] = useState<AuditEvent | null>(null)
  const query = useQuery<AuditPage>({ queryKey: ['audit', filters, page, limit], queryFn: ({ signal }) => api.get<AuditPage>('/api/v1/audit', auditQuery(filters, page, limit), signal) })
  const auditOptions = query.data?.items ?? []
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content">
    <PageHeader title="Audit events" description="Trace security and scheduler changes with redacted values." />
    <div className="gf-filter-bar"><FilterInput label="Actor" options={auditOptions.flatMap((event) => [event.actorName, event.actor, event.actorEmail].filter((value): value is string => Boolean(value)))} value={filters.actor} onChange={(value) => update('actor', value)} /><FilterInput label="Action" options={auditOptions.flatMap((event) => event.action ? [event.action] : [])} value={filters.action} onChange={(value) => update('action', value)} /><FilterInput label="Target" options={auditOptions.flatMap((event) => event.target ? [event.target] : [])} value={filters.target} onChange={(value) => update('target', value)} /><label>Result<select className="gf-input" value={filters.result} onChange={(event) => update('result', event.target.value)}><option value="">All</option><option>success</option><option>failure</option></select></label><FilterInput label="Correlation ID" options={auditOptions.flatMap((event) => event.correlationId ? [event.correlationId] : [])} value={filters.correlation} onChange={(value) => update('correlation', value)} /><label><input type="checkbox" checked={filters.excludeAuditReads} onChange={(event) => { setFilters((current) => ({ ...current, excludeAuditReads: event.target.checked })); setPage(1) }} /> Exclude audit reads</label><label><input type="checkbox" checked={filters.excludeRunLogs} onChange={(event) => { setFilters((current) => ({ ...current, excludeRunLogs: event.target.checked })); setPage(1) }} /> Exclude run logs</label><label><input type="checkbox" checked={filters.excludeGet} onChange={(event) => { setFilters((current) => ({ ...current, excludeGet: event.target.checked })); setPage(1) }} /> Exclude GET requests</label></div>
    <QueryState query={query} empty="No audit events match these filters.">{(data) => <><div className="gf-metric-grid"><MetricCard label="Number of events" value={data.total ?? 0} detail="Events matching the filters" /><MetricCard label="Number of failure events" value={data.failureCount ?? 0} detail="Filtered events with failure results" /><MetricCard label="Number of write events" value={data.writeCount ?? 0} detail="Filtered POST, PUT, and DELETE events" /></div>{data.items.length ? <><DataTable className="gf-audit-table" caption="Audit events" rows={data.items} columns={[
      { key: 'createdAt', label: 'Time', className: 'gf-cell-nowrap', render: (event) => <time dateTime={event.createdAt}>{formatDateTime(event.createdAt)}</time> },
      { key: 'actor', label: 'Actor', className: 'gf-cell-actor', render: (event) => { const name = event.actorName ?? event.actor; return event.actor && (canOpenUsers || event.actor === profile?.id) ? <Identifier id={event.actor} name={name} href={`/admin/users/${encodeURIComponent(event.actor)}`} copyLabel="Copy actor ID" /> : event.actor ? <Identifier id={event.actor} name={name} copyLabel="Copy actor ID" /> : '—' } },
      { key: 'description', label: 'Description', render: (event) => event.description ?? '—' },
      { key: 'action', label: 'Method', className: 'gf-cell-nowrap' },
      { key: 'target', label: 'Endpoint', className: 'gf-cell-endpoint', render: (event) => event.target ? <SafeLink href={event.target}><span title={event.target}>{event.target}</span></SafeLink> : '—' },
      { key: 'result', label: 'Result', className: 'gf-cell-nowrap', render: (event) => <StatusPill status={event.result ?? '—'} /> },
      { key: 'details', label: 'Details', className: 'gf-cell-nowrap', render: (event) => <Button variant="secondary" onClick={() => setDetails(event)}>Details</Button> },
    ]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No audit events">Try a wider time range or remove a filter.</EmptyState>}</>}</QueryState>
    <Dialog open={details !== null} title="Audit event details" className="gf-audit-dialog" onClose={() => setDetails(null)}>{details && <div className="gf-audit-details">
      <AuditMetadata event={details} />
      <div className="gf-audit-grid"><AuditValuePanel title="Input" value={details.input ?? details.request} /><AuditValuePanel title="Output" value={details.output} /></div>
      <div className="gf-audit-grid"><AuditValuePanel title="Before" value={details.before} /><AuditValuePanel title="After" value={details.after} /></div>
      <AuditValuePanel title="Traceback" value={details.traceback} />
      <AuditValuePanel title="Correlation ID" value={details.correlationId} />
    </div>}</Dialog>
  </main>
}
