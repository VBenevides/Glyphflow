import { describe, expect, it } from 'vitest'
import { placeholderRoutes } from './routes'
import { ROUTES } from './permissions'

describe('application route organization', () => {
  it('does not duplicate implemented pages as placeholders', () => {
    expect(placeholderRoutes(ROUTES).map((route) => route.path)).toEqual([])
  })
})
