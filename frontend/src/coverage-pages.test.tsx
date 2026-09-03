import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { renderToStaticMarkup } from 'react-dom/server'
import { act, type ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AccountPage } from './account-pages'
import { AuthenticationSettingsPage, ExecutionStatusPage, RoleManagementPage, SecretsPage, SessionManagementPage, SsoSettingsPage, UserManagementPage } from './admin-pages'
import { AuditPage } from './audit-page'
import { LoginPage, OidcCallbackPage, RegistrationPage } from './auth-pages'
import { DashboardPage } from './dashboard'
import { EnrollmentPage } from './enrollment-page'
import { GlobalVariableInput } from './global-variable-input'
import { GlobalVariablesPage } from './global-variables-page'
import { LiveLogPanel } from './run-logs'
import { ManualRunPage, RunDetailPage, RunInventoryPage } from './run-pages'
import { ResourceDetailPage, ResourceInventoryPage } from './resource-pages'
import { RunnerPoolsPage } from './runner-pools-page'
import { RunnerDetailPage, RunnerInventoryPage } from './runner-pages'
import { ScheduleEditorPage, ScheduleInventoryPage } from './schedule-pages'
import { SchedulingGantt } from './schedule-gantt'
import { Shell } from './shell'
import { AppRoutes } from './routes'
import { SystemMetricsPage } from './system-metrics-page'
import { TaskEditorPage } from './task-editor'
import { TaskDetailPage, TaskInventoryPage } from './task-pages'
import { TaskPicker } from './task-picker'
import { UserDetailsPage } from './user-details-page'
import { ApiError, api } from './api'

const authFixture = vi.hoisted(() => {
  const permissions = ['users.read', 'users.manage', 'roles.read', 'roles.manage', 'sso.read', 'sso.manage', 'secrets.read', 'secrets.manage', 'auth.settings.manage', 'tasks.read', 'tasks.manage', 'runs.read', 'runs.execute', 'runs.cancel', 'runs.retry', 'logs.read', 'resources.read', 'resources.manage', 'runners.read', 'runners.manage', 'audit.read', 'system.metrics.read', 'system.deadletter.read', 'system.deadletter.manage']
  return {
    fullPermissions: permissions,
    permissions: [...permissions],
    profile: { id: 'user-1', username: 'admin', email: 'admin@example.com', displayName: 'Admin', permissions } as { id: string; username: string; email: string; displayName: string; permissions: string[] } | null,
    config: { brand: 'Glyphflow', passwordLogin: true, registration: true, requireUserApproval: false, oidc: true, csrfCookie: 'glyphflow_csrf', lockdownScheduler: false, defaultRoleId: 'admin' },
  }
})

vi.mock('./auth', async () => {
  const actual = await vi.importActual<typeof import('./auth')>('./auth')
  return {
    ...actual,
    useAuth: () => ({
      config: authFixture.config,
      profile: authFixture.profile,
      permissions: authFixture.permissions,
      loading: false,
      error: null,
      restore: () => undefined,
      setProfile: () => undefined,
      setConfig: () => undefined,
    }),
  }
})

