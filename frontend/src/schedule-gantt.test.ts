import { describe, expect, it } from 'vitest'
import { ganttConflictNumberMap, ganttConflictsInRange, ganttDayDivisions, ganttHourDivisions, ganttLanes, ganttRange, ganttRunnerDividerAt, ganttSegmentMatchesFilters, ganttSegmentsInRange, projectionIsStale, projectionSegmentPercent, truncateGanttLabel } from './schedule-gantt'
import type { ScheduleProjection, ScheduleProjectionConflict, ScheduleProjectionSegment } from './api'

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
    expect(lanes.map((lane) => lane.label)).toEqual(['A', 'B'])
  })

  it('groups rows by task when requested', () => {
    const lanes = ganttLanes([
      { ...segment('a', 'pool-1', 'Pool A', '2026-08-26T01:00:00Z', '2026-08-26T01:01:00Z'), taskId: 'task-1', taskName: 'Backup' },
      { ...segment('b', 'pool-2', 'Pool B', '2026-08-26T02:00:00Z', '2026-08-26T02:01:00Z'), taskId: 'task-1', taskName: 'Backup' },
      { ...segment('c', 'pool-1', 'Pool A', '2026-08-26T03:00:00Z', '2026-08-26T03:01:00Z'), taskId: 'task-2', taskName: 'Deploy' },
    ], 'task')
    expect(lanes.map((lane) => [lane.id, lane.label, lane.segments.length])).toEqual([
      ['task:task-1', 'Backup', 2],
      ['task:task-2', 'Deploy', 1],
    ])
  })

  it('sorts runners alphabetically and tasks by runner then task name', () => {
    const segments = [
      { ...segment('task-z', 'runner-b', 'Runner: Runner B', '2026-08-26T01:00:00Z', '2026-08-26T01:01:00Z'), taskId: 'task-z', taskName: 'Alpha Task' },
      { ...segment('task-zulu', 'runner-a', 'Runner: Runner A', '2026-08-26T02:00:00Z', '2026-08-26T02:01:00Z'), taskId: 'task-zulu', taskName: 'Zulu Task' },
      { ...segment('task-beta', 'runner-a', 'Runner: Runner A', '2026-08-26T03:00:00Z', '2026-08-26T03:01:00Z'), taskId: 'task-beta', taskName: 'Beta Task' },
    ]
    expect(ganttLanes(segments).map((lane) => lane.label)).toEqual(['Runner A', 'Runner B'])
    const taskLanes = ganttLanes(segments, 'task')
    expect(taskLanes.map((lane) => lane.label)).toEqual(['Beta Task', 'Zulu Task', 'Alpha Task'])
    expect(ganttRunnerDividerAt(taskLanes, 'task', 0)).toBe(false)
    expect(ganttRunnerDividerAt(taskLanes, 'task', 1)).toBe(false)
    expect(ganttRunnerDividerAt(taskLanes, 'task', 2)).toBe(true)
    expect(ganttRunnerDividerAt(taskLanes, 'runner', 2)).toBe(false)
  })

  it('positions segments within the selected range and detects stale snapshots', () => {
    const report: ScheduleProjection = { available: true, windowStart: '2026-08-26T00:00:00Z', windowEnd: '2026-09-02T00:00:00Z' }
    const position = projectionSegmentPercent(segment('a', 'pool-1', 'Pool', '2026-08-27T00:00:00Z', '2026-08-27T12:00:00Z'), report)
    expect(position.left).toBeCloseTo(14.2857, 3)
    expect(position.width).toBeCloseTo(7.1428, 3)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T01:00:01Z'))).toBe(true)
    expect(projectionIsStale('2026-08-26T00:00:00Z', Date.parse('2026-08-26T00:59:59Z'))).toBe(false)
  })

  it('clamps daily navigation to today through today plus seven days', () => {
    const report: ScheduleProjection = { available: true, windowStart: '2026-08-26T15:30:00Z', windowEnd: '2026-09-02T15:30:00Z' }
    const now = new Date('2026-08-26T15:30:00Z')
    expect(ganttRange('week', report)).toEqual({ startAt: '2026-08-26T15:30:00.000Z', endAt: '2026-09-02T15:30:00.000Z' })
    expect(ganttRange('daily', report, 0, now)).toEqual({ startAt: '2026-08-26T00:00:00.000Z', endAt: '2026-08-27T00:00:00.000Z' })
    expect(ganttRange('daily', report, 7, now).startAt).toBe('2026-09-02T00:00:00.000Z')
    expect(ganttRange('daily', report, 99, now).startAt).toBe('2026-09-02T00:00:00.000Z')
  })

  it('draws day divisions and keeps all overlapping occurrences', () => {
    const range = { startAt: '2026-08-26T00:00:00.000Z', endAt: '2026-09-02T00:00:00.000Z' }
    const divisions = ganttDayDivisions(range)
    expect(divisions[0]).toEqual({ at: '2026-08-26T00:00:00.000Z', label: '2026-08-26' })
    expect(divisions[divisions.length - 1]?.label).toBe('2026-09-02')
    const segments = [
      segment('a', 'pool-1', 'Pool', '2026-08-27T00:00:00Z', '2026-08-27T01:00:00Z'),
      segment('b', 'pool-1', 'Pool', '2026-08-27T00:00:00Z', '2026-08-27T01:00:00Z'),
    ]
    expect(ganttSegmentsInRange(segments, range)).toHaveLength(2)
    expect(ganttConflictsInRange([{ id: 'conflict', resourceId: 'resource-1', resourceName: 'Database', startAt: '2026-08-27T00:00:00Z', endAt: '2026-08-27T01:00:00Z', occurrences: [] }], range)).toHaveLength(1)
  })

  it('draws hourly divisions for a daily range', () => {
    const divisions = ganttHourDivisions({ startAt: '2026-08-27T00:00:00.000Z', endAt: '2026-08-28T00:00:00.000Z' })
    expect(divisions).toHaveLength(25)
    expect(divisions[0]).toEqual({ at: '2026-08-27T00:00:00.000Z', label: '00:00' })
    expect(divisions[divisions.length - 1]).toEqual({ at: '2026-08-28T00:00:00.000Z', label: '24:00' })
  })

  it('truncates labels that would crowd the timeline', () => {
    expect(truncateGanttLabel('short', 5)).toBe('short')
    expect(truncateGanttLabel('long label', 5)).toBe('long…')
  })

  it('matches optional runner and task filters', () => {
    const value = { laneId: 'runner-1', taskId: 'task-1' }
    expect(ganttSegmentMatchesFilters(value)).toBe(true)
    expect(ganttSegmentMatchesFilters(value, 'runner-2')).toBe(false)
    expect(ganttSegmentMatchesFilters(value, '', 'task-2')).toBe(false)
    expect(ganttSegmentMatchesFilters(value, 'runner-1', 'task-1')).toBe(true)
    expect(ganttSegmentMatchesFilters({ ...value, conflicted: true }, '', '', true)).toBe(true)
    expect(ganttSegmentMatchesFilters(value, '', '', true)).toBe(false)
  })

  it('maps segments to every numbered conflict that affects them', () => {
    const occurrence = (id: string) => ({ id } as ScheduleProjectionConflict['occurrences'][number])
    const numbers = ganttConflictNumberMap([
      { id: 'conflict-1', resourceId: 'resource-1', resourceName: 'Database', startAt: '', endAt: '', occurrences: [occurrence('segment-1')] },
      { id: 'conflict-2', resourceId: 'resource-2', resourceName: 'Warehouse', startAt: '', endAt: '', occurrences: [occurrence('segment-1'), occurrence('segment-2')] },
    ])
    expect(numbers.get('segment-1')).toEqual([1, 2])
    expect(numbers.get('segment-2')).toEqual([2])
  })
})
