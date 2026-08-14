import { describe, expect, it } from 'vitest'
import { resourceState } from './resource-pages'

describe('resource pages', () => {
  it('distinguishes available, leased, and disabled resources', () => {
    expect(resourceState({ id: '1', name: 'db' })).toBe('available')
    expect(resourceState({ id: '1', name: 'db', holder: 'run-1' })).toBe('leased')
    expect(resourceState({ id: '1', name: 'db', enabled: false, holder: 'run-1' })).toBe('disabled')
  })
})
