import { StartupPage } from './feedback'
import { AuthProvider, useAuth } from './auth'
import { FatalErrorPage, LoginRequiredPage } from './feedback'
import { AppRoutes } from './routes'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { LoginPage, OidcCallbackPage, RegistrationPage } from './auth-pages'

function App() {
  return <AuthProvider><BrowserRouter><BootstrapGate /></BrowserRouter></AuthProvider>
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
  const isPublic = location.pathname === '/login' || location.pathname === '/register' || location.pathname === '/auth/oidc/callback'
  return <Routes><Route path="/login" element={<LoginPage />} /><Route path="/register" element={<RegistrationPage />} /><Route path="/auth/oidc/callback" element={<OidcCallbackPage />} />{!isPublic && <Route path="*" element={<Navigate to={`/login?redirect=${encodeURIComponent(requested)}`} replace />} />}</Routes>
}

export default App
