import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type UserRecord } from './api'
import { PageHeader, StatusPill } from './components'
import { QueryState } from './query'

export function UserDetailsPage({ userId, self }: { userId: string; self: boolean }) {
  const query = useQuery({ queryKey: ['admin-user', userId], queryFn: ({ signal }) => api.get<UserRecord>(`/api/v1/users/${encodeURIComponent(userId)}`, undefined, signal) })
  return <main className="gf-content"><PageHeader title="User details" description={self ? 'Your account details are read-only here.' : 'Review identity, access, and session details.'} action={<Link className="gf-button gf-button-secondary" to="/admin/users">Back to users</Link>} /><QueryState query={query}>{(user) => <>
    <section className="gf-card-panel"><h2>{user.displayName ?? user.email ?? user.username}</h2><p>{user.email ?? user.username}</p><StatusPill status={user.status ?? 'active'} />{user.systemAdmin && <p className="gf-muted">System administrator; access cannot be removed.</p>}</section>
    <section className="gf-card-panel"><h2>Access</h2><p><strong>Roles:</strong> {user.roles?.join(', ') || '—'}</p><p><strong>Role sources:</strong> {user.roleSources?.join(', ') || '—'}</p><p><strong>Permissions:</strong> {user.permissions?.join(', ') || '—'}</p><p><strong>Login methods:</strong> {user.loginMethods?.join(', ') || '—'}</p></section>
    <section className="gf-card-panel"><h2>Sessions</h2>{user.sessions?.length ? <ul className="gf-dashboard-list">{user.sessions.map((session) => <li key={session.id}><span>{session.id}<br /><small>{session.expiresAt ?? 'Expiry unavailable'}</small></span></li>)}</ul> : <p className="gf-muted">No active sessions.</p>}</section>
    <section className="gf-card-panel"><h2>Identities</h2>{user.identities?.length ? <ul className="gf-dashboard-list">{user.identities.map((identity) => <li key={identity.id}>{identity.provider}{identity.email ? ` · ${identity.email}` : ''}</li>)}</ul> : <p className="gf-muted">No linked identities.</p>}</section>
    {self && <p className="gf-muted">Account actions are unavailable in this view.</p>}
  </>}</QueryState></main>
}
