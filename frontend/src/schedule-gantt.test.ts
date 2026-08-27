import { describe, expect, it } from 'vitest'
import { ganttLanes, projectionIsStale, projectionSegmentPercent } from './schedule-gantt'
import type { ScheduleProjection, ScheduleProjectionSegment } from './api'

const segment = (id: string, lane_id: string, lane_label: string, start_at: string, end_at: string): ScheduleProjectionSegment => ({
  id, schedule_id: 'schedule-1', schedule_name: 'Nightly', schedule_version_id: 'schedule-1-v1', task_id: 'task-1', task_name: 'Backup', task_version_id: 'task-1-v1', timezone: 'UTC', lane_id, lane_label, start_at, end_at, occurrence_count: 1, conflicted: false, exclusive_resources: [],
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
    const report: ScheduleProjection = { available: true, window_start: '2026-08-26T00:00:00Z', window_end: '2026-09-02T00:00:00Z' }
    const position = projectionSegmentPercent(segment('a', 'pool-1', 'Pool', '2026-08-27T00:00:00Z', '2026-08-27T12:00:00Z'), report)
    expect(position.left).toBeCloseTo(14.2857, 3)
    expect(position.width).toBeCloseTo(7.1428, 3)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T01:00:01Z'))).toBe(true)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T00:59:59Z'))).toBe(false)
  })
})
