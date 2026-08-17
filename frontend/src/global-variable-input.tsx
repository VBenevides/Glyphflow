import { useRef, useState, type ChangeEvent, type UIEvent } from 'react'
import type { GlobalVariable } from './api'

type Props = {
  id?: string
  value: string
  variables: GlobalVariable[]
  onChange: (value: string) => void
  multiline?: boolean
  className?: string
  'aria-label'?: string
  'aria-invalid'?: boolean
  title?: string
  onScroll?: (event: UIEvent<HTMLInputElement | HTMLTextAreaElement>) => void
}

export function GlobalVariableInput({ value, variables, onChange, multiline, className = '', ...props }: Props) {
  const ref = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null)
  const setRef = (node: HTMLInputElement | HTMLTextAreaElement | null) => { ref.current = node }
  const [open, setOpen] = useState(false)
  const beforeCursor = value.slice(0, ref.current?.selectionStart ?? value.length)
  const match = /\$ENV:([A-Z0-9_]*)$/.exec(beforeCursor)
  const options = match ? variables.filter((item) => item.name.startsWith(match[1])).slice(0, 20) : []
  const tooltip = Array.from(value.matchAll(/\$ENV:([A-Z_][A-Z0-9_]*)/g)).map(([, name]) => `${name}: ${variables.find((item) => item.name === name)?.value ?? 'Not found'}`).join('\n')
  const choose = (name: string) => {
    if (!match || !ref.current) return
    const cursor = ref.current.selectionStart ?? value.length
    const next = `${value.slice(0, cursor - match[0].length)}$ENV:${name}${value.slice(cursor)}`
    onChange(next)
    setOpen(false)
  }
  const common = { ...props, ref: setRef, value, className: `gf-input ${className}`.trim(), onFocus: () => setOpen(true), onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => { onChange(event.target.value); setOpen(true) }, onBlur: () => window.setTimeout(() => setOpen(false), 100) }
  const inputTitle = (props.title ?? tooltip) || undefined
  const input = multiline ? <textarea {...common} title={inputTitle} rows={4} /> : <input {...common} title={inputTitle} />
  return <div className="gf-env-variable-picker">{input}{tooltip && <span className="gf-env-variable-tooltip" role="tooltip">{tooltip}</span>}{open && options.length > 0 && <div className="gf-task-options" role="listbox">{options.map((item) => <button key={item.id} type="button" className="gf-task-option" role="option" title={item.value} onMouseDown={(event) => { event.preventDefault(); choose(item.name) }}><strong>$ENV:{item.name}</strong><small>{item.value}</small></button>)}</div>}</div>
}
