import { createElement } from 'react'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { LiveLogPanel, logDownloadUrl, logStreamUrl } from './run-logs'
import { mergeChunks } from './log-stream'

describe('log stream sequencing', () => {
  it('deduplicates chunks, preserves order, and detects gaps', () => {
    const merged = mergeChunks([{ sequence: 1, text: 'a' }], [{ sequence: 2, text: 'b' }, { sequence: 2, text: 'duplicate' }, { sequence: 4, text: 'd' }])
    expect(merged.chunks.map((chunk) => chunk.text)).toEqual(['a', 'b', 'd'])
    expect(merged.duplicates).toBe(1)
    expect(merged.gap).toBe(true)
    expect(merged.lastSequence).toBe(4)
    expect(logStreamUrl('run/1', 'stdout', merged.lastSequence)).toBe('/api/v1/runs/run%2F1/logs?stream=stdout&after=4')
    expect(logDownloadUrl('run/1', 'stdout')).toBe('/api/v1/runs/run%2F1/logs/download?stream=stdout')
    expect(mergeChunks([], [{ sequence: 1, text: '1234' }, { sequence: 2, text: '5678' }], 5).chunks.map((chunk) => chunk.text)).toEqual(['5678'])
  })

  it('handles plain lines and reconnect errors', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response('plain line\n', { status: 200 })).mockRejectedValueOnce(new Error('offline'))
    vi.stubGlobal('fetch', fetchMock)
    const host = document.createElement('div')
    document.body.appendChild(host)
    let root = createRoot(host)
    await act(async () => {
      root.render(createElement(LiveLogPanel, { runId: 'run-1', stream: 'stdout' }))
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
    expect(host.textContent).toContain('plain line')
    root.unmount()
    root = createRoot(host)
    await act(async () => {
      root.render(createElement(LiveLogPanel, { runId: 'run-1', stream: 'stdout' }))
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
    expect(host.textContent).toContain('offline')
    root.unmount()
    fetchMock.mockRejectedValueOnce(new Error('terminal offline'))
    root = createRoot(host)
    await act(async () => {
      root.render(createElement(LiveLogPanel, { runId: 'run-1', stream: 'stdout', terminal: true }))
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
    expect(host.textContent).toContain('terminal offline')
    root.unmount()
    host.remove()
    vi.unstubAllGlobals()
  })
})
