import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { DASHBOARD_WIDGETS, permittedWidgets, projectionDismissalKey } from './dashboard'

const dashboardSource = readFileSync(resolve(process.cwd(), 'src/dashboard.tsx'), 'utf8')

describe('dashboard permissions', () => {
  it('does not request widgets hidden by permissions', () => {
    expect(permittedWidgets(['tasks.read']).map((widget) => widget.key)).toEqual(['schedules'])
    expect(permittedWidgets([])).toEqual([])
    expect(permittedWidgets(['runs.read', 'runners.read']).map((widget) => widget.key)).toEqual(['runs', 'runners'])
  })

  it('keeps the overview metrics separate from recent activity', () => {
    expect(DASHBOARD_WIDGETS.filter((widget) => widget.kind === 'metric').map((widget) => widget.key)).toEqual(['runs', 'schedules', 'runners'])
    expect(DASHBOARD_WIDGETS.find((widget) => widget.key === 'audit')?.kind).toBe('list')
    expect(DASHBOARD_WIDGETS.find((widget) => widget.key === 'secrets')?.label).toBe('Secrets requiring attention')
    expect(DASHBOARD_WIDGETS.find((widget) => widget.key === 'secrets')?.permission).toBe('sso.read')
  })

  it('keys conflict dismissals by the projection calculation timestamp', () => {
    expect(projectionDismissalKey('2026-08-26T12:00:00Z')).toBe('glyphflow:schedule-projection-dismissed:2026-08-26T12:00:00Z')
  })

  it('keeps the dismissed conflict notice distinct from overview metrics', () => {
    const notice = dashboardSource.indexOf('gf-overview-conflict-notice')
    const metrics = dashboardSource.indexOf('<div className="gf-metric-grid">')
    expect(notice).toBeGreaterThanOrEqual(0)
    expect(notice).toBeLessThan(metrics)
    expect(dashboardSource).toContain('!warningKey && projection?.available && projection.conflicts?.length')
    expect(dashboardSource).toContain('warningKey && projection && <Dialog')
  })
})
