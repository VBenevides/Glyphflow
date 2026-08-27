import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type ScheduleProjection, type ScheduleProjectionConflict, type ScheduleProjectionSegment } from './api'
import { Button, EmptyState, PageHeader } from './components'
import { QueryRefresh, QueryState } from './query'
import { formatDateTime } from './format'

export type GanttGrouping = 'runner' | 'task'
export type GanttView = 'week' | 'daily'
export type GanttRange = { startAt: string; endAt: string }
export type GanttLane = { id: string; label: string; segments: ScheduleProjectionSegment[] }

export const GANTT_DAY_MS = 24 * 60 * 60 * 1000
export const GANTT_HOUR_MS = 60 * 60 * 1000
export const DAILY_MIN_OFFSET = 0
export const DAILY_MAX_OFFSET = 7

function runnerLabel(segment: ScheduleProjectionSegment) {
  return (segment.laneLabel.replace(/^Runner:\s*/, '').replace(/^Any runner in\s*/, '') || segment.laneId)
}

export function ganttLanes(segments: ScheduleProjectionSegment[], grouping: GanttGrouping = 'runner'): GanttLane[] {
  const lanes = new Map<string, GanttLane>()
  for (const segment of segments) {
    const id = grouping === 'task' ? 'task:' + segment.taskId : segment.laneId
    const label = grouping === 'task' ? (segment.taskName || segment.taskId) : runnerLabel(segment)
    const lane = lanes.get(id) ?? { id, label, segments: [] }
    lane.segments.push(segment)
    lanes.set(id, lane)
  }
  return [...lanes.values()]
    .map((lane) => ({ ...lane, segments: [...lane.segments].sort((a, b) => Date.parse(a.startAt) - Date.parse(b.startAt) || a.id.localeCompare(b.id)) }))
    .sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id))
}

function startOfUtcDay(value: Date | number = new Date()) {
  const date = new Date(value)
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()))
}

function isoRange(start: Date, end: Date): GanttRange {
  return { startAt: start.toISOString(), endAt: end.toISOString() }
}

function validRange(startAt?: string, endAt?: string): GanttRange | undefined {
  const start = Date.parse(startAt ?? '')
  const end = Date.parse(endAt ?? '')
  return Number.isFinite(start) && Number.isFinite(end) && end > start ? isoRange(new Date(start), new Date(end)) : undefined
}

export function ganttRange(view: GanttView, report: ScheduleProjection, dayOffset = 0, now: Date | number = new Date()): GanttRange {
  if (view === 'week') {
    const reportRange = validRange(report.windowStart, report.windowEnd)
    if (reportRange) return reportRange
  }
  const offset = Math.max(DAILY_MIN_OFFSET, Math.min(DAILY_MAX_OFFSET, dayOffset))
  const start = new Date(startOfUtcDay(now).getTime() + offset * GANTT_DAY_MS)
  return isoRange(start, new Date(start.getTime() + GANTT_DAY_MS))
}

export function projectionIsStale(calculatedAt?: string, now = Date.now()) {
  const timestamp = calculatedAt ? Date.parse(calculatedAt) : NaN
  return Number.isFinite(timestamp) && now - timestamp > 60 * 60 * 1000
}

export function projectionSegmentPercent(segment: ScheduleProjectionSegment, report: ScheduleProjection, range?: GanttRange) {
  const selectedRange = range ?? validRange(report.windowStart, report.windowEnd)
  const start = Date.parse(selectedRange?.startAt ?? '')
  const end = Date.parse(selectedRange?.endAt ?? '')
  const segmentStart = Date.parse(segment.startAt)
  const segmentEnd = Date.parse(segment.endAt)
  const span = end - start
  if (![start, end, segmentStart, segmentEnd].every(Number.isFinite) || span <= 0) return { left: 0, width: 0 }
  const left = Math.max(0, Math.min(100, ((segmentStart - start) / span) * 100))
  const right = Math.max(left, Math.min(100, ((segmentEnd - start) / span) * 100))
  return { left, width: right - left }
}

export function ganttSegmentsInRange(segments: ScheduleProjectionSegment[], range: GanttRange) {
  const start = Date.parse(range.startAt)
  const end = Date.parse(range.endAt)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  return segments.filter((segment) => {
    const segmentStart = Date.parse(segment.startAt)
    const segmentEnd = Date.parse(segment.endAt)
    return Number.isFinite(segmentStart) && Number.isFinite(segmentEnd) && segmentEnd > start && segmentStart < end
  })
}

export function ganttConflictsInRange(conflicts: ScheduleProjectionConflict[], range: GanttRange) {
  const start = Date.parse(range.startAt)
  const end = Date.parse(range.endAt)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  return conflicts.filter((conflict) => {
    const conflictStart = Date.parse(conflict.startAt)
    const conflictEnd = Date.parse(conflict.endAt)
    return Number.isFinite(conflictStart) && Number.isFinite(conflictEnd) && conflictEnd > start && conflictStart < end
  })
}

