import { useQuery } from '@tanstack/react-query'
import { api, type ScheduleProjection, type ScheduleProjectionSegment } from './api'
import { EmptyState, PageHeader } from './components'
import { QueryRefresh, QueryState } from './query'
import { formatDateTime } from './format'

export type GanttLane = { id: string; label: string; segments: ScheduleProjectionSegment[] }

export function ganttLanes(segments: ScheduleProjectionSegment[]): GanttLane[] {
  const lanes = new Map<string, GanttLane>()
  for (const segment of segments) {
    const lane = lanes.get(segment.lane_id) ?? { id: segment.lane_id, label: segment.lane_label, segments: [] }
    lane.segments.push(segment)
    lanes.set(segment.lane_id, lane)
  }
  return [...lanes.values()].sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id))
}

export function projectionIsStale(calculatedAt?: string, now = Date.now()) {
  const timestamp = calculatedAt ? Date.parse(calculatedAt) : NaN
  return Number.isFinite(timestamp) && now - timestamp > 60 * 60 * 1000
}

export function projectionSegmentPercent(segment: ScheduleProjectionSegment, report: ScheduleProjection) {
  const start = Date.parse(report.window_start ?? '')
  const end = Date.parse(report.window_end ?? '')
  const segmentStart = Date.parse(segment.start_at)
  const segmentEnd = Date.parse(segment.end_at)
  const span = end - start
  if (![start, end, segmentStart, segmentEnd].every(Number.isFinite) || span <= 0) return { left: 0, width: 0 }
  const left = Math.max(0, Math.min(100, ((segmentStart - start) / span) * 100))
  const right = Math.max(left, Math.min(100, ((segmentEnd - start) / span) * 100))
  return { left, width: right - left }
}

function segmentDetails(segment: ScheduleProjectionSegment) {
  return `${segment.task_name} · ${segment.schedule_name} · ${segment.lane_label} · ${formatDateTime(segment.start_at)}–${formatDateTime(segment.end_at)}${segment.conflicted ? ' · Conflict' : ''}`
}

export function SchedulingGantt({ report }: { report: ScheduleProjection }) {
  if (!report.available) return <EmptyState title="Scheduling projection unavailable">The background calculation has not completed yet.</EmptyState>
  const segments = report.segments ?? []
  if (!segments.length) return <EmptyState title="No projected schedules">Enable a schedule with an active task to populate the seven-day view.</EmptyState>
  const lanes = ganttLanes(segments)
  const width = 1200
  const timelineLeft = 270
  const timelineWidth = 900
  const rowHeight = 58
  const height = 34 + lanes.length * rowHeight
  const stale = projectionIsStale(report.calculated_at)
  return <section className="gf-gantt" aria-labelledby="schedule-gantt-title">
    <div className="gf-gantt-meta"><div><h2 id="schedule-gantt-title">Seven-day placement</h2><p className="gf-muted">Calculated {formatDateTime(report.calculated_at)} · {report.duration_source === 'execution_timeout' ? 'Intervals use each task execution timeout.' : 'Intervals use the calculated duration.'}</p></div><span className="gf-gantt-count">{segments.length} displayed segments · {report.conflicts?.length ?? 0} conflicts</span></div>
    {stale && <p className="gf-stale-warning" role="status">This projection is older than one hour. The last successful snapshot is shown.</p>}
    <div className="gf-gantt-scroll">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Seven-day schedule placement by runner or runner pool">
        <text x={timelineLeft} y="16" className="gf-gantt-axis-label">{formatDateTime(report.window_start)}</text>
        <text x={width - 10} y="16" textAnchor="end" className="gf-gantt-axis-label">{formatDateTime(report.window_end)}</text>
        {lanes.map((lane, laneIndex) => {
          const y = 28 + laneIndex * rowHeight
          return <g key={lane.id}><text x="8" y={y + 17} className="gf-gantt-lane-label">{lane.label}</text><line x1={timelineLeft} y1={y + 12} x2={width - 10} y2={y + 12} className="gf-gantt-lane-line" />{lane.segments.map((segment) => { const position = projectionSegmentPercent(segment, report); const x = timelineLeft + (position.left / 100) * timelineWidth; const segmentWidth = Math.max(2, (position.width / 100) * timelineWidth); const details = segmentDetails(segment); return <g key={segment.id}><title>{details}</title><rect x={x} y={y} width={segmentWidth} height="24" rx="4" className={`gf-gantt-segment${segment.conflicted ? ' is-conflicted' : ''}`} tabIndex={0} aria-label={details} strokeDasharray={segment.conflicted ? '4 2' : undefined} /><text x={x + 4} y={y + 17} className="gf-gantt-segment-mark" aria-hidden="true">{segment.conflicted ? '!' : ''}</text></g> })}</g>
        })}
      </svg>
    </div>
    <p className="gf-gantt-legend"><span className="gf-gantt-legend-conflict" aria-hidden="true">!</span> Conflict markers are also listed below; placement lanes show pinned runners or eligible runner pools.</p>
    <ul className="gf-sr-only" aria-label="Projected schedule segments">{segments.map((segment) => <li key={segment.id}>{segmentDetails(segment)} · {segment.occurrence_count} occurrences</li>)}</ul>
    <section className="gf-gantt-conflicts" aria-labelledby="schedule-conflicts-title"><h2 id="schedule-conflicts-title">Exclusive-resource conflicts</h2>{report.conflicts?.length ? <ol>{report.conflicts.map((conflict) => <li key={conflict.id}><strong>{conflict.resource_name}</strong><span> · {formatDateTime(conflict.start_at)}–{formatDateTime(conflict.end_at)}</span><ul>{conflict.occurrences.map((occurrence) => <li key={occurrence.id}>{occurrence.task_name} · {occurrence.schedule_name} · {occurrence.lane_label} · {formatDateTime(occurrence.start_at)}–{formatDateTime(occurrence.end_at)}</li>)}</ul></li>)}</ol> : <p className="gf-muted">No exclusive-resource conflicts found in this window.</p>}</section>
  </section>
}

export function ScheduleGanttPage() {
  const query = useQuery({ queryKey: ['schedule-projection'], queryFn: ({ signal }) => api.get<ScheduleProjection>('/api/v1/schedule-projection', undefined, signal), refetchInterval: 30_000 })
  return <main className="gf-content"><PageHeader title="Scheduling Gantt" description="Seven-day cron projection by runner placement and exclusive resource." refresh={<QueryRefresh query={query} />} /><QueryState query={query} empty="The schedule projection is not available yet.">{(report) => <SchedulingGantt report={report} />}</QueryState></main>
}
