import { describe, expect, it } from 'vitest'
import { enrollmentPayload } from './enrollment-page'

describe('runner enrollment', () => {
  it('builds explicit platform and architecture payloads', () => {
    expect(enrollmentPayload(' runner-1 ', 'linux', 'arm64')).toEqual({ runner_id: 'runner-1', platform: 'linux', architecture: 'arm64' })
  })
})
