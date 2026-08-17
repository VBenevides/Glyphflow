import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { filterAndSortRoles, roleMappingsValue, UserAccessEditor, UserCreationForm } from './admin-pages'
import type { RoleDefinition, UserRecord } from './api'

describe('role selectors', () => {
  it('sends only complete group-role mappings', () => {
    expect(roleMappingsValue([{ group: ' admins ', role: 'admin' }, { group: '', role: 'user' }, { group: 'users', role: '' }])).toEqual({ admins: 'admin' })
  })

  it('filters roles and keeps system roles first in alphabetical order', () => {
    const roles: RoleDefinition[] = [
      { id: 'z-custom', name: 'zeta', permissions: ['runs.read'] },
      { id: 'system-operator', name: 'operator', permissions: ['tasks.read'], system: true },
      { id: 'a-custom', name: 'alpha', permissions: ['runs.read'] },
      { id: 'system-admin', name: 'admin', permissions: ['tasks.read'], system: true },
    ]
    expect(filterAndSortRoles(roles, 'tasks.read').map((role) => role.name)).toEqual(['admin', 'operator'])
    expect(filterAndSortRoles(roles, 'custom').map((role) => role.name)).toEqual(['alpha', 'zeta'])
  })
})

describe('admin access workflow', () => {
  const roles: RoleDefinition[] = [{ id: 'system-user', name: 'user', permissions: [], system: true }, { id: 'system-admin', name: 'admin', permissions: [], system: true }]
  const user: UserRecord = { id: 'user-1', username: 'user@example.com', email: 'user@example.com', roles: ['user'] }

  it('renders the user creation controls', () => {
    const html = renderToStaticMarkup(createElement(UserCreationForm, { onCreated: async () => undefined }))
    expect(html).toContain('Create user')
    expect(html).toContain('admin-user-email')
    expect(html).toContain('admin-user-password')
  })

  it('renders role assignment and revoke controls for a user', () => {
    const html = renderToStaticMarkup(createElement(UserAccessEditor, { user, roles, onChanged: async () => undefined, onClose: () => undefined }))
    expect(html).toContain('Assign role')
    expect(html).toContain('Assigned roles')
    expect(html).toContain('Revoke')
    expect(html).toContain('admin')
  })
})
