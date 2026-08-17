import { QueryClient, QueryClientProvider, type UseQueryResult } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { Button, EmptyState, ErrorState, LoadingState } from './components'
import { describeError } from './errors'

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnMount: 'always', refetchOnWindowFocus: true, refetchIntervalInBackground: false } },
})

export function QueryProvider({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

export function QueryState<T>({ query, children, empty = 'Nothing to show yet.' }: { query: UseQueryResult<T>; children: (data: T) => ReactNode; empty?: string }) {
  const [online, setOnline] = useState(() => typeof navigator === 'undefined' || navigator.onLine)
  useEffect(() => {
    const onOnline = () => setOnline(true)
    const onOffline = () => setOnline(false)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
    return () => { window.removeEventListener('online', onOnline); window.removeEventListener('offline', onOffline) }
  }, [])
  if (query.isPending) return <LoadingState />
  if (query.isError && query.data === undefined) { const error = describeError(query.error); return <ErrorState title={error.title} message={`${error.message}${error.correlationId ? ` (Correlation ID: ${error.correlationId})` : ''}`} onRetry={error.retryable ? () => query.refetch() : undefined} /> }
  if (query.data == null || (Array.isArray(query.data) && query.data.length === 0)) return <EmptyState title="No results">{empty}</EmptyState>
  const updated = query.dataUpdatedAt ? new Date(query.dataUpdatedAt).toLocaleTimeString() : ''
  const status = query.isFetching ? 'Refreshing…' : !online ? 'Offline; showing last successful data' : query.isStale ? `Stale; last successful refresh ${updated}` : `Last successful refresh ${updated}`
  return <><div className="gf-query-status" aria-live="polite"><span>{status}</span><Button variant="secondary" busy={query.isFetching} onClick={() => query.refetch()}>Refresh</Button></div>{children(query.data)}</>
}

export function queryHasData<T>(query: Pick<UseQueryResult<T>, 'data' | 'isPending' | 'isError'>): boolean {
  return !query.isPending && !query.isError && query.data !== undefined
}

export function useDebouncedValue<T>(value: T, delay = 250): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => { const timer = window.setTimeout(() => setDebounced(value), delay); return () => window.clearTimeout(timer) }, [value, delay])
  return debounced
}
