import { describe, expect, it } from 'vitest'
import { safeReturnPath } from './auth-pages'

describe('authentication entry routes', () => {
  it('accepts only same-origin relative return paths', () => {
    expect(safeReturnPath('/runs?page=2#latest')).toBe('/runs?page=2#latest')
    expect(safeReturnPath('https://evil.example/steal')).toBe('/')
    expect(safeReturnPath('//evil.example/steal')).toBe('/')
    expect(safeReturnPath(undefined, '/overview')).toBe('/overview')
  })
})
