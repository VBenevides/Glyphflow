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

  it('sends CSRF and shares one refresh across concurrent 401 responses', async () => {
    document.cookie = 'glyphflow_csrf=csrf-1'
    const calls: string[] = []
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calls.push(url)
      if (url.endsWith('/auth/refresh')) return new Response(null, { status: 204 })
      if (calls.filter((value) => value.endsWith('/tasks')).length <= 2) return new Response('{"error":"expired"}', { status: 401, headers: { 'content-type': 'application/json' } })
      expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('csrf-1')
      return new Response('{"ok":true}', { status: 200, headers: { 'content-type': 'application/json' } })
    }))
    const client = new ApiClient('https://console.example')
    const [first, second] = await Promise.all([client.post<{ ok: boolean }>('/api/v1/tasks', {}), client.post<{ ok: boolean }>('/api/v1/tasks', {})])
    expect(first.ok && second.ok).toBe(true)
    expect(calls.filter((value) => value.endsWith('/auth/refresh'))).toHaveLength(1)
  })

  it('clears the session when refresh fails and does not loop', async () => {
    const expired = vi.fn()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":"expired"}', { status: 401, headers: { 'content-type': 'application/json' } })))
    const client = new ApiClient('https://console.example')
    client.onSessionExpired = expired
    await expect(client.get('/api/v1/tasks')).rejects.toMatchObject({ status: 401 })
    expect(expired).toHaveBeenCalledOnce()
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
