export type PrivilegeLevel = 'standard' | 'elevated'
export type PermissionDefinition = { name: string; privilegeLevel: PrivilegeLevel }

export const PERMISSIONS = [
  { name: 'users.read', privilegeLevel: 'elevated' }, { name: 'users.manage', privilegeLevel: 'elevated' },
  { name: 'roles.read', privilegeLevel: 'elevated' }, { name: 'roles.manage', privilegeLevel: 'elevated' },
  { name: 'sso.read', privilegeLevel: 'elevated' }, { name: 'sso.manage', privilegeLevel: 'elevated' },
  { name: 'auth.settings.manage', privilegeLevel: 'elevated' }, { name: 'tasks.read', privilegeLevel: 'standard' },
  { name: 'tasks.manage', privilegeLevel: 'standard' }, { name: 'runs.read', privilegeLevel: 'standard' },
  { name: 'runs.execute', privilegeLevel: 'standard' }, { name: 'runs.cancel', privilegeLevel: 'elevated' },
  { name: 'runs.retry', privilegeLevel: 'elevated' }, { name: 'logs.read', privilegeLevel: 'elevated' },
  { name: 'resources.read', privilegeLevel: 'standard' }, { name: 'resources.manage', privilegeLevel: 'standard' },
  { name: 'runners.read', privilegeLevel: 'standard' }, { name: 'runners.manage', privilegeLevel: 'standard' },
  { name: 'audit.read', privilegeLevel: 'elevated' }, { name: 'system.metrics.read', privilegeLevel: 'standard' },
  { name: 'system.deadletter.read', privilegeLevel: 'standard' }, { name: 'system.deadletter.manage', privilegeLevel: 'standard' },
] as const

export type Permission = (typeof PERMISSIONS)[number]['name']
export type Access = 'public' | 'authenticated' | 'permission'
export type RouteRule = { path: string; label: string; access: Access; permission?: string }

export function permissionPrivilegeLevel(permission: string): PrivilegeLevel {
  return PERMISSIONS.find(({ name }) => name === permission)?.privilegeLevel ?? 'standard'
}

export function sortedPermissions(): readonly PermissionDefinition[] {
  return [...PERMISSIONS].sort((left, right) => left.name.localeCompare(right.name))
}

export const ROUTES: RouteRule[] = [
  { path: '/', label: 'Overview', access: 'authenticated' },
  { path: '/tasks', label: 'Tasks', access: 'permission', permission: 'tasks.read|tasks.manage' },
  { path: '/schedules', label: 'Schedules', access: 'permission', permission: 'tasks.read|tasks.manage' },
  { path: '/runs', label: 'Runs', access: 'permission', permission: 'runs.read' },
  { path: '/runners', label: 'Runners', access: 'permission', permission: 'runners.read' },
  { path: '/runners/pools', label: 'Pools', access: 'permission', permission: 'runners.read' },
  { path: '/resources', label: 'Resources', access: 'permission', permission: 'resources.read|resources.manage' },
  { path: '/audit', label: 'Audit', access: 'permission', permission: 'audit.read' },
	{ path: '/global-variables', label: 'Global Variables', access: 'permission', permission: 'users.manage' },
  { path: '/admin/users', label: 'Users & SSO', access: 'permission', permission: 'users.read|users.manage' },
  { path: '/admin/roles', label: 'Roles', access: 'permission', permission: 'roles.read|roles.manage' },
  { path: '/admin/sso', label: 'SSO', access: 'permission', permission: 'sso.read|sso.manage' },
  { path: '/admin/auth', label: 'General Settings', access: 'permission', permission: 'auth.settings.manage' },
  { path: '/admin/execution-status', label: 'Execution Status', access: 'permission', permission: 'auth.settings.manage' },
  { path: '/admin/system', label: 'System Metrics', access: 'permission', permission: 'system.metrics.read' },
]

export function hasPermission(grants: Iterable<string>, required?: string): boolean {
  if (!required) return true
  const granted = new Set(grants)
  return required.split('|').some((permission) => granted.has(permission))
}

export function visibleRoutes(grants: Iterable<string>): RouteRule[] {
  return ROUTES.filter((route) => route.access !== 'permission' || hasPermission(grants, route.permission))
}
