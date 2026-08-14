import { describe, expect, it } from 'vitest'
import { logStreamUrl } from './run-logs'
import { mergeChunks } from './log-stream'

describe('log stream sequencing', () => {
  it('deduplicates chunks, preserves order, and detects gaps', () => {
    const merged = mergeChunks([{ sequence: 1, text: 'a' }], [{ sequence: 2, text: 'b' }, { sequence: 2, text: 'duplicate' }, { sequence: 4, text: 'd' }])
    expect(merged.chunks.map((chunk) => chunk.text)).toEqual(['a', 'b', 'd'])
    expect(merged.duplicates).toBe(1)
    expect(merged.gap).toBe(true)
    expect(merged.lastSequence).toBe(4)
    expect(logStreamUrl('run/1', 'stdout', merged.lastSequence)).toBe('/api/v1/runs/run%2F1/logs?stream=stdout&after=4')
  })
})
