import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { createRoot } from 'react-dom/client'
import { act } from 'react'
import { Activity } from 'lucide-react'
import { Button, DataTable, Dialog, DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, EmptyState, InfoTooltip, Input, matchingFilterOptions, MetricCard, PageHeader, StatusPill } from './components'

describe('shared components', () => {
  it('renders status text and table headers accessibly', () => {
    const html = renderToStaticMarkup(
      <>
        <StatusPill status="UNKNOWN" />
        <DataTable className="gf-audit-table" caption="Runs" columns={[{ key: 'name', label: 'Name', className: 'gf-cell-nowrap' }, { key: 'metadata', label: 'Metadata' }]} rows={[{ id: 'r1', name: 'Nightly', metadata: { owner: 'ops' } }]} />
      </>,
    )
    expect(html).toContain('UNKNOWN')
    expect(html).toContain('scope="col"')
    expect(html).toContain('Nightly')
    expect(html).toContain('{&quot;owner&quot;:&quot;ops&quot;}')
    expect(html).toContain('gf-audit-table')
    expect(html).toContain('gf-cell-nowrap')
  })

  it('keeps empty states explicit', () => {
    expect(renderToStaticMarkup(<EmptyState title="No tasks">Create one to begin.</EmptyState>)).toContain('No tasks')
  })

  it('adds native tooltips to action buttons', () => {
    expect(renderToStaticMarkup(<Button aria-label="Refresh data">Refresh</Button>)).toContain('title="Refresh data"')
    expect(renderToStaticMarkup(<InfoTooltip text="Helpful detail" />)).toContain('title="Helpful detail"')
  })

  it('preserves shared control classes and exposes busy state', () => {
    const html = renderToStaticMarkup(<><Input className="compact" /><Button busy>Save</Button></>)
    expect(html).toContain('class="gf-input compact"')
    expect(html).toContain('aria-busy="true"')
    expect(html).toContain('Working…')
  })

  it('filters and deduplicates searchable filter values', () => {
    expect(matchingFilterOptions(['Admin', 'Runner', 'Admin'], 'run')).toEqual(['Runner'])
  })

  it('portals dialogs and gives each title a unique label target', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    act(() => root.render(<><Dialog open title="First" onClose={() => undefined}>Content</Dialog><Dialog open title="Second" onClose={() => undefined}>Content</Dialog></>))
    const ids = [...document.body.querySelectorAll<HTMLElement>('[aria-labelledby]')].map((element) => element.getAttribute('aria-labelledby')).filter((id): id is string => Boolean(id))
    expect(ids).toHaveLength(2)
    expect(ids[0]).not.toBe(ids[1])
    expect(document.body.querySelector('.gf-dialog-header')).not.toBeNull()
    expect(document.body.querySelector('.gf-dialog-body')).not.toBeNull()
    expect(document.body.querySelector('.gf-dialog-close')).not.toBeNull()
    act(() => root.unmount())
    host.remove()
  })

  it('supports reference-style header metadata and metric icons', () => {
    const html = renderToStaticMarkup(<><PageHeader title="Overview" meta={<span>Live</span>} /><MetricCard label="Runs" value={3} icon={Activity} tone="success" /></>)
    expect(html).toContain('gf-page-header-meta')
    expect(html).toContain('gf-metric-success')
    expect(html).toContain('gf-metric-icon')
    expect(html).toContain('aria-hidden="true"')
  })

  it('renders menu wrappers and primitive table values', () => {
    const menu = renderToStaticMarkup(<DropdownMenu open><DropdownMenuContent><DropdownMenuItem>Inspect</DropdownMenuItem><DropdownMenuSeparator /></DropdownMenuContent></DropdownMenu>)
    expect(menu).toContain('gf-dropdown-item')
    expect(menu).toContain('gf-dropdown-separator')
    const table = renderToStaticMarkup(<DataTable caption="Values" rows={[{ id: 'values', missing: undefined, empty: null, number: 1, boolean: true, bigint: 2n, symbol: Symbol('x'), unsupported: () => undefined }]} columns={[{ key: 'missing', label: 'Missing' }, { key: 'empty', label: 'Empty' }, { key: 'number', label: 'Number' }, { key: 'boolean', label: 'Boolean' }, { key: 'bigint', label: 'Bigint' }, { key: 'symbol', label: 'Symbol' }, { key: 'unsupported', label: 'Unsupported' }]} />)
    expect(table).toContain('1')
    expect(table).toContain('true')
    expect(table).toContain('2')
    expect(table).toContain('x')
    expect(table).toContain('—')
  })
})
