import * as TabsPrimitive from '@radix-ui/react-tabs'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu'
import { Copy, MoreHorizontal } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Children, forwardRef, useId, useState, type ButtonHTMLAttributes, type ComponentPropsWithoutRef, type ComponentType, type ElementRef, type InputHTMLAttributes, type ReactNode } from 'react'

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  busy?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button({ variant = 'primary', busy = false, disabled, children, className, title, ...props }, ref) {
  const tooltip = title ?? props['aria-label'] ?? (Children.toArray(children).filter((child) => typeof child === 'string' || typeof child === 'number').join(' ').trim() || undefined)
  return (
    <button ref={ref} {...props} title={tooltip} className={'gf-button gf-button-' + variant + (className ? ' ' + className : '')} disabled={disabled || busy} aria-busy={busy || undefined}>
      {busy ? 'Working…' : children}
    </button>
  )
})

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(props, ref) {
  const { className, ...inputProps } = props
  return <input ref={ref} {...inputProps} className={'gf-input' + (className ? ' ' + className : '')} />
})

export function matchingFilterOptions(options: readonly string[], value: string) {
  const needle = value.trim().toLowerCase()
  return [...new Set(options)].filter((option) => option && (!needle || option.toLowerCase().includes(needle)))
}

export function compactIdentifier(id: string) {
  return id.length > 10 ? `${id.slice(0, 5)}…${id.slice(-5)}` : id
}

export function FilterInput({ label, options = [], value, onChange, id, ...props }: { label: string; options?: readonly string[]; value: string; onChange: (value: string) => void; id?: string } & Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  const generatedID = useId()
  const inputID = id ?? generatedID
  const [open, setOpen] = useState(false)
  const matches = matchingFilterOptions(options, value)
  return <div className="gf-filter-field"><label htmlFor={inputID}>{label}</label><div className="gf-filter-input"><Input {...props} id={inputID} value={value} autoComplete="off" onFocus={() => setOpen(true)} onChange={(event) => { onChange(event.target.value); setOpen(true) }} onBlur={() => window.setTimeout(() => setOpen(false), 100)} aria-autocomplete="list" aria-expanded={open} aria-controls={open ? `${inputID}-options` : undefined} />{open && <div id={`${inputID}-options`} className="gf-task-options" role="listbox">{matches.length ? matches.map((option) => <button type="button" role="option" aria-selected={option === value} className="gf-task-option" key={option} onMouseDown={(event) => { event.preventDefault(); onChange(option); setOpen(false) }}>{option}</button>) : <span className="gf-task-empty">No matching values</span>}</div>}</div></div>
}

export function InfoTooltip({ text }: Readonly<{ text: string }>) {
  return <span className="gf-info-tooltip"><button type="button" className="gf-info-tooltip-trigger" aria-label="More information" title={text}>i</button></span>
}

export function Identifier({ id, name, href, className, linkClassName, copyLabel = 'Copy identifier' }: Readonly<{ id?: string; name?: string; href?: string; className?: string; linkClassName?: string; copyLabel?: string }>) {
  if (!id) return <span>—</span>
  const compact = compactIdentifier(id)
  const label = name ? `${name} · ${compact}` : compact
  const content = href ? <Link className={linkClassName} to={href} title={id}>{label}</Link> : <span title={id}>{label}</span>
  return <span className={'gf-identifier' + (className ? ' ' + className : '')}>{content}<Button variant="ghost" className="gf-identifier-copy" aria-label={copyLabel} onClick={() => { void navigator.clipboard?.writeText(id).catch(() => undefined) }}><Copy size={15} aria-hidden="true" /></Button></span>
}

export function FieldLabel({ children, info, htmlFor }: Readonly<{ children: ReactNode; info?: string; htmlFor?: string }>) {
  return <span className="gf-field-label">{htmlFor ? <label htmlFor={htmlFor}>{children}</label> : children}{info && <InfoTooltip text={info} />}</span>
}

