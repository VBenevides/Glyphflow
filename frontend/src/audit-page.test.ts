import { describe, expect, it } from 'vitest'
import { auditQuery } from './audit-page'

describe('audit filters', () => {
  it('sends server-side actor/action/target/result/correlation filters', () => {
    const filters = { actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation: 'c1', from: '', to: '' }
    expect(auditQuery(filters, 2)).toMatchObject({ actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation_id: 'c1', page: 2 })
  })
})
