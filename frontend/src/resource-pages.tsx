import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, Box, Lock, Server, Timer } from 'lucide-react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Resource } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, DropdownMenuItem, EmptyState, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill, TableActions } from './components'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'
import { formatDateTime } from './format'

export function resourceState(resource: Resource) { return resource.enabled === false ? 'disabled' : resource.holder ? 'leased' : 'available' }
export function resourceKindLabel(kind?: string) { return kind?.toLowerCase().replace('_', '-') === 'non-blocking' ? 'Non-blocking' : 'Exclusive' }
export function resourceNameLabel(name: string) { return name.length > 30 ? `${name.slice(0, 29)}…` : name }

export function ResourceInventoryPage() {
  const { permissions } = useAuth()
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [creating, setCreating] = useState(false)
  const [draft, setDraft] = useState({ name: '', kind: 'exclusive' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['resources', page, limit], queryFn: ({ signal }) => api.get<Page<Resource>>('/api/v1/resources', { page, limit }, signal) })
  const summaryQuery = useQuery({ queryKey: ['resource-summary'], queryFn: async ({ signal }) => {
    const [all, exclusive] = await Promise.all([
      api.get<Page<Resource>>('/api/v1/resources', { page: 1, limit: 1 }, signal),
      api.get<Page<Resource>>('/api/v1/resources', { page: 1, limit: 1, kind: 'exclusive' }, signal),
    ])
    return { total: all.total ?? 0, exclusive: exclusive.total ?? 0 }
  }, refetchInterval: 5_000 })
  const manage = hasPermission(permissions, 'resources.manage')
  const create = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.post('/api/v1/resources', { name: draft.name.trim(), kind: draft.kind })
      setDraft({ name: '', kind: 'exclusive' })
      setCreating(false)
      await query.refetch()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Resource creation failed')
    } finally {
      setBusy(false)
    }
  }
  return <main className="gf-content">
    <PageHeader title="Resources and leases" description="Exclusive and non-blocking resources, fencing counters, and active holders." refresh={<QueryRefresh query={query} />} />
    <div className="gf-metric-grid"><MetricCard label="Total number of resources" value={summaryQuery.data?.total ?? '—'} detail="All configured resources" icon={Box} /><MetricCard label="Total number of exclusive resources" value={summaryQuery.data?.exclusive ?? '—'} detail="Resources that block concurrent runs" icon={Lock} /></div>
    {manage && <div className="gf-table-toolbar"><Button onClick={() => { setCreating(true); setError('') }}>Create resource</Button></div>}
    {creating && <section className="gf-card-panel"><h2>Create resource</h2><form className="gf-editor-form" onSubmit={create}>
      <label>Name<Input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label>
      <label>Kind<select className="gf-input" value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value })}><option value="exclusive">Exclusive</option><option value="non-blocking">Non-blocking</option></select></label>
      {error && <p className="gf-form-error" role="alert">{error}</p>}
      <div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => setCreating(false)}>Cancel</Button><Button type="submit" busy={busy}>Create resource</Button></div>
    </form></section>}
    <QueryState query={query} empty="Create a resource for task placement.">
      {(data) => data.items.length ? <>
        <DataTable caption="Resources" rows={data.items} columns={[
          { key: 'name', label: 'Resource Name', render: (resource) => <Link to={`/resources/${encodeURIComponent(resource.id)}`} title={resource.name}>{resourceNameLabel(resource.name)}</Link> },
          { key: 'id', label: 'Resource Id', render: (resource) => <Identifier id={resource.id} copyLabel="Copy resource ID" /> },
          { key: 'kind', label: 'Type', render: (resource) => resourceKindLabel(resource.kind) },
          { key: 'enabled', label: 'State', render: (resource) => <StatusPill status={resourceState(resource)} /> },
          { key: 'holder', label: 'Holder', render: (resource) => resource.holder ? <Identifier id={resource.holder} href={`/runs/${resource.holder}`} copyLabel="Copy run ID" /> : '—' },
          { key: 'expiresAt', label: 'Lease expiry', className: 'gf-cell-nowrap', render: (resource) => <time dateTime={resource.expiresAt}>{formatDateTime(resource.expiresAt)}</time> },
          { key: 'fencingToken', label: 'Fencing token' },
          { key: 'actions', label: 'Actions', render: (resource) => manage && <TableActions label={`Actions for ${resource.name}`}>
            <DangerousAction label="Delete" onConfirm={() => api.delete(`/api/v1/resources/${encodeURIComponent(resource.id)}`).then(() => { void query.refetch() })} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} />
          </TableActions> },
        ]} />
        <Pagination page={data.page ?? page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} />
      </> : <EmptyState title="No resources">Create a resource for task placement.</EmptyState>}
    </QueryState>
  </main>
}

export function ResourceDetailPage() {
  const { resourceId = '' } = useParams()
  const navigate = useNavigate()
  const query = useQuery({ queryKey: ['resource', resourceId], queryFn: ({ signal }) => api.get<Resource>(`/api/v1/resources/${encodeURIComponent(resourceId)}`, undefined, signal), enabled: Boolean(resourceId) })
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'resources.manage')
  return <main className="gf-content"><QueryState query={query}>{(resource) => <>
    <PageHeader title={resource.name} description="Lease ownership and fencing state." refresh={<QueryRefresh query={query} />} />
    <section className="gf-metric-grid"><MetricCard label="State" value={<StatusPill status={resourceState(resource)} />} icon={Server} /><MetricCard label="Fencing counter" value={resource.fencingToken ?? 0} icon={Activity} /><MetricCard label="Expiry" value={<time dateTime={resource.expiresAt}>{formatDateTime(resource.expiresAt)}</time>} icon={Timer} /></section>
    <section className="gf-card-panel"><h2>Active holder</h2>{resource.holder ? <Identifier id={resource.holder} href={`/runs/${resource.holder}`} copyLabel="Copy run ID" /> : <p className="gf-muted">No active lease.</p>}</section>
    {manage && <div className="gf-dialog-actions"><DangerousAction label="Delete resource" onConfirm={() => api.delete(`/api/v1/resources/${encodeURIComponent(resource.id)}`).then(() => navigate('/resources'))} /></div>}
  </>}</QueryState></main>
}
