import { describe, expect, it } from 'vitest'
import { accountDirty } from './account-pages'

describe('account dirty baseline', () => {
  it('does not mark the loaded profile dirty', () => {
    expect(accountDirty('Ada', 'Ada', { current: '', next: '', confirm: '' })).toBe(false)
  })

  it('marks profile edits and password edits dirty', () => {
    expect(accountDirty('Grace', 'Ada', { current: '', next: '', confirm: '' })).toBe(true)
    expect(accountDirty('Ada', 'Ada', { current: 'old', next: '', confirm: '' })).toBe(true)
  })
})
