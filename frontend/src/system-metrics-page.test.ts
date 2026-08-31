import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { alertTone, formatBytes, signalTone, SystemMetricsView } from './system-metrics-page'

const data = {
  generatedAt: '2026-08-25T18:00:00Z',
  ready: true,
  metrics: { signature_rejects: 2 },
  signals: { queueLagSeconds: 60, deadLetters: { open: 1, oldestAgeSeconds: 30 }, stuckRuns: 0, disk: { freeBytes: 1024, freePercent: 15 } },
  alerts: [{ code: 'queue_lag', severity: 'warning', status: 'firing', value: 60, threshold: 30 }],
}

describe('system metrics page', () => {
  it('maps alert severity to metric tones and formats storage', () => {
    expect(alertTone('critical')).toBe('danger')
    expect(alertTone('warning')).toBe('warning')
    expect(signalTone(data, 'queue_lag')).toBe('warning')
    expect(formatBytes(1024)).toBe('1.0 KiB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MiB')
  })

  it('keeps the rendered page operational-only', () => {
    const html = renderToStaticMarkup(createElement(SystemMetricsView, { data }))
    expect(html).toContain('Operational alerts')
    expect(html).toContain('Low-cardinality counters')
    expect(html).not.toContain('payload')
  })

  it('shows unavailable database capacity explicitly', () => {
    const unavailable = { ...data, signals: { ...data.signals, disk: { freeBytes: 0, freePercent: 0, state: 'UNAVAILABLE', code: 'database_storage_capacity_unconfigured' } }, alerts: [{ code: 'storage_unavailable', severity: 'critical', status: 'firing', value: 0, threshold: 0 }] }
    const html = renderToStaticMarkup(createElement(SystemMetricsView, { data: unavailable }))
    expect(html).toContain('Unavailable')
    expect(html).toContain('database_storage_capacity_unconfigured')
  })
})
