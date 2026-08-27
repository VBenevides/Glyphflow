import { describe, expect, it } from 'vitest'
import { taskDetailLinks, taskNameLabel, taskQuery, taskStateMatches, taskVersionDiff } from './task-pages'

describe('task pages', () => {
  it('builds server-side filters and related links safely', () => {
    expect(taskQuery('nightly', 'enabled', 2)).toEqual({ search: 'nightly', state: 'enabled', page: 2 })
    expect(taskQuery('', '', 1, 10, true)).toEqual({ search: undefined, state: undefined, archived: true, page: 1, limit: 10 })
    expect(taskQuery('', '', 1)).toEqual({ search: undefined, state: undefined, page: 1 })
    expect(taskDetailLinks('task/1').runs).toBe('/runs?task=task%2F1')
    expect(taskDetailLinks('task/1').versions).toBe('/api/v1/tasks/task%2F1/versions')
  })

  it('matches task state filters', () => {
    expect(taskStateMatches({ enabled: true }, 'enabled')).toBe(true)
    expect(taskStateMatches({ enabled: true }, 'disabled')).toBe(false)
    expect(taskStateMatches({ enabled: false }, 'disabled')).toBe(true)
  })

  it('truncates task names only after 30 characters', () => {
    expect(taskNameLabel('a'.repeat(30))).toBe('a'.repeat(30))
    expect(taskNameLabel('a'.repeat(31))).toBe(`${'a'.repeat(30)}…`)
  })

  it('compares a task version with its previous immutable version', () => {
    expect(taskVersionDiff(
      { id: 'v1', version: 1, command: ['echo', 'one'], pool: 'default', resources: ['db'], timeoutSeconds: 30 },
      { id: 'v2', version: 2, command: ['echo', 'two'], pool: 'default', resources: [], timeoutSeconds: 60 },
    )).toEqual([
      { id: 'command', field: 'Command', previous: 'echo one', current: 'echo two' },
      { id: 'resources', field: 'Resources', previous: 'db', current: 'None' },
      { id: 'timeout', field: 'Execution Timeout Seconds', previous: '30s', current: '60s' },
    ])
  })
})
