import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Eye, EyeOff, Monitor, MoreHorizontal, UserPlus, Users } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, type AdminSession, type ExitCode, type OidcProvider, type Page, type QueryValue, type RoleDefinition, type SecretMetadata, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, Dialog, DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuPortal, DropdownMenuSeparator, DropdownMenuTrigger, EmptyState, FilterInput, Identifier, Input, MetricCard, PageHeader, Pagination, StatusPill, TableActions, Tabs, TabsList, TabsTrigger } from './components'
import { QueryRefresh, QueryState } from './query'
import { hasPermission, permissionPrivilegeLevel, sortedPermissions } from './permissions'
import { useAuth } from './auth'
import { useUnsavedChanges } from './unsaved'
import { formatDateTime } from './format'
import { sessionDeviceLabel } from './session-device'

type GroupRoleMapping = { group: string; role: string }

export function roleMappingsValue(mappings: GroupRoleMapping[]): Record<string, string> {
  return Object.fromEntries(mappings.map(({ group, role }) => [group.trim(), role]).filter(([group, role]) => group && role))
}

function RoleSelect({ id, value, roles, onChange, disabled = false }: Readonly<{ id: string; value: string; roles?: RoleDefinition[]; onChange: (value: string) => void; disabled?: boolean }>) {
  return <select id={id} className="gf-input" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} required><option value="">Select a role</option>{roles?.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select>
}

function asPage(value: Page<UserRecord> | UserRecord[], page = 1, limit = 10): Page<UserRecord> {
  return Array.isArray(value) ? { items: value.slice((page - 1) * limit, page * limit), page, limit, total: value.length, pages: Math.max(1, Math.ceil(value.length / limit)) } : value
}

export function filterUsersPage(users: UserRecord[], page: number, limit: number, email: string, status: string, role: string): Page<UserRecord> {
  const emailNeedle = email.trim().toLowerCase()
  const roleNeedle = role.trim().toLowerCase()
  const filtered = users.filter((user) => (!emailNeedle || (user.email ?? user.username).toLowerCase().includes(emailNeedle)) && (!status || user.status === status) && (!roleNeedle || user.roles?.some((assigned) => assigned.toLowerCase() === roleNeedle)))
  const start = (page - 1) * limit
  return { items: filtered.slice(start, start + limit), page, limit, total: filtered.length, pages: Math.max(1, Math.ceil(filtered.length / limit)) }
}

export function userListQuery(page: number, limit: number, email: string, status: string, role = ''): Record<string, QueryValue> {
  return { page, limit, email: email.trim() || undefined, status: status || undefined, roles: role.trim() || undefined }
}

function InlinePills({ values, markElevated = false }: { values?: string[]; markElevated?: boolean }) {
  return values?.length ? <span className="gf-pill-list">{values.map((value) => <span className={`gf-inline-pill${markElevated && permissionPrivilegeLevel(value) === 'elevated' ? ' gf-permission-elevated' : ''}`} key={value}>{value}</span>)}</span> : '—'
}

export function filterAndSortRoles(roles: RoleDefinition[], search: string): RoleDefinition[] {
  const needle = search.trim().toLowerCase()
  return roles.filter((role) => !needle || [role.id, role.name, role.description ?? '', ...role.permissions].some((value) => value.toLowerCase().includes(needle))).sort((left, right) => Number(Boolean(right.system)) - Number(Boolean(left.system)) || left.name.localeCompare(right.name))
}