const task = { id: 'task-1', name: 'Nightly task', command: ['echo', 'hello'], workingDirectory: '/tmp', runnerPool: 'default', pool: 'default', enabled: true, resources: ['db'], environment: { MODE: 'test' }, selectors: { platform: 'linux' }, secretReferences: { TOKEN: 'secret-1' }, durationSeconds: 60, maxAttempts: 2, ambiguityPolicy: 'RETRY' }
const schedule = { id: 'schedule-1', name: 'Nightly schedule', taskId: 'task-1', expression: '0 * * * *', timezone: '0', misfirePolicy: 'RUN_UP_TO_N', catchupLimit: 2, startDeadlineSeconds: 60, concurrencyPolicy: 'ALLOW', maxConcurrentRuns: 2, enabled: true, nextFireAt: '2026-09-03T01:00:00Z' }
const runner = { id: 'runner-1', name: 'Runner one', poolId: 'default', pool: 'default', desiredState: 'ENABLED', observedState: 'ONLINE', capacity: 2, activeCount: 1, heartbeatAt: '2026-09-03T01:59:30Z', platform: 'linux', architecture: 'amd64', natsEndpoint: 'nats://localhost:4222', controlPlaneUrl: 'http://localhost:8080', currentMetrics: { sampledAt: '2026-09-03T01:59:00Z', cpuPercent: 12, memoryPercent: 24, memoryUsedBytes: 10, memoryTotalBytes: 100 } }
const run = { id: 'run-1', state: 'RUNNING', taskId: 'task-1', taskVersionId: 'task-version-1', scheduleId: 'schedule-1', scheduleVersionId: 'schedule-version-1', taskName: task.name, trigger: 'SCHEDULE', runner: runner.id, attempt: 1, exitCode: 0, exitCodeMeaning: 'Success', maxMemoryUsedBytes: 1024 * 1024, averageMemoryUsedBytes: 512 * 1024, scheduledFor: '2026-09-03T01:00:00Z', attempts: [{ id: 'attempt-1', attemptNumber: 1, state: 'RUNNING', runnerId: runner.id, runnerSessionId: 'session-1', fencingToken: 2, dispatchedAt: '2026-09-03T01:00:00Z', startedAt: '2026-09-03T01:00:01Z' }], events: [{ eventId: 'event-1', attemptId: 'attempt-1', eventKind: 'started', stateSequence: 1, reportedAt: '2026-09-03T01:00:01Z', payload: { source: 'test' } }], sessions: [{ id: 'session-1', runnerId: runner.id, bootId: 'boot-1', connectedAt: '2026-09-03T00:59:00Z', lastHeartbeatAt: '2026-09-03T01:59:00Z' }], leases: [{ id: 'lease-1', resourceId: 'resource-1', resourceName: 'Database', state: 'ACTIVE', fencingToken: 3, expiresAt: '2026-09-03T02:00:00Z' }], cancellation: { state: 'NONE', reason: 'none' }, logGaps: [{ stream: 'stdout', fromSequence: 3, toSequence: 4 }] }
const resource = { id: 'resource-1', name: 'Database', kind: 'exclusive', enabled: true, holder: run.id, expiresAt: '2026-09-03T02:00:00Z', fencingToken: 3 }
const user = { id: 'user-2', username: 'user@example.com', email: 'user@example.com', displayName: 'User', status: 'active', roles: ['user'], roleSources: ['manual'], permissions: ['tasks.read'], loginMethods: ['password'], sessions: [{ id: 'session-2', current: false, userAgent: 'Chrome on Linux', ipAddress: '127.0.0.1', lastSeenAt: '2026-09-03T01:00:00Z', expiresAt: '2026-09-04T01:00:00Z' }], identities: [{ id: 'identity-1', provider: 'example', email: 'user@example.com' }] }
const page = (items: unknown[]) => ({ items, page: 1, limit: 10, total: items.length, pages: 1 })

beforeEach(() => {
  authFixture.permissions = [...authFixture.fullPermissions]
  authFixture.profile = { id: 'user-1', username: 'admin', email: 'admin@example.com', displayName: 'Admin', permissions: authFixture.permissions }
  authFixture.config = { ...authFixture.config, passwordLogin: true, registration: true, requireUserApproval: false, oidc: true, lockdownScheduler: false }
})

