import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, type AuthSession, type ExitCode, type OidcProvider, type Page, type RoleDefinition, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, Input, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission, PERMISSIONS } from './permissions'
import { useAuth } from './auth'
import { useUnsavedChanges } from './unsaved'

type GroupRoleMapping = { group: string; role: string }

export function roleMappingsValue(mappings: GroupRoleMapping[]): Record<string, string> {
  return Object.fromEntries(mappings.map(({ group, role }) => [group.trim(), role]).filter(([group, role]) => group && role))
}

function RoleSelect({ id, value, roles, onChange, disabled = false }: { id: string; value: string; roles?: RoleDefinition[]; onChange: (value: string) => void; disabled?: boolean }) {
  return <select id={id} className="gf-input" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} required><option value="">Select a role</option>{roles?.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select>
}

function asPage(value: Page<UserRecord> | UserRecord[]): Page<UserRecord> {
  return Array.isArray(value) ? { items: value, page: 1, limit: value.length || 20, pages: 1 } : value
}

function sessionLabel(session: AuthSession): string {
  return `${session.current ? 'Current session' : session.userAgent ?? 'Session'}${session.lastSeenAt ? ` · ${session.lastSeenAt}` : ''}`
}

export function UserManagementPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const [page, setPage] = useState(1)
  const query = useQuery({ queryKey: ['admin-users', page], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', { page, limit: 50 }, signal).then(asPage) })
  const revoke = async (session: AuthSession) => { await api.post(`/api/v1/admin/auth/sessions/revoke?session_id=${encodeURIComponent(session.id)}`); await query.refetch() }
  const disable = async (user: UserRecord) => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/disable`); await query.refetch() }
  return <main className="gf-content">
    <PageHeader title="Users and sessions" description="Review identity methods, role sources, permissions, and active sessions." />
    <QueryState query={query} empty="No users are available.">{(raw) => {
      const data = asPage(raw)
      if (!data.items.length) return <EmptyState title="No users">Create or provision a user before managing access.</EmptyState>
      return <><DataTable caption="Users" rows={data.items} columns={[
        { key: 'email', label: 'User', render: (user) => <span><strong>{user.displayName ?? user.email ?? user.username}</strong><br /><small>{user.email ?? user.username}</small></span> },
        { key: 'status', label: 'Status', render: (user) => <StatusPill status={user.status ?? (user.enabled === false ? 'disabled' : 'active')} /> },
        { key: 'loginMethods', label: 'Login methods', render: (user) => user.loginMethods?.join(', ') || '—' },
        { key: 'roles', label: 'Roles', render: (user) => user.roles?.join(', ') || '—' },
        { key: 'sessions', label: 'Sessions', render: (user) => user.sessions?.length ?? 0 },
        { key: 'actions', label: 'Actions', render: (user) => manage && <div className="gf-dialog-actions">{!user.systemAdmin && <DangerousAction label="Disable" onConfirm={() => disable(user)} />}<Link to={`/admin/users/${encodeURIComponent(user.id)}`}>Details</Link></div> },
      ]} />
      <Pagination page={data.page ?? page} pages={data.pages ?? 1} onChange={setPage} />
      {data.items.map((user) => user.sessions?.length ? <section className="gf-card-panel" key={`${user.id}-sessions`}><h2>{user.username} sessions</h2><ul className="gf-dashboard-list">{user.sessions.map((session) => <li key={session.id}><span>{sessionLabel(session)}<br /><small>{session.expiresAt ? `Expires ${session.expiresAt}` : 'Expiry unavailable'}</small></span>{manage && !session.current && <Button variant="danger" onClick={() => void revoke(session)}>Revoke</Button>}</li>)}</ul></section> : null)}
    </> }}</QueryState>
  </main>
}

function RoleEditor({ role, onDone }: { role?: RoleDefinition; onDone: () => void }) {
  const [name, setName] = useState(role?.name ?? '')
  const [selected, setSelected] = useState(() => new Set(role?.permissions ?? []))
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useUnsavedChanges(Boolean(name.trim() || selected.size))
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      if (role) await api.put(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`, { name: name.trim(), permissions: [...selected] })
      else await api.post('/api/v1/admin/roles', { name: name.trim(), permissions: [...selected] })
      onDone()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Role update failed') } finally { setBusy(false) }
  }
  return <form className="gf-editor-form" onSubmit={submit}>
    <label htmlFor="role-name">Role name<Input id="role-name" value={name} onChange={(event) => setName(event.target.value)} disabled={role?.system} required /></label>
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
  const remove = async (role: RoleDefinition) => { await api.delete(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`); await query.refetch() }
  return <main className="gf-content"><PageHeader title="Roles and permissions" description="Seeded roles are immutable. Custom roles select from the application permission catalog." action={manage && <Button onClick={() => setEditing(null)}>Create role</Button>} />
    {editing !== undefined && <section className="gf-card-panel"><h2>{editing ? `Edit ${editing.name}` : 'New custom role'}</h2><RoleEditor role={editing ?? undefined} onDone={refresh} /></section>}
    <QueryState query={query} empty="No roles are configured.">{(roles) => roles.length ? <DataTable caption="Roles" rows={roles} columns={[
      { key: 'name', label: 'Role', render: (role) => <strong>{role.name}</strong> },
      { key: 'system', label: 'Source', render: (role) => <StatusPill status={role.system ? 'system' : 'custom'} /> },
      { key: 'permissions', label: 'Permissions', render: (role) => role.permissions?.join(', ') || '—' },
      { key: 'assignedUsers', label: 'Affected users', render: (role) => Array.isArray(role.assignedUsers) ? role.assignedUsers.length : role.assignedUsers ?? 0 },
      { key: 'actions', label: 'Actions', render: (role) => manage && !role.system && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => setEditing(role)}>Edit</Button><DangerousAction label="Delete" warning={`Review ${Array.isArray(role.assignedUsers) ? role.assignedUsers.length : role.assignedUsers ?? 0} affected users before deleting this role.`} onConfirm={() => remove(role)} /></div> },
    ]} /> : <EmptyState title="No roles">Seed or create a role before assigning access.</EmptyState>}</QueryState>
  </main>
}

export function SsoSettingsPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'sso.manage')
  const query = useQuery({ queryKey: ['admin-sso'], queryFn: ({ signal }) => api.get<OidcProvider[]>('/api/v1/admin/auth/providers', undefined, signal) })
  const rolesQuery = useQuery({ queryKey: ['admin-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal) })
  const [draft, setDraft] = useState({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '' })
  const [groupMappings, setGroupMappings] = useState<GroupRoleMapping[]>([{ group: '', role: '' }])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useUnsavedChanges(Object.values(draft).some(Boolean))
  const addProvider = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const parseMapping = (value: string) => Object.fromEntries(value.split(',').map((entry) => entry.trim().split('=').map((part) => part.trim())).filter(([key, value]) => key && value))
      await api.post('/api/v1/admin/auth/providers', { key: draft.key.trim(), name: draft.name.trim(), issuer: draft.issuer.trim(), client_id: draft.clientId.trim(), secret_reference: draft.secretReference.trim(), claim_mapping: parseMapping(draft.claimMapping), group_mapping: roleMappingsValue(groupMappings), enabled: true })
      setDraft({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '' }); setGroupMappings([{ group: '', role: '' }]); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Provider update failed') } finally { setBusy(false) }
  }
  const toggle = async (provider: OidcProvider) => { await api.post('/api/v1/admin/auth/providers', { ...provider, enabled: provider.enabled === false }); await query.refetch() }
  return <main className="gf-content"><PageHeader title="SSO providers" description="Configure generic OIDC providers, claim mappings, and group-to-role mappings. Resolved secrets are never shown." />
    {manage && <form className="gf-editor-form" onSubmit={addProvider}><h2>Add provider</h2><div className="gf-form-grid"><label htmlFor="sso-key">Key<Input id="sso-key" value={draft.key} onChange={(event) => setDraft({ ...draft, key: event.target.value })} required /></label><label htmlFor="sso-name">Name<Input id="sso-name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label><label htmlFor="sso-issuer">Issuer URL<Input id="sso-issuer" type="url" value={draft.issuer} onChange={(event) => setDraft({ ...draft, issuer: event.target.value })} required /></label><label htmlFor="sso-client">Client ID<Input id="sso-client" value={draft.clientId} onChange={(event) => setDraft({ ...draft, clientId: event.target.value })} /></label><label htmlFor="sso-secret">Secret reference<Input id="sso-secret" value={draft.secretReference} onChange={(event) => setDraft({ ...draft, secretReference: event.target.value })} placeholder="vault://oidc/client-secret" /><small>Reference only; the secret value is never rendered.</small></label><label htmlFor="sso-claims">Claim mapping<Input id="sso-claims" value={draft.claimMapping} onChange={(event) => setDraft({ ...draft, claimMapping: event.target.value })} placeholder="email=email,username=preferred_username" /></label></div><fieldset><legend>Group roles</legend>{groupMappings.map((mapping, index) => <div className="gf-form-grid" key={index}><label htmlFor={`sso-group-${index}`}>Group name<Input id={`sso-group-${index}`} value={mapping.group} onChange={(event) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, group: event.target.value } : item))} /></label><label htmlFor={`sso-group-role-${index}`}>Role<RoleSelect id={`sso-group-role-${index}`} value={mapping.role} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={(role) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, role } : item))} /></label></div>)}<Button type="button" variant="secondary" onClick={() => setGroupMappings((current) => [...current, { group: '', role: '' }])}>Add group</Button>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}</fieldset>{error && <p className="gf-form-error" role="alert">{error}</p>}<Button type="submit" busy={busy}>Add provider</Button></form>}
    <QueryState query={query} empty="No SSO providers are configured.">{(providers) => providers.length ? <DataTable caption="SSO providers" rows={providers} columns={[{ key: 'key', label: 'Provider', render: (provider) => <strong>{provider.name ?? provider.key}</strong> }, { key: 'issuer', label: 'Issuer', render: (provider) => <span>{provider.issuer}</span> }, { key: 'enabled', label: 'State', render: (provider) => <StatusPill status={provider.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'secretReference', label: 'Secret', render: (provider) => <span className="gf-secret-reference">{provider.secretReference ?? 'Configured by deployment'}</span> }, { key: 'actions', label: 'Actions', render: (provider) => manage && <DangerousAction label={provider.enabled === false ? 'Enable' : 'Disable'} warning="Disabling a provider can remove a login method. Confirm another administrator login method is available." onConfirm={() => toggle(provider)} /> }]} /> : <EmptyState title="No providers">Add an OIDC provider to enable single sign-on.</EmptyState>}</QueryState>
  </main>
}

export function AuthenticationSettingsPage() {
  const { config, permissions, setConfig } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const rolesQuery = useQuery({ queryKey: ['admin-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal) })
  const [passwordLogin, setPasswordLogin] = useState(config.passwordLogin)
  const [registration, setRegistration] = useState(config.registration)
  const [defaultRoleId, setDefaultRoleId] = useState(config.defaultRoleId ?? '')
  const [saved, setSaved] = useState({ passwordLogin: config.passwordLogin, registration: config.registration, defaultRoleId: config.defaultRoleId ?? '' })
  const [error, setError] = useState('')
  useUnsavedChanges(passwordLogin !== saved.passwordLogin || registration !== saved.registration || defaultRoleId !== saved.defaultRoleId)
  const save = async () => { setError(''); try { await api.post('/api/v1/admin/auth/settings', { enabled: passwordLogin, registration, default_role_id: defaultRoleId }); setSaved({ passwordLogin, registration, defaultRoleId }); setConfig({ ...config, passwordLogin, registration, defaultRoleId }) } catch (cause) { setError(cause instanceof Error ? cause.message : 'Authentication settings update failed') } }
  const defaultRole = defaultRoleId
  const setDefaultRole = setDefaultRoleId
  return <main className="gf-content"><PageHeader title="Authentication settings" description="Control password sign-in, registration, and the default role for new identities." /><section className="gf-card-panel"><div className="gf-editor-form"><label><input type="checkbox" checked={passwordLogin} onChange={(event) => setPasswordLogin(event.target.checked)} /> Enable password login</label><label><input type="checkbox" checked={registration} onChange={(event) => setRegistration(event.target.checked)} /> Allow password registration</label><label htmlFor="default-role">Default role<RoleSelect id="default-role" value={defaultRole} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={setDefaultRole} /></label>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}{error && <p className="gf-form-error" role="alert">{error}</p>}{manage && <DangerousAction label="Save settings" warning="Changing login methods can lock out administrators. Verify that another working login method remains before saving." onConfirm={save} />}</div></section></main>
}

export function ExecutionStatusPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const query = useQuery({ queryKey: ['execution-status'], queryFn: ({ signal }) => api.get<ExitCode[]>('/api/v1/admin/execution-status', undefined, signal) })
  const [editing, setEditing] = useState<ExitCode | null>(null)
  const [creating, setCreating] = useState(false)
  const [draft, setDraft] = useState({ code: '', meaning: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const formOpen = creating || editing !== null
  const closeForm = () => { setCreating(false); setEditing(null); setDraft({ code: '', meaning: '' }); setError('') }
  const create = () => { setCreating(true); setEditing(null); setDraft({ code: '', meaning: '' }); setError('') }
  const edit = (item: ExitCode) => { setEditing(item); setCreating(false); setDraft({ code: String(item.code), meaning: item.meaning }); setError('') }
  const save = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      if (editing !== null) await api.put(`/api/v1/admin/execution-status/${encodeURIComponent(editing.code)}`, { code: Number(draft.code), meaning: draft.meaning })
      else if (draft.code.trim() !== '' && Number.isInteger(Number(draft.code))) await api.post('/api/v1/admin/execution-status', { code: Number(draft.code), meaning: draft.meaning })
      else throw new Error('Exit code must be an integer.')
      closeForm(); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Exit code update failed') } finally { setBusy(false) }
  }
  const remove = async (item: ExitCode) => { setError(''); try { await api.delete(`/api/v1/admin/execution-status/${encodeURIComponent(item.code)}`); await query.refetch() } catch (cause) { setError(cause instanceof Error ? cause.message : 'Exit code deletion failed') } }
  return <main className="gf-content"><PageHeader title="Execution Status" description="Exit codes reported by completed task processes." action={manage && !formOpen && <Button onClick={create}>Create exit code</Button>} />
    {formOpen && <section className="gf-card-panel"><form className="gf-editor-form" onSubmit={save}><div className="gf-form-grid"><label>Exit Code<Input type="number" step="1" value={draft.code} onChange={(event) => setDraft({ ...draft, code: event.target.value })} required /></label><label>Meaning<Input value={draft.meaning} onChange={(event) => setDraft({ ...draft, meaning: event.target.value })} required /></label></div>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>{editing !== null ? 'Save exit code' : 'Create exit code'}</Button><Button type="button" variant="ghost" onClick={closeForm}>Cancel</Button></div></form></section>}
    <QueryState query={query} empty="No exit code meanings are configured.">{(items) => items.length ? <DataTable caption="Execution status" rows={items.map((item) => ({ ...item, id: item.code }))} columns={[{ key: 'code', label: 'Exit Code' }, { key: 'meaning', label: 'Meaning' }, { key: 'isSystem', label: 'Type', render: (item) => <StatusPill status={item.isSystem ? 'system' : 'custom'} /> }, { key: 'actions', label: 'Actions', render: (item) => !item.isSystem && manage && <div className="gf-dialog-actions"><Button variant="secondary" onClick={() => edit(item)}>Edit</Button><DangerousAction label="Delete" onConfirm={() => remove(item)} /></div> }]} /> : <EmptyState title="No execution statuses">Create an exit code meaning.</EmptyState>}</QueryState>{error && !formOpen && <p className="gf-form-error" role="alert">{error}</p>}</main>
}
