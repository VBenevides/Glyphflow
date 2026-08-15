export const PERMISSIONS = [
  'users.read', 'users.manage', 'roles.read', 'roles.manage', 'sso.read', 'sso.manage',
  'auth.settings.manage', 'tasks.read', 'tasks.manage', 'runs.read', 'runs.execute',
  'runs.cancel', 'runs.retry', 'logs.read', 'resources.read', 'resources.manage',
  'runners.read', 'runners.manage', 'audit.read',
] as const

export type Permission = (typeof PERMISSIONS)[number]
export type Access = 'public' | 'authenticated' | 'permission'
export type RouteRule = { path: string; label: string; access: Access; permission?: string }

export const ROUTES: RouteRule[] = [
  { path: '/', label: 'Overview', access: 'authenticated' },
  { path: '/tasks', label: 'Tasks', access: 'permission', permission: 'tasks.read|tasks.manage' },
  { path: '/schedules', label: 'Schedules', access: 'permission', permission: 'tasks.read|tasks.manage' },
  { path: '/runs', label: 'Runs', access: 'permission', permission: 'runs.read' },
  { path: '/runners', label: 'Runners', access: 'permission', permission: 'runners.read' },
  { path: '/runners/pools', label: 'Pools', access: 'permission', permission: 'runners.read' },
  { path: '/resources', label: 'Resources', access: 'permission', permission: 'resources.read|resources.manage' },
  { path: '/audit', label: 'Audit', access: 'permission', permission: 'audit.read' },
  { path: '/global-variables', label: 'Global Variables', access: 'permission', permission: 'tasks.read|tasks.manage' },
  { path: '/admin/users', label: 'Users', access: 'permission', permission: 'users.read|users.manage' },
  { path: '/admin/roles', label: 'Roles', access: 'permission', permission: 'roles.read|roles.manage' },
  { path: '/admin/sso', label: 'SSO', access: 'permission', permission: 'sso.read|sso.manage' },
  { path: '/admin/auth', label: 'Authentication', access: 'permission', permission: 'auth.settings.manage' },
  { path: '/admin/execution-status', label: 'Execution Status', access: 'permission', permission: 'auth.settings.manage' },
]

export function hasPermission(grants: Iterable<string>, required?: string): boolean {
  if (!required) return true
  const granted = new Set(grants)
  return required.split('|').some((permission) => granted.has(permission))
}

export function visibleRoutes(grants: Iterable<string>): RouteRule[] {
  return ROUTES.filter((route) => route.access !== 'permission' || hasPermission(grants, route.permission))
}
