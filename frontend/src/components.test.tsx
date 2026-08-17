import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Activity } from 'lucide-react'
import { Button, DataTable, Dialog, EmptyState, Input, MetricCard, PageHeader, StatusPill } from './components'

describe('shared components', () => {
  it('renders status text and table headers accessibly', () => {
    const html = renderToStaticMarkup(
      <>
        <StatusPill status="UNKNOWN" />
        <DataTable className="gf-audit-table" caption="Runs" columns={[{ key: 'name', label: 'Name', className: 'gf-cell-nowrap' }]} rows={[{ id: 'r1', name: 'Nightly' }]} />
      </>,
    )
    expect(html).toContain('UNKNOWN')
    expect(html).toContain('scope="col"')
    expect(html).toContain('Nightly')
    expect(html).toContain('gf-audit-table')
    expect(html).toContain('gf-cell-nowrap')
  })

  it('keeps empty states explicit', () => {
    expect(renderToStaticMarkup(<EmptyState title="No tasks">Create one to begin.</EmptyState>)).toContain('No tasks')
  })

  it('adds native tooltips to action buttons', () => {
    expect(renderToStaticMarkup(<Button aria-label="Refresh data">Refresh</Button>)).toContain('title="Refresh data"')
  })

  it('preserves shared control classes and exposes busy state', () => {
    const html = renderToStaticMarkup(<><Input className="compact" /><Button busy>Save</Button></>)
    expect(html).toContain('class="gf-input compact"')
    expect(html).toContain('aria-busy="true"')
    expect(html).toContain('Working…')
  })

  it('gives each dialog title a unique label target', () => {
    const html = renderToStaticMarkup(<><Dialog open title="First" onClose={() => undefined}>Content</Dialog><Dialog open title="Second" onClose={() => undefined}>Content</Dialog></>)
    const ids = [...html.matchAll(/aria-labelledby="([^"]+)"/g)].map((match) => match[1])
    expect(ids).toHaveLength(2)
    expect(ids[0]).not.toBe(ids[1])
  })

  it('supports reference-style header metadata and metric icons', () => {
    const html = renderToStaticMarkup(<><PageHeader title="Overview" meta={<span>Live</span>} /><MetricCard label="Runs" value={3} icon={Activity} tone="success" /></>)
    expect(html).toContain('gf-page-header-meta')
    expect(html).toContain('gf-metric-success')
    expect(html).toContain('aria-hidden="true"')
  })
})
