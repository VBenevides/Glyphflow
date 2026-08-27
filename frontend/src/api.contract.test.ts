import { describe, expect, it, vi } from 'vitest'
import { ApiClient, type Task } from './api'

describe('task API contract', () => {
  it('sends the backend request shape and reads the response shape', async () => {
	const input = { name: 'demo', command: ['echo', 'ok'], runner_pool: 'default', duration_seconds: 30 }
    vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(JSON.parse(String(init?.body))).toEqual(input)
		return new Response(JSON.stringify({ id: 'task-1', name: 'demo', pool: 'default', durationSeconds: 30 }), { status: 201, headers: { 'content-type': 'application/json' } })
    }))

    await expect(new ApiClient('https://console.example').post<Task>('/api/v1/tasks', input)).resolves.toMatchObject({
      id: 'task-1',
      name: 'demo',
      pool: 'default',
	  durationSeconds: 30,
    })
  })
})
