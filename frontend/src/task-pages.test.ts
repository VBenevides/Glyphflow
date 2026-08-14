import { describe, expect, it } from 'vitest'
import { taskDetailLinks, taskQuery } from './task-pages'

describe('task pages', () => {
  it('builds server-side filters and related links safely', () => {
    expect(taskQuery('nightly', 'enabled', 2)).toEqual({ search: 'nightly', state: 'enabled', page: 2 })
    expect(taskQuery('', '', 1)).toEqual({ search: undefined, state: undefined, page: 1 })
    expect(taskDetailLinks('task/1').runs).toBe('/runs?task=task%2F1')
  })
})
