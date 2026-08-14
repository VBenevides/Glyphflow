import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Button, DataTable, EmptyState, StatusPill } from './components'

describe('shared components', () => {
  it('renders status text and table headers accessibly', () => {
    const html = renderToStaticMarkup(
      <>
        <StatusPill status="UNKNOWN" />
        <DataTable caption="Runs" columns={[{ key: 'name', label: 'Name' }]} rows={[{ id: 'r1', name: 'Nightly' }]} />
      </>,
    )
    expect(html).toContain('UNKNOWN')
    expect(html).toContain('scope="col"')
    expect(html).toContain('Nightly')
  })

  it('keeps empty states explicit', () => {
    expect(renderToStaticMarkup(<EmptyState title="No tasks">Create one to begin.</EmptyState>)).toContain('No tasks')
  })

  it('adds native tooltips to action buttons', () => {
    expect(renderToStaticMarkup(<Button aria-label="Refresh data">Refresh</Button>)).toContain('title="Refresh data"')
  })
})
