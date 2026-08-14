import { describe, expect, it } from 'vitest'
import { roleMappingsValue } from './admin-pages'

describe('role selectors', () => {
  it('sends only complete group-role mappings', () => {
    expect(roleMappingsValue([{ group: ' admins ', role: 'admin' }, { group: '', role: 'user' }, { group: 'users', role: '' }])).toEqual({ admins: 'admin' })
  })
})
