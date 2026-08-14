import type { ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './auth'
import { PageHeader } from './components'
import { ForbiddenPage, LoginRequiredPage, NotFoundPage } from './feedback'
import { hasPermission, ROUTES } from './permissions'
import { Shell } from './shell'
import { TaskDetailPage, TaskInventoryPage } from './task-pages'
import { TaskEditorPage } from './task-editor'

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
    <Route path="/" element={<Placeholder title="Overview" />} />
    <Route path="/tasks" element={<PermissionRoute permission="tasks.read|tasks.manage"><TaskInventoryPage /></PermissionRoute>} />
    <Route path="/tasks/:taskId" element={<PermissionRoute permission="tasks.read|tasks.manage"><TaskDetailPage /></PermissionRoute>} />
    <Route path="/tasks/new" element={<PermissionRoute permission="tasks.manage"><TaskEditorPage /></PermissionRoute>} />
    <Route path="/tasks/:taskId/edit" element={<PermissionRoute permission="tasks.manage"><TaskEditorPage /></PermissionRoute>} />
    {ROUTES.filter((route) => route.path !== '/' && route.path !== '/tasks').map((route) => <Route key={route.path} path={route.path} element={<PermissionRoute permission={route.permission}><Placeholder title={route.label} /></PermissionRoute>} />)}
    <Route path="/login" element={<Navigate to="/" replace />} />
    <Route path="*" element={<NotFoundPage />} />
  </Routes></Shell>
}
