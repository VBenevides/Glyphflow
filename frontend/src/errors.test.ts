import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { describeError } from './errors'

describe('API error mapping', () => {
  it('maps status classes and preserves fields/correlation IDs', () => {
    expect(describeError(new ApiError(400, { error: 'runner_id must contain only letters, digits, dot, underscore, or hyphen' })).message).toContain('runner_id must contain')
    expect(describeError(new ApiError(409, { error: 'runner is already enrolled' })).message).toBe('runner is already enrolled')
    expect(describeError(new ApiError(422, { message: 'Invalid', fields: { name: 'Required' } }, new Headers({ 'X-Correlation-ID': 'c1' })))).toMatchObject({ title: 'Check the form', fields: { name: 'Required' }, correlationId: 'c1', retryable: false })
    expect(describeError(new ApiError(503, 'down')).retryable).toBe(true)
    expect(describeError(new ApiError(403, 'no')).title).toBe('Access denied')
  })
})
