import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type AuditEvent, type Page } from './api'
import { AuditValue, SafeLink } from './safe'
import { DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState, useDebouncedValue } from './query'

export function auditQuery(filters: { actor: string; action: string; target: string; result: string; correlation: string; from: string; to: string; excludeAuditReads?: boolean }, page: number) {
  return { actor: filters.actor || undefined, action: filters.action || undefined, target: filters.target || undefined, result: filters.result || undefined, correlation_id: filters.correlation || undefined, from: filters.from || undefined, to: filters.to || undefined, exclude_target: filters.excludeAuditReads === false ? undefined : '/api/v1/audit', page }
}

export function AuditPage() {
  const [filters, setFilters] = useState({ actor: '', action: '', target: '', result: '', correlation: '', from: '', to: '', excludeAuditReads: true }); const [page, setPage] = useState(1)
  const debouncedFilters = useDebouncedValue(filters)
  const query = useQuery({ queryKey: ['audit', debouncedFilters, page], queryFn: ({ signal }) => api.get<Page<AuditEvent>>('/api/v1/audit', auditQuery(debouncedFilters, page), signal) })
  const update = (key: keyof typeof filters, value: string) => { setFilters((current) => ({ ...current, [key]: value })); setPage(1) }
  return <main className="gf-content"><PageHeader title="Audit events" description="Trace security and scheduler changes with redacted values." /><div className="gf-filter-bar"><label>Actor<Input value={filters.actor} onChange={(event) => update('actor', event.target.value)} /></label><label>Action<Input value={filters.action} onChange={(event) => update('action', event.target.value)} /></label><label>Target<Input value={filters.target} onChange={(event) => update('target', event.target.value)} /></label><label>Result<select className="gf-input" value={filters.result} onChange={(event) => update('result', event.target.value)}><option value="">All</option><option>success</option><option>failure</option></select></label><label>Correlation ID<Input value={filters.correlation} onChange={(event) => update('correlation', event.target.value)} /></label><label><input type="checkbox" checked={filters.excludeAuditReads} onChange={(event) => setFilters((current) => ({ ...current, excludeAuditReads: event.target.checked }))} /> Exclude audit reads</label></div><QueryState query={query} empty="No audit events match these filters.">{(data) => data.items.length ? <><DataTable caption="Audit events" rows={data.items} columns={[{ key: 'createdAt', label: 'Time' }, { key: 'actor', label: 'Actor' }, { key: 'action', label: 'Action' }, { key: 'target', label: 'Target', render: (event) => event.target ? <SafeLink href={event.target}>{event.target}</SafeLink> : '—' }, { key: 'result', label: 'Result', render: (event) => <StatusPill status={event.result ?? '—'} /> }, { key: 'correlationId', label: 'Correlation' }, { key: 'before', label: 'Before', render: (event) => <AuditValue value={event.before} /> }, { key: 'after', label: 'After', render: (event) => <AuditValue value={event.after} /> }]} /><Pagination page={data.page} pages={data.pages ?? 1} onChange={setPage} /></> : <EmptyState title="No audit events">Try a wider time range or remove a filter.</EmptyState>}</QueryState></main>
}
