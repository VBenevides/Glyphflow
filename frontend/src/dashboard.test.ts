import { describe, expect, it } from 'vitest'
import { permittedWidgets } from './dashboard'

describe('dashboard permissions', () => {
  it('does not request widgets hidden by permissions', () => {
    expect(permittedWidgets(['tasks.read']).map((widget) => widget.key)).toEqual(['schedules'])
    expect(permittedWidgets([])).toEqual([])
    expect(permittedWidgets(['runs.read', 'runners.read']).map((widget) => widget.key)).toEqual(['runs', 'runners'])
  })
})
