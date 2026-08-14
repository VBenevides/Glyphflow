import { AlertTriangle, LoaderCircle, SearchX, ShieldX } from 'lucide-react'
import { Button } from './components'
import glyphflowIcon from '../../assets/glyphflow.png'

export function BrandMark() {
  return <div className="gf-brand-mark" aria-hidden="true"><img src={glyphflowIcon} alt="" /></div>
}

export function StartupPage({ status = 'Starting control plane…' }: { status?: string }) {
  return <main className="gf-startup" aria-busy="true"><BrandMark /><p className="gf-brand-name">Glyphflow</p><LoaderCircle className="gf-startup-spinner" aria-hidden="true" /><p role="status">{status}</p></main>
}

export function FatalErrorPage({ title = 'Glyphflow cannot start', message, onRetry }: { title?: string; message: string; onRetry?: () => void }) {
  return <main className="gf-startup" role="alert"><AlertTriangle size={36} aria-hidden="true" /><h1>{title}</h1><p>{message}</p>{onRetry && <Button onClick={onRetry}>Retry</Button>}</main>
}

export function LoginRequiredPage({ onLogin }: { onLogin: () => void }) {
  return <main className="gf-startup"><ShieldX size={36} aria-hidden="true" /><h1>Sign in required</h1><p>Your session is not active.</p><Button onClick={onLogin}>Go to sign in</Button></main>
}

export function ForbiddenPage() {
  return <main className="gf-startup" role="alert"><ShieldX size={36} aria-hidden="true" /><h1>Access denied</h1><p>You do not have permission to view this page.</p></main>
}

export function NotFoundPage() {
  return <main className="gf-startup"><SearchX size={36} aria-hidden="true" /><h1>Page not found</h1><p>The requested route does not exist.</p></main>
}
