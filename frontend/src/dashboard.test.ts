import { describe, expect, it } from 'vitest'
import { DASHBOARD_WIDGETS, permittedWidgets } from './dashboard'

describe('dashboard permissions', () => {
  it('does not request widgets hidden by permissions', () => {
    expect(permittedWidgets(['tasks.read']).map((widget) => widget.key)).toEqual(['schedules'])
    expect(permittedWidgets([])).toEqual([])
    expect(permittedWidgets(['runs.read', 'runners.read']).map((widget) => widget.key)).toEqual(['runs', 'runners'])
  })

  it('keeps the overview metrics separate from recent activity', () => {
    expect(DASHBOARD_WIDGETS.filter((widget) => widget.kind === 'metric').map((widget) => widget.key)).toEqual(['runs', 'schedules', 'runners'])
    expect(DASHBOARD_WIDGETS.find((widget) => widget.key === 'audit')?.kind).toBe('list')
  })
})
