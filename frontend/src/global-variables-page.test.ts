import { describe, expect, it } from 'vitest'
import { validGlobalVariableName } from './global-variables-page'

describe('global variable names', () => {
  it('accepts environment names and rejects shell punctuation', () => {
    expect(validGlobalVariableName(' CACHE_PATH ')).toBe(true)
    expect(validGlobalVariableName('CACHE-PATH')).toBe(false)
    expect(validGlobalVariableName('')).toBe(false)
  })
})
