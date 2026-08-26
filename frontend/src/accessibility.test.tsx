import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { createRoot } from 'react-dom/client'
import { act } from 'react'
import { Button, DataTable, Dialog, EmptyState, FieldLabel, LoadingState } from './components'

describe('accessibility contracts', () => {
  it('keeps controls named and tables associated with column headers', () => {
    const html = renderToStaticMarkup(<><Button aria-label="Refresh data">Refresh</Button><DataTable caption="Users" rows={[{ id: 'u1', name: 'Ada' }]} columns={[{ key: 'name', label: 'Name' }]} /></>)
    expect(html).toContain('aria-label="Refresh data"')
    expect(html).toContain('scope="col"')
    expect(html).toContain('Users')
  })

  it('keeps tooltip buttons outside explicit labels', () => {
    const html = renderToStaticMarkup(<FieldLabel htmlFor="schedule-name" info="Schedule details">Name</FieldLabel>)
    expect(html).toContain('<label for="schedule-name">Name</label>')
    expect(html).toContain('aria-label="More information"')
  })

  it('keeps dialog focus semantics and live states explicit', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    act(() => root.render(<Dialog open title="Confirm" onClose={() => undefined}><Button>Confirm</Button></Dialog>))
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    act(() => root.unmount())
    host.remove()
    const html = renderToStaticMarkup(<><LoadingState label="Loading users" /><EmptyState title="No users">Create a user.</EmptyState></>)
    expect(html).toContain('role="status"')
    expect(html).toContain('No users')
  })
})
