import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, type AdminSession, type ExitCode, type OidcProvider, type Page, type RoleDefinition, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, Dialog, DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuPortal, DropdownMenuSeparator, DropdownMenuTrigger, EmptyState, FilterInput, Input, PageHeader, Pagination, StatusPill, TableActions, Tabs, TabsList, TabsTrigger } from './components'
import { QueryState } from './query'
import { hasPermission, PERMISSIONS } from './permissions'
import { useAuth } from './auth'
import { useUnsavedChanges } from './unsaved'
import { formatDateTime } from './format'

type GroupRoleMapping = { group: string; role: string }

export function roleMappingsValue(mappings: GroupRoleMapping[]): Record<string, string> {
  return Object.fromEntries(mappings.map(({ group, role }) => [group.trim(), role]).filter(([group, role]) => group && role))
}

function RoleSelect({ id, value, roles, onChange, disabled = false }: { id: string; value: string; roles?: RoleDefinition[]; onChange: (value: string) => void; disabled?: boolean }) {
  return <select id={id} className="gf-input" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} required><option value="">Select a role</option>{roles?.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select>
}

function asPage(value: Page<UserRecord> | UserRecord[], page = 1, limit = 10): Page<UserRecord> {
  return Array.isArray(value) ? { items: value.slice((page - 1) * limit, page * limit), page, limit, total: value.length, pages: Math.max(1, Math.ceil(value.length / limit)) } : value
}

function InlinePills({ values }: { values?: string[] }) {
  return values?.length ? <span className="gf-pill-list">{values.map((value) => <span className="gf-inline-pill" key={value}>{value}</span>)}</span> : '—'
}

export function filterAndSortRoles(roles: RoleDefinition[], search: string): RoleDefinition[] {
  const needle = search.trim().toLowerCase()
  return roles.filter((role) => !needle || [role.id, role.name, role.description ?? '', ...role.permissions].some((value) => value.toLowerCase().includes(needle))).sort((left, right) => Number(Boolean(right.system)) - Number(Boolean(left.system)) || left.name.localeCompare(right.name))
}

function UserActionsMenu({ user, manage, onAccess, onDisable }: { user: UserRecord; manage: boolean; onAccess: () => void; onDisable: () => void }) {
  const userLabel = user.displayName ?? user.email ?? user.username
  return <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" variant="ghost" aria-label={`Actions for ${userLabel}`}><MoreHorizontal size={18} /></Button></DropdownMenuTrigger><DropdownMenuPortal><DropdownMenuContent align="end">{manage && <DropdownMenuItem onSelect={onAccess}>Manage access</DropdownMenuItem>}{manage && !user.systemAdmin && <><DropdownMenuSeparator /><DangerousAction label="Disable" title="Disable user" onConfirm={onDisable} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Disable</DropdownMenuItem>} /></>}<DropdownMenuSeparator /><DropdownMenuItem asChild><Link to={`/admin/users/${encodeURIComponent(user.id)}`}>Details</Link></DropdownMenuItem></DropdownMenuContent></DropdownMenuPortal></DropdownMenu>
}

type IdentityView = 'users' | 'sessions' | 'sso'

function IdentityAdminLayout({ view, title, description, action, children }: { view: IdentityView; title: string; description: string; action?: ReactNode; children: ReactNode }) {
  const navigate = useNavigate()
  return <main className="gf-content"><PageHeader title={title} description={description} action={action} /><Tabs value={view} onValueChange={(next) => navigate(next === 'sso' ? '/admin/sso' : next === 'sessions' ? '/admin/users/sessions' : '/admin/users')}><TabsList aria-label="Identity administration"><TabsTrigger value="users">Users</TabsTrigger><TabsTrigger value="sessions">Sessions</TabsTrigger><TabsTrigger value="sso">SSO</TabsTrigger></TabsList></Tabs>{children}</main>
}

export function UserCreationForm({ onCreated }: { onCreated: (userID: string) => Promise<void> }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const user = await api.post<{ id: string }>('/api/v1/users', { email: email.trim(), password })
      await onCreated(user.id)
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'User creation failed') } finally { setBusy(false) }
  }
  return <form className="gf-editor-form" onSubmit={submit}><div className="gf-form-grid"><label htmlFor="admin-user-email">Email<Input id="admin-user-email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label><label htmlFor="admin-user-password">Temporary password<Input id="admin-user-password" type="password" autoComplete="new-password" minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} required /></label></div>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>Create user</Button></div></form>
}

