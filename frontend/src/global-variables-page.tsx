import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type GlobalVariable, type Page } from './api'
import { Button, DataTable, DropdownMenuItem, DropdownMenuSeparator, EmptyState, Input, PageHeader, Pagination, StatusPill, TableActions } from './components'
import { DangerousAction } from './actions'
import { QueryRefresh, QueryState } from './query'
import { hasPermission } from './permissions'
import { useAuth } from './auth'

export function validGlobalVariableName(value: string): boolean {
	return /^[A-Z_][A-Z0-9_]*$/.test(value.trim())
}

export function globalVariableDeleteWarning(item: Pick<GlobalVariable, 'name' | 'references'>): string {
  const references = item.references ?? 0
  if (!references) return `${item.name} is not referenced by any task or schedule definitions. Delete it?`
  const plural = references === 1 ? '' : 's'
  return `${item.name} is referenced by ${references} task or schedule definition${plural}. Deleting it may affect those definitions and will be blocked until the references are removed.`
}

export function GlobalVariablesPage() {
  const { permissions } = useAuth()
	const manage = hasPermission(permissions, 'users.manage')
  const [editing, setEditing] = useState<GlobalVariable | null>(null)
  const [draft, setDraft] = useState({ name: '', value: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const query = useQuery({ queryKey: ['global-variables', page, limit], queryFn: ({ signal }) => api.get<Page<GlobalVariable> | GlobalVariable[]>('/api/v1/global-variables', { page, limit }, signal).then((value) => Array.isArray(value) ? { items: value.slice((page - 1) * limit, page * limit), page, limit, total: value.length, pages: Math.max(1, Math.ceil(value.length / limit)) } : value) })
  const close = () => { setEditing(null); setDraft({ name: '', value: '' }); setError('') }
  const submit = async (event: FormEvent) => {
    event.preventDefault()
	if (!validGlobalVariableName(draft.name)) { setError('Use uppercase letters, numbers, and underscores.'); return }
    setBusy(true); setError('')
    try {
      if (editing?.id) await api.put(`/api/v1/global-variables/${encodeURIComponent(editing.id)}`, draft)
      else await api.post('/api/v1/global-variables', draft)
      close(); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Global variable update failed') } finally { setBusy(false) }
  }
  const remove = async (item: GlobalVariable) => {
    setError('')
    try { await api.delete(`/api/v1/global-variables/${encodeURIComponent(item.id)}`); await query.refetch() } catch (cause) { setError(cause instanceof Error ? cause.message : 'Global variable deletion failed') }
  }
	return <main className="gf-content"><PageHeader title="Global environment variables" description="Reusable non-secret values. Reference them as $ENV:VARIABLE_NAME in supported task and schedule fields." refresh={<QueryRefresh query={query} />} />
	    {manage && !editing && <div className="gf-table-toolbar"><Button onClick={() => setEditing({ id: '', name: '', value: '' })}>Create variable</Button></div>}
    {manage && editing && <form className="gf-editor-form" onSubmit={submit}><div className="gf-form-grid"><label>Name<Input list="global-variable-names" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} disabled={Boolean(editing.id)} required /></label><label>Value<Input value={draft.value} onChange={(event) => setDraft({ ...draft, value: event.target.value })} required /></label></div><datalist id="global-variable-names">{(query.data?.items ?? []).map((item) => <option key={item.id} value={item.name} />)}</datalist>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>Save variable</Button><Button type="button" variant="ghost" onClick={close}>Cancel</Button></div></form>}
	    <QueryState query={query} empty="No global variables are configured.">{(data) => data.items.length ? <><DataTable caption="Global environment variables" rows={data.items} columns={[{ key: 'name', label: 'Name', render: (item) => <strong>{item.name}</strong> }, { key: 'value', label: 'Value' }, { key: 'references', label: 'References', render: (item) => <StatusPill status={item.references ? 'in use' : 'unused'} /> }, { key: 'actions', label: 'Actions', render: (item) => manage && <TableActions label={`Actions for ${item.name}`}><DropdownMenuItem onSelect={() => { setEditing(item); setDraft({ name: item.name, value: item.value }) }}>Edit</DropdownMenuItem><DropdownMenuSeparator /><DangerousAction label="Delete" title={`Delete ${item.name}`} warning={globalVariableDeleteWarning(item)} onConfirm={() => remove(item)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={data.page ?? page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No global variables">Create a reusable value for task and schedule fields.</EmptyState>}</QueryState>{error && !editing && <p className="gf-form-error" role="alert">{error}</p>}</main>
}
