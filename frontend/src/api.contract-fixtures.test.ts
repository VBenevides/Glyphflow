import { describe, expect, it } from 'vitest'
import taskFixture from '../../contracts/fixtures/task.json'
import runFixture from '../../contracts/fixtures/run.json'
import projectionFixture from '../../contracts/fixtures/schedule-projection.json'
import type { Run, ScheduleProjection, Task } from './api'

describe('shared API contract fixtures', () => {
  it('matches the frontend API types', () => {
    const task: Task = taskFixture
    const run: Run = runFixture
    const projection: ScheduleProjection = projectionFixture

    expect(task.id).toBe('task-1')
    expect(run.exitCode).toBe(0)
    expect(projection.segments?.[0].exclusiveResources[0].id).toBe('resource-1')
  })
})
