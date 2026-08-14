import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, type AuthSession, type Page, type RoleDefinition, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission, PERMISSIONS } from './permissions'
import { useAuth } from './auth'

function asPage(value: Page<UserRecord> | UserRecord[]): Page<UserRecord> {
  return Array.isArray(value) ? { items: value, page: 1, limit: value.length || 20, pages: 1 } : value
}

function sessionLabel(session: AuthSession): string {
  return `${session.current ? 'Current session' : session.userAgent ?? 'Session'}${session.lastSeenAt ? ` · ${session.lastSeenAt}` : ''}`
}

export function UserManagementPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const query = useQuery({ queryKey: ['admin-users'], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', undefined, signal).then(asPage) })
  const revoke = async (session: AuthSession) => { await api.post(`/api/v1/admin/auth/sessions/revoke?session_id=${encodeURIComponent(session.id)}`); await query.refetch() }
  const disable = async (user: UserRecord) => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/disable`); await query.refetch() }
  return <main className="gf-content">
    <PageHeader title="Users and sessions" description="Review identity methods, role sources, permissions, and active sessions." />
    <QueryState query={query} empty="No users are available.">{(raw) => {
      const data = asPage(raw)
      if (!data.items.length) return <EmptyState title="No users">Create or provision a user before managing access.</EmptyState>
      return <><DataTable caption="Users" rows={data.items} columns={[
        { key: 'username', label: 'User', render: (user) => <span><strong>{user.displayName ?? user.username}</strong><br /><small>{user.username}{user.email ? ` · ${user.email}` : ''}</small></span> },
        { key: 'status', label: 'Status', render: (user) => <StatusPill status={user.status ?? (user.enabled === false ? 'disabled' : 'active')} /> },
        { key: 'loginMethods', label: 'Login methods', render: (user) => user.loginMethods?.join(', ') || '—' },
        { key: 'roles', label: 'Roles', render: (user) => user.roles?.join(', ') || '—' },
        { key: 'sessions', label: 'Sessions', render: (user) => user.sessions?.length ?? 0 },
        { key: 'actions', label: 'Actions', render: (user) => manage && <div className="gf-dialog-actions"><DangerousAction label="Disable" onConfirm={() => disable(user)} /><Link to={`/admin/users/${encodeURIComponent(user.id)}`}>Details</Link></div> },
      ]} />
      <Pagination page={data.page ?? 1} pages={data.pages ?? 1} onChange={() => undefined} />
      {data.items.map((user) => user.sessions?.length ? <section className="gf-card-panel" key={`${user.id}-sessions`}><h2>{user.username} sessions</h2><ul className="gf-dashboard-list">{user.sessions.map((session) => <li key={session.id}><span>{sessionLabel(session)}<br /><small>{session.expiresAt ? `Expires ${session.expiresAt}` : 'Expiry unavailable'}</small></span>{manage && !session.current && <Button variant="danger" onClick={() => void revoke(session)}>Revoke</Button>}</li>)}</ul></section> : null)}
    </> }}</QueryState>
  </main>
}

function RoleEditor({ role, onDone }: { role?: RoleDefinition; onDone: () => void }) {
  const [key, setKey] = useState(role?.key ?? '')
  const [selected, setSelected] = useState(() => new Set(role?.permissions ?? []))
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      if (role) await api.put(`/api/v1/admin/roles/${encodeURIComponent(role.key)}`, { permissions: [...selected] })
      else await api.post('/api/v1/admin/roles', { key: key.trim(), permissions: [...selected] })
      onDone()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Role update failed') } finally { setBusy(false) }
  }
  return <form className="gf-editor-form" onSubmit={submit}>
    {!role && <label htmlFor="role-key">Role key<Input id="role-key" value={key} onChange={(event) => setKey(event.target.value)} pattern="[A-Za-z0-9._-]+" required /></label>}
    {role && <p><strong>{role.key}</strong>{role.description ? ` — ${role.description}` : ''}</p>}
    <fieldset><legend>Permissions</legend><div className="gf-permission-grid">{PERMISSIONS.map((permission) => <label key={permission}><input type="checkbox" checked={selected.has(permission)} onChange={(event) => setSelected((current) => { const next = new Set(current); event.target.checked ? next.add(permission) : next.delete(permission); return next })} /> {permission}</label>)}</div></fieldset>
    {error && <p className="gf-form-error" role="alert">{error}</p>}
    <div className="gf-dialog-actions"><Button type="submit" busy={busy}>{role ? 'Save permissions' : 'Create role'}</Button><Button type="button" variant="ghost" onClick={onDone}>Cancel</Button></div>
  </form>
}

export function RoleManagementPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'roles.manage')
  const query = useQuery({ queryKey: ['admin-roles'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal) })
  const [editing, setEditing] = useState<RoleDefinition | null | undefined>(undefined)
  const refresh = async () => { setEditing(undefined); await query.refetch() }
  const remove = async (role: RoleDefinition) => { await api.delete(`/api/v1/admin/roles/${encodeURIComponent(role.key)}`); await query.refetch() }
  return <main className="gf-content"><PageHeader title="Roles and permissions" description="Seeded roles are immutable. Custom roles select from the application permission catalog." action={manage && <Button onClick={() => setEditing(null)}>Create role</Button>} />
    {editing !== undefined && <section className="gf-card-panel"><h2>{editing ? `Edit ${editing.key}` : 'New custom role'}</h2><RoleEditor role={editing ?? undefined} onDone={refresh} /></section>}
    <QueryState query={query} empty="No roles are configured.">{(roles) => roles.length ? <DataTable caption="Roles" rows={roles} columns={[
      { key: 'key', label: 'Role', render: (role) => <strong>{role.name ?? role.key}</strong> },
      { key: 'system', label: 'Source', render: (role) => <StatusPill status={role.system ? 'system' : 'custom'} /> },
      { key: 'permissions', label: 'Permissions', render: (role) => role.permissions.join(', ') || '—' },
      { key: 'assignedUsers', label: 'Affected users', render: (role) => Array.isArray(role.assignedUsers) ? role.assignedUsers.length : role.assignedUsers ?? 0 },
      { key: 'actions', label: 'Actions', render: (role) => manage && !role.system && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => setEditing(role)}>Edit</Button><DangerousAction label="Delete" warning={`Review ${Array.isArray(role.assignedUsers) ? role.assignedUsers.length : role.assignedUsers ?? 0} affected users before deleting this role.`} onConfirm={() => remove(role)} /></div> },
    ]} /> : <EmptyState title="No roles">Seed or create a role before assigning access.</EmptyState>}</QueryState>
  </main>
}