export function Dialog({ open, title, children, onClose, className }: Readonly<{ open: boolean; title: string; children: ReactNode; onClose: () => void; className?: string }>) {
  const titleId = useId()
  return <DialogPrimitive.Root open={open} onOpenChange={(nextOpen) => { if (!nextOpen) onClose() }}><DialogPrimitive.Portal><DialogPrimitive.Overlay className="gf-dialog-backdrop" /><DialogPrimitive.Content aria-modal="true" aria-labelledby={titleId} className={'gf-dialog' + (className ? ' ' + className : '')}><div className="gf-dialog-header"><DialogPrimitive.Title asChild><h2 id={titleId}>{title}</h2></DialogPrimitive.Title><DialogPrimitive.Close asChild><Button className="gf-dialog-close" variant="ghost" aria-label="Close dialog">×</Button></DialogPrimitive.Close></div><div className="gf-dialog-body">{children}</div></DialogPrimitive.Content></DialogPrimitive.Portal></DialogPrimitive.Root>
}

export function StatusPill({ status }: Readonly<{ status: string }>) {
  const normalized = status.toLowerCase().replace(/[^a-z0-9]+/g, '-')
  return <span className={'gf-status gf-status-' + normalized}>{status}</span>
}

export function PageHeader({ title, description, action, meta, refresh }: Readonly<{ title: string; description?: string; action?: ReactNode; meta?: ReactNode; refresh?: ReactNode }>) {
  return <header className="gf-page-header"><div><h1>{title}</h1>{description && <p>{description}</p>}{refresh && <div className="gf-page-header-refresh">{refresh}</div>}</div><div className="gf-page-header-actions">{meta && <div className="gf-page-header-meta">{meta}</div>}{action}</div></header>
}

export const Tabs = TabsPrimitive.Root

export const TabsList = forwardRef<ElementRef<typeof TabsPrimitive.List>, ComponentPropsWithoutRef<typeof TabsPrimitive.List>>(function TabsList({ className, ...props }, ref) {
  return <TabsPrimitive.List ref={ref} className={'gf-tabs-list' + (className ? ' ' + className : '')} {...props} />
})

export const TabsTrigger = forwardRef<ElementRef<typeof TabsPrimitive.Trigger>, ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>>(function TabsTrigger({ className, ...props }, ref) {
  return <TabsPrimitive.Trigger ref={ref} className={'gf-tabs-trigger' + (className ? ' ' + className : '')} {...props} />
})

export const TabsContent = forwardRef<ElementRef<typeof TabsPrimitive.Content>, ComponentPropsWithoutRef<typeof TabsPrimitive.Content>>(function TabsContent({ className, ...props }, ref) {
  return <TabsPrimitive.Content ref={ref} className={'gf-tabs-content' + (className ? ' ' + className : '')} {...props} />
})

export const DropdownMenu = DropdownMenuPrimitive.Root
export const DropdownMenuPortal = DropdownMenuPrimitive.Portal
export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger

export const DropdownMenuContent = forwardRef<ElementRef<typeof DropdownMenuPrimitive.Content>, ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Content>>(function DropdownMenuContent({ className, ...props }, ref) {
  return <DropdownMenuPrimitive.Content ref={ref} className={'gf-dropdown-content' + (className ? ' ' + className : '')} {...props} />
})

export const DropdownMenuItem = forwardRef<ElementRef<typeof DropdownMenuPrimitive.Item>, ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Item>>(function DropdownMenuItem({ className, ...props }, ref) {
  return <DropdownMenuPrimitive.Item ref={ref} className={'gf-dropdown-item' + (className ? ' ' + className : '')} {...props} />
})

export const DropdownMenuSeparator = forwardRef<ElementRef<typeof DropdownMenuPrimitive.Separator>, ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Separator>>(function DropdownMenuSeparator({ className, ...props }, ref) {
  return <DropdownMenuPrimitive.Separator ref={ref} className={'gf-dropdown-separator' + (className ? ' ' + className : '')} {...props} />
})

