import { describe, expect, it } from 'vitest'
import { auditQuery } from './audit-page'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('audit filters', () => {
  it('sends server-side actor/action/target/result/correlation filters', () => {
    const filters = { actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation: 'c1', from: '', to: '' }
    expect(auditQuery(filters, 2)).toMatchObject({ actor: 'u1', action: 'role.update', target: 'role:admin', result: 'success', correlation_id: 'c1', exclude_target: '/api/v1/audit', exclude_run_logs: true, exclude_method: 'GET', page: 2 })
    expect(auditQuery({ ...filters, excludeAuditReads: false }, 2).exclude_target).toBeUndefined()
    expect(auditQuery({ ...filters, excludeRunLogs: false }, 2).exclude_run_logs).toBeUndefined()
    expect(auditQuery({ ...filters, excludeGet: false }, 2)).toMatchObject({ include_get: true })
    expect(auditQuery({ ...filters, excludeGet: false }, 2).exclude_method).toBeUndefined()
  })

  it('keeps filter suggestions on the bounded audit page', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/audit-page.tsx'), 'utf8')
    expect(source).not.toContain("queryKey: ['audit-filter-options'")
    expect(source).not.toContain('all: true')
    expect(source).toContain('Hide GET requests')
  })
})
