import { useState, type FormEvent, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from './auth'
import { api, type Page, type RunnerPool } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, Dialog, DropdownMenuItem, DropdownMenuSeparator, EmptyState, Input, MetricCard, PageHeader, Pagination, StatusPill, TableActions } from './components'
import { describeError } from './errors'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'

type PoolDraft = { name: string; description: string; enabled: boolean }
const emptyDraft: PoolDraft = { name: '', description: '', enabled: true }

export function RunnerPoolsPage({ navigation, title = 'Pools', description = 'Group runners for task placement and enrollment.' }: { navigation?: ReactNode; title?: string; description?: string } = {}) {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'runners.manage')
  const [draft, setDraft] = useState<PoolDraft>(emptyDraft)
  const [editing, setEditing] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['runner-pools', page, limit], queryFn: ({ signal }) => api.get<Page<RunnerPool>>('/api/v1/runners/pools', { page, limit }, signal) })
  const save = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      if (editing) await api.put(`/api/v1/runners/pools/${encodeURIComponent(editing)}`, draft)
      else await api.post('/api/v1/runners/pools', draft)
      setDraft(emptyDraft)
      setEditing(null)
      await query.refetch()
    } catch (cause) {
      setError(describeError(cause).message)
    } finally {
      setBusy(false)
    }
  }
  const close = () => {
    if (busy) return
    setDraft(emptyDraft)
    setEditing(null)
    setError('')
  }
  const create = manage && <Button onClick={() => { setDraft(emptyDraft); setEditing(''); setError('') }}>Create pool</Button>
  const form = manage && editing !== null ? <Dialog open title={editing ? 'Edit pool' : 'Create pool'} onClose={close}>
    <form className="gf-editor-form" onSubmit={save}>
      <label>Name<Input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label>
      <label>Description<Input value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} /></label>
      <label><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enabled</label>
      {error && <p className="gf-form-error" role="alert">{error}</p>}
      <div className="gf-dialog-actions"><Button type="button" variant="secondary" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" busy={busy}>{editing ? 'Save pool' : 'Create pool'}</Button></div>
    </form>
  </Dialog> : null
  const content = <>
    {form}
    <QueryState query={query} empty="Create a pool before enrolling runners.">
      {(data) => <>
        <div className="gf-metric-grid"><MetricCard label="Number of pools" value={data.total ?? data.items.length} detail="Configured runner pools" /></div>
        {data.items.length ? <>
          <DataTable caption="Runner pools" rows={data.items} columns={[
            { key: 'name', label: 'Pool' },
            { key: 'description', label: 'Description' },
            { key: 'enabled', label: 'State', render: (pool) => <StatusPill status={pool.enabled === false ? 'disabled' : 'enabled'} /> },
            { key: 'actions', label: 'Actions', render: (pool) => manage && <TableActions label={`Actions for ${pool.name}`}>
              <DropdownMenuItem onSelect={() => { setDraft({ name: pool.name, description: pool.description ?? '', enabled: pool.enabled !== false }); setEditing(pool.id); setError('') }}>Edit</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DangerousAction label="Archive" confirmLabel="Archive" warning="Archives this pool and hides it from active placement. Archive its runners and tasks first; historical task versions are retained." onConfirm={async () => { await api.delete(`/api/v1/runners/pools/${encodeURIComponent(pool.id)}`); void query.refetch() }} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Archive</DropdownMenuItem>} />
            </TableActions> },
          ]} />
          <Pagination page={data.page ?? page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} />
        </> : <EmptyState title="No pools">Create a pool before enrolling runners.</EmptyState>}
      </>}
    </QueryState>
  </>
  return <main className="gf-content"><PageHeader title={title} description={description} action={create} refresh={<QueryRefresh query={query} />} />{navigation}{content}</main>
}