function UserActionsMenu({ user, manage, onAccess, onDisable, onApprove }: Readonly<{ user: UserRecord; manage: boolean; onAccess: () => void; onDisable: () => void; onApprove: () => void }>) {
  const userLabel = user.displayName ?? user.email ?? user.username
  let statusAction = 'Disable'
  if (user.status === 'pending') statusAction = 'Approve'
  else if (user.status === 'disabled') statusAction = 'Enable'
  const statusChange = user.status === 'active' ? onDisable : onApprove
  return <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" variant="ghost" aria-label={`Actions for ${userLabel}`}><MoreHorizontal size={18} /></Button></DropdownMenuTrigger><DropdownMenuPortal><DropdownMenuContent align="end">{manage && <DropdownMenuItem onSelect={onAccess}>Manage access</DropdownMenuItem>}{manage && !user.systemAdmin && <><DropdownMenuSeparator /><DangerousAction label={statusAction} title={`${statusAction} user`} onConfirm={statusChange} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>{statusAction}</DropdownMenuItem>} /></>}<DropdownMenuSeparator /><DropdownMenuItem asChild><Link to={`/admin/users/${encodeURIComponent(user.id)}`}>Details</Link></DropdownMenuItem></DropdownMenuContent></DropdownMenuPortal></DropdownMenu>
}

type IdentityView = 'users' | 'sessions' | 'sso' | 'secrets'

function IdentityAdminLayout({ view, title, description, refresh, children }: Readonly<{ view: IdentityView; title: string; description: string; refresh?: ReactNode; children: ReactNode }>) {
  const navigate = useNavigate()
  const paths: Record<IdentityView, string> = { users: '/admin/users', sessions: '/admin/users/sessions', sso: '/admin/sso', secrets: '/admin/secrets' }
  return <main className="gf-content"><PageHeader title={title} description={description} refresh={refresh} /><Tabs value={view} onValueChange={(next) => navigate(paths[next as IdentityView])}><TabsList aria-label="Identity administration"><TabsTrigger value="users">Users</TabsTrigger><TabsTrigger value="sessions">Sessions</TabsTrigger><TabsTrigger value="sso">SSO</TabsTrigger><TabsTrigger value="secrets">Secrets</TabsTrigger></TabsList></Tabs>{children}</main>
}

export function UserCreationForm({ onCreated }: Readonly<{ onCreated: (userID: string) => Promise<void> }>) {
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
  return <form className="gf-editor-form" onSubmit={submit}><div className="gf-form-grid"><label htmlFor="admin-user-email">Email<Input id="admin-user-email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label><label htmlFor="admin-user-password">Temporary password<Input id="admin-user-password" type="password" autoComplete="new-password" minLength={8} value={password} onChange={(event) => setPassword(event.target.value)} required /></label></div>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>Create user</Button></div></form>
}

