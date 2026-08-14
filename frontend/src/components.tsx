import { Children, forwardRef, useEffect, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react'

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  busy?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button({ variant = 'primary', busy = false, disabled, children, className, title, ...props }, ref) {
  const tooltip = title ?? props['aria-label'] ?? (Children.toArray(children).filter((child) => typeof child === 'string' || typeof child === 'number').join(' ').trim() || undefined)
  return (
    <button ref={ref} title={tooltip} className={`gf-button gf-button-${variant}${className ? ` ${className}` : ''}`} disabled={disabled || busy} {...props}>
      {busy ? 'Working…' : children}
    </button>
  )
})

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(props, ref) {
  return <input ref={ref} className="gf-input" {...props} />
})

export function Dialog({ open, title, children, onClose, className }: { open: boolean; title: string; children: ReactNode; onClose: () => void; className?: string }) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  useEffect(() => {
    if (!open) return
    const previous = document.activeElement as HTMLElement | null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const dialog = dialogRef.current
    const focusable = dialog?.querySelector<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')
    focusable?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCloseRef.current()
      if (event.key !== 'Tab' || !dialog) return
      const items = [...dialog.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')]
      if (!items.length) return
      const first = items[0]
      const last = items[items.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previousOverflow
      previous?.focus()
    }
  }, [open])
  if (!open) return null
  return (
    <div className="gf-dialog-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div ref={dialogRef} className={`gf-dialog${className ? ` ${className}` : ''}`} role="dialog" aria-modal="true" aria-labelledby="gf-dialog-title">
        <div className="gf-dialog-header">
          <h2 id="gf-dialog-title">{title}</h2>
          <Button variant="ghost" aria-label="Close dialog" onClick={onClose}>×</Button>
        </div>
        {children}
      </div>
    </div>
  )
}

export function StatusPill({ status }: { status: string }) {
  const normalized = status.toLowerCase().replace(/[^a-z0-9]+/g, '-')
  return <span className={`gf-status gf-status-${normalized}`}>{status}</span>
}

export function PageHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return <header className="gf-page-header"><div><h1>{title}</h1>{description && <p>{description}</p>}</div>{action}</header>
}

export function MetricCard({ label, value, detail }: { label: string; value: ReactNode; detail?: string }) {
  return <section className="gf-metric"><span>{label}</span><strong>{value}</strong>{detail && <small>{detail}</small>}</section>
}

export type Column<T> = { key: string; label: string; render?: (row: T) => ReactNode }

export function DataTable<T extends { id?: string | number }>({ columns, rows, caption }: { columns: Column<T>[]; rows: T[]; caption: string }) {
  return (
    <div className="gf-table-wrap">
      <table className="gf-table">
        <caption className="gf-visually-hidden">{caption}</caption>
        <thead><tr>{columns.map((column) => <th key={column.key} scope="col">{column.label}</th>)}</tr></thead>
        <tbody>{rows.map((row, index) => <tr key={row.id ?? index}>{columns.map((column) => <td key={column.key}>{column.render ? column.render(row) : String((row as Record<string, unknown>)[column.key] ?? '—')}</td>)}</tr>)}</tbody>
      </table>
    </div>
  )
}

export function Pagination({ page, pages, onChange }: { page: number; pages: number; onChange: (page: number) => void }) {
  return <nav className="gf-pagination" aria-label="Pagination"><Button variant="secondary" disabled={page <= 1} onClick={() => onChange(page - 1)}>Previous</Button><span>Page {page} of {Math.max(1, pages)}</span><Button variant="secondary" disabled={page >= pages} onClick={() => onChange(page + 1)}>Next</Button></nav>
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return <section className="gf-state gf-empty"><h2>{title}</h2>{children && <p>{children}</p>}</section>
}

export function ErrorState({ title = 'Something went wrong', message, onRetry }: { title?: string; message?: string; onRetry?: () => void }) {
  return <section className="gf-state gf-error" role="alert"><h2>{title}</h2>{message && <p>{message}</p>}{onRetry && <Button variant="secondary" onClick={onRetry}>Try again</Button>}</section>
}

export function LoadingState({ label = 'Loading' }: { label?: string }) {
  return <section className="gf-state gf-loading" role="status" aria-live="polite"><span className="gf-spinner" aria-hidden="true" />{label}…</section>
}
