import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type Page, type Resource } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'

export function resourceState(resource: Resource) { return resource.enabled === false ? 'disabled' : resource.holder ? 'leased' : 'available' }

export function ResourceInventoryPage() {
  const { permissions } = useAuth(); const query = useQuery({ queryKey: ['resources'], queryFn: ({ signal }) => api.get<Page<Resource>>('/api/v1/resources', undefined, signal) }); const manage = hasPermission(permissions, 'resources.manage')
  return <main className="gf-content"><PageHeader title="Resources and leases" description="Exclusive resources, fencing counters, and active holders." action={manage && <Button>Create resource</Button>} /><QueryState query={query} empty="Create an exclusive resource for task placement.">{(data) => data.items.length ? <DataTable caption="Resources" rows={data.items} columns={[{ key: 'name', label: 'Resource', render: (resource) => <Link to={`/resources/${resource.id}`}>{resource.name}</Link> }, { key: 'enabled', label: 'State', render: (resource) => <StatusPill status={resourceState(resource)} /> }, { key: 'holder', label: 'Holder', render: (resource) => resource.holder ? <Link to={`/runs/${resource.holder}`}>{resource.holder}</Link> : '—' }, { key: 'expiresAt', label: 'Lease expiry' }, { key: 'fencingToken', label: 'Fencing token' }, { key: 'actions', label: 'Actions', render: (resource) => manage && <DangerousAction label="Delete" onConfirm={() => api.delete(`/api/v1/resources/${encodeURIComponent(resource.id)}`)} /> }]} /> : <EmptyState title="No resources">Create an exclusive resource for task placement.</EmptyState>}</QueryState></main>
}

export function ResourceDetailPage() {
  const { resourceId = '' } = useParams(); const query = useQuery({ queryKey: ['resource', resourceId], queryFn: ({ signal }) => api.get<Resource>(`/api/v1/resources/${encodeURIComponent(resourceId)}`, undefined, signal), enabled: Boolean(resourceId) }); const { permissions } = useAuth(); const manage = hasPermission(permissions, 'resources.manage')
  return <main className="gf-content"><QueryState query={query}>{(resource) => <><PageHeader title={resource.name} description="Lease ownership and fencing state." /><section className="gf-metric-grid"><div className="gf-metric"><span>State</span><strong><StatusPill status={resourceState(resource)} /></strong></div><div className="gf-metric"><span>Fencing counter</span><strong>{resource.fencingToken ?? 0}</strong></div><div className="gf-metric"><span>Expiry</span><strong>{resource.expiresAt ?? '—'}</strong></div></section><section className="gf-card-panel"><h2>Active holder</h2>{resource.holder ? <Link to={`/runs/${resource.holder}`}>{resource.holder}</Link> : <p className="gf-muted">No active lease.</p>}</section>{manage && <div className="gf-dialog-actions"><Button variant="secondary">Edit resource</Button><DangerousAction label="Delete resource" onConfirm={() => api.delete(`/api/v1/resources/${encodeURIComponent(resource.id)}`)} /></div>}</>}</QueryState></main>
}
