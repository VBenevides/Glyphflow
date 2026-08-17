import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type GlobalVariable, type Page } from './api'
import { Button, DataTable, EmptyState, Input, PageHeader, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'
import { useAuth } from './auth'

export function validGlobalVariableName(value: string): boolean {
	return /^[A-Z_][A-Z0-9_]*$/.test(value.trim())
}

export function GlobalVariablesPage() {
  const { permissions } = useAuth()
	const manage = hasPermission(permissions, 'users.manage')
  const [editing, setEditing] = useState<GlobalVariable | null>(null)
  const [draft, setDraft] = useState({ name: '', value: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const query = useQuery({ queryKey: ['global-variables'], queryFn: ({ signal }) => api.get<Page<GlobalVariable> | GlobalVariable[]>('/api/v1/global-variables', undefined, signal).then((value) => Array.isArray(value) ? value : value.items) })
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
	return <main className="gf-content"><PageHeader title="Global environment variables" description="Reusable non-secret values. Reference them as $ENV:VARIABLE_NAME in supported task and schedule fields." action={manage && !editing && <Button onClick={() => setEditing({ id: '', name: '', value: '' })}>Create variable</Button>} />
    {manage && editing && <form className="gf-editor-form" onSubmit={submit}><div className="gf-form-grid"><label>Name<Input list="global-variable-names" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} disabled={Boolean(editing.id)} required /></label><label>Value<Input value={draft.value} onChange={(event) => setDraft({ ...draft, value: event.target.value })} required /></label></div><datalist id="global-variable-names">{(query.data ?? []).map((item) => <option key={item.id} value={item.name} />)}</datalist>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>Save variable</Button><Button type="button" variant="ghost" onClick={close}>Cancel</Button></div></form>}
    <QueryState query={query} empty="No global variables are configured.">{(items) => items.length ? <DataTable caption="Global environment variables" rows={items} columns={[{ key: 'name', label: 'Name', render: (item) => <strong>{item.name}</strong> }, { key: 'value', label: 'Value' }, { key: 'references', label: 'References', render: (item) => <StatusPill status={item.references ? 'in use' : 'unused'} /> }, { key: 'actions', label: 'Actions', render: (item) => manage && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => { setEditing(item); setDraft({ name: item.name, value: item.value }) }}>Edit</Button><Button variant="danger" onClick={() => void remove(item)}>Delete</Button></div> }]} /> : <EmptyState title="No global variables">Create a reusable value for task and schedule fields.</EmptyState>}</QueryState>{error && !editing && <p className="gf-form-error" role="alert">{error}</p>}</main>
}
