import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { ApiError, api, type PermissionSnapshot, type Profile, type RuntimeConfig } from './api'

export type BootstrapResult = { config: RuntimeConfig; profile: Profile | null; permissions: string[] }
export type BootstrapClient = { get<T>(path: string, query?: Record<string, string | number | boolean | null | undefined>, signal?: AbortSignal): Promise<T> }

export async function bootstrapSession(client: BootstrapClient = api, signal?: AbortSignal): Promise<BootstrapResult> {
  const config = await client.get<RuntimeConfig>('/api/v1/config', undefined, signal)
  let profile: Profile | null = null
  try {
    profile = await client.get<Profile>('/api/v1/me', undefined, signal)
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) throw error
  }
  const permissions = profile?.permissions ?? []
  return { config, profile, permissions }
}

type AuthContextValue = BootstrapResult & { loading: boolean; error: Error | null; restore: () => void; setProfile: (profile: Profile | null, permissions?: PermissionSnapshot) => void; setConfig: (config: RuntimeConfig) => void }
const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<BootstrapResult>({ config: { brand: 'Glyphflow', passwordLogin: false, registration: false, oidc: false, csrfCookie: 'glyphflow_csrf' }, profile: null, permissions: [] })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    api.onSessionExpired = () => setState((current) => ({ ...current, profile: null, permissions: [] }))
    return () => { api.onSessionExpired = undefined }
  }, [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    bootstrapSession(api, controller.signal).then((next) => setState(next)).catch((cause) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause : new Error('Startup failed'))
    }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [attempt])
  const value = useMemo<AuthContextValue>(() => ({
    ...state,
    loading,
    error,
    restore: () => setAttempt((value) => value + 1),
    setProfile: (profile, permissions) => setState((current) => ({ ...current, profile, permissions: permissions?.permissions ?? profile?.permissions ?? [] })),
    setConfig: (config) => setState((current) => ({ ...current, config })),
  }), [error, loading, state])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}
