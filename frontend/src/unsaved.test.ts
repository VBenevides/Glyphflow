import { describe, expect, it } from 'vitest'
import { shouldWarnUnsaved } from './unsaved'

describe('unsaved changes', () => {
  it('warns only for dirty, unconfirmed navigation', () => {
    expect(shouldWarnUnsaved(true, false)).toBe(true)
    expect(shouldWarnUnsaved(true, true)).toBe(false)
    expect(shouldWarnUnsaved(false, false)).toBe(false)
  })
})
