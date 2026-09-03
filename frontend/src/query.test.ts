import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { QueryRefresh, queryHasData } from './query'

describe('query states', () => {
  it('does not treat pending or failed responses as successful data', () => {
    expect(queryHasData({ data: undefined, isPending: true, isError: false })).toBe(false)
    expect(queryHasData({ data: undefined, isPending: false, isError: true })).toBe(false)
    expect(queryHasData({ data: [], isPending: false, isError: false })).toBe(true)
  })

  it('describes refreshing and offline data states', () => {
    const query = (state: Partial<{ isFetching: boolean; isStale: boolean; dataUpdatedAt: number }>) => ({ isFetching: false, isStale: false, dataUpdatedAt: 0, refetch: async () => undefined, ...state }) as never
    expect(renderToStaticMarkup(createElement(QueryRefresh, { query: query({ isFetching: true }) }))).toContain('Refreshing…')
    const online = navigator.onLine
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
    expect(renderToStaticMarkup(createElement(QueryRefresh, { query: query({ dataUpdatedAt: 1, isStale: true }) }))).toContain('Offline; showing data')
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: online })
  })
})
