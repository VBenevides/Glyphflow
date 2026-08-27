import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useAuth } from './auth'
import { api, type UserRecord } from './api'
import { DangerousAction } from './actions'
import { Identifier, PageHeader, StatusPill } from './components'
import { QueryRefresh, QueryState } from './query'
import { formatDateTime } from './format'
import { hasPermission } from './permissions'
import { sessionDeviceLabel } from './session-device'

export function UserDetailsPage({ userId, self }: { userId: string; self: boolean }) {
  const { permissions } = useAuth()
  const manage = hasPermission(permissions, 'users.manage')
  const query = useQuery({ queryKey: ['admin-user', userId], queryFn: ({ signal }) => api.get<UserRecord>(`/api/v1/users/${encodeURIComponent(userId)}`, undefined, signal) })
  const revoke = async (sessionId: string) => { await api.post(`/api/v1/admin/auth/sessions/revoke?session_id=${encodeURIComponent(sessionId)}`); await query.refetch() }
  const revokeAll = async () => { await api.post(`/api/v1/admin/auth/users/${encodeURIComponent(userId)}/sessions/revoke-all`); await query.refetch() }
  return <main className="gf-content"><PageHeader title="User details" description={self ? 'Your account details are read-only here.' : 'Review identity, access, and session details.'} action={<Link className="gf-button gf-button-secondary" title="Return to the users list" to="/admin/users">Back to users</Link>} refresh={<QueryRefresh query={query} />} /><QueryState query={query}>{(user) => <>
    <section className="gf-card-panel"><h2>{user.displayName ?? user.email ?? user.username}</h2><p>{user.email ?? user.username}</p><StatusPill status={user.status ?? 'active'} />{user.systemAdmin && <p className="gf-muted">System administrator; access cannot be removed.</p>}</section>
    <section className="gf-card-panel"><h2>Access</h2><p><strong>Roles:</strong> {user.roles?.join(', ') || '—'}</p><p><strong>Role sources:</strong> {user.roleSources?.join(', ') || '—'}</p><p><strong>Permissions:</strong> {user.permissions?.join(', ') || '—'}</p><p><strong>Login methods:</strong> {user.loginMethods?.join(', ') || '—'}</p></section>
    <section className="gf-card-panel"><div className="gf-section-heading"><h2>Sessions</h2>{manage && user.sessions?.length ? <DangerousAction label="Revoke All" confirmLabel="Revoke All" warning="This will immediately invalidate every active session for this user." onConfirm={revokeAll} /> : null}</div>{user.sessions?.length ? <ul className="gf-dashboard-list">{user.sessions.map((session) => <li key={session.id}><span><Identifier id={session.id} copyLabel="Copy session ID" /><br /><small>Device: {sessionDeviceLabel(session.userAgent, session.ipAddress)}<br />Last interaction: {session.lastSeenAt ? formatDateTime(session.lastSeenAt) : 'Unavailable'}<br />Expires: {session.expiresAt ? formatDateTime(session.expiresAt) : 'Unavailable'}</small></span>{manage && <DangerousAction label="Revoke" warning="This will immediately invalidate the session." onConfirm={() => revoke(session.id)} />}</li>)}</ul> : <p className="gf-muted">No active sessions.</p>}</section>
    <section className="gf-card-panel"><h2>Identities</h2>{user.identities?.length ? <ul className="gf-dashboard-list">{user.identities.map((identity) => <li key={identity.id}>{identity.provider}{identity.email ? ` · ${identity.email}` : ''}</li>)}</ul> : <p className="gf-muted">No linked identities.</p>}</section>
  </>}</QueryState></main>
}
