import { describe, expect, it } from 'vitest'
import { ganttLanes, projectionIsStale, projectionSegmentPercent } from './schedule-gantt'
import type { ScheduleProjection, ScheduleProjectionSegment } from './api'

const segment = (id: string, laneId: string, laneLabel: string, startAt: string, endAt: string): ScheduleProjectionSegment => ({
  id, scheduleId: 'schedule-1', scheduleName: 'Nightly', scheduleVersionId: 'schedule-1-v1', taskId: 'task-1', taskName: 'Backup', taskVersionId: 'task-1-v1', timezone: 'UTC', laneId, laneLabel, startAt, endAt, occurrenceCount: 1, conflicted: false, exclusiveResources: [],
})

describe('schedule gantt helpers', () => {
  it('groups segments into deterministic placement lanes', () => {
    const lanes = ganttLanes([
      segment('b', 'pool-2', 'Any runner in B', '2026-08-26T02:00:00Z', '2026-08-26T02:01:00Z'),
      segment('a', 'pool-1', 'Any runner in A', '2026-08-26T01:00:00Z', '2026-08-26T01:01:00Z'),
      segment('a2', 'pool-1', 'Any runner in A', '2026-08-26T03:00:00Z', '2026-08-26T03:01:00Z'),
    ])
    expect(lanes.map((lane) => [lane.id, lane.segments.length])).toEqual([['pool-1', 2], ['pool-2', 1]])
  })

  it('positions segments within the seven-day window and detects stale snapshots', () => {
    const report: ScheduleProjection = { available: true, windowStart: '2026-08-26T00:00:00Z', windowEnd: '2026-09-02T00:00:00Z' }
    const position = projectionSegmentPercent(segment('a', 'pool-1', 'Pool', '2026-08-27T00:00:00Z', '2026-08-27T12:00:00Z'), report)
    expect(position.left).toBeCloseTo(14.2857, 3)
    expect(position.width).toBeCloseTo(7.1428, 3)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T01:00:01Z'))).toBe(true)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T00:59:59Z'))).toBe(false)
  })
})
