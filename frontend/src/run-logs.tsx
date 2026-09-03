import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from './components'
import { LogOutput } from './safe'
import { mergeChunks, type LogChunk } from './log-stream'

export const MAX_VISIBLE_LOG_CHARS = 200_000

export function isTerminalRunState(state: string) {
  return ['SUCCEEDED', 'FAILED', 'TIMED_OUT', 'CANCELLED', 'UNKNOWN'].includes(state.toUpperCase())
}

type StreamState = { chunks: LogChunk[]; reconnecting: boolean; stopped: boolean; error?: string; gap: boolean }

export function logStreamUrl(runId: string, stream: string, after: number) {
  return `/api/v1/runs/${encodeURIComponent(runId)}/logs?stream=${encodeURIComponent(stream)}&after=${after}`
}

export function logDownloadUrl(runId: string, stream: string) {
  return `/api/v1/runs/${encodeURIComponent(runId)}/logs/download?stream=${encodeURIComponent(stream)}`
}

export function useLogStream(runId: string, stream: 'stdout' | 'stderr', enabled = true, terminal = false) {
  const [state, setState] = useState<StreamState>({ chunks: [], reconnecting: false, stopped: false, gap: false })
  const [paused, setPaused] = useState(false)
  const [generation, setGeneration] = useState(0)
  const lastSequence = useRef(0)
  useEffect(() => {
    if (!enabled || paused) return
    const controller = new AbortController()
    let stopped = false
    let timer: number | undefined
    const connect = async () => {
      const last = lastSequence.current
      try {
        setState((current) => ({ ...current, reconnecting: current.chunks.length > 0, stopped: terminal, error: undefined }))
        const response = await fetch(logStreamUrl(runId, stream, last), { credentials: 'include', signal: controller.signal })
        if (!response.ok || !response.body) throw new Error(`Log stream failed (${response.status})`)
        const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''
        while (!stopped) {
          const next = await reader.read(); if (next.done) break
          buffer += decoder.decode(next.value, { stream: true })
          const lines = buffer.split('\n'); buffer = lines.pop() ?? ''
          const chunks = lines.filter(Boolean).map((line) => { try { const parsed = JSON.parse(line) as { sequence: number; text: string }; return parsed } catch { return { sequence: lastSequence.current + 1, text: line } } })
          if (chunks.length) setState((current) => { const merged = mergeChunks(current.chunks, chunks, MAX_VISIBLE_LOG_CHARS); lastSequence.current = merged.lastSequence; return { ...current, chunks: merged.chunks, gap: merged.gap, reconnecting: false } })
        }
        if (!stopped) {
          if (terminal) setState((current) => ({ ...current, reconnecting: false, stopped: true }))
          else timer = window.setTimeout(connect, 1000)
        }
      } catch (cause) { if (!stopped && !controller.signal.aborted) { if (terminal) setState((current) => ({ ...current, reconnecting: false, stopped: true, error: cause instanceof Error ? cause.message : 'Log stream failed' })); else { setState((current) => ({ ...current, reconnecting: true, error: cause instanceof Error ? cause.message : 'Log stream failed' })); timer = window.setTimeout(connect, 1500) } } }
    }
    connect()
    return () => { stopped = true; controller.abort(); if (timer) window.clearTimeout(timer) }
  // The generation forces a fresh stream after an explicit reconnect.
  }, [runId, stream, enabled, paused, generation, terminal])
  const text = useMemo(() => state.chunks.map((chunk) => chunk.text).join(''), [state.chunks])
  return { ...state, text, paused, pause: () => setPaused(true), resume: () => setPaused(false), reconnect: () => { setPaused(false); setState((current) => ({ ...current, reconnecting: true })); setGeneration((value) => value + 1) } }
}

export function LiveLogPanel({ runId, stream, terminal = false }: { runId: string; stream: 'stdout' | 'stderr'; terminal?: boolean }) {
  const log = useLogStream(runId, stream, true, terminal)
  const stopped = terminal || log.stopped
  return <section className="gf-card-panel"><div className="gf-log-toolbar"><h2>{stream}</h2><output>{stopped ? 'Stopped' : log.gap ? 'Gap detected' : 'Live View'}</output><Button variant="secondary" disabled={stopped} onClick={log.paused ? log.resume : log.pause}>{log.paused ? 'Resume' : 'Pause'}</Button><Button variant="secondary" disabled={stopped} onClick={log.reconnect}>Reconnect</Button><a className="gf-button gf-button-secondary" title="Download the complete log file" href={logDownloadUrl(runId, stream)} download={`${runId}-${stream}.log`}>Download source</a></div>{log.error && <p className="gf-form-error" role="alert">{log.error}</p>}<LogOutput stream={stream} value={log.text} /></section>
}
