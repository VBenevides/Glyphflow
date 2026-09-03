import { useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { Button } from './components'
import { LogOutput } from './safe'
import { mergeChunks, type LogChunk } from './log-stream'

export const MAX_VISIBLE_LOG_CHARS = 200_000

export function isTerminalRunState(state: string) {
  return ['SUCCEEDED', 'FAILED', 'TIMED_OUT', 'CANCELLED', 'UNKNOWN'].includes(state.toUpperCase())
}

type StreamState = { chunks: LogChunk[]; reconnecting: boolean; stopped: boolean; error?: string; gap: boolean }
type StreamStateSetter = Dispatch<SetStateAction<StreamState>>

function parseLogLine(line: string, fallbackSequence: () => number): LogChunk {
  try {
    return JSON.parse(line) as LogChunk
  } catch {
    return { sequence: fallbackSequence(), text: line }
  }
}

async function consumeLogStream(response: Response, isStopped: () => boolean, onChunks: (chunks: LogChunk[]) => void, fallbackSequence: () => number) {
  if (!response.body) throw new Error(`Log stream failed (${response.status})`)
  const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''
  while (!isStopped()) {
    const next = await reader.read(); if (next.done) break
    buffer += decoder.decode(next.value, { stream: true })
    const lines = buffer.split('\n'); buffer = lines.pop() ?? ''
    const chunks = lines.filter(Boolean).map((line) => parseLogLine(line, fallbackSequence))
    if (chunks.length) onChunks(chunks)
  }
}

function assertLogStreamResponse(response: Response) {
  if (!response.ok || !response.body) throw new Error(`Log stream failed (${response.status})`)
}

function finishLogStream(terminal: boolean, stopped: boolean, setState: StreamStateSetter, schedule: (delay: number) => void) {
  if (stopped) return
  if (terminal) {
    setState((current) => ({ ...current, reconnecting: false, stopped: true }))
    return
  }
  schedule(1000)
}

function failLogStream(cause: unknown, terminal: boolean, stopped: boolean, aborted: boolean, setState: StreamStateSetter, schedule: (delay: number) => void) {
  if (stopped || aborted) return
  const error = cause instanceof Error ? cause.message : 'Log stream failed'
  if (terminal) {
    setState((current) => ({ ...current, reconnecting: false, stopped: true, error }))
    return
  }
  setState((current) => ({ ...current, reconnecting: true, error }))
  schedule(1500)
}

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
        assertLogStreamResponse(response)
        await consumeLogStream(response, () => stopped, (chunks) => setState((current) => { const merged = mergeChunks(current.chunks, chunks, MAX_VISIBLE_LOG_CHARS); lastSequence.current = merged.lastSequence; return { ...current, chunks: merged.chunks, gap: merged.gap, reconnecting: false } }), () => lastSequence.current + 1)
        finishLogStream(terminal, stopped, setState, (delay) => { timer = window.setTimeout(connect, delay) })
      } catch (cause) { failLogStream(cause, terminal, stopped, controller.signal.aborted, setState, (delay) => { timer = window.setTimeout(connect, delay) }) }
    }
    connect()
    return () => { stopped = true; controller.abort(); if (timer) window.clearTimeout(timer) }
  // The generation forces a fresh stream after an explicit reconnect.
  }, [runId, stream, enabled, paused, generation, terminal])
  const text = useMemo(() => state.chunks.map((chunk) => chunk.text).join(''), [state.chunks])
  return { ...state, text, paused, pause: () => setPaused(true), resume: () => setPaused(false), reconnect: () => { setPaused(false); setState((current) => ({ ...current, reconnecting: true })); setGeneration((value) => value + 1) } }
}

export function LiveLogPanel({ runId, stream, terminal = false }: Readonly<{ runId: string; stream: 'stdout' | 'stderr'; terminal?: boolean }>) {
  const log = useLogStream(runId, stream, true, terminal)
  const stopped = terminal || log.stopped
  let statusLabel = 'Live View'
  if (log.gap) statusLabel = 'Gap detected'
  if (stopped) statusLabel = 'Stopped'
  return <section className="gf-card-panel"><div className="gf-log-toolbar"><h2>{stream}</h2><output>{statusLabel}</output><Button variant="secondary" disabled={stopped} onClick={log.paused ? log.resume : log.pause}>{log.paused ? 'Resume' : 'Pause'}</Button><Button variant="secondary" disabled={stopped} onClick={log.reconnect}>Reconnect</Button><a className="gf-button gf-button-secondary" title="Download the complete log file" href={logDownloadUrl(runId, stream)} download={`${runId}-${stream}.log`}>Download source</a></div>{log.error && <p className="gf-form-error" role="alert">{log.error}</p>}<LogOutput stream={stream} value={log.text} /></section>
}
