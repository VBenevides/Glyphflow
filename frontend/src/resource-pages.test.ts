import { describe, expect, it } from 'vitest'
import { compactIdentifier } from './components'
import { resourceKindLabel, resourceNameLabel, resourceState } from './resource-pages'

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

  it('keeps resource names and ids readable', () => {
    expect(resourceNameLabel('a'.repeat(30))).toBe('a'.repeat(30))
    expect(resourceNameLabel('a'.repeat(31))).toBe(`${'a'.repeat(29)}…`)
    expect(compactIdentifier('abcdefghij1234567890')).toBe('abcde…67890')
    expect(compactIdentifier('short-id')).toBe('short-id')
  })
})
