import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, type AuthSession, type OidcProvider, type Page, type RoleDefinition, type UserRecord } from './api'
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

export function SsoSettingsPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'sso.manage')
  const query = useQuery({ queryKey: ['admin-sso'], queryFn: ({ signal }) => api.get<OidcProvider[]>('/api/v1/admin/auth/providers', undefined, signal) })
  const [draft, setDraft] = useState({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '', groupMapping: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const addProvider = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const parseMapping = (value: string) => Object.fromEntries(value.split(',').map((entry) => entry.trim().split('=').map((part) => part.trim())).filter(([key, value]) => key && value))
      await api.post('/api/v1/admin/auth/providers', { key: draft.key.trim(), name: draft.name.trim(), issuer: draft.issuer.trim(), client_id: draft.clientId.trim(), secret_reference: draft.secretReference.trim(), claim_mapping: parseMapping(draft.claimMapping), group_mapping: parseMapping(draft.groupMapping), enabled: true })
      setDraft({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '', groupMapping: '' }); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Provider update failed') } finally { setBusy(false) }
  }
  const toggle = async (provider: OidcProvider) => { await api.post('/api/v1/admin/auth/providers', { ...provider, enabled: provider.enabled === false }); await query.refetch() }
  return <main className="gf-content"><PageHeader title="SSO providers" description="Configure generic OIDC providers, claim mappings, and group-to-role mappings. Resolved secrets are never shown." />
    {manage && <form className="gf-editor-form" onSubmit={addProvider}><h2>Add provider</h2><div className="gf-form-grid"><label htmlFor="sso-key">Key<Input id="sso-key" value={draft.key} onChange={(event) => setDraft({ ...draft, key: event.target.value })} required /></label><label htmlFor="sso-name">Name<Input id="sso-name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label><label htmlFor="sso-issuer">Issuer URL<Input id="sso-issuer" type="url" value={draft.issuer} onChange={(event) => setDraft({ ...draft, issuer: event.target.value })} required /></label><label htmlFor="sso-client">Client ID<Input id="sso-client" value={draft.clientId} onChange={(event) => setDraft({ ...draft, clientId: event.target.value })} /></label><label htmlFor="sso-secret">Secret reference<Input id="sso-secret" value={draft.secretReference} onChange={(event) => setDraft({ ...draft, secretReference: event.target.value })} placeholder="vault://oidc/client-secret" /><small>Reference only; the secret value is never rendered.</small></label><label htmlFor="sso-claims">Claim mapping<Input id="sso-claims" value={draft.claimMapping} onChange={(event) => setDraft({ ...draft, claimMapping: event.target.value })} placeholder="email=email,username=preferred_username" /></label><label htmlFor="sso-groups">Group mapping<Input id="sso-groups" value={draft.groupMapping} onChange={(event) => setDraft({ ...draft, groupMapping: event.target.value })} placeholder="admins=admin" /></label></div>{error && <p className="gf-form-error" role="alert">{error}</p>}<Button type="submit" busy={busy}>Add provider</Button></form>}
    <QueryState query={query} empty="No SSO providers are configured.">{(providers) => providers.length ? <DataTable caption="SSO providers" rows={providers} columns={[{ key: 'key', label: 'Provider', render: (provider) => <strong>{provider.name ?? provider.key}</strong> }, { key: 'issuer', label: 'Issuer', render: (provider) => <span>{provider.issuer}</span> }, { key: 'enabled', label: 'State', render: (provider) => <StatusPill status={provider.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'secretReference', label: 'Secret', render: (provider) => <span className="gf-secret-reference">{provider.secretReference ?? 'Configured by deployment'}</span> }, { key: 'actions', label: 'Actions', render: (provider) => manage && <DangerousAction label={provider.enabled === false ? 'Enable' : 'Disable'} warning="Disabling a provider can remove a login method. Confirm another administrator login method is available." onConfirm={() => toggle(provider)} /> }]} /> : <EmptyState title="No providers">Add an OIDC provider to enable single sign-on.</EmptyState>}</QueryState>
  </main>
}

export function AuthenticationSettingsPage() {
  const { config, permissions } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const [passwordLogin, setPasswordLogin] = useState(config.passwordLogin)
  const [registration, setRegistration] = useState(config.registration)
  const [defaultRole, setDefaultRole] = useState('user')
  const [error, setError] = useState('')
  const save = async () => { setError(''); try { await api.post('/api/v1/admin/auth/settings', { enabled: passwordLogin, registration, default_role: defaultRole.trim() || 'user' }) } catch (cause) { setError(cause instanceof Error ? cause.message : 'Authentication settings update failed') } }
  return <main className="gf-content"><PageHeader title="Authentication settings" description="Control password sign-in, registration, and the default role for new identities." /><section className="gf-card-panel"><div className="gf-editor-form"><label><input type="checkbox" checked={passwordLogin} onChange={(event) => setPasswordLogin(event.target.checked)} /> Enable password login</label><label><input type="checkbox" checked={registration} onChange={(event) => setRegistration(event.target.checked)} /> Allow password registration</label><label htmlFor="default-role">Default role<Input id="default-role" value={defaultRole} onChange={(event) => setDefaultRole(event.target.value)} /></label>{error && <p className="gf-form-error" role="alert">{error}</p>}{manage && <DangerousAction label="Save settings" warning="Changing login methods can lock out administrators. Verify that another working login method remains before saving." onConfirm={save} />}</div></section></main>
}
