import type { ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
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
import { UserManagementPage } from './admin-pages'

function Placeholder({ title }: { title: string }) {
  return <main className="gf-content"><PageHeader title={title} description="This workspace is ready for its data view." /></main>
}

function PermissionRoute({ permission, children }: { permission?: string; children: ReactNode }) {
  const { profile, permissions } = useAuth()
  if (!profile) return <LoginRequiredPage onLogin={() => undefined} />
  return permission && !hasPermission(permissions, permission) ? <ForbiddenPage /> : <>{children}</>
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
    <Route path="/runners/:runnerId" element={<PermissionRoute permission="runners.read"><RunnerDetailPage /></PermissionRoute>} />
    <Route path="/runners/enroll" element={<PermissionRoute permission="runners.manage"><EnrollmentPage /></PermissionRoute>} />
    <Route path="/resources" element={<PermissionRoute permission="resources.read|resources.manage"><ResourceInventoryPage /></PermissionRoute>} />
    <Route path="/resources/:resourceId" element={<PermissionRoute permission="resources.read|resources.manage"><ResourceDetailPage /></PermissionRoute>} />
    <Route path="/audit" element={<PermissionRoute permission="audit.read"><AuditPage /></PermissionRoute>} />
    <Route path="/admin/users" element={<PermissionRoute permission="users.read|users.manage"><UserManagementPage /></PermissionRoute>} />
    {ROUTES.filter((route) => !['/', '/tasks', '/schedules', '/runs', '/runners', '/resources', '/audit', '/admin/users'].includes(route.path)).map((route) => <Route key={route.path} path={route.path} element={<PermissionRoute permission={route.permission}><Placeholder title={route.label} /></PermissionRoute>} />)}
    <Route path="/login" element={<Navigate to="/" replace />} />
    <Route path="*" element={<NotFoundPage />} />
  </Routes></Shell>
}
