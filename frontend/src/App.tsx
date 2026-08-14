import { StartupPage } from './feedback'
import { AuthProvider, useAuth } from './auth'
import { FatalErrorPage, LoginRequiredPage } from './feedback'

function App() {
  return <AuthProvider><BootstrapGate /></AuthProvider>
}

function BootstrapGate() {
  const auth = useAuth()
  if (auth.loading) return <StartupPage status="Restoring server session…" />
  if (auth.error) return <FatalErrorPage message={auth.error.message} onRetry={auth.restore} />
  if (!auth.profile) return <LoginRequiredPage onLogin={() => undefined} />
  return <main className="gf-startup"><h1>Welcome, {auth.profile.displayName ?? auth.profile.username}</h1><p>Control plane session restored.</p></main>
}

export default App