export function UserAccessEditor({ user, roles, onChanged, onClose }: { user: UserRecord; roles?: RoleDefinition[]; onChanged: () => Promise<void>; onClose: () => void }) {
  const [roleID, setRoleID] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const role = roles?.find((item) => item.id === roleID)
  const assign = async (event: FormEvent) => {
    event.preventDefault()
    if (!role) return
    setBusy(true); setError('')
    try { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/roles`, { role: role.name }); setRoleID(''); await onChanged() } catch (cause) { setError(cause instanceof Error ? cause.message : 'Role assignment failed') } finally { setBusy(false) }
  }
  const revoke = async (name: string) => { await api.delete(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/roles/${encodeURIComponent(name)}`); await onChanged() }
  return <Dialog open title={`Manage access for ${user.email ?? user.username}`} onClose={onClose}><section className="gf-card-panel"><form className="gf-editor-form" onSubmit={assign}><label htmlFor="admin-user-role">Assign role<RoleSelect id="admin-user-role" value={roleID} roles={roles} disabled={!roles?.length || busy} onChange={setRoleID} /></label>{!roles?.length && <p className="gf-form-error">Roles could not be loaded. Verify that you have role-read access.</p>}{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy} disabled={!role}>Assign role</Button><Button type="button" variant="ghost" onClick={onClose}>Close</Button></div></form><h3>Assigned roles</h3>{user.roles?.length ? <ul className="gf-dashboard-list">{user.roles.map((name) => <li key={name}><span>{name}</span><DangerousAction label="Revoke" warning={`Remove the ${name} role from this user.`} onConfirm={() => revoke(name)} /></li>)}</ul> : <p className="gf-muted">No roles are assigned.</p>}</section></Dialog>
}