export function ganttDayDivisions(range: GanttRange) {
  const start = Date.parse(range.startAt)
  const end = Date.parse(range.endAt)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  const divisions: Array<{ at: string; label: string }> = []
  for (let cursor = startOfUtcDay(start).getTime(); cursor <= end; cursor += GANTT_DAY_MS) {
    if (cursor >= start && cursor <= end) {
      const date = new Date(cursor)
      divisions.push({ at: date.toISOString(), label: date.toISOString().slice(0, 10) })
    }
  }
  return divisions
}

export function ganttHourDivisions(range: GanttRange) {
  const start = Date.parse(range.startAt)
  const end = Date.parse(range.endAt)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  const divisions: Array<{ at: string; label: string }> = []
  for (let cursor = Math.ceil(start / GANTT_HOUR_MS) * GANTT_HOUR_MS; cursor <= end; cursor += GANTT_HOUR_MS) {
    const date = new Date(cursor)
    const hours = cursor === end && end - start === GANTT_DAY_MS ? 24 : date.getUTCHours()
    divisions.push({ at: date.toISOString(), label: String(hours).padStart(2, '0') + ':00' })
  }
  return divisions
}

function segmentDetails(segment: ScheduleProjectionSegment) {
  return segment.taskName + ' · ' + segment.scheduleName + ' · ' + segment.laneLabel + ' · ' + formatDateTime(segment.startAt) + '–' + formatDateTime(segment.endAt) + (segment.conflicted ? ' · Conflict' : '')
}

function conflictOccurrenceDetails(occurrence: ScheduleProjectionConflict['occurrences'][number]) {
  return occurrence.taskName + ' · ' + occurrence.scheduleName + ' · ' + occurrence.laneLabel + ' · ' + formatDateTime(occurrence.startAt) + '–' + formatDateTime(occurrence.endAt)
}

function rangeLabel(range: GanttRange) {
  return formatDateTime(range.startAt) + ' – ' + formatDateTime(range.endAt)
}

