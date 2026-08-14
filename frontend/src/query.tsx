import { QueryClient, QueryClientProvider, type UseQueryResult } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { EmptyState, ErrorState, LoadingState } from './components'

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false } },
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
  if (query.isError && query.data === undefined) return <ErrorState message={query.error instanceof Error ? query.error.message : 'Request failed'} onRetry={() => query.refetch()} />
  if (query.data == null || (Array.isArray(query.data) && query.data.length === 0)) return <EmptyState title="No results">{empty}</EmptyState>
  return <><div className="gf-query-status" aria-live="polite">{query.isFetching ? 'Refreshing…' : !online ? 'Offline; showing last successful data' : ''}</div>{children(query.data)}</>
}

export function queryHasData<T>(query: Pick<UseQueryResult<T>, 'data' | 'isPending' | 'isError'>): boolean {
  return !query.isPending && !query.isError && query.data !== undefined
}
