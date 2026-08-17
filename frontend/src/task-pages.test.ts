import { describe, expect, it } from 'vitest'
import { taskDetailLinks, taskQuery, taskVersionDiff } from './task-pages'

describe('task pages', () => {
  it('builds server-side filters and related links safely', () => {
    expect(taskQuery('nightly', 'enabled', 2)).toEqual({ search: 'nightly', state: 'enabled', page: 2 })
    expect(taskQuery('', '', 1)).toEqual({ search: undefined, state: undefined, page: 1 })
    expect(taskDetailLinks('task/1').runs).toBe('/runs?task=task%2F1')
    expect(taskDetailLinks('task/1').versions).toBe('/api/v1/tasks/task%2F1/versions')
  })

  it('compares a task version with its previous immutable version', () => {
    expect(taskVersionDiff(
      { id: 'v1', version: 1, command: ['echo', 'one'], pool: 'default', resources: ['db'], timeoutSeconds: 30 },
      { id: 'v2', version: 2, command: ['echo', 'two'], pool: 'default', resources: [], timeoutSeconds: 60 },
    )).toEqual([
      { id: 'command', field: 'Command', previous: 'echo one', current: 'echo two' },
      { id: 'resources', field: 'Resources', previous: 'db', current: 'None' },
      { id: 'timeout', field: 'Timeout', previous: '30s', current: '60s' },
    ])
  })
})