export function SchedulingGantt({ report }: { report: ScheduleProjection }) {
  const [view, setView] = useState<GanttView>('week')
  const [grouping, setGrouping] = useState<GanttGrouping>('runner')
  const [dayOffset, setDayOffset] = useState(0)
  if (!report.available) return <EmptyState title="Scheduling projection unavailable">The background calculation has not completed yet.</EmptyState>
  const allSegments = report.segments ?? []
  if (!allSegments.length) return <EmptyState title="No projected schedules">Enable a schedule with an active task to populate the seven-day view.</EmptyState>
  const range = ganttRange(view, report, dayOffset)
  const segments = ganttSegmentsInRange(allSegments, range)
  const conflicts = ganttConflictsInRange(report.conflicts ?? [], range)
  const lanes = ganttLanes(segments, grouping)
  const width = 1200
  const timelineLeft = 270
  const timelineWidth = 900
  const rowHeight = 58
  const chartTop = 56
  const height = chartTop + Math.max(1, lanes.length) * rowHeight + 12
  const divisions = view === 'daily' ? ganttHourDivisions(range) : ganttDayDivisions(range)
  const stale = projectionIsStale(report.calculatedAt)
  const previousDisabled = dayOffset <= DAILY_MIN_OFFSET
  const nextDisabled = dayOffset >= DAILY_MAX_OFFSET
  const viewLabel = view === 'week' ? 'Week placement' : 'Daily placement'
  const groupLabel = grouping === 'runner' ? 'Runner' : 'Task'
  return <section className="gf-gantt" aria-labelledby="schedule-gantt-title">
    <div className="gf-gantt-meta">
      <div><h2 id="schedule-gantt-title">{viewLabel}</h2><p className="gf-muted">{rangeLabel(range)} · Calculated {formatDateTime(report.calculatedAt)} · {report.durationSource === 'task_duration' ? 'Intervals use each task duration.' : 'Intervals use the calculated duration.'}</p></div>
      <span className="gf-gantt-count">{segments.length} displayed occurrences · {conflicts.length} conflicts</span>
    </div>
    <div className="gf-gantt-controls" aria-label="Gantt display controls">
      <div className="gf-gantt-control-group" role="group" aria-label="Time view">
        <Button variant={view === 'week' ? 'primary' : 'secondary'} aria-pressed={view === 'week'} onClick={() => { setView('week'); setDayOffset(0) }}>Week</Button>
        <Button variant={view === 'daily' ? 'primary' : 'secondary'} aria-pressed={view === 'daily'} onClick={() => setView('daily')}>Daily</Button>
      </div>
      <div className="gf-gantt-control-group" role="group" aria-label="Row grouping">
        <Button variant={grouping === 'runner' ? 'primary' : 'secondary'} aria-pressed={grouping === 'runner'} onClick={() => setGrouping('runner')}>By runner</Button>
        <Button variant={grouping === 'task' ? 'primary' : 'secondary'} aria-pressed={grouping === 'task'} onClick={() => setGrouping('task')}>By task</Button>
      </div>
      {view === 'daily' && <div className="gf-gantt-day-navigation" aria-label="Daily range navigation">
        <Button variant="secondary" disabled={previousDisabled} onClick={() => setDayOffset((offset) => Math.max(DAILY_MIN_OFFSET, offset - 1))}>Previous day</Button>
        <span>{range.startAt.slice(0, 10)}</span>
        <Button variant="secondary" disabled={nextDisabled} onClick={() => setDayOffset((offset) => Math.min(DAILY_MAX_OFFSET, offset + 1))}>Next day</Button>
      </div>}
    </div>
    {stale && <p className="gf-stale-warning" role="status">This projection is older than one hour. The last successful snapshot is shown.</p>}
    {!segments.length && <p className="gf-gantt-empty gf-muted">No projected executions in this range.</p>}
    <div className="gf-gantt-scroll">
      <svg viewBox={'0 0 ' + width + ' ' + height} role="img" aria-label={viewLabel + ' by ' + groupLabel.toLowerCase()}>
        <text x={timelineLeft + 4} y="16" className="gf-gantt-axis-label">{formatDateTime(range.startAt)}</text>
        <text x={timelineLeft + timelineWidth - 4} y="16" textAnchor="end" className="gf-gantt-axis-label">{formatDateTime(range.endAt)}</text>
        <text x="8" y="16" className="gf-gantt-group-label">{groupLabel}</text>
        {divisions.map((division) => {
          const position = projectionSegmentPercent({ startAt: division.at, endAt: division.at, id: division.at } as ScheduleProjectionSegment, report, range)
          const x = timelineLeft + (position.left / 100) * timelineWidth
          const atEnd = x >= timelineLeft + timelineWidth - 1
          return <g key={division.at}><line x1={x} y1={chartTop - 8} x2={x} y2={height - 6} className="gf-gantt-day-line" /><text x={atEnd ? x - 4 : x + 4} y={chartTop - 12} textAnchor={atEnd ? 'end' : undefined} className="gf-gantt-day-label">{division.label}</text></g>
        })}
        {lanes.map((lane, laneIndex) => {
          const y = chartTop + laneIndex * rowHeight
          return <g key={lane.id}><text x="8" y={y + 17} className="gf-gantt-lane-label">{lane.label}</text><line x1={timelineLeft} y1={y + 12} x2={width - 10} y2={y + 12} className="gf-gantt-lane-line" />{lane.segments.map((segment) => {
            const position = projectionSegmentPercent(segment, report, range)
            const x = timelineLeft + (position.left / 100) * timelineWidth
            const segmentWidth = Math.max(2, (position.width / 100) * timelineWidth)
            const details = segmentDetails(segment)
            return <g key={segment.id}><title>{details}</title><rect x={x} y={y} width={segmentWidth} height="24" rx="4" className={segment.conflicted ? 'gf-gantt-segment is-conflicted' : 'gf-gantt-segment'} tabIndex={0} aria-label={details} strokeDasharray={segment.conflicted ? '4 2' : undefined} /><text x={x + 4} y={y + 17} className="gf-gantt-segment-mark" aria-hidden="true">{segment.conflicted ? '!' : ''}</text></g>
          })}</g>
        })}
      </svg>
    </div>
    <p className="gf-gantt-legend"><span className="gf-gantt-legend-conflict" aria-hidden="true">!</span> Conflict markers are listed below; rows are grouped by {groupLabel.toLowerCase()}.</p>
    <ul className="gf-sr-only" aria-label="Projected schedule occurrences">{segments.map((segment) => <li key={segment.id}>{segmentDetails(segment)} · occurrence</li>)}</ul>
    <section className="gf-gantt-conflicts" aria-labelledby="schedule-conflicts-title"><h2 id="schedule-conflicts-title">Exclusive-resource conflicts</h2>{conflicts.length ? <ol>{conflicts.map((conflict) => <li key={conflict.id}><strong>{conflict.resourceName}</strong><span> · {formatDateTime(conflict.startAt)}–{formatDateTime(conflict.endAt)}</span><ul>{conflict.occurrences.filter((occurrence) => ganttSegmentsInRange([{ ...occurrence, occurrenceCount: 1, conflicted: false, exclusiveResources: [] }], range).length > 0).map((occurrence) => <li key={occurrence.id}>{conflictOccurrenceDetails(occurrence)}</li>)}</ul></li>)}</ol> : <p className="gf-muted">No exclusive-resource conflicts found in this window.</p>}</section>
  </section>
}

export function ScheduleGanttPage({ navigation }: { navigation?: ReactNode } = {}) {
  const query = useQuery({ queryKey: ['schedule-projection'], queryFn: ({ signal }) => api.get<ScheduleProjection>('/api/v1/schedule-projection', undefined, signal), refetchInterval: 30_000 })
  return <main className="gf-content"><PageHeader title="Scheduling Gantt" description="Seven-day cron projection by runner placement and exclusive resource." refresh={<QueryRefresh query={query} />} />{navigation}<QueryState query={query} empty="The schedule projection is not available yet.">{(report) => <SchedulingGantt report={report} />}</QueryState></main>
}
