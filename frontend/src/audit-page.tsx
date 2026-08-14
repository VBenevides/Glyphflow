import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type AuditEvent, type Page } from './api'
import { AuditValue, SafeLink } from './safe'
import { Button, DataTable, Dialog, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'
import { hasPermission } from './permissions'
import { useAuth } from './auth'

export function auditQuery(filters: { actor: string; action: string; target: string; result: string; correlation: string; from: string; to: string; excludeAuditReads?: boolean }, page: number) {
  return { actor: filters.actor || undefined, action: filters.action || undefined, target: filters.target || undefined, result: filters.result || undefined, correlation_id: filters.correlation || undefined, from: filters.from || undefined, to: filters.to || undefined, exclude_target: filters.excludeAuditReads === false ? undefined : '/api/v1/audit', page }
}

function AuditMetadata({ event }: { event: AuditEvent }) {
  const actor = event.actorName ?? event.actor ?? '—'
  return <dl className="gf-audit-meta">
    <div><dt>Timestamp</dt><dd>{event.createdAt ?? '—'}</dd></div>
    <div><dt>Actor</dt><dd>{actor}</dd></div>
    <div><dt>Actor email</dt><dd>{event.actorEmail ?? '—'}</dd></div>
    <div><dt>Description</dt><dd>{event.description ?? '—'}</dd></div>
    <div><dt>Method</dt><dd>{event.action ?? '—'}</dd></div>
    <div><dt>Endpoint</dt><dd>{event.target ?? '—'}</dd></div>
    <div><dt>Result</dt><dd><StatusPill status={event.result ?? '—'} /></dd></div>
    <div><dt>Event ID</dt><dd>{event.id}</dd></div>
  </dl>
}

function AuditValuePanel({ title, value }: { title: string; value: unknown }) {
  return <section className="gf-audit-value-panel"><h3>{title}</h3><AuditValue value={value} /></section>
}

export function AuditPage() {
  const { permissions, profile } = useAuth()
  const canOpenUsers = hasPermission(permissions, 'users.read|users.manage')
  const [filters, setFilters] = useState({ actor: '', action: '', target: '', result: '', correlation: '', from: '', to: '', excludeAuditReads: true }); const [page, setPage] = useState(1)
  const [details, setDetails] = useState<AuditEvent | null>(null)
  const debouncedFilters = useDebouncedValue(filters)
  const query = useQuery({ queryKey: ['audit', debouncedFilters, page], queryFn: ({ signal }) => api.get<Page<AuditEvent>>('/api/v1/audit', auditQuery(debouncedFilters, page), signal) })
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content">
    <PageHeader title="Audit events" description="Trace security and scheduler changes with redacted values." />
    <div className="gf-filter-bar"><label>Actor<Input value={filters.actor} onChange={(event) => update('actor', event.target.value)} /></label><label>Action<Input value={filters.action} onChange={(event) => update('action', event.target.value)} /></label><label>Target<Input value={filters.target} onChange={(event) => update('target', event.target.value)} /></label><label>Result<select className="gf-input" value={filters.result} onChange={(event) => update('result', event.target.value)}><option value="">All</option><option>success</option><option>failure</option></select></label><label>Correlation ID<Input value={filters.correlation} onChange={(event) => update('correlation', event.target.value)} /></label><label><input type="checkbox" checked={filters.excludeAuditReads} onChange={(event) => setFilters((current) => ({ ...current, excludeAuditReads: event.target.checked }))} /> Exclude audit reads</label></div>
    <QueryState query={query} empty="No audit events match these filters.">{(data) => data.items.length ? <><DataTable caption="Audit events" rows={data.items} columns={[
      { key: 'createdAt', label: 'Time' },
      { key: 'actor', label: 'Actor', render: (event) => { const name = event.actorName ?? event.actor ?? '—'; return event.actor && (canOpenUsers || event.actor === profile?.id) ? <Link to={`/admin/users/${encodeURIComponent(event.actor)}`}>{name}</Link> : name } },
      { key: 'description', label: 'Description', render: (event) => event.description ?? '—' },
      { key: 'action', label: 'Method' },
      { key: 'target', label: 'Endpoint', render: (event) => event.target ? <SafeLink href={event.target}>{event.target}</SafeLink> : '—' },
      { key: 'result', label: 'Result', render: (event) => <StatusPill status={event.result ?? '—'} /> },
      { key: 'details', label: 'Details', render: (event) => <Button variant="secondary" onClick={() => setDetails(event)}>Details</Button> },
    ]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No audit events">Try a wider time range or remove a filter.</EmptyState>}</QueryState>
    <Dialog open={details !== null} title="Audit event details" className="gf-audit-dialog" onClose={() => setDetails(null)}>{details && <div className="gf-audit-details">
      <AuditMetadata event={details} />
      <div className="gf-audit-grid"><AuditValuePanel title="Input" value={details.input ?? details.request} /><AuditValuePanel title="Output" value={details.output} /></div>
      <div className="gf-audit-grid"><AuditValuePanel title="Before" value={details.before} /><AuditValuePanel title="After" value={details.after} /></div>
      <AuditValuePanel title="Traceback" value={details.traceback} />
      <AuditValuePanel title="Correlation ID" value={details.correlationId} />
    </div>}</Dialog>
  </main>
}
