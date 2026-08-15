import { describe, expect, it } from 'vitest'
import { enrollmentPayload } from './enrollment-page'

describe('runner enrollment', () => {
  it('builds explicit platform and architecture payloads', () => {
    expect(enrollmentPayload(' runner-1 ', 'linux', 'amd64')).toEqual({ runner_id: 'runner-1', platform: 'linux', architecture: 'amd64', capacity: 10 })
    expect(enrollmentPayload('runner-1', 'linux', 'amd64', 'default', 42)).toEqual({ runner_id: 'runner-1', platform: 'linux', architecture: 'amd64', pool_id: 'default', capacity: 42 })
  })
})
