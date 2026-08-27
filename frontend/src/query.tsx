import { QueryClient, QueryClientProvider, type UseQueryResult } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button, EmptyState, ErrorState, LoadingState } from './components'
import { describeError } from './errors'

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnMount: 'always', refetchOnWindowFocus: true, refetchIntervalInBackground: false } },
})

export function QueryProvider({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

export function QueryRefresh<T>({ query }: { query: UseQueryResult<T> | UseQueryResult<T>[] }) {
  const queries = Array.isArray(query) ? query : [query]
  const isFetching = queries.some((item) => item.isFetching)
  const isStale = queries.some((item) => item.isStale)
  const dataUpdatedAt = Math.max(0, ...queries.map((item) => item.dataUpdatedAt))
  const [online, setOnline] = useState(() => typeof navigator === 'undefined' || navigator.onLine)
  useEffect(() => {
    const onOnline = () => setOnline(true)
    const onOffline = () => setOnline(false)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
    return () => { window.removeEventListener('online', onOnline); window.removeEventListener('offline', onOffline) }
  }, [])
  const updated = dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString(undefined, { timeZone: 'UTC' }) : '—'
  const staleLabel = isStale ? ' (Stale)' : ''
  const status = isFetching ? 'Refreshing…' : !online ? `Offline; showing data from ${updated} UTC${staleLabel}` : `Last refresh at ${updated} UTC${staleLabel}`
  return <div className="gf-query-status" aria-live="polite"><Button type="button" variant="secondary" className="gf-query-refresh-button" aria-label="Refresh" disabled={isFetching} onClick={() => { void Promise.all(queries.map((item) => item.refetch())) }}><RefreshCw size={16} className={`gf-query-spinner${isFetching ? ' is-spinning' : ''}`} aria-hidden="true" /></Button><span>{status}</span></div>
}

export function QueryState<T>({ query, children, empty = 'Nothing to show yet.' }: { query: UseQueryResult<T>; children: (data: T) => ReactNode; empty?: string }) {
  if (query.isPending) return <LoadingState />
  if (query.isError && query.data === undefined) { const error = describeError(query.error); return <ErrorState title={error.title} message={`${error.message}${error.correlationId ? ` (Correlation ID: ${error.correlationId})` : ''}`} onRetry={error.retryable ? () => query.refetch() : undefined} /> }
  if (query.data == null || (Array.isArray(query.data) && query.data.length === 0)) return <EmptyState title="No results">{empty}</EmptyState>
  return <>{children(query.data)}</>
}

export function queryHasData<T>(query: Pick<UseQueryResult<T>, 'data' | 'isPending' | 'isError'>): boolean {
  return !query.isPending && !query.isError && query.data !== undefined
}
