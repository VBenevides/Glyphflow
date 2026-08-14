import { describe, expect, it } from 'vitest'
import { auditQuery } from './audit-page'

describe('audit filters', () => {
  it('sends server-side actor/action/target/result/correlation filters', () => {
    const filters = { actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation: 'c1', from: '', to: '' }
    expect(auditQuery(filters, 2)).toMatchObject({ actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation_id: 'c1', exclude_target: '/api/v1/audit', exclude_run_logs: true, page: 2 })
    expect(auditQuery({ ...filters, excludeAuditReads: false }, 2).exclude_target).toBeUndefined()
    expect(auditQuery({ ...filters, excludeRunLogs: false }, 2).exclude_run_logs).toBeUndefined()
  })
})
