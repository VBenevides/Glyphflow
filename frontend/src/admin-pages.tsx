import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type AuthSession, type Page, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Button, DataTable, EmptyState, PageHeader, Pagination, StatusPill } from './components'
import { QueryState } from './query'
import { hasPermission } from './permissions'
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
