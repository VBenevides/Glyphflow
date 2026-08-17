import { StartupPage } from './feedback'
import { AuthProvider, useAuth } from './auth'
import { FatalErrorPage } from './feedback'
import { AppRoutes } from './routes'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { LoginPage, OidcCallbackPage, RegistrationPage } from './auth-pages'
import { QueryProvider } from './query'

export const PUBLIC_AUTH_PATHS = ['/login', '/register', '/auth/oidc/callback'] as const

export function isPublicAuthPath(path: string): boolean {
  return PUBLIC_AUTH_PATHS.includes(path as (typeof PUBLIC_AUTH_PATHS)[number])
}

function App() {
  return <QueryProvider><AuthProvider><BrowserRouter><BootstrapGate /></BrowserRouter></AuthProvider></QueryProvider>
}

function BootstrapGate() {
  const auth = useAuth()
  if (auth.loading) return <StartupPage status="Restoring server session…" />
  if (auth.error) return <FatalErrorPage message={auth.error.message} onRetry={auth.restore} />
  if (!auth.profile) return <PublicAuthRoutes />
  return <AppRoutes />
}

function PublicAuthRoutes() {
  const location = useLocation()
  const requested = `${location.pathname}${location.search}${location.hash}`
  return <Routes><Route path="/login" element={<LoginPage />} /><Route path="/register" element={<RegistrationPage />} /><Route path="/auth/oidc/callback" element={<OidcCallbackPage />} />{!isPublicAuthPath(location.pathname) && <Route path="*" element={<Navigate to={`/login?redirect=${encodeURIComponent(requested)}`} replace />} />}</Routes>
}

export default App
