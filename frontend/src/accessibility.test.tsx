import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Button, DataTable, Dialog, EmptyState, LoadingState } from './components'

describe('accessibility contracts', () => {
  it('keeps controls named and tables associated with column headers', () => {
    const html = renderToStaticMarkup(<><Button aria-label="Refresh data">Refresh</Button><DataTable caption="Users" rows={[{ id: 'u1', name: 'Ada' }]} columns={[{ key: 'name', label: 'Name' }]} /></>)
    expect(html).toContain('aria-label="Refresh data"')
    expect(html).toContain('scope="col"')
    expect(html).toContain('Users')
  })

  it('keeps dialog focus semantics and live states explicit', () => {
    const html = renderToStaticMarkup(<><Dialog open title="Confirm" onClose={() => undefined}><Button>Confirm</Button></Dialog><LoadingState label="Loading users" /><EmptyState title="No users">Create a user.</EmptyState></>)
    expect(html).toContain('role="dialog"')
    expect(html).toContain('aria-modal="true"')
    expect(html).toContain('role="status"')
    expect(html).toContain('No users')
  })
})
