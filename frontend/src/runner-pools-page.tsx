import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from './auth'
import { api, type Page, type RunnerPool } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, StatusPill } from './components'
import { describeError } from './errors'
import { QueryState } from './query'
import { hasPermission } from './permissions'

type PoolDraft = { name: string; description: string; enabled: boolean }
const emptyDraft: PoolDraft = { name: '', description: '', enabled: true }

export function RunnerPoolsPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'runners.manage')
  const [draft, setDraft] = useState<PoolDraft>(emptyDraft)
  const [editing, setEditing] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['runner-pools'], queryFn: ({ signal }) => api.get<Page<RunnerPool>>('/api/v1/runners/pools', { limit: 100 }, signal) })
  const save = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      if (editing) await api.put(`/api/v1/runners/pools/${encodeURIComponent(editing)}`, draft)
      else await api.post('/api/v1/runners/pools', draft)
      setDraft(emptyDraft); setEditing(null); await query.refetch()
    } catch (cause) { setError(describeError(cause).message) } finally { setBusy(false) }
  }
  const form = manage && (editing !== null || draft.name !== '') ? <section className="gf-card-panel"><h2>{editing ? 'Edit pool' : 'Create pool'}</h2><form className="gf-editor-form" onSubmit={save}><label>Name<Input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label><label>Description<Input value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} /></label><label><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enabled</label>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={() => { setDraft(emptyDraft); setEditing(null); setError('') }}>Cancel</Button><Button type="submit" busy={busy}>{editing ? 'Save pool' : 'Create pool'}</Button></div></form></section> : null
  return <main className="gf-content"><PageHeader title="Pools" description="Group runners for task placement and enrollment." action={manage && <Button onClick={() => { setDraft(emptyDraft); setEditing(''); setError('') }}>Create pool</Button>} />{form}<QueryState query={query} empty="Create a pool before enrolling runners.">{(data) => data.items.length ? <DataTable caption="Runner pools" rows={data.items} columns={[{ key: 'name', label: 'Pool' }, { key: 'description', label: 'Description' }, { key: 'enabled', label: 'State', render: (pool) => <StatusPill status={pool.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'actions', label: 'Actions', render: (pool) => manage && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => { setDraft({ name: pool.name, description: pool.description ?? '', enabled: pool.enabled !== false }); setEditing(pool.id); setError('') }}>Edit</Button><DangerousAction label="Archive" confirmLabel="Archive" warning="Archives this pool and hides it from active placement. Archive its runners and tasks first; historical task versions are retained." onConfirm={async () => { await api.delete(`/api/v1/runners/pools/${encodeURIComponent(pool.id)}`); void query.refetch() }} /></div> }]} /> : <EmptyState title="No pools">Create a pool before enrolling runners.</EmptyState>}</QueryState></main>
}
