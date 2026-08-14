import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiClient } from './api'
import { bootstrapSession } from './auth'
import { safeReturnPath } from './auth-pages'

afterEach(() => vi.unstubAllGlobals())

describe('frontend workflows', () => {
  it('restores a password session, preserves an OIDC return path, and signs out', async () => {
    const calls: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input); calls.push(`${init?.method ?? 'GET'} ${url}`)
      if (url.endsWith('/config')) return new Response(JSON.stringify({ brand: 'Glyphflow', passwordLogin: true, registration: true, oidc: true, csrfCookie: 'glyphflow_csrf' }), { headers: { 'content-type': 'application/json' } })
      if (url.endsWith('/me')) return new Response(JSON.stringify({ id: 'u1', username: 'ada', permissions: ['tasks.manage'] }), { headers: { 'content-type': 'application/json' } })
      return new Response(null, { status: 204 })
    }))
    const client = new ApiClient('https://console.example')
    const session = await bootstrapSession(client)
    await client.post('/api/v1/auth/login', { email: 'ada@example.com', password: 'correct horse battery staple' })
    await client.post('/api/v1/auth/logout')
    expect(session.profile?.username).toBe('ada')
    expect(safeReturnPath('/runs?page=2#latest')).toBe('/runs?page=2#latest')
    expect(safeReturnPath('https://evil.example')).toBe('/')
    expect(calls).toEqual(expect.arrayContaining(['POST https://console.example/api/v1/auth/login', 'POST https://console.example/api/v1/auth/logout']))
  })

  it('keeps protected operations explicit and refreshes once after expiry', async () => {
    const calls: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input); calls.push(`${init?.method ?? 'GET'} ${url}`)
      if (url.endsWith('/auth/refresh')) return new Response(null, { status: 204 })
      if (url.endsWith('/runs/execute')) return new Response('{"id":"run-1"}', { headers: { 'content-type': 'application/json' } })
      if (url.endsWith('/admin/roles') && (init?.method ?? 'GET') === 'GET') return new Response('expired', { status: 401 })
      if (url.endsWith('/admin/roles') && init?.method === 'POST') return new Response(null, { status: 204 })
      return new Response(null, { status: 204 })
    }))
    const client = new ApiClient('https://console.example')
    await client.post('/api/v1/runs/execute', { task_id: 'task-1', reason: 'manual check' })
    await expect(client.get('/api/v1/admin/roles')).rejects.toMatchObject({ status: 401 })
    await client.post('/api/v1/admin/roles', { key: 'operator', permissions: ['runs.read'] })
    expect(calls).toContain('POST https://console.example/api/v1/runs/execute')
    expect(calls.filter((call) => call.includes('/auth/refresh'))).toHaveLength(1)
  })
})
