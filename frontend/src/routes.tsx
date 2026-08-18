import type { ReactNode } from 'react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { useAuth } from './auth'
import { PageHeader } from './components'
import { ForbiddenPage, LoginRequiredPage, NotFoundPage } from './feedback'
import { hasPermission, ROUTES } from './permissions'
import { Shell } from './shell'
import { TaskDetailPage, TaskInventoryPage } from './task-pages'
import { TaskEditorPage } from './task-editor'
import { ScheduleEditorPage, ScheduleInventoryPage } from './schedule-pages'
import { DashboardPage } from './dashboard'
import { ManualRunPage, RunDetailPage, RunInventoryPage } from './run-pages'
import { RunnerDetailPage, RunnerInventoryPage } from './runner-pages'
import { EnrollmentPage } from './enrollment-page'
import { ResourceDetailPage, ResourceInventoryPage } from './resource-pages'
import { AuditPage } from './audit-page'
import { AuthenticationSettingsPage, ExecutionStatusPage, RoleManagementPage, SessionManagementPage, SsoSettingsPage, UserManagementPage } from './admin-pages'
import { AccountPage } from './account-pages'
import { UserDetailsPage } from './user-details-page'
import { GlobalVariablesPage } from './global-variables-page'

function Placeholder({ title }: { title: string }) {
  return <section className="gf-content"><PageHeader title={title} description="This workspace is ready for its data view." /></section>
}

const IMPLEMENTED_ROUTE_PATHS = new Set([
  '/', '/tasks', '/schedules', '/runs', '/runners', '/runners/pools', '/resources', '/audit',
  '/global-variables', '/admin/users', '/admin/roles', '/admin/sso', '/admin/auth', '/admin/execution-status',
])

export function placeholderRoutes(routes: typeof ROUTES = ROUTES) {
  return routes.filter((route) => !IMPLEMENTED_ROUTE_PATHS.has(route.path))
}

function PermissionRoute({ permission, children }: { permission?: string; children: ReactNode }) {
  const { profile, permissions } = useAuth()
  if (!profile) return <LoginRequiredPage onLogin={() => undefined} />
  return permission && !hasPermission(permissions, permission) ? <ForbiddenPage /> : <>{children}</>
}

function UserDetailsRoute() {
  const { userId } = useParams()
  const { profile, permissions } = useAuth()
  if (!profile || !userId) return <NotFoundPage />
  const self = profile.id === userId
  return !self && !hasPermission(permissions, 'users.read|users.manage') ? <ForbiddenPage /> : <UserDetailsPage userId={userId} self={self} />
}

export function AppRoutes() {
  return <Shell><Routes>
    <Route path="/" element={<DashboardPage />} />
    <Route path="/tasks" element={<PermissionRoute permission="tasks.read|tasks.manage"><TaskInventoryPage /></PermissionRoute>} />
    <Route path="/tasks/:taskId" element={<PermissionRoute permission="tasks.read|tasks.manage"><TaskDetailPage /></PermissionRoute>} />
    <Route path="/tasks/new" element={<PermissionRoute permission="tasks.manage"><TaskEditorPage /></PermissionRoute>} />
    <Route path="/tasks/:taskId/edit" element={<PermissionRoute permission="tasks.manage"><TaskEditorPage /></PermissionRoute>} />
    <Route path="/schedules" element={<PermissionRoute permission="tasks.read|tasks.manage"><ScheduleInventoryPage /></PermissionRoute>} />
    <Route path="/schedules/new" element={<PermissionRoute permission="tasks.manage"><ScheduleEditorPage /></PermissionRoute>} />
    <Route path="/schedules/:scheduleId/edit" element={<PermissionRoute permission="tasks.manage"><ScheduleEditorPage /></PermissionRoute>} />
    <Route path="/runs" element={<PermissionRoute permission="runs.read"><RunInventoryPage /></PermissionRoute>} />
    <Route path="/runs/execute" element={<PermissionRoute permission="runs.execute"><ManualRunPage /></PermissionRoute>} />
    <Route path="/runs/:runId" element={<PermissionRoute permission="runs.read"><RunDetailPage /></PermissionRoute>} />
    <Route path="/runners" element={<PermissionRoute permission="runners.read"><RunnerInventoryPage /></PermissionRoute>} />
    <Route path="/runners/pools" element={<PermissionRoute permission="runners.read"><RunnerInventoryPage view="pools" /></PermissionRoute>} />
    <Route path="/runners/:runnerId" element={<PermissionRoute permission="runners.read"><RunnerDetailPage /></PermissionRoute>} />
    <Route path="/runners/enroll" element={<PermissionRoute permission="runners.manage"><EnrollmentPage /></PermissionRoute>} />
    <Route path="/resources" element={<PermissionRoute permission="resources.read|resources.manage"><ResourceInventoryPage /></PermissionRoute>} />
    <Route path="/resources/:resourceId" element={<PermissionRoute permission="resources.read|resources.manage"><ResourceDetailPage /></PermissionRoute>} />
    <Route path="/audit" element={<PermissionRoute permission="audit.read"><AuditPage /></PermissionRoute>} />
    <Route path="/admin/users" element={<PermissionRoute permission="users.read|users.manage"><UserManagementPage /></PermissionRoute>} />
    <Route path="/admin/users/sessions" element={<PermissionRoute permission="users.read|users.manage"><SessionManagementPage /></PermissionRoute>} />
    <Route path="/admin/users/:userId" element={<UserDetailsRoute />} />
    <Route path="/admin/roles" element={<PermissionRoute permission="roles.read|roles.manage"><RoleManagementPage /></PermissionRoute>} />
    <Route path="/admin/sso" element={<PermissionRoute permission="sso.read|sso.manage"><SsoSettingsPage /></PermissionRoute>} />
    <Route path="/admin/auth" element={<PermissionRoute permission="auth.settings.manage"><AuthenticationSettingsPage /></PermissionRoute>} />
    <Route path="/admin/execution-status" element={<PermissionRoute permission="auth.settings.manage"><ExecutionStatusPage /></PermissionRoute>} />
    <Route path="/global-variables" element={<PermissionRoute permission="users.manage"><GlobalVariablesPage /></PermissionRoute>} />
    <Route path="/account" element={<AccountPage />} />
    <Route path="/account/password" element={<AccountPage />} />
    <Route path="/account/identities" element={<AccountPage />} />
    <Route path="/account/sessions" element={<AccountPage />} />
    {placeholderRoutes().map((route) => <Route key={route.path} path={route.path} element={<PermissionRoute permission={route.permission}><Placeholder title={route.label} /></PermissionRoute>} />)}
    <Route path="/login" element={<Navigate to="/" replace />} />
    <Route path="*" element={<NotFoundPage />} />
  </Routes></Shell>
}
