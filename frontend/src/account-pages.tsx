import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api, type AuthSession, type Profile } from './api'
import { DangerousAction } from './actions'
import { Button, Identifier, Input, PageHeader, StatusPill } from './components'
import { QueryRefresh, QueryState as DataState } from './query'
import { useAuth } from './auth'
import { useUnsavedChanges } from './unsaved'
import { formatDateTime } from './format'
import { sessionDeviceLabel } from './session-device'

export function accountDirty(displayName: string, baseline: string, password: { current: string; next: string; confirm: string }): boolean {
  return displayName !== baseline || Boolean(password.current || password.next || password.confirm)
}

export function sessionMetadata(session: AuthSession): Array<{ label: string; value: string }> {
  return [
    { label: 'Device', value: sessionDeviceLabel(session.userAgent, session.ipAddress) },
    { label: 'IP address', value: session.ipAddress ?? 'Unknown' },
    { label: 'Last seen', value: formatDateTime(session.lastSeenAt) },
    { label: 'Created', value: formatDateTime(session.createdAt) },
    { label: 'Expires', value: formatDateTime(session.expiresAt) },
  ]
}

export function AccountPage() {
  const { config, setProfile } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const query = useQuery({ queryKey: ['me-account'], queryFn: ({ signal }) => api.get<Profile>('/api/v1/me', undefined, signal) })
  const [displayName, setDisplayName] = useState('')
  const [displayNameBaseline, setDisplayNameBaseline] = useState('')
  const [profileError, setProfileError] = useState('')
  const [password, setPassword] = useState({ current: '', next: '', confirm: '' })
  const [passwordError, setPasswordError] = useState('')
  useEffect(() => { if (query.data) { const value = query.data.displayName ?? ''; setDisplayName(value); setDisplayNameBaseline(value) } }, [query.data])
  const saveProfile = async (event: FormEvent) => { event.preventDefault(); setProfileError(''); try { const saved = await api.put<Profile>('/api/v1/me', { display_name: displayName.trim() }); setProfile(saved); await query.refetch() } catch (cause) { setProfileError(cause instanceof Error ? cause.message : 'Profile update failed') } }
  const changePassword = async (event: FormEvent) => { event.preventDefault(); setPasswordError(''); if (password.next !== password.confirm) { setPasswordError('New passwords must match.'); return } try { await api.post('/api/v1/me/password', { current_password: password.current, new_password: password.next }); setPassword({ current: '', next: '', confirm: '' }) } catch (cause) { setPasswordError(cause instanceof Error ? cause.message : 'Password update failed') } }
  const revoke = async (sessionId: string, current: boolean) => { await api.post(`/api/v1/me/sessions/revoke?session_id=${encodeURIComponent(sessionId)}`); if (current) { setProfile(null); navigate('/login', { replace: true }) } else await query.refetch() }
  const identities = query.data?.identities ?? []
  const sessions = query.data?.sessions ?? []
  const section = location.pathname.split('/')[2] ?? 'profile'
  useUnsavedChanges(accountDirty(displayName, displayNameBaseline, password))
  const accountTabs = <nav className="gf-account-tabs" aria-label="Account sections"><Link className={section === 'profile' || section === 'account' ? 'is-active' : ''} to="/account">Profile</Link><Link className={section === 'password' ? 'is-active' : ''} to="/account/password">Password</Link><Link className={section === 'identities' ? 'is-active' : ''} to="/account/identities">Identities</Link><Link className={section === 'sessions' ? 'is-active' : ''} to="/account/sessions">Sessions</Link></nav>
  return <main className="gf-content"><PageHeader title="Account" description="Manage your profile, login methods, and active sessions." refresh={<QueryRefresh query={query} />} />
    <DataState query={query}>{(profile) => <>
      {accountTabs}
      {(section === 'profile' || section === 'account') && <section className="gf-card-panel"><h2>Profile</h2><form className="gf-editor-form" onSubmit={saveProfile}><label htmlFor="account-email">Email<Input id="account-email" type="text" inputMode="email" value={profile.email ?? profile.username} readOnly disabled /></label><label htmlFor="account-display-name">Display name<Input id="account-display-name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>{profileError && <p className="gf-form-error" role="alert">{profileError}</p>}{profile.email && <p className="gf-muted">Email is used as the login identifier.</p>}<Button type="submit">Save profile</Button></form></section>}
      {(section === 'password' || section === 'account') && config.passwordLogin && <section className="gf-card-panel"><h2>Change password</h2><form className="gf-editor-form" onSubmit={changePassword}><label htmlFor="current-password">Current password<Input id="current-password" type="password" autoComplete="current-password" value={password.current} onChange={(event) => setPassword({ ...password, current: event.target.value })} required /></label><label htmlFor="new-password">New password<Input id="new-password" type="password" autoComplete="new-password" minLength={8} value={password.next} onChange={(event) => setPassword({ ...password, next: event.target.value })} required /></label><label htmlFor="confirm-password">Confirm new password<Input id="confirm-password" type="password" autoComplete="new-password" value={password.confirm} onChange={(event) => setPassword({ ...password, confirm: event.target.value })} required /></label>{passwordError && <p className="gf-form-error" role="alert">{passwordError}</p>}<Button type="submit">Change password</Button></form></section>}
      {(section === 'identities' || section === 'account') && <section className="gf-card-panel"><h2>OIDC identities</h2>{!config.oidc && <p className="gf-muted">Single sign-on is disabled.</p>}{config.oidc && <><Button variant="secondary" onClick={() => window.location.assign(`/api/v1/auth/oidc/link?redirect=${encodeURIComponent(`${window.location.origin}/account/identities`)}`)}>Link OIDC identity</Button>{identities.length ? <ul className="gf-dashboard-list">{identities.map((identity) => <li key={identity.id}><span><strong>{identity.provider}</strong>{identity.email ? ` · ${identity.email}` : ''}</span>{(config.passwordLogin || identities.length > 1) && <DangerousAction label="Unlink" warning="Removing a login method can lock you out. Keep another working method first." onConfirm={() => api.delete(`/api/v1/me/identities/${encodeURIComponent(identity.id)}`)} />}</li>)}</ul> : <p className="gf-muted">No linked identities.</p>}</>}</section>}
      {(section === 'sessions' || section === 'account') && <section className="gf-card-panel"><h2>Owned sessions</h2>{sessions.length ? <ul className="gf-dashboard-list">{sessions.map((session) => <li key={session.id}><span className="gf-session-details"><strong>{session.current ? 'Current session' : 'Session'} <Identifier id={session.id} copyLabel="Copy session ID" /></strong>{sessionMetadata(session).map((item) => <small key={item.label}>{item.label}: {item.value}</small>)}<StatusPill status={session.current ? 'active' : 'session'} /></span><DangerousAction label={session.current ? 'Sign out' : 'Revoke'} warning={session.current ? 'Sign out of this browser?' : 'Revoke this session?' } onConfirm={() => revoke(session.id, Boolean(session.current))} /></li>)}</ul> : <p className="gf-muted">No active sessions were returned.</p>}</section>}
    </>}</DataState>
  </main>
}