function seed(client: QueryClient) {
  client.setQueryData(['tasks', false, '', '', 1, 10], page([task]))
  client.setQueryData(['task-filter-options', false], page([task]))
  client.setQueryData(['task-summary'], { total: 1, disabled: 0, archived: 0 })
  client.setQueryData(['task', 'task-1'], task)
  client.setQueryData(['task-versions', 'task-1'], [{ id: 'task-version-1', version: 1, command: task.command, pool: task.pool, resources: task.resources, durationSeconds: 60 }])
  client.setQueryData(['task-edit', 'task-1'], task)
  client.setQueryData(['runner-pools'], page([{ id: 'default', name: 'Default', description: 'Default pool', enabled: true }]))
  client.setQueryData(['runner-pools', 1, 10], page([{ id: 'default', name: 'Default', description: 'Default pool', enabled: true }]))
  client.setQueryData(['runners'], page([runner]))
  client.setQueryData(['global-variable-options'], page([{ id: 'var-1', name: 'MODE', value: 'test', references: 1 }]))
  client.setQueryData(['task-resource-options'], page([resource]))
  client.setQueryData(['task-secret-options'], [{ id: 'secret-1', name: 'Token', status: 'VALID', tasks: [], canDelete: true }])
  client.setQueryData(['global-variables', 1, 10], page([{ id: 'var-1', name: 'MODE', value: 'test', references: 1 }]))
  client.setQueryData(['resources', 1, 10], page([resource]))
  client.setQueryData(['resource-summary'], { total: 1, exclusive: 1 })
  client.setQueryData(['resource', 'resource-1'], resource)
  client.setQueryData(['runs', { task: '', runner: '', state: '', trigger: '', from: '', to: '' }, 1, 10], page([run]))
  client.setQueryData(['run', 'run-1'], run)
  client.setQueryData(['runners', false, '', 1, 10], page([runner]))
  client.setQueryData(['runner-filter-options', false], page([runner]))
  client.setQueryData(['runner-summary'], { total: 1, disabled: 0, offline: 0, archived: 0 })
  client.setQueryData(['runner', 'runner-1'], runner)
  client.setQueryData(['runner-runs', 'runner-1', 1, 10], page([run]))
  client.setQueryData(['runner-metrics', 'runner-1', '24h'], { items: [runner.currentMetrics], from: '2026-09-02T01:00:00Z', to: '2026-09-03T01:00:00Z' })
  client.setQueryData(['schedules', 1, 10, ''], page([schedule]))
  client.setQueryData(['schedule-summary'], { total: 1, disabled: 0 })
  client.setQueryData(['schedule-edit', 'schedule-1'], schedule)
  client.setQueryData(['schedule-projection'], { available: true, calculatedAt: '2026-09-03T01:00:00Z', windowStart: '2026-09-03T00:00:00Z', windowEnd: '2026-09-10T00:00:00Z', segments: [], conflicts: [] })
  client.setQueryData(['admin-users', 1, 10, '', '', ''], page([user]))
  client.setQueryData(['admin-user-filter-options'], page([user]))
  client.setQueryData(['admin-pending-users'], page([]))
  client.setQueryData(['admin-user-role-options'], [{ id: 'admin', name: 'Administrator', permissions: ['users.manage'], system: true }])
  client.setQueryData(['admin-sessions', 1, 10, ''], page([{ id: 'admin-session', userId: user.id, userEmail: user.email, lastSeenAt: '2026-09-03T01:00:00Z' }]))
  client.setQueryData(['admin-session-filter-options'], page([{ id: 'admin-session', userId: user.id, userEmail: user.email }]))
  client.setQueryData(['admin-roles'], [{ id: 'admin', name: 'Administrator', permissions: ['users.manage'], system: true }])
  client.setQueryData(['admin-sso'], [{ key: 'example', name: 'Example', issuer: 'https://issuer.example', enabled: true }])
  client.setQueryData(['admin-role-options'], [{ id: 'admin', name: 'Administrator', permissions: ['users.manage'], system: true }])
  client.setQueryData(['admin-secrets'], [{ id: 'secret-1', name: 'Token', status: 'VALID', tasks: [{ id: task.id, name: task.name }], canDelete: false, lastValidatedAt: '2026-09-03T01:00:00Z' }])
  client.setQueryData(['me-account'], { ...authFixture.profile, identities: user.identities, sessions: user.sessions })
  client.setQueryData(['execution-status'], [{ code: 0, meaning: 'Success', isSystem: true }, { code: 1, meaning: 'Failure', isSystem: false }])
  client.setQueryData(['admin-user', user.id], user)
  client.setQueryData(['audit', { actor: '', action: '', target: '', result: '', correlation: '', from: '', to: '', excludeAuditReads: true, excludeRunLogs: true, excludeGet: true }, 1, 10], page([{ id: 'audit-1', actor: user.id, actorName: user.displayName, actorEmail: user.email, action: 'POST', description: 'Created task', target: '/api/v1/tasks', result: 'success', createdAt: '2026-09-03T01:00:00Z', correlationId: 'corr-1', input: { name: task.name }, output: { id: task.id } }]))
  client.setQueryData(['system-metrics'], { generatedAt: '2026-09-03T01:00:00Z', ready: true, metrics: { requests: 1 }, signals: { queueLagSeconds: 0, deadLetters: { open: 0, oldestAgeSeconds: 0 }, stuckRuns: 0, disk: { freeBytes: 1024 * 1024, freePercent: 80 } }, alerts: [] })
  client.setQueryData(['dead-letters', 1], page([{ id: 'dead-1', stream: 'events', consumer: 'worker', subject: 'runs', messageId: 'msg-1', state: 'OPEN', attempts: 1, firstFailedAt: '2026-09-03T00:00:00Z', lastFailedAt: '2026-09-03T01:00:00Z', error: 'temporary' }]))
  for (const key of ['runs', 'schedules', 'runners']) client.setQueryData(['dashboard', key], page([{ id: `${key}-1`, name: key, state: 'ACTIVE', status: 'active' }]))
  client.setQueryData(['dashboard', 'audit'], [{ id: 'audit-1', action: 'POST', description: 'Created task', result: 'success' }])
  client.setQueryData(['dashboard', 'secrets'], [{ id: 'secret-1', name: 'Token', status: 'VALID' }])
  client.setQueryData(['dashboard-schedule-projection'], { available: true, calculatedAt: '2026-09-03T01:00:00Z', conflicts: [] })
}

