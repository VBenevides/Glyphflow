import { describe, expect, it } from 'vitest'
import { activeGroupName, groupedRoutes } from './shell'
import { ROUTES } from './permissions'

describe('application shell navigation', () => {
  it('groups the visible route catalog into product areas', () => {
    const groups = groupedRoutes(ROUTES)
    expect(groups.map(({ group }) => group.name)).toEqual(['Operations', 'Infrastructure', 'Security', 'Administration'])
    expect(groups[0].routes.map((route) => route.path)).toContain('/runs')
    expect(activeGroupName('/admin/roles')).toBe('Administration')
    expect(activeGroupName('/unknown')).toBeUndefined()
  })
})
