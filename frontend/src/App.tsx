import { StartupPage } from './feedback'
import { AuthProvider, useAuth } from './auth'
import { FatalErrorPage, LoginRequiredPage } from './feedback'
import { AppRoutes } from './routes'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { LoginPage, OidcCallbackPage, RegistrationPage } from './auth-pages'

function App() {
  return <AuthProvider><BrowserRouter><BootstrapGate /></BrowserRouter></AuthProvider>
}

function BootstrapGate() {
  const auth = useAuth()
  if (auth.loading) return <StartupPage status="Restoring server session…" />
  if (auth.error) return <FatalErrorPage message={auth.error.message} onRetry={auth.restore} />
  if (!auth.profile) return <Routes><Route path="/register" element={<RegistrationPage />} /><Route path="/auth/oidc/callback" element={<OidcCallbackPage />} /><Route path="*" element={<LoginPage />} /></Routes>
  return <AppRoutes />
}

export default App