export function TableActions({ label = 'Actions', children }: Readonly<{ label?: string; children: ReactNode }>) {
  return <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" variant="ghost" aria-label={label}><MoreHorizontal size={18} /></Button></DropdownMenuTrigger><DropdownMenuPortal><DropdownMenuContent align="end">{children}</DropdownMenuContent></DropdownMenuPortal></DropdownMenu>
}

export type MetricTone = 'default' | 'success' | 'warning' | 'danger' | 'info'

export function MetricCard({ label, value, detail, icon: Icon, tone = 'default' }: Readonly<{ label: string; value: ReactNode; detail?: ReactNode; icon: ComponentType<{ className?: string; size?: string | number }>; tone?: MetricTone }>) {
  return <section className={'gf-metric gf-metric-' + tone}><div className="gf-metric-heading"><span>{label}</span><span className="gf-metric-icon" aria-hidden="true"><Icon size={16} /></span></div><strong>{value}</strong>{detail && <small>{detail}</small>}</section>
}

export type Column<T> = { key: string; label: string; className?: string; render?: (row: T) => ReactNode }

function tableValue(value: unknown): ReactNode {
  if (value === undefined || value === null) return '—'
  if (typeof value === 'object') return JSON.stringify(value) ?? '—'
  return String(value)
}

export function DataTable<T extends { id?: string | number }>({ columns, rows, caption, className }: Readonly<{ columns: Column<T>[]; rows: T[]; caption: string; className?: string }>) {
  return (
    <div className="gf-table-wrap">
      <table className={'gf-table' + (className ? ' ' + className : '')}>
        <caption className="gf-visually-hidden">{caption}</caption>
        <thead><tr>{columns.map((column) => <th className={column.className} key={column.key} scope="col">{column.label}</th>)}</tr></thead>
        <tbody>{rows.map((row, index) => <tr key={row.id ?? index}>{columns.map((column) => { const value = (row as Record<string, unknown>)[column.key]; return <td className={column.className} key={column.key}>{column.render ? column.render(row) : tableValue(value)}</td> })}</tr>)}</tbody>
      </table>
    </div>
  )
}

export function Pagination({ page, pages, limit = 10, onChange, onLimitChange }: Readonly<{ page: number; pages: number; limit?: number; onChange: (page: number) => void; onLimitChange?: (limit: number) => void }>) {
  return <nav className="gf-pagination" aria-label="Pagination"><Button variant="secondary" disabled={page <= 1} onClick={() => onChange(page - 1)}>Previous</Button><span>Page {page} of {Math.max(1, pages)}</span>{onLimitChange && <label className="gf-pagination-size">Items per page<select className="gf-input" aria-label="Items per page" value={limit} onChange={(event) => onLimitChange(Number(event.target.value))}><option value="10">10</option><option value="20">20</option><option value="50">50</option><option value="100">100</option></select></label>}<Button variant="secondary" disabled={page >= pages} onClick={() => onChange(page + 1)}>Next</Button></nav>
}

export function EmptyState({ title, children }: Readonly<{ title: string; children?: ReactNode }>) {
  return <section className="gf-state gf-empty"><h2>{title}</h2>{children && <p>{children}</p>}</section>
}

export function ErrorState({ title = 'Something went wrong', message, onRetry }: Readonly<{ title?: string; message?: string; onRetry?: () => void }>) {
  return <section className="gf-state gf-error" role="alert"><h2>{title}</h2>{message && <p>{message}</p>}{onRetry && <Button variant="secondary" onClick={onRetry}>Try again</Button>}</section>
}

export function LoadingState({ label = 'Loading' }: Readonly<{ label?: string }>) {
  return <section className="gf-state gf-loading" role="status" aria-live="polite"><span className="gf-spinner" aria-hidden="true" />{label}…</section>
}
