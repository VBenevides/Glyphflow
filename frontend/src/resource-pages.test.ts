import { describe, expect, it } from 'vitest'
import { resourceKindLabel, resourceState } from './resource-pages'

describe('resource pages', () => {
  it('distinguishes available, leased, and disabled resources', () => {
    expect(resourceState({ id: '1', name: 'db' })).toBe('available')
    expect(resourceState({ id: '1', name: 'db', holder: 'run-1' })).toBe('leased')
    expect(resourceState({ id: '1', name: 'db', enabled: false, holder: 'run-1' })).toBe('disabled')
  })

  it('labels non-blocking resources', () => {
    expect(resourceKindLabel('non-blocking')).toBe('Non-blocking')
    expect(resourceKindLabel('exclusive')).toBe('Exclusive')
  })
})