export function UserAccessEditor({ user, roles, onChanged, onClose }: Readonly<{ user: UserRecord; roles?: RoleDefinition[]; onChanged: () => Promise<void>; onClose: () => void }>) {
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
  const [status, setStatus] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [creating, setCreating] = useState(false)
  const [accessUserID, setAccessUserID] = useState<string | null>(null)
  const query = useQuery({ queryKey: ['admin-users', page, limit, email, status, roleFilter], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', userListQuery(page, limit, email, status, roleFilter), signal).then((value) => asPage(value, page, limit)) })
  const optionsQuery = useQuery({ queryKey: ['admin-user-filter-options'], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', { all: true }, signal).then((value) => asPage(value)) })
  const pendingUsersQuery = useQuery({ queryKey: ['admin-pending-users'], queryFn: ({ signal }) => api.get<Page<UserRecord> | UserRecord[]>('/api/v1/users', { page: 1, limit: 1, status: 'pending' }, signal).then((value) => asPage(value, 1, 1)) })
  const rolesQuery = useQuery({ queryKey: ['admin-user-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal), enabled: manage })
  const filterUsers = optionsQuery.data?.items ?? query.data?.items ?? []
  const roleOptions = [...new Set([...filterUsers.flatMap((user) => user.roles ?? []), ...(rolesQuery.data ?? []).map((role) => role.name), ...(roleFilter ? [roleFilter] : [])])].sort((left, right) => left.localeCompare(right))
  const totalUsers = optionsQuery.data?.total
  const pendingUsers = pendingUsersQuery.data?.total ?? pendingUsersQuery.data?.items.length
  const registeredUsers = totalUsers !== undefined && pendingUsers !== undefined ? totalUsers - pendingUsers : '—'
  const refreshUsers = async () => { await Promise.all([query.refetch(), optionsQuery.refetch(), pendingUsersQuery.refetch()]) }
  const disable = async (user: UserRecord) => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/disable`); await refreshUsers() }
  const approve = async (user: UserRecord) => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(user.id)}/approve`); await refreshUsers() }
  const created = async (userID: string) => { setCreating(false); setAccessUserID(userID); await refreshUsers() }
  return <IdentityAdminLayout view={view} title="Users and sessions" description="Review identity methods, role sources, permissions, and active sessions." refresh={<QueryRefresh query={[query, optionsQuery, pendingUsersQuery]} />}>
    {creating && <Dialog open title="Create user" onClose={() => setCreating(false)}><UserCreationForm onCreated={created} /></Dialog>}
    <div className="gf-metric-grid gf-identity-metrics"><MetricCard label="Number of Registered Users" value={registeredUsers} detail="Active and disabled accounts" icon={Users} /><MetricCard label="Number of Pending Users" value={pendingUsers ?? '—'} detail="Awaiting administrator approval" icon={UserPlus} tone={pendingUsers ? 'warning' : 'default'} /></div>
    <div className="gf-filter-bar"><FilterInput label="Email" type="email" value={email} options={filterUsers.map((user) => user.email ?? user.username)} onChange={(value) => { setEmail(value); setPage(1) }} placeholder="Filter by email" /><label>Status<select className="gf-input" value={status} onChange={(event) => { setStatus(event.target.value); setPage(1) }}><option value="">All statuses</option><option value="active">Active</option><option value="pending">Pending</option><option value="disabled">Disabled</option></select></label><label>Roles<select className="gf-input" value={roleFilter} onChange={(event) => { setRoleFilter(event.target.value); setPage(1) }}><option value="">All roles</option>{roleOptions.map((role) => <option key={role} value={role}>{role}</option>)}</select></label></div>
    {manage && <div className="gf-table-toolbar"><Button onClick={() => setCreating((value) => !value)}>{creating ? 'Cancel' : 'Create user'}</Button></div>}
    <QueryState query={query} empty="No users are available.">{(raw) => {
      const data = asPage(raw)
      const serverIgnoredRole = roleFilter && data.items.some((user) => !user.roles?.some((assigned) => assigned.toLowerCase() === roleFilter.toLowerCase()))
      const visibleData = serverIgnoredRole ? filterUsersPage(optionsQuery.data?.items ?? data.items, page, limit, email, status, roleFilter) : data
      if (!visibleData.items.length) return <EmptyState title="No users">Create or provision a user before managing access.</EmptyState>
      const accessUser = visibleData.items.find((user) => user.id === accessUserID)
      return <><DataTable caption="Users" rows={visibleData.items} columns={[
        { key: 'email', label: 'User', render: (user) => <span><strong>{user.displayName ?? user.email ?? user.username}</strong><br /><small>{user.email ?? user.username}</small></span> },
        { key: 'status', label: 'Status', render: (user) => <StatusPill status={user.status ?? (user.enabled === false ? 'disabled' : 'active')} /> },
        { key: 'loginMethods', label: 'Login methods', render: (user) => <InlinePills values={user.loginMethods} /> },
        { key: 'roles', label: 'Roles', render: (user) => <InlinePills values={user.roles} /> },
        { key: 'actions', label: 'Actions', render: (user) => <UserActionsMenu user={user} manage={manage} onAccess={() => setAccessUserID(user.id)} onDisable={() => disable(user)} onApprove={() => approve(user)} /> },
      ]} />
      <Pagination page={visibleData.page ?? page} pages={visibleData.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} />
      {accessUser && <UserAccessEditor user={accessUser} roles={rolesQuery.data} onChanged={refreshUsers} onClose={() => setAccessUserID(null)} />}
    </> }}</QueryState>
  </IdentityAdminLayout>
}

export function SessionManagementPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const [page, setPage] = useState(1); const [limit, setLimit] = useState(10); const [email, setEmail] = useState('')
  const query = useQuery({ queryKey: ['admin-sessions', page, limit, email], queryFn: ({ signal }) => api.get<Page<AdminSession>>('/api/v1/admin/auth/sessions', { page, limit, email: email || undefined }, signal) })
  const optionsQuery = useQuery({ queryKey: ['admin-session-filter-options'], queryFn: ({ signal }) => api.get<Page<AdminSession>>('/api/v1/admin/auth/sessions', { all: true }, signal) })
  const revoke = async (session: AdminSession) => { await api.post(`/api/v1/admin/auth/sessions/revoke?session_id=${encodeURIComponent(session.id)}`); await query.refetch() }
  return <IdentityAdminLayout view="sessions" title="Sessions" description="Review active authentication sessions across users." refresh={<QueryRefresh query={query} />}>
    <div className="gf-metric-grid gf-identity-metrics"><MetricCard label="Number of Sessions" value={optionsQuery.data?.total ?? '—'} detail="Active authentication sessions" icon={Monitor} /></div>
    <div className="gf-filter-bar"><FilterInput label="User email" type="email" value={email} options={(optionsQuery.data?.items ?? query.data?.items ?? []).map((session) => session.userEmail)} onChange={(value) => { setEmail(value); setPage(1) }} placeholder="Filter by user email" /></div>
    <QueryState query={query} empty="No active sessions match this filter.">{(data) => data.items.length ? <><DataTable caption="Sessions" rows={data.items} columns={[{ key: 'userEmail', label: 'User', render: (session) => <Link to={`/admin/users/${encodeURIComponent(session.userId)}`}>{session.userEmail}</Link> }, { key: 'id', label: 'Session ID', render: (session) => <Identifier id={session.id} copyLabel="Copy session ID" /> }, { key: 'lastSeenAt', label: 'Last seen', render: (session) => session.lastSeenAt ? formatDateTime(session.lastSeenAt) : '—' }, { key: 'expiresAt', label: 'Expires', render: (session) => session.expiresAt ? formatDateTime(session.expiresAt) : '—' }, { key: 'userAgent', label: 'Device', render: (session) => sessionDeviceLabel(session.userAgent, session.ipAddress) }, { key: 'actions', label: 'Actions', render: (session) => manage && <TableActions label={`Actions for ${session.userEmail}`}><DangerousAction label="Revoke" warning="This will immediately invalidate the session." onConfirm={() => revoke(session)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Revoke</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={data.page} pages={data.pages ?? 1} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No active sessions">No active sessions match this filter.</EmptyState>}</QueryState>
  </IdentityAdminLayout>
}

function RoleEditor({ role, onDone }: Readonly<{ role?: RoleDefinition; onDone: () => void }>) {
  const [name, setName] = useState(role?.name ?? '')
  const [selected, setSelected] = useState(() => new Set(role?.permissions ?? []))
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const permissionOptions = sortedPermissions()
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
    <fieldset><legend>Permissions</legend><div className="gf-permission-grid">{permissionOptions.map(({ name, privilegeLevel }) => <label className={privilegeLevel === 'elevated' ? 'gf-permission-elevated' : undefined} key={name}><input type="checkbox" checked={selected.has(name)} onChange={(event) => setSelected((current) => { const next = new Set(current); event.target.checked ? next.add(name) : next.delete(name); return next })} /> {name}</label>)}</div></fieldset>
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
  return <main className="gf-content"><PageHeader title="Roles and permissions" description="Seeded roles are immutable. Custom roles select from the application permission catalog." refresh={<QueryRefresh query={query} />} />
    {editing !== undefined && <Dialog open title={editing ? `Edit ${editing.name}` : 'New custom role'} onClose={() => setEditing(undefined)}><RoleEditor role={editing ?? undefined} onDone={refresh} /></Dialog>}
    <QueryState query={query} empty="No roles are configured.">{(roles) => { const filteredRoles = filterAndSortRoles(roles, search); return <><div className="gf-filter-bar"><FilterInput label="Search" options={roles.flatMap((role) => [role.name, role.id, ...role.permissions])} value={search} onChange={(value) => { setSearch(value); setPage(1) }} placeholder="Role name, key, or permission" /></div>{manage && <div className="gf-table-toolbar"><Button onClick={() => setEditing(null)}>Create role</Button></div>}{filteredRoles.length ? <div className="gf-role-table"><DataTable caption="Roles" rows={filteredRoles.slice((page - 1) * limit, page * limit)} columns={[
      { key: 'name', label: 'Role', render: (role) => <strong>{role.name}</strong> },
      { key: 'system', label: 'Source', render: (role) => <StatusPill status={role.system ? 'system' : 'custom'} /> },
      { key: 'permissions', label: 'Permissions', render: (role) => <InlinePills values={role.permissions} markElevated /> },
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
  const [draft, setDraft] = useState({ key: '', name: '', issuer: '', clientId: '', clientSecret: '' })
  const [groupMappings, setGroupMappings] = useState<GroupRoleMapping[]>([{ group: '', role: '' }])
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useUnsavedChanges(Object.values(draft).some(Boolean))
  const resetProvider = () => { setDraft({ key: '', name: '', issuer: '', clientId: '', clientSecret: '' }); setGroupMappings([{ group: '', role: '' }]); setError('') }
  const closeProvider = () => { setCreating(false); resetProvider() }
  const addProvider = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      await api.post('/api/v1/admin/auth/providers', { key: draft.key.trim(), name: draft.name.trim(), issuer: draft.issuer.trim(), clientId: draft.clientId.trim(), clientSecret: draft.clientSecret, groupMapping: roleMappingsValue(groupMappings), enabled: true })
      resetProvider(); setCreating(false); await query.refetch()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Provider update failed') } finally { setBusy(false) }
  }
  const toggle = async (provider: OidcProvider) => { await api.post('/api/v1/admin/auth/providers', { ...provider, enabled: provider.enabled === false }); await query.refetch() }
  return <IdentityAdminLayout view="sso" title="Single sign-on" description="Configure generic OIDC providers and group-to-role mappings. Client secrets are encrypted locally and never shown." refresh={<QueryRefresh query={query} />}>
    {manage && <div className="gf-table-toolbar"><Button onClick={() => setCreating(true)}>Add provider</Button></div>}
    {manage && <Dialog open={creating} title="Add provider" onClose={closeProvider}><form className="gf-editor-form" onSubmit={addProvider}><div className="gf-form-grid"><label htmlFor="sso-key">Key<Input id="sso-key" value={draft.key} onChange={(event) => setDraft({ ...draft, key: event.target.value })} required /></label><label htmlFor="sso-name">Name<Input id="sso-name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} required /></label><label htmlFor="sso-issuer">Issuer URL<Input id="sso-issuer" type="url" value={draft.issuer} onChange={(event) => setDraft({ ...draft, issuer: event.target.value })} required /></label><label htmlFor="sso-client">Client ID<Input id="sso-client" value={draft.clientId} onChange={(event) => setDraft({ ...draft, clientId: event.target.value })} /></label><label htmlFor="sso-secret">Client secret<Input id="sso-secret" type="password" autoComplete="new-password" value={draft.clientSecret} onChange={(event) => setDraft({ ...draft, clientSecret: event.target.value })} required /><small>Stored encrypted locally and never shown again.</small></label></div><fieldset><legend>Group roles</legend>{groupMappings.map((mapping, index) => <div className="gf-form-grid" key={mapping.group || mapping.role || 'new-group'}><label htmlFor={`sso-group-${index}`}>Group name<Input id={`sso-group-${index}`} value={mapping.group} onChange={(event) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, group: event.target.value } : item))} /></label><label htmlFor={`sso-group-role-${index}`}>Role<RoleSelect id={`sso-group-role-${index}`} value={mapping.role} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={(role) => setGroupMappings((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, role } : item))} /></label></div>)}<Button type="button" variant="secondary" onClick={() => setGroupMappings((current) => current.some((mapping) => !mapping.group && !mapping.role) ? current : [...current, { group: '', role: '' }])}>Add group</Button>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}</fieldset>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="button" variant="secondary" onClick={closeProvider}>Cancel</Button><Button type="submit" busy={busy}>Add provider</Button></div></form></Dialog>}
    <QueryState query={query} empty="No SSO providers are configured.">{(providers) => providers.length ? <><DataTable caption="SSO providers" rows={providers.slice((page - 1) * limit, page * limit)} columns={[{ key: 'key', label: 'Provider', render: (provider) => <strong>{provider.name ?? provider.key}</strong> }, { key: 'issuer', label: 'Issuer', render: (provider) => <span>{provider.issuer}</span> }, { key: 'enabled', label: 'State', render: (provider) => <StatusPill status={provider.enabled === false ? 'disabled' : 'enabled'} /> }, { key: 'secret', label: 'Secret', render: () => <span className="gf-secret-reference">Stored encrypted locally</span> }, { key: 'actions', label: 'Actions', render: (provider) => manage && <TableActions label={`Actions for ${provider.name ?? provider.key}`}><DangerousAction label={provider.enabled === false ? 'Enable' : 'Disable'} warning="Disabling a provider can remove a login method. Confirm another administrator login method is available." onConfirm={() => toggle(provider)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>{provider.enabled === false ? 'Enable' : 'Disable'}</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={page} pages={Math.max(1, Math.ceil(providers.length / limit))} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No providers">Add an OIDC provider to enable single sign-on.</EmptyState>}</QueryState>
  </IdentityAdminLayout>
}

function secretStatusLabel(status: string) {
  return { UNKNOWN: 'Unknown', VALID: 'Valid', INTEGRITY_FAILED: 'Integrity failed', KEY_UNAVAILABLE: 'Key unavailable', DECRYPTION_FAILED: 'Decryption failed' }[status] ?? status
}

function secretTaskUsage(secret: SecretMetadata): ReactNode {
  if (secret.tasks.length) return <span>{secret.tasks.map((task, index) => <span key={task.id}>{index > 0 && ', '}<Link to={`/tasks/${encodeURIComponent(task.id)}`}>{task.name}</Link></span>)}</span>
  if (secret.canDelete) return <span className="gf-muted">No tasks</span>
  return <span className="gf-muted">SSO configuration</span>
}

function SecretEditor({ secret, onDone }: Readonly<{ secret?: SecretMetadata; onDone: () => Promise<void> }>) {
  const [name, setName] = useState(secret?.name ?? '')
  const [value, setValue] = useState('')
  const [valueVisible, setValueVisible] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const body = { name: name.trim(), secret_value: value }
      if (secret) await api.put(`/api/v1/admin/secrets/${encodeURIComponent(secret.id)}`, body)
      else await api.post('/api/v1/admin/secrets', body)
      await onDone()
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Secret update failed') } finally { setBusy(false) }
  }
  return <form className="gf-editor-form" onSubmit={submit}><label htmlFor="secret-name">Name<Input id="secret-name" value={name} onChange={(event) => setName(event.target.value)} required /></label><label htmlFor="secret-value">Secret value<div className="gf-password-field"><Input id="secret-value" type={valueVisible ? 'text' : 'password'} className="gf-password-input" autoComplete="new-password" value={value} onChange={(event) => setValue(event.target.value)} required /><button type="button" className="gf-password-toggle" aria-label={valueVisible ? 'Hide secret' : 'Show secret'} aria-pressed={valueVisible} onClick={() => setValueVisible((visible) => !visible)}>{valueVisible ? <EyeOff size={16} aria-hidden="true" /> : <Eye size={16} aria-hidden="true" />}</button></div><small>Stored encrypted and never shown again.</small></label>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>{secret ? 'Replace secret' : 'Create secret'}</Button><Button type="button" variant="ghost" onClick={() => void onDone()}>Cancel</Button></div></form>
}

export function SecretsPage() {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'secrets.manage')
  const query = useQuery({ queryKey: ['admin-secrets'], queryFn: ({ signal }) => api.get<SecretMetadata[]>('/api/v1/admin/secrets', undefined, signal) })
  const [editing, setEditing] = useState<SecretMetadata | null | undefined>(undefined)
  const done = async () => { setEditing(undefined); await query.refetch() }
  const deleteSecret = async (secret: SecretMetadata) => { await api.delete(`/api/v1/admin/secrets/${encodeURIComponent(secret.id)}`); await query.refetch() }
  return <IdentityAdminLayout view="secrets" title="Secrets" description="Manage named encrypted values. Integrity status is separate from whether an external credential is still accepted." refresh={<QueryRefresh query={query} />}>
    {manage && editing === undefined && <div className="gf-table-toolbar"><Button onClick={() => setEditing(null)}>Create secret</Button></div>}
    {editing !== undefined && <Dialog open title={editing ? `Replace ${editing.name}` : 'Create secret'} onClose={() => setEditing(undefined)}><SecretEditor secret={editing ?? undefined} onDone={done} /></Dialog>}
    <QueryState query={query} empty="No named secrets are configured.">{(secrets) => secrets.length ? <DataTable caption="Named secrets" rows={secrets} columns={[{ key: 'name', label: 'Secret', render: (secret) => <strong>{secret.name}</strong> }, { key: 'tasks', label: 'Used by tasks', render: (secret) => secretTaskUsage(secret) }, { key: 'status', label: 'Encryption status', render: (secret) => <StatusPill status={secretStatusLabel(secret.status)} /> }, { key: 'lastValidatedAt', label: 'Last validated', render: (secret) => secret.lastValidatedAt ? <time dateTime={secret.lastValidatedAt}>{formatDateTime(secret.lastValidatedAt)}</time> : 'Not yet validated' }, { key: 'actions', label: 'Actions', render: (secret) => manage && <TableActions label={`Actions for ${secret.name}`}><DropdownMenuItem onSelect={() => setEditing(secret)}>Replace value</DropdownMenuItem>{secret.canDelete && <><DropdownMenuSeparator /><DangerousAction label="Delete" title={`Delete ${secret.name}`} warning="Permanently removes this secret. Secrets used by a task or SSO configuration cannot be deleted." onConfirm={() => deleteSecret(secret)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></>}</TableActions> }]} /> : <EmptyState title="No named secrets">Create a named secret for use by a task.</EmptyState>}</QueryState>
  </IdentityAdminLayout>
}

function AuthenticationTab() {
  const { config, permissions, setConfig } = useAuth()
  const manage = hasPermission(permissions, 'auth.settings.manage')
  const rolesQuery = useQuery({ queryKey: ['admin-role-options'], queryFn: ({ signal }) => api.get<RoleDefinition[]>('/api/v1/admin/roles', undefined, signal) })
  const [passwordLogin, setPasswordLogin] = useState(config.passwordLogin)
  const [registration, setRegistration] = useState(config.registration)
  const [requireUserApproval, setRequireUserApproval] = useState(config.requireUserApproval !== false)
  const [defaultRoleId, setDefaultRoleId] = useState(config.defaultRoleId ?? '')
  const [saved, setSaved] = useState({ passwordLogin: config.passwordLogin, registration: config.registration, requireUserApproval: config.requireUserApproval !== false, defaultRoleId: config.defaultRoleId ?? '' })
  const [error, setError] = useState('')
  useUnsavedChanges(passwordLogin !== saved.passwordLogin || registration !== saved.registration || requireUserApproval !== saved.requireUserApproval || defaultRoleId !== saved.defaultRoleId)
  const save = async () => { setError(''); try { await api.post('/api/v1/admin/auth/settings', { enabled: passwordLogin, registration, require_user_approval: requireUserApproval, default_role_id: defaultRoleId }); setSaved({ passwordLogin, registration, requireUserApproval, defaultRoleId }); setConfig({ ...config, passwordLogin, registration, requireUserApproval, defaultRoleId }) } catch (cause) { setError(cause instanceof Error ? cause.message : 'Authentication settings update failed') } }
  const defaultRole = defaultRoleId
  const setDefaultRole = setDefaultRoleId
  return <section className="gf-card-panel"><div className="gf-editor-form"><label><input type="checkbox" checked={passwordLogin} onChange={(event) => setPasswordLogin(event.target.checked)} /> Enable password login</label><label><input type="checkbox" checked={registration} onChange={(event) => setRegistration(event.target.checked)} /> Allow password registration</label><label><input type="checkbox" checked={requireUserApproval} onChange={(event) => setRequireUserApproval(event.target.checked)} /> Require administrator approval for new users</label><p className="gf-muted">Pending users cannot sign in until an administrator approves them.</p><label htmlFor="default-role">Default role<RoleSelect id="default-role" value={defaultRole} roles={rolesQuery.data} disabled={rolesQuery.isPending || rolesQuery.isError} onChange={setDefaultRole} /></label>{rolesQuery.isError && <small className="gf-form-error">Roles could not be loaded.</small>}{error && <p className="gf-form-error" role="alert">{error}</p>}{manage && <DangerousAction label="Save settings" warning="Changing login methods can lock out administrators. Verify that another working login method remains available before saving." onConfirm={save} />}</div></section>
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
  return <main className="gf-content"><PageHeader title="Execution Status" description="Exit codes reported by completed task processes." refresh={<QueryRefresh query={query} />} />
    {manage && !formOpen && <div className="gf-table-toolbar"><Button onClick={create}>Create exit code</Button></div>}
    {formOpen && <section className="gf-card-panel"><form className="gf-editor-form" onSubmit={save}><div className="gf-form-grid"><label>Exit Code<Input type="number" step="1" value={draft.code} onChange={(event) => setDraft({ ...draft, code: event.target.value })} required /></label><label>Meaning<Input value={draft.meaning} onChange={(event) => setDraft({ ...draft, meaning: event.target.value })} required /></label></div>{error && <p className="gf-form-error" role="alert">{error}</p>}<div className="gf-dialog-actions"><Button type="submit" busy={busy}>{editing !== null ? 'Save exit code' : 'Create exit code'}</Button><Button type="button" variant="ghost" onClick={closeForm}>Cancel</Button></div></form></section>}
    <QueryState query={query} empty="No exit code meanings are configured.">{(items) => items.length ? <><DataTable caption="Execution status" rows={items.slice((page - 1) * limit, page * limit).map((item) => ({ ...item, id: item.code }))} columns={[{ key: 'code', label: 'Exit Code' }, { key: 'meaning', label: 'Meaning' }, { key: 'isSystem', label: 'Type', render: (item) => <StatusPill status={item.isSystem ? 'system' : 'custom'} /> }, { key: 'actions', label: 'Actions', render: (item) => !item.isSystem && manage && <TableActions label={`Actions for exit code ${item.code}`}><DropdownMenuItem onSelect={() => edit(item)}>Edit</DropdownMenuItem><DropdownMenuSeparator /><DangerousAction label="Delete" onConfirm={() => remove(item)} renderTrigger={(open) => <DropdownMenuItem onSelect={(event) => { event.preventDefault(); open() }}>Delete</DropdownMenuItem>} /></TableActions> }]} /><Pagination page={page} pages={Math.max(1, Math.ceil(items.length / limit))} limit={limit} onChange={setPage} onLimitChange={(next) => { setLimit(next); setPage(1) }} /></> : <EmptyState title="No execution statuses">Create an exit code meaning.</EmptyState>}</QueryState>{error && !formOpen && <p className="gf-form-error" role="alert">{error}</p>}</main>
}
