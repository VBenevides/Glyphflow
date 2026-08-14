import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { bootstrapSession } from './auth'

describe('session bootstrap', () => {
  it('loads config and treats an expired session as logged out', async () => {
    const client = {
      get: async <T>(path: string) => {
        if (path.endsWith('/config')) return { brand: 'Glyphflow', passwordLogin: true, registration: false, oidc: true, csrfCookie: 'glyphflow_csrf' } as T
        throw new ApiError(401, 'expired')
      },
    }
    const result = await bootstrapSession(client)
    expect(result.profile).toBeNull()
    expect(result.permissions).toEqual([])
  })

  it('keeps a valid profile and permissions in memory', async () => {
    const client = { get: async <T>(path: string) => (path.endsWith('/config') ? { brand: 'Glyphflow', passwordLogin: true, registration: true, oidc: false, csrfCookie: 'glyphflow_csrf' } : { id: 'u1', username: 'ada', permissions: ['tasks.read'] }) as T }
    const result = await bootstrapSession(client)
    expect(result.profile?.username).toBe('ada')
    expect(result.permissions).toEqual(['tasks.read'])
  })
})
