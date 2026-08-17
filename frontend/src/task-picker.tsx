import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Page, type Task } from './api'
import { FieldError } from './errors'
import { FieldLabel, Input } from './components'

export function taskOptionLabel(task: Pick<Task, 'id' | 'name'>) {
  return `${task.name} (${task.id})`
}

export function TaskPicker({ id = 'task-picker', value, onChange, error, label = 'Task', required = false, info }: { id?: string; value: string; onChange: (value: string) => void; error?: string; label?: string; required?: boolean; info?: string }) {
  const [filter, setFilter] = useState('')
  const [open, setOpen] = useState(false)
  const query = useQuery({ queryKey: ['task-picker'], queryFn: ({ signal }) => api.get<Page<Task>>('/api/v1/tasks', { limit: 100 }, signal) })
  const tasks = query.data?.items ?? []
  const selected = tasks.find((task) => task.id === value)
  const options = tasks.filter((task) => taskOptionLabel(task).toLowerCase().includes(filter.trim().toLowerCase()))
  const display = filter || (selected ? taskOptionLabel(selected) : value)
  const choose = (task: Task) => { onChange(task.id); setFilter(''); setOpen(false) }
  return <div className="gf-task-picker"><FieldLabel htmlFor={id} info={info}>{label}</FieldLabel><div className="gf-task-picker-control"><Input id={id} role="combobox" value={display} onFocus={(event) => { setOpen(true); event.currentTarget.select() }} onBlur={() => { setOpen(false); setFilter('') }} onKeyDown={(event) => { if (event.key === 'Escape') { setOpen(false); event.currentTarget.blur() } else if (event.key === 'Enter' && options[0]) { event.preventDefault(); choose(options[0]) } }} onChange={(event) => { setFilter(event.target.value); onChange(''); setOpen(true) }} autoComplete="off" required={required} aria-invalid={Boolean(error)} aria-expanded={open} aria-autocomplete="list" aria-controls={open ? `${id}-options` : undefined} />{open && <div id={`${id}-options`} className="gf-task-options" role="listbox">{options.length ? options.map((task) => <button type="button" role="option" aria-selected={task.id === value} className="gf-task-option" key={task.id} onMouseDown={(event) => { event.preventDefault(); choose(task) }}>{taskOptionLabel(task)}</button>) : <span className="gf-task-empty">No matching tasks</span>}</div>}</div><FieldError message={error} /></div>
}
