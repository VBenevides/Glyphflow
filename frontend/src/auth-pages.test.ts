import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { availableLoginMethods, safeReturnPath } from './auth-pages'

describe('authentication entry routes', () => {
  it('accepts only same-origin relative return paths', () => {
    expect(safeReturnPath('/runs?page=2#latest')).toBe('/runs?page=2#latest')
    expect(safeReturnPath('https://evil.example/steal')).toBe('/')
    expect(safeReturnPath('//evil.example/steal')).toBe('/')
    expect(safeReturnPath(undefined, '/overview')).toBe('/overview')
  })

  it('represents every supported login mode', () => {
    expect(availableLoginMethods({ passwordLogin: true, oidc: false })).toEqual(['password'])
    expect(availableLoginMethods({ passwordLogin: false, oidc: true })).toEqual(['oidc'])
    expect(availableLoginMethods({ passwordLogin: true, oidc: true })).toEqual(['password', 'oidc'])
    expect(availableLoginMethods({ passwordLogin: false, oidc: false })).toEqual([])
  })

  it('explains when registration created a pending account', () => {
    expect(readFileSync(resolve(process.cwd(), 'src/auth-pages.tsx'), 'utf8')).toContain('Your account is awaiting administrator approval.')
  })
})
