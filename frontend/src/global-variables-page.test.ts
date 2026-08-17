import { describe, expect, it } from 'vitest'
import { globalVariableDeleteWarning, validGlobalVariableName } from './global-variables-page'

describe('global variable names', () => {
  it('accepts environment names and rejects shell punctuation', () => {
    expect(validGlobalVariableName(' CACHE_PATH ')).toBe(true)
    expect(validGlobalVariableName('variable2_nok')).toBe(false)
    expect(validGlobalVariableName('VARIABLE3 NOK')).toBe(false)
    expect(validGlobalVariableName('CACHE-PATH')).toBe(false)
    expect(validGlobalVariableName('')).toBe(false)
  })

  it('warns about references and deletion impact', () => {
    expect(globalVariableDeleteWarning({ name: 'CACHE_PATH', references: 2 })).toContain('referenced by 2 task or schedule definitions')
    expect(globalVariableDeleteWarning({ name: 'CACHE_PATH', references: 0 })).toContain('not referenced by any task or schedule definitions')
    expect(globalVariableDeleteWarning({ name: 'CACHE_PATH' })).toContain('Delete it?')
  })
})
