import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiClient, ApiError, buildUrl } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('api client', () => {
  it('builds encoded URLs and parses JSON', async () => {
    expect(buildUrl('https://console.example', '/api/v1/tasks', { q: 'a b', page: 2, empty: '' })).toBe('https://console.example/api/v1/tasks?q=a+b&page=2')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"items":[]}', { status: 200, headers: { 'content-type': 'application/json' } })))
    const result = await new ApiClient('https://console.example').get<{ items: unknown[] }>('/api/v1/tasks')
    expect(result.items).toEqual([])
    expect(fetch).toHaveBeenCalledWith('https://console.example/api/v1/tasks', expect.objectContaining({ credentials: 'include' }))
  })

  it('parses text errors and preserves cancellation', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('bad gateway', { status: 502, headers: { 'X-Correlation-ID': 'c-1' } })))
    await expect(new ApiClient().get('/api/v1/tasks')).rejects.toMatchObject({ status: 502, correlationId: 'c-1' })
    const controller = new AbortController()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new DOMException('aborted', 'AbortError')))
    await expect(new ApiClient().get('/api/v1/tasks', undefined, controller.signal)).rejects.toBeInstanceOf(DOMException)
    expect(new ApiError(422, { code: 'validation', fields: { name: 'required' } }).fields.name).toBe('required')
  })
})
