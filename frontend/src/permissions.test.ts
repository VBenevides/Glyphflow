import { describe, expect, it } from 'vitest'
import { hasPermission, permissionPrivilegeLevel, PERMISSIONS, sortedPermissions, visibleRoutes } from './permissions'

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

  it('classifies elevated permissions and sorts the catalog', () => {
    expect(PERMISSIONS.filter(({ privilegeLevel }) => privilegeLevel === 'elevated').map(({ name }) => name)).toEqual([
      'users.read', 'users.manage', 'roles.read', 'roles.manage', 'sso.read', 'sso.manage',
      'auth.settings.manage', 'runs.cancel', 'runs.retry', 'logs.read', 'audit.read',
    ])
    expect(permissionPrivilegeLevel('tasks.read')).toBe('standard')
    const names = sortedPermissions().map(({ name }) => name)
    expect(names).toEqual([...names].sort())
  })
})
