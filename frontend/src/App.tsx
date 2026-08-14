import { StartupPage } from './feedback'
import { AuthProvider, useAuth } from './auth'
import { FatalErrorPage, LoginRequiredPage } from './feedback'
import { AppRoutes } from './routes'

function App() {
  return <AuthProvider><BootstrapGate /></AuthProvider>
}

function BootstrapGate() {
  const auth = useAuth()
  if (auth.loading) return <StartupPage status="Restoring server session…" />
  if (auth.error) return <FatalErrorPage message={auth.error.message} onRetry={auth.restore} />
  if (!auth.profile) return <LoginRequiredPage onLogin={() => undefined} />
  return <AppRoutes />
}

export default App
