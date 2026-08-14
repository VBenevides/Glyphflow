import type { ReactNode } from 'react'

const secretKey = /(secret|token|password|credential|private.?key)/i

export function safeOutput(value: string, maxChars = 200_000): string {
  return value.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '�').slice(0, maxChars)
}

export function SafeText({ value }: { value: string }) {
  return <span>{safeOutput(value)}</span>
}

export function LogOutput({ stream, value }: { stream: 'stdout' | 'stderr'; value: string }) {
  return <pre className={`gf-log gf-log-${stream}`} aria-label={`${stream} output`}><SafeText value={value} /></pre>
}

export function SecretReference({ value }: { value: string }) {
  return <span className="gf-secret-reference" title="The secret value is never displayed">Secret reference: {value}</span>
}

export function EnvironmentValue({ name, value, secret = false }: { name: string; value: string; secret?: boolean }) {
  return <div className="gf-environment-field"><strong>{name}</strong>{secret ? <SecretReference value={value} /> : <SafeText value={value} />}</div>
}

export function redactAuditValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redactAuditValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, secretKey.test(key) ? '[REDACTED]' : redactAuditValue(entry)]))
}

export function AuditValue({ value }: { value: unknown }) {
  return <pre className="gf-audit-value"><SafeText value={JSON.stringify(redactAuditValue(value), null, 2) ?? '—'} /></pre>
}

export function SafeLink({ href, children }: { href: string; children: ReactNode }) {
  const safe = href.startsWith('/') && !href.startsWith('//')
  return safe ? <a href={href}>{children}</a> : <span>{children}</span>
}
