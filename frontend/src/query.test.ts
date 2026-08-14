import { describe, expect, it } from 'vitest'
import { queryHasData } from './query'

describe('query states', () => {
  it('does not treat pending or failed responses as successful data', () => {
    expect(queryHasData({ data: undefined, isPending: true, isError: false })).toBe(false)
    expect(queryHasData({ data: undefined, isPending: false, isError: true })).toBe(false)
    expect(queryHasData({ data: [], isPending: false, isError: false })).toBe(true)
  })
})
