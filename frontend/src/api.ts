export type QueryValue = string | number | boolean | null | undefined

export type ApiErrorBody = {
  error?: string
  code?: string
  message?: string
  fields?: Record<string, string>
}

export class ApiError extends Error {
  readonly status: number
  readonly code?: string
  readonly fields: Record<string, string>
  readonly correlationId?: string
  readonly retryAfter?: number

  constructor(status: number, body: ApiErrorBody | string, headers?: Headers) {
    const message = typeof body === 'string' ? body : body.message ?? body.error ?? `Request failed (${status})`
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = typeof body === 'string' ? undefined : body.code
    this.fields = typeof body === 'string' ? {} : body.fields ?? {}
    this.correlationId = headers?.get('X-Correlation-ID') ?? undefined
    const retryAfter = headers?.get('Retry-After')
    this.retryAfter = retryAfter ? Number(retryAfter) || undefined : undefined
  }
}

export type Page<T> = { items: T[]; page: number; limit: number; total?: number; pages?: number }
export type Identity = { id: string; provider: string; subject?: string; email?: string; createdAt?: string }
export type Profile = { id: string; username: string; displayName?: string; status?: string; email?: string; permissions?: string[]; roles?: string[]; sessions?: AuthSession[]; identities?: Identity[] }
export type PermissionSnapshot = { permissions: string[]; roles?: string[] }
export type RuntimeConfig = { brand: string; passwordLogin: boolean; registration: boolean; oidc: boolean; csrfCookie: string; defaultRoleId?: string }
export type OidcProvider = { id?: string; key: string; name?: string; issuer: string; icon?: string; enabled?: boolean; clientId?: string; secretReference?: string; claimMapping?: Record<string, string>; groupMapping?: Record<string, string> }
export type TaskVersion = { id: string; version: number; pool?: string; pinnedRunner?: string; command?: string[]; workingDirectory?: string; timeoutSeconds?: number; maxOutputBytes?: number; maxAttempts?: number; ambiguityPolicy?: string; resources?: string[]; executionSpecDigest?: string; createdAt?: string }
export type Task = { id: string; name: string; enabled?: boolean; isDeleted?: boolean; state?: string; activeVersion?: number; pool?: string; pinnedRunner?: string; command?: string[]; workingDirectory?: string; placementSelectors?: Record<string, unknown>; environment?: Record<string, string>; timeoutSeconds?: number; maxOutputBytes?: number; maxAttempts?: number; ambiguityPolicy?: string; resources?: string[]; latestRun?: Run }
export type Schedule = { id: string; name: string; taskId: string; enabled?: boolean; nextFireAt?: string; state?: string; timezone?: string; expression?: string; misfirePolicy?: string; catchupLimit?: number; deadlineSeconds?: number; concurrencyPolicy?: string; maxConcurrentRuns?: number }
export type RunAttempt = { id?: string; attemptNumber?: number; state: string; runnerId?: string; runnerSessionId?: string; fencingToken?: number; dispatchedAt?: string; startedAt?: string; finishedAt?: string }
export type RunEvent = { id?: string; eventId?: string; attemptId?: string; stateSequence?: number; eventKind?: string; reportedAt?: string; payload?: unknown }
export type RunSession = { id: string; runnerId?: string; bootId?: string; connectedAt?: string; lastHeartbeatAt?: string; disconnectedAt?: string }
export type RunLease = { id: string; resourceId?: string; resourceName?: string; state: string; fencingToken?: number; acquiredAt?: string; expiresAt?: string; releasedAt?: string }
export type RunCancellation = { state?: string; requestedAt?: string; reason?: string; acknowledgedAt?: string }
export type RunLogGap = { stream: string; fromSequence: number; toSequence: number }
export type Run = { id: string; taskId?: string; taskName?: string; state: string; placementBlocker?: string; attempt?: number; exitCode?: number; exitCodeMeaning?: string; error?: string; runner?: string; trigger?: string; scheduledFor?: string; maxMemoryUsedBytes?: number; averageMemoryUsedBytes?: number; duration?: number; attempts?: RunAttempt[]; events?: RunEvent[]; sessions?: RunSession[]; leases?: RunLease[]; cancellation?: RunCancellation; logGaps?: RunLogGap[] }
export type ExitCode = { code: number; meaning: string; isSystem?: boolean }
export type Runner = { id: string; name: string; poolId?: string; desiredState?: string; observedState?: string; pool?: string; capacity?: number; currentCapacity?: number; activeCount?: number; heartbeatAt?: string; platform?: string; architecture?: string; natsEndpoint?: string; controlPlaneUrl?: string; isArchived?: boolean; isDeleted?: boolean }
export type RunnerPool = { id: string; name: string; description?: string; enabled?: boolean; isDeleted?: boolean }
export type Resource = { id: string; name: string; kind?: string; enabled?: boolean; holder?: string; expiresAt?: string; fencingToken?: number }
export type GlobalVariable = { id: string; name: string; value: string; updatedAt?: string; references?: number }
export type AuditEvent = { id: string; actor?: string; actorName?: string; actorEmail?: string; action?: string; description?: string; target?: string; result?: string; createdAt?: string; correlationId?: string; request?: string; input?: unknown; output?: unknown; traceback?: string; before?: unknown; after?: unknown }
export type AuthSession = { id: string; createdAt?: string; expiresAt?: string; lastSeenAt?: string; current?: boolean; userAgent?: string; ipAddress?: string }
export type UserRecord = { id: string; username: string; email?: string; displayName?: string; status?: string; enabled?: boolean; systemAdmin?: boolean; loginMethods?: string[]; roles?: string[]; roleSources?: string[]; permissions?: string[]; identities?: Identity[]; sessions?: AuthSession[] }
export type RoleDefinition = { id: string; name: string; description?: string; permissions: string[]; system?: boolean; assignedUsers?: number }

const unsafeMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

function readCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined
  const match = document.cookie.split('; ').find((part) => part.startsWith(`${name}=`))
  return match ? decodeURIComponent(match.slice(name.length + 1)) : undefined
}

export function buildUrl(baseUrl: string, path: string, query?: Record<string, QueryValue>): string {
  const url = new URL(path, baseUrl || window.location.origin)
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== '') url.searchParams.set(key, String(value))
  }
  return url.toString()
}

async function readResponse(response: Response): Promise<unknown> {
  if (response.status === 204) return undefined
  const text = await response.text()
  if (!text) return undefined
  const contentType = response.headers.get('content-type') ?? ''
  if (contentType.includes('json')) {
    try { return JSON.parse(text) } catch { return text }
  }
  return text
}

export class ApiClient {
  private refreshPromise: Promise<boolean> | null = null
  onSessionExpired?: () => void

  constructor(private readonly baseUrl = '') {}

  async request<T>(path: string, init: RequestInit = {}, query?: Record<string, QueryValue>, retried = false): Promise<T> {
    const method = (init.method ?? 'GET').toUpperCase()
    const headers = new Headers(init.headers)
    if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
    if (unsafeMethods.has(method)) {
      const token = readCookie('glyphflow_csrf')
      if (token) headers.set('X-CSRF-Token', token)
    }
    const response = await fetch(buildUrl(this.baseUrl, path, query), { ...init, method, headers, credentials: 'include' })
    const body = await readResponse(response)
    if (response.status === 401 && !retried && path !== '/api/v1/auth/refresh') {
      const refreshed = await this.refresh()
      if (refreshed) return this.request<T>(path, init, query, true)
    }
    if (!response.ok) throw new ApiError(response.status, (body ?? 'Request failed') as ApiErrorBody | string, response.headers)
    return body as T
  }

  get<T>(path: string, query?: Record<string, QueryValue>, signal?: AbortSignal) {
    return this.request<T>(path, { signal }, query)
  }

  post<T>(path: string, body?: unknown, signal?: AbortSignal) {
    return this.request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body), signal })
  }

  put<T>(path: string, body: unknown, signal?: AbortSignal) {
    return this.request<T>(path, { method: 'PUT', body: JSON.stringify(body), signal })
  }

  delete<T>(path: string, signal?: AbortSignal) {
    return this.request<T>(path, { method: 'DELETE', signal })
  }

  private refresh(): Promise<boolean> {
    if (!this.refreshPromise) {
      this.refreshPromise = this.request<unknown>('/api/v1/auth/refresh', { method: 'POST' }, undefined, true)
        .then(() => true)
        .catch(() => false)
        .then((refreshed) => {
          if (!refreshed) this.onSessionExpired?.()
          return refreshed
        })
        .finally(() => { this.refreshPromise = null })
    }
    return this.refreshPromise
  }
}

export const api = new ApiClient()