function renderPage(element: ReactElement, path: string, routePath = '*', override?: (client: QueryClient) => void) {
  const storage = { getItem: () => null, setItem: () => undefined }
  Object.defineProperty(window, 'localStorage', { configurable: true, value: storage })
  Object.defineProperty(window, 'sessionStorage', { configurable: true, value: storage })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  seed(client)
  override?.(client)
  return renderToStaticMarkup(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><Routes><Route path={routePath} element={element} /></Routes></MemoryRouter></QueryClientProvider>)
}

async function mountPage(element: ReactElement, path: string, routePath = '*', override?: (client: QueryClient) => void) {
  const storage = { getItem: () => null, setItem: () => undefined }
  Object.defineProperty(window, 'localStorage', { configurable: true, value: storage })
  Object.defineProperty(window, 'sessionStorage', { configurable: true, value: storage })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  seed(client)
  override?.(client)
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><Routes><Route path={routePath} element={element} /></Routes></MemoryRouter></QueryClientProvider>)
    await Promise.resolve()
  })
  return { host, root }
}

async function clickButton(host: HTMLElement, label: string) {
  const button = [...document.body.querySelectorAll('button')].find((item) => item.textContent?.trim() === label || item.getAttribute('aria-label') === label)
  if (!button) throw new Error(`button not found: ${label}`)
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await Promise.resolve()
  })
}

async function submitForm(host: HTMLElement) {
  const form = document.body.querySelector('form')
  if (!form) throw new Error('form not found')
  await act(async () => {
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
  })
}

