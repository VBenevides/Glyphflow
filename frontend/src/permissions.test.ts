import { describe, expect, it } from 'vitest'
import { hasPermission, visibleRoutes } from './permissions'

describe('frontend permissions', () => {
  it('supports canonical OR permission groups', () => {
    expect(hasPermission(['tasks.manage'], 'tasks.read|tasks.manage')).toBe(true)
    expect(hasPermission(['runs.read'], 'runs.cancel|runs.retry')).toBe(false)
  })

  it('hides routes without grants', () => {
    const paths = visibleRoutes(['tasks.read']).map((route) => route.path)
    expect(paths).toContain('/tasks')
    expect(paths).not.toContain('/runs')
    expect(paths).toContain('/')
  })
})
