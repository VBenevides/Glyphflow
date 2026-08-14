import { describe, expect, it } from 'vitest'
import { safeReturnPath } from './auth-pages'

describe('login redirect contract', () => {
  it('preserves query and fragment only for relative routes', () => {
    expect(safeReturnPath('/tasks?filter=failed#runs')).toBe('/tasks?filter=failed#runs')
    expect(safeReturnPath('/')).toBe('/')
    expect(safeReturnPath('javascript:alert(1)')).toBe('/')
  })
})
