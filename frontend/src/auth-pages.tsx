import { useEffect, useState, type FormEvent } from 'react'
import { LogIn, ShieldCheck } from 'lucide-react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from './auth'
import { api, ApiError, type OidcProvider } from './api'
import { Button, Input, LoadingState, ErrorState } from './components'
import { BrandMark } from './feedback'

export function safeReturnPath(value: string | null | undefined, fallback = '/') {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return fallback
  try {
    const target = new URL(value, window.location.origin)
    return target.origin === window.location.origin ? `${target.pathname}${target.search}${target.hash}` : fallback
  } catch {
    return fallback
  }
}

function AuthFrame({ title, children }: { title: string; children: React.ReactNode }) {
  return <main className="gf-auth-page"><section className="gf-auth-card"><BrandMark /><p className="gf-brand-name">Glyphflow</p><h1>{title}</h1>{children}</section></main>
}

export function LoginPage() {
  const { config, restore } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const redirect = safeReturnPath(new URLSearchParams(location.search).get('redirect'))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [providers, setProviders] = useState<OidcProvider[]>([])
  const [providersError, setProvidersError] = useState('')
  useEffect(() => {
    if (!config.oidc) return
    api.get<OidcProvider[]>('/api/v1/auth/oidc/providers').then(setProviders).catch((cause) => setProvidersError(cause instanceof Error ? cause.message : 'Unable to load providers'))
  }, [config.oidc])
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.post('/api/v1/auth/login', { username, password })
      await restore()
      navigate(redirect, { replace: true })
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Unable to sign in')
    } finally {
      setBusy(false)
    }
  }
  return <AuthFrame title="Sign in"><form className="gf-form" onSubmit={submit}>
    {config.passwordLogin && <><label htmlFor="username">Username</label><Input id="username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /><label htmlFor="password">Password</label><Input id="password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /><Button type="submit" busy={busy}>Sign in</Button></>}
    {!config.passwordLogin && <p className="gf-muted">Password sign-in is disabled.</p>}
    {error && <p className="gf-form-error" role="alert">{error}</p>}
    {config.registration && config.passwordLogin && <Button type="button" variant="ghost" onClick={() => navigate(`/register?redirect=${encodeURIComponent(redirect)}`)}>Create an account</Button>}
    {config.oidc && <div className="gf-provider-list"><h2>Single sign-on</h2>{providersError && <p className="gf-form-error" role="alert">{providersError}</p>}{!providersError && !providers.length && <LoadingState label="Loading providers" />}{providers.map((provider) => <Button key={provider.key} type="button" variant="secondary" onClick={() => { window.location.assign(`/api/v1/auth/oidc/login?provider=${encodeURIComponent(provider.key)}&redirect_uri=${encodeURIComponent(`${window.location.origin}/auth/oidc/callback`)}`) }}><LogIn size={16} aria-hidden="true" /> Continue with {provider.name ?? provider.key}</Button>)}</div>}
  </form></AuthFrame>
}

export function RegistrationPage() {
  const { config, restore } = useAuth()
  const navigate = useNavigate()
  const redirect = safeReturnPath(new URLSearchParams(useLocation().search).get('redirect'))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  if (!config.passwordLogin || !config.registration) return <AuthFrame title="Registration unavailable"><p className="gf-muted">Registration is disabled.</p><Button onClick={() => navigate('/login')}>Back to sign in</Button></AuthFrame>
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try { await api.post('/api/v1/auth/register', { username, password }); await api.post('/api/v1/auth/login', { username, password }); await restore(); navigate(redirect, { replace: true }) } catch (cause) { setError(cause instanceof Error ? cause.message : 'Unable to register') } finally { setBusy(false) }
  }
  return <AuthFrame title="Create account"><form className="gf-form" onSubmit={submit}><label htmlFor="register-username">Username</label><Input id="register-username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /><label htmlFor="register-password">Password</label><Input id="register-password" type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} required />{error && <p className="gf-form-error" role="alert">{error}</p>}<Button type="submit" busy={busy}>Register</Button><Button type="button" variant="ghost" onClick={() => navigate('/login')}>Back to sign in</Button></form></AuthFrame>
}

export function OidcCallbackPage() {
  const { restore } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useState('')
  useEffect(() => {
    const query = new URLSearchParams(location.search)
    const redirect = safeReturnPath(query.get('redirect'))
    api.get('/api/v1/auth/oidc/callback', Object.fromEntries(query.entries())).then(async () => { await restore(); navigate(redirect, { replace: true }) }).catch((cause) => setError(cause instanceof Error ? cause.message : 'Single sign-on failed'))
  }, [location.search, navigate, restore])
  if (error) return <AuthFrame title="Single sign-on failed"><ErrorState message={error} onRetry={() => navigate('/login', { replace: true })} /></AuthFrame>
  return <AuthFrame title="Completing sign-in"><LoadingState label="Verifying provider response" /></AuthFrame>
}

export function AccountHint() {
  return <p className="gf-auth-hint"><ShieldCheck size={16} aria-hidden="true" /> Sessions use secure server cookies.</p>
}