export function UserManagementPage({ view = 'users' }: { view?: IdentityView } = {}) {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [email, setEmail] = useState('')
  const [creating, setCreating] = useState(false)
  const [accessUserID, setAccessUserID] = useState<string | null>(null)
  const query = useQuery({ queryKey: ['admin-users', page, limit, email], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', { page, limit, email: email || undefined }, signal).then((value) => asPage(value, page, limit)) })
  const rolesQuery = useQuery({ queryKey: ['admin-user-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal), enabled: manage })
  const disable = async (user: UserRecord) => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/disable`); await query.refetch() }
  const refreshUsers = async () => { await query.refetch() }
  const created = async (userID: string) => { setCreating(false); setAccessUserID(userID); await query.refetch() }
  return <IdentityAdminLayout view={view} title="Users and sessions" description="Review identity methods, role sources, permissions, and active sessions." action={manage && <Button onClick={() => setCreating((value) => !value)}>{creating ? 'Cancel' : 'Create user'}</Button>}>
    {creating && <Dialog open title="Create user" onClose={() => setCreating(false)}><UserCreationForm onCreated={created} /></Dialog>}
    <div className="gf-filter-bar"><FilterInput label="Email" type="email" value={email} options={(query.data?.items ?? []).map((user) => user.email ?? user.username)} onChange={(value) => { setEmail(value); setPage(1) }} placeholder="Filter by email" /></div>
    <QueryState query={query} empty="No users are available.">{(raw) => {
      const data = asPage(raw)
      if (!data.items.length) return <EmptyState title="No users">Create or provision a user before managing access.</EmptyState>
      const accessUser = data.items.find((user) => user.id === accessUserID)
      return <><DataTable caption="Users" rows={data.items} columns={[
        { key: 'email', label: 'User', render: (user) => <span><strong>{user.displayName ?? user.email ?? user.username}</strong><br /><small>{user.email ?? user.username}</small></span> },
        { key: 'status', label: 'Status', render: (user) => <StatusPill status={user.status ?? (user.enabled === false ? 'disabled' : 'active')} /> },
        { key: 'loginMethods', label: 'Login methods', render: (user) => <InlinePills values={user.loginMethods} /> },
        { key: 'roles', label: 'Roles', render: (user) => <InlinePills values={user.roles} /> },
        { key: 'actions', label: 'Actions', render: (user) => <UserActionsMenu user={user} manage={manage} onAccess={() => setAccessUserID(user.id)} onDisable={() => disable(user)} /> },
      ]} />
      <Pagination page={data.page ?? page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} />
      {accessUser && <UserAccessEditor user={accessUser} roles={rolesQuery.data} onChanged={refreshUsers} onClose={() => setAccessUserID(null)} />}
    </> }}</QueryState>
  </IdentityAdminLayout>
}

export function SessionManagementPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10); const [email, setEmail] = useState('')
  const query = useQuery({ queryKey: ['admin-sessions', page, limit, email], queryFn: ({ signal }) => api.get<Page<AdminSession>>('/api/v1/admin/auth/sessions', { page, limit, email: email || undefined }, signal) })
  const revoke = async (session: AdminSession) => { await api.post(`/api/v1/admin/auth/sessions/revoke?session_id=${encodeURIComponent(session.id)}`); await query.refetch() }
  return <IdentityAdminLayout view="sessions" title="Sessions" description="Review active authentication sessions across users.">
    <div className="gf-filter-bar"><FilterInput label="User email" type="email" value={email} options={(query.data?.items ?? []).map((session) => session.userEmail)} onChange={(value) => { setEmail(value); setPage(1) }} placeholder="Filter by user email" /></div>
    <QueryState query={query} empty="No active sessions match this filter.">{(data) => data.items.length ? <><DataTable caption="Sessions" rows={data.items} columns={[{ key: 'userEmail', label: 'User', render: (session) => <Link to={`/admin/users/${encodeURIComponent(session.userId)}`}>{session.userEmail}</Link> }, { key: 'id', label: 'Session ID', render: (session) => <code>{session.id}</code> }, { key: 'lastSeenAt', label: 'Last seen', render: (session) => session.lastSeenAt ? formatDateTime(session.lastSeenAt) : '—' }, { key: 'expiresAt', label: 'Expires', render: (session) => session.expiresAt ? formatDateTime(session.expiresAt) : '—' }, { key: 'userAgent', label: 'Client', render: (session) => session.userAgent ?? '—' }, { key: 'actions', label: 'Actions', render: (session) => manage && <TableActions label={`Actions for ${session.userEmail}`}><DangerousAction label="Revoke" warning="This will immediately invalidate the session." onConfirm={() => revoke(session)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Revoke</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No active sessions">No active sessions match this filter.</EmptyState>}</QueryState>
  </IdentityAdminLayout>
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
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const [search, setSearch] = useState('')
  const refresh = async () => { setEditing(undefined); await query.refetch() }
  const remove = async (role: RoleDefinition) => { await api.delete(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`); await query.refetch() }
  return <main className="gf-content"><PageHeader title="Roles and permissions" description="Seeded roles are immutable. Custom roles select from the application permission catalog." action={manage && <Button onClick={() => setEditing(null)}>Create role</Button>} />
    {editing !== undefined && <Dialog open title={editing ? `Edit ${editing.name}` : 'New custom role'} onClose={() => setEditing(undefined)}><RoleEditor role={editing ?? undefined} onDone={refresh} /></Dialog>}
    <QueryState query={query} empty="No roles are configured.">{(roles) => { const filteredRoles = filterAndSortRoles(roles, search); return <><div className="gf-filter-bar"><FilterInput label="Search" options={roles.flatMap((role) => [role.name, role.id, ...role.permissions])} value={search} onChange={(value) => { setSearch(value); setPage(1) }} placeholder="Role name, key, or permission" /></div>{filteredRoles.length ? <div className="gf-role-table"><DataTable caption="Roles" rows={filteredRoles.slice((page - 1) * limit, page * limit)} columns={[
      { key: 'name', label: 'Role', render: (role) => <strong>{role.name}</strong> },
      { key: 'system', label: 'Source', render: (role) => <StatusPill status={role.system ? 'system' : 'custom'} /> },
      { key: 'permissions', label: 'Permissions', render: (role) => <InlinePills values={role.permissions} /> },
      { key: 'assignedUsers', label: 'Affected users', render: (role) => role.assignedUsers ?? 0 },
      { key: 'actions', label: 'Actions', render: (role) => manage && !role.system && <TableActions label={`Actions for ${role.name}`}><DropdownMenuItem onSelect={() => setEditing(role)}>Edit</DropdownMenuItem><DropdownMenuSeparator /><DangerousAction label="Delete" warning={`Review ${role.assignedUsers ?? 0} affected users before deleting this role.`} onConfirm={() => remove(role)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></TableActions> },
    ]} /><Pagination page={page} pages={Math.max(1, Math.ceil(filteredRoles.length / limit))} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></div> : <EmptyState title="No matching roles">Try another role name, key, or permission.</EmptyState>}</> }}</QueryState>
  </main>
}

export function SsoSettingsPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'sso.manage')
  const query = useQuery({ queryKey: ['admin-sso'], queryFn: ({ signal }) => api.get<OidcProvider[]>('/api/v1/admin/auth/providers', undefined, signal) })
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
  const rolesQuery = useQuery({ queryKey: ['admin-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal) })
  const [draft, setDraft] = useState({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '' })
  const [groupMappings, setGroupMappings] = useState<GroupRoleMapping[]>([{ group: '', role: '' }])
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useUnsavedChanges(Object.values(draft).some(Boolean))
  const resetProvider = () => { setDraft({ key: '', name: '', issuer: '', clientId: '', secretReference: '', claimMapping: '' }); setGroupMappings([{ group: '', role: '' }]); setError('') }
  const closeProvider = () => { setCreating(false); resetProvider() }
  const addProvider = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const parseMapping = (value: string) => Object.fromEntries(value.split(',').map((entry) => entry.trim().split('=').map((part) => part.trim())).filter(([key, value]) => key && value))
      await api.post('/api/v1/admin/auth/providers', { key: draft.key.trim(), name: draft.name.trim(), issuer: draft.issuer.trim(), client_id: draft.clientId.trim(), secret_reference: draft.secretReference.trim(), claim_mapping: parseMapping(draft.claimMapping), group_mapping: roleMappingsValue(groupMappings), enabled: true })
      resetProvider(); setCreating(false); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Provider update failed') } finally { setBusy(false) }
  }
  const toggle = async (provider: OidcProvider) => { await api.post('/api/v1/admin/auth/providers', { ...provider, enabled: provider.enabled === false }); await query.refetch() }
  return <IdentityAdminLayout view="sso" title="Single sign-on" description="Configure generic OIDC providers, claim mappings, and group-to-role mappings. Resolved secrets are never shown." action={manage && <Button onClick={() => setCreating(true)}>Add provider</Button>}>
    {manage && <Dialog open={creating} title="Add provider" onClose={closeProvider}><form className="gf-editor-form" onSubmit={addProvider}><div className="gf-form-grid"><label htmlFor="sso-key">Key<Input id="sso-key" value={draft.key} onChange={(event) => setDraft({ ...draft, key: event.target.value })} required /></label><label htmlFor="sso-name">Name<Input id="sso-name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label><label htmlFor="sso-issuer">Issuer URL<Input id="sso-issuer" type="url" value={draft.issuer} onChange={(event) => setDraft({ ...draft, issuer: event.target.value })} required /></label><label htmlFor="sso-client">Client ID<Input id="sso-client" value={draft.clientId} onChange={(event) => setDraft({ ...draft, clientId: event.target.value })} /></label><label htmlFor="sso-secret">Secret reference<Input id="sso-secret" value={draft.secretReference} onChange={(event) => setDraft({ ...draft, secretReference: event.target.value })} placeholder="vault://oidc/client-secret" /><small>Reference only; the secret value is never rendered.</small></label><label htmlFor="sso-claims">Claim mapping<Input id="sso-claims" value={draft.claimMapping} onChange={(event) => setDraft({ ...draft, claimMapping: event.target.value })} placeholder="email=email,username=preferred_username" /></label></div><fieldset><legend>Group roles</legend>{groupMappings.map((mapping, index) => <div className="gf-form-grid" key={index}><label htmlFor={`sso-group-${index}`}>Group name<Input id={`sso-group-${index}`} value={mapping.group} onChange={(event) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, group: event.target.value } : item))} /></label><label htmlFor={`sso-group-role-${index}`}>Role<RoleSelect id={`sso-group-role-${index}`} value={mapping.role} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={(role) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, role } : item))} /></label></div>)}<Button type="button" variant="secondary" onClick={() => setGroupMappings((current) => [...current, { group: '', role: '' }])}>Add group</Button>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}</fieldset>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={closeProvider}>Cancel</Button><Button type="submit" busy={busy}>Add provider</Button></div></form></Dialog>}
    <QueryState query={query} empty="No SSO providers are configured.">{(providers) => providers.length ? <><DataTable caption="SSO providers" rows={providers.slice((page - 1) * limit, page * limit)} columns={[{ key: 'key', label: 'Provider', render: (provider) => <strong>{provider.name ?? provider.key}</strong> }, { key: 'issuer', label: 'Issuer', render: (provider) => <span>{provider.issuer}</span> }, { key: 'enabled', label: 'State', render: (provider) => <StatusPill status={provider.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'secretReference', label: 'Secret', render: (provider) => <span className="gf-secret-reference">{provider.secretReference ?? 'Configured by deployment'}</span> }, { key: 'actions', label: 'Actions', render: (provider) => manage && <TableActions label={`Actions for ${provider.name ?? provider.key}`}><DangerousAction label={provider.enabled === false ? 'Enable' : 'Disable'} warning="Disabling a provider can remove a login method. Confirm another administrator login method is available." onConfirm={() => toggle(provider)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>{provider.enabled === false ? 'Enable' : 'Disable'}</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={page} pages={Math.max(1, Math.ceil(providers.length / limit))} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No providers">Add an OIDC provider to enable single sign-on.</EmptyState>}</QueryState>
  </IdentityAdminLayout>
}

function AuthenticationTab() {
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
  return <section className="gf-card-panel"><div className="gf-editor-form"><label><input type="checkbox" checked={passwordLogin} onChange={(event) => setPasswordLogin(event.target.checked)} /> Enable password login</label><label><input type="checkbox" checked={registration} onChange={(event) => setRegistration(event.target.checked)} /> Allow password registration</label><label htmlFor="default-role">Default role<RoleSelect id="default-role" value={defaultRole} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={setDefaultRole} /></label>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}{error && <p className="gf-form-error" role="alert">{error}</p>}{manage && <DangerousAction label="Save settings" warning="Changing login methods can lock out administrators. Verify that another working login method remains before saving." onConfirm={save} />}</div></section>
}

function GeneralSettingsTab() {
  const { config, permissions, setConfig } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const [lockdown, setLockdown] = useState(config.lockdownScheduler === true)
  const [saved, setSaved] = useState(config.lockdownScheduler === true)
  const [error, setError] = useState('')
  useUnsavedChanges(lockdown !== saved)
  const save = async () => {
    setError('')
    try {
      await api.post('/api/v1/admin/auth/settings', { lockdown_scheduler: lockdown })
      setSaved(lockdown)
      setConfig({ ...config, lockdownScheduler: lockdown })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'General settings update failed')
    }
  }
  return <section className="gf-card-panel"><div className="gf-editor-form"><label><input type="checkbox" checked={lockdown} onChange={(event) => setLockdown(event.target.checked)} /> Lockdown Scheduler</label><p className="gf-muted">Blocks POST, PUT, and DELETE actions while enabled. Authentication and this setting remain available so an administrator can restore writes.</p>{error && <p className="gf-form-error" role="alert">{error}</p>}{manage && <Button onClick={() => void save()} disabled={lockdown === saved}>Save settings</Button>}</div></section>
}

export function AuthenticationSettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const view = searchParams.get('tab') === 'general' ? 'general' : 'authentication'
  const changeView = (next: string) => setSearchParams(next === 'general' ? { tab: 'general' } : {})
  return <main className="gf-content"><PageHeader title="General Settings" description="Manage scheduler authentication and general controls." /><Tabs value={view} onValueChange={changeView}><TabsList aria-label="General settings"><TabsTrigger value="authentication">Authentication</TabsTrigger><TabsTrigger value="general">General</TabsTrigger></TabsList></Tabs>{view === 'general' ? <GeneralSettingsTab /> : <AuthenticationTab />}</main>
}

export function ExecutionStatusPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const query = useQuery({ queryKey: ['execution-status'], queryFn: ({ signal }) => api.get<ExitCode[]>('/api/v1/admin/execution-status', undefined, signal) })
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10)
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
    <QueryState query={query} empty="No exit code meanings are configured.">{(items) => items.length ? <><DataTable caption="Execution status" rows={items.slice((page - 1) * limit, page * limit).map((item) => ({ ...item, id: item.code }))} columns={[{ key: 'code', label: 'Exit Code' }, { key: 'meaning', label: 'Meaning' }, { key: 'isSystem', label: 'Type', render: (item) => <StatusPill status={item.isSystem ? 'system' : 'custom'} /> }, { key: 'actions', label: 'Actions', render: (item) => !item.isSystem && manage && <TableActions label={`Actions for exit code ${item.code}`}><DropdownMenuItem onSelect={() => edit(item)}>Edit</DropdownMenuItem><DropdownMenuSeparator /><DangerousAction label="Delete" onConfirm={() => remove(item)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={page} pages={Math.max(1, Math.ceil(items.length / limit))} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No execution statuses">Create an exit code meaning.</EmptyState>}</QueryState>{error && !formOpen && <p className="gf-form-error" role="alert">{error}</p>}</main>
}