function setField(host: HTMLElement, selector: string, value: string) {
  const field = document.body.querySelector(selector) as HTMLInputElement | HTMLSelectElement | null
  if (!field) throw new Error(`field not found: ${selector}`)
  const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(field), 'value')?.set
  setter?.call(field, value)
  field.dispatchEvent(new Event(field instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }))
}

function setInputAt(host: HTMLElement, index: number, value: string) {
  const input = [...document.body.querySelectorAll('input')][index]
  if (!input) throw new Error(`input not found: ${index}`)
  const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value')?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

async function clickLastButton(host: HTMLElement, label: string) {
  const buttons = [...document.body.querySelectorAll('button')].filter((item) => item.textContent?.trim() === label || item.getAttribute('aria-label') === label)
  const button = buttons[buttons.length - 1]
  if (!button) throw new Error(`button not found: ${label}`)
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await Promise.resolve()
  })
}

function mockApi() {
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    if (path.includes('/admin/roles') || path.includes('/admin/auth/providers') || path.includes('/admin/execution-status')) return [] as never
    return page([]) as never
  })
  vi.spyOn(api, 'post').mockResolvedValue({ id: 'new-id', artifact: 'AQ==', runner_id: 'runner-new' } as never)
  vi.spyOn(api, 'put').mockResolvedValue({} as never)
  vi.spyOn(api, 'delete').mockResolvedValue(undefined as never)
  vi.spyOn(api, 'request').mockResolvedValue({ artifact: 'AQ==', runner_id: 'runner-new' } as never)
}

afterEach(() => vi.restoreAllMocks())

describe('data-backed page coverage', () => {
  it('renders the authenticated application pages with populated data', () => {
    const markup = [
      renderPage(<ShellCoverage />, '/'),
      renderPage(<AppRoutes />, '/tasks'),
      renderPage(<TaskInventoryPage />, '/tasks'),
      renderPage(<TaskDetailPage />, '/tasks/task-1', '/tasks/:taskId'),
      renderPage(<TaskEditorPage />, '/tasks/task-1/edit', '/tasks/:taskId/edit'),
      renderPage(<ScheduleInventoryPage />, '/schedules'),
      renderPage(<ScheduleInventoryPage />, '/schedules?tab=gantt'),
      renderPage(<ScheduleEditorPage />, '/schedules/schedule-1/edit', '/schedules/:scheduleId/edit'),
      renderPage(<RunInventoryPage />, '/runs'),
      renderPage(<RunDetailPage />, '/runs/run-1', '/runs/:runId'),
      renderPage(<ManualRunPage />, '/runs/execute'),
      renderPage(<RunnerInventoryPage />, '/runners'),
      renderPage(<RunnerDetailPage />, '/runners/runner-1', '/runners/:runnerId'),
      renderPage(<RunnerPoolsPage />, '/runners/pools'),
      renderPage(<EnrollmentPage />, '/runners/enroll?runner=Runner&pool=default'),
      renderPage(<ResourceInventoryPage />, '/resources'),
      renderPage(<ResourceDetailPage />, '/resources/resource-1', '/resources/:resourceId'),
      renderPage(<GlobalVariablesPage />, '/global-variables'),
      renderPage(<AuditPage />, '/audit'),
      renderPage(<DashboardPage />, '/'),
      renderPage(<AccountPage />, '/account'),
      renderPage(<UserManagementPage />, '/admin/users'),
      renderPage(<SessionManagementPage />, '/admin/users/sessions'),
      renderPage(<UserDetailsPage userId={user.id} self={false} />, '/admin/users/user-2'),
      renderPage(<RoleManagementPage />, '/admin/roles'),
      renderPage(<SsoSettingsPage />, '/admin/sso'),
      renderPage(<SecretsPage />, '/admin/secrets'),
      renderPage(<AuthenticationSettingsPage />, '/admin/auth'),
      renderPage(<ExecutionStatusPage />, '/admin/execution-status'),
      renderPage(<SystemMetricsPage />, '/admin/system'),
      renderPage(<SchedulingGantt report={{ available: true, calculatedAt: '2026-09-03T01:00:00Z', windowStart: '2026-09-03T00:00:00Z', windowEnd: '2026-09-10T00:00:00Z', segments: [], conflicts: [] }} />, '/schedules'),
      renderPage(<TaskPicker value={task.id} onChange={() => undefined} />, '/tasks'),
      renderPage(<GlobalVariableInput value="$ENV:MODE" variables={[{ id: 'var-1', name: 'MODE', value: 'test' }]} onChange={() => undefined} />, '/tasks'),
      renderPage(<LiveLogPanel runId={run.id} stream="stdout" terminal />, '/runs/run-1'),
    ].join('\n')
    expect(markup).toContain('Nightly task')
    expect(markup).toContain('Single sign-on')
    expect(markup).toContain('Operational alerts')
  })

  it('renders public authentication states', () => {
    expect(renderPage(<LoginPage />, '/login?redirect=%2Fruns')).toContain('Sign in')
    expect(renderPage(<RegistrationPage />, '/register')).toContain('Create account')
    expect(renderPage(<OidcCallbackPage />, '/auth/oidc/callback?code=one&state=two')).toContain('Completing sign-in')
  })

  it('covers alternate authentication, account, and task states', async () => {
    authFixture.config = { ...authFixture.config, passwordLogin: false, registration: false, oidc: false }
    expect(renderPage(<LoginPage />, '/login')).toContain('No sign-in methods are configured')
    expect(renderPage(<RegistrationPage />, '/register')).toContain('Registration is disabled')
    expect(renderPage(<AccountPage />, '/account/identities')).toContain('Single sign-on is disabled')

    authFixture.config = { ...authFixture.config, passwordLogin: true, registration: true, oidc: true }
    vi.spyOn(api, 'get').mockImplementation(async (path) => path.includes('/oidc/providers') ? [{ key: 'github', name: 'GitHub', issuer: 'https://github.example' }] as never : page([]) as never)
    vi.spyOn(api, 'post').mockResolvedValue({} as never)
    let mounted = await mountPage(<LoginPage />, '/login?redirect=%2Faccount')
    await act(async () => { await Promise.resolve() })
    expect(mounted.host.textContent).toContain('Continue with GitHub')
    mounted.root.unmount(); mounted.host.remove()

    vi.restoreAllMocks()
    vi.spyOn(api, 'post').mockRejectedValue(new ApiError(403, 'pending'))
    mounted = await mountPage(<RegistrationPage />, '/register')
    await submitForm(mounted.host)
    expect(mounted.host.textContent).toContain('awaiting administrator approval')
    mounted.root.unmount(); mounted.host.remove()

    vi.restoreAllMocks()
    vi.spyOn(api, 'post').mockRejectedValue(new Error('registration failed'))
    mounted = await mountPage(<RegistrationPage />, '/register')
    await submitForm(mounted.host)
    expect(mounted.host.textContent).toContain('registration failed')
    mounted.root.unmount(); mounted.host.remove()
  })

  it('covers populated gantt and task comparison views', async () => {
    const occurrence = { id: 'segment-1', scheduleId: 'schedule-1', scheduleName: 'Nightly', scheduleVersionId: 'schedule-v1', taskId: 'task-1', taskName: 'Nightly task', taskVersionId: 'task-v1', timezone: 'UTC', laneId: 'runner-1', laneLabel: 'Runner: Runner one', startAt: '2026-09-03T01:00:00Z', endAt: '2026-09-03T02:00:00Z' }
    const report = { available: true, calculatedAt: '2026-09-02T00:00:00Z', windowStart: '2026-09-01T00:00:00Z', windowEnd: '2026-09-08T00:00:00Z', segments: [{ ...occurrence, occurrenceCount: 1, conflicted: true, exclusiveResources: [] }], conflicts: [{ id: 'conflict-1', resourceId: 'resource-1', resourceName: 'Database', startAt: occurrence.startAt, endAt: occurrence.endAt, occurrences: [occurrence] }] }
    expect(renderPage(<SchedulingGantt report={report} />, '/schedules')).toContain('This projection is older than one hour')
    let mounted = await mountPage(<SchedulingGantt report={report} />, '/schedules')
    await clickButton(mounted.host, 'Daily')
    await clickButton(mounted.host, 'By task')
    mounted.root.unmount(); mounted.host.remove()

    const versions = [{ id: 'v1', version: 1, command: ['echo', 'one'], pool: 'default', resources: ['a', 'b'], durationSeconds: 30 }, { id: 'v2', version: 2, command: ['echo', 'two'], pool: 'default', resources: [], durationSeconds: 60 }]
    expect(renderPage(<TaskDetailPage />, '/tasks/task-1', '/tasks/:taskId', (client) => client.setQueryData(['task-versions', 'task-1'], versions))).toContain('Version history')
    mounted = await mountPage(<TaskDetailPage />, '/tasks/task-1', '/tasks/:taskId', (client) => client.setQueryData(['task-versions', 'task-1'], versions))
    await clickButton(mounted.host, 'v2')
    expect(mounted.host.textContent).toContain('Changes from v1 to v2')
    mounted.root.unmount(); mounted.host.remove()
  })

  it('renders empty collection states and permission boundaries', () => {
    const empty = (element: React.ReactElement, path: string, key: unknown[], value: unknown = page([])) => renderPage(element, path, '*', (client) => client.setQueryData(key, value))
    const markup = [
      empty(<TaskInventoryPage />, '/tasks', ['tasks', false, '', '', 1, 10]),
      empty(<ScheduleInventoryPage />, '/schedules', ['schedules', 1, 10, '']),
      empty(<RunInventoryPage />, '/runs', ['runs', { task: '', runner: '', state: '', trigger: '', from: '', to: '' }, 1, 10]),
      empty(<RunnerInventoryPage />, '/runners', ['runners', false, '', 1, 10]),
      empty(<RunnerPoolsPage />, '/runners/pools', ['runner-pools', 1, 10]),
      empty(<ResourceInventoryPage />, '/resources', ['resources', 1, 10]),
      empty(<GlobalVariablesPage />, '/global-variables', ['global-variables', 1, 10]),
      empty(<AuditPage />, '/audit', ['audit', { actor: '', action: '', target: '', result: '', correlation: '', from: '', to: '', excludeAuditReads: true, excludeRunLogs: true, excludeGet: true }, 1, 10]),
      empty(<UserManagementPage />, '/admin/users', ['admin-users', 1, 10, '', '', '']),
      empty(<SessionManagementPage />, '/admin/users/sessions', ['admin-sessions', 1, 10, '']),
      empty(<RoleManagementPage />, '/admin/roles', ['admin-roles'], []),
      empty(<SsoSettingsPage />, '/admin/sso', ['admin-sso'], []),
      empty(<SecretsPage />, '/admin/secrets', ['admin-secrets'], []),
      empty(<ExecutionStatusPage />, '/admin/execution-status', ['execution-status'], []),
    ].join('\n')
    expect(markup).toContain('No matching tasks')
    expect(markup).toContain('No exit code meanings')

    authFixture.permissions = []
    expect(renderPage(<DashboardPage />, '/')).toContain('No dashboard widgets available')
    expect(renderPage(<TaskEditorPage />, '/tasks/new')).toContain('Access denied')
    expect(renderPage(<ScheduleEditorPage />, '/schedules/new')).toContain('Access denied')
    authFixture.profile = null
    expect(renderPage(<AppRoutes />, '/tasks')).toContain('Sign in required')
  })

  it('executes the page editor and administration workflows', async () => {
    mockApi()

    let mounted = await mountPage(<RoleManagementPage />, '/admin/roles')
    await clickButton(mounted.host, 'Create role')
    setField(mounted.host, '#role-name', 'operator')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<SsoSettingsPage />, '/admin/sso')
    await clickButton(mounted.host, 'Add provider')
    setField(mounted.host, '#sso-key', 'example')
    setField(mounted.host, '#sso-name', 'Example')
    setField(mounted.host, '#sso-issuer', 'https://issuer.example')
    setField(mounted.host, '#sso-secret', 'secret')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<SecretsPage />, '/admin/secrets')
    await clickButton(mounted.host, 'Create secret')
    setField(mounted.host, '#secret-name', 'token')
    setField(mounted.host, '#secret-value', 'value')
    await clickButton(mounted.host, 'Show secret')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<AuthenticationSettingsPage />, '/admin/auth')
    await clickButton(mounted.host, 'Save settings')
    await clickLastButton(mounted.host, 'Save settings')
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<ExecutionStatusPage />, '/admin/execution-status')
    await clickButton(mounted.host, 'Create exit code')
    setField(mounted.host, 'input[type="number"]', '2')
    setField(mounted.host, 'input:not([type="number"])', 'Custom')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<UserManagementPage />, '/admin/users')
    await clickButton(mounted.host, 'Create user')
    setField(mounted.host, '#admin-user-email', 'new@example.com')
    setField(mounted.host, '#admin-user-password', 'password123')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<RunnerPoolsPage />, '/runners/pools')
    await clickButton(mounted.host, 'Create pool')
    setField(mounted.host, 'input', 'new-pool')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<GlobalVariablesPage />, '/global-variables')
    await clickButton(mounted.host, 'Create variable')
    await submitForm(mounted.host)
    setInputAt(mounted.host, 0, 'MODE')
    setInputAt(mounted.host, 1, 'test')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<ResourceInventoryPage />, '/resources')
    await clickButton(mounted.host, 'Create resource')
    setField(mounted.host, 'input', 'new-resource')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<TaskEditorPage />, '/tasks/new')
    await clickButton(mounted.host, 'Add variable')
    await clickButton(mounted.host, 'Add secret')
    await clickButton(mounted.host, 'Add resource')
    await clickButton(mounted.host, 'Add row')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<ScheduleEditorPage />, '/schedules/new')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<ManualRunPage />, '/runs/execute')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<AccountPage />, '/account/password')
    setField(mounted.host, '#current-password', 'current')
    setField(mounted.host, '#new-password', 'password123')
    setField(mounted.host, '#confirm-password', 'different')
    await submitForm(mounted.host)
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<EnrollmentPage />, '/runners/enroll?runner=Runner&pool=default')
    await submitForm(mounted.host)
    await clickButton(mounted.host, 'Download runner')
    mounted.root.unmount(); mounted.host.remove()

    expect(api.post).toHaveBeenCalled()
  })

  it('runs the live log stream and shell controls', async () => {
    mockApi()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"sequence":1,"text":"hello\\n"}\nplain line\\n', { status: 200 })))

    let mounted = await mountPage(<LiveLogPanel runId={run.id} stream="stdout" terminal />, '/runs/run-1')
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)) })
    expect(mounted.host.textContent).toContain('hello')
    mounted.root.unmount(); mounted.host.remove()

    mounted = await mountPage(<Shell><DashboardPage /></Shell>, '/admin/sso')
    await clickButton(mounted.host, 'Collapse sidebar')
    await clickButton(mounted.host, 'Expand sidebar')
    await clickButton(mounted.host, 'Collapse Operations')
    await clickButton(mounted.host, 'Expand Operations')
    await clickButton(mounted.host, 'Open navigation')
    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await Promise.resolve()
    })
    mounted.root.unmount(); mounted.host.remove()
  })
})

function ShellCoverage() {
  return <Shell><AccountPage /></Shell>
}
