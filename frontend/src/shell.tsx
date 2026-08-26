import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Activity, ChevronDown, ChevronRight, FolderKanban, KeyRound, LayoutDashboard, LogOut, Menu, Moon, PanelLeftClose, PanelLeftOpen, Server, Settings, Shield, Sun, Users, Variable, X } from 'lucide-react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from './auth'
import { api } from './api'
import { Button } from './components'
import { applyTheme, currentTheme, type Theme } from './theme'
import { ROUTES, visibleRoutes, type RouteRule } from './permissions'
import { BrandMark } from './feedback'

export const SIDEBAR_KEY = 'glyphflow:sidebar-collapsed'
const appVersion = import.meta.env.VITE_APP_VERSION ?? 'dev'
type Group = { name: string; icon: typeof LayoutDashboard; paths: string[] }
export const NAVIGATION_GROUPS: Group[] = [
  { name: 'Operations', icon: LayoutDashboard, paths: ['/', '/tasks', '/schedules', '/runs'] },
  { name: 'Infrastructure', icon: Server, paths: ['/runners', '/resources', '/admin/execution-status'] },
  { name: 'Security', icon: Shield, paths: ['/audit'] },
  { name: 'Administration', icon: Users, paths: ['/admin/users', '/admin/roles', '/admin/auth', '/global-variables', '/admin/system'] },
]

export function groupedRoutes(routes: RouteRule[]): Array<{ group: Group; routes: RouteRule[] }> {
  return NAVIGATION_GROUPS.map((group) => ({ group, routes: group.paths.map((path) => routes.find((route) => route.path === path)).filter((route): route is RouteRule => Boolean(route)) })).filter(({ routes }) => routes.length > 0)
}

export function activeGroupName(path: string): string | undefined {
  const menuPath = path === '/admin/sso' ? '/admin/users' : path === '/runners/pools' ? '/runners' : path
  return NAVIGATION_GROUPS.find((group) => group.paths.includes(menuPath) || (menuPath !== '/' && group.paths.some((candidate) => menuPath.startsWith(`${candidate}/`))))?.name
}

export function activeRoutePath(path: string, routes: RouteRule[]): string | undefined {
  const menuPath = path === '/admin/sso' ? '/admin/users' : path === '/runners/pools' ? '/runners' : path
  return routes.filter((route) => route.path === '/' ? menuPath === '/' : menuPath === route.path || menuPath.startsWith(`${route.path}/`)).sort((left, right) => right.path.length - left.path.length)[0]?.path
}

const navigationLabels: Record<string, string> = {
  '/runners': 'Runners & Pools',
  '/admin/users': 'Users & SSO',
  '/admin/sso': 'Single sign-on',
  '/admin/auth': 'General Settings',
}

export function navigationLabel(route: RouteRule): string {
  return navigationLabels[route.path] ?? route.label
}

function routeIcon(path: string) {
  if (path === '/') return LayoutDashboard
  if (path === '/runs') return Activity
  if (path === '/audit' || path === '/admin/sso') return Shield
  if (path === '/global-variables') return Variable
  if (path === '/admin/auth') return Settings
  if (path === '/resources' || path === '/admin/roles') return KeyRound
  if (path === '/admin/execution-status') return Activity
  if (path === '/admin/system') return Activity
  if (path.startsWith('/admin')) return Users
  if (path === '/tasks' || path === '/schedules') return FolderKanban
  return Server
}

export function AppearanceChoices({ theme, onSelect }: { theme: Theme; onSelect: (theme: Theme) => void }) {
  const light = theme === 'light'
  return <Button variant="ghost" className={`gf-theme-toggle${light ? '' : ' is-dark'}`} role="switch" aria-checked={!light} aria-label={`Switch to ${light ? 'dark' : 'light'} mode`} onClick={() => onSelect(light ? 'dark' : 'light')}><span className={`gf-theme-option${light ? ' is-current' : ''}`}><Sun size={15} aria-hidden="true" /><span className="gf-theme-label">Light mode</span></span><span className="gf-theme-switch" aria-hidden="true"><span /></span><span className={`gf-theme-option${light ? '' : ' is-current'}`}><Moon size={15} aria-hidden="true" /><span className="gf-theme-label">Dark mode</span></span></Button>
}

export function Shell({ children }: { children: ReactNode }) {
  const { config, profile, permissions, restore, setProfile } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem(SIDEBAR_KEY) === 'true')
  const [mobileOpen, setMobileOpen] = useState(false)
  const [theme, setTheme] = useState<Theme>(currentTheme())
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({ Operations: true, Infrastructure: true, Security: true, Administration: true })
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const sidebarRef = useRef<HTMLElement>(null)
  const visible = useMemo(() => visibleRoutes(permissions), [permissions])
  const grouped = groupedRoutes(visible)
  const activePath = activeRoutePath(location.pathname, visible)
  useEffect(() => { window.localStorage.setItem(SIDEBAR_KEY, String(collapsed)) }, [collapsed])
  useEffect(() => { setMobileOpen(false) }, [location.pathname])
  useEffect(() => {
    const active = activeGroupName(location.pathname)
    if (active) setOpenGroups((current) => ({ ...current, [active]: true }))
  }, [location.pathname])
  useEffect(() => {
    if (!mobileOpen) return
    const previous = document.activeElement as HTMLElement | null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const focusable = () => [...(sidebarRef.current?.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])') ?? [])]
    focusable()[0]?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { setMobileOpen(false); return }
      if (event.key !== 'Tab') return
      const items = focusable()
      if (!items.length) return
      if (event.shiftKey && document.activeElement === items[0]) { event.preventDefault(); items[items.length - 1].focus() }
      if (!event.shiftKey && document.activeElement === items[items.length - 1]) { event.preventDefault(); items[0].focus() }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => { document.body.style.overflow = previousOverflow; document.removeEventListener('keydown', onKeyDown); previous?.focus() }
  }, [mobileOpen])
  const logout = async () => { try { await api.post('/api/v1/auth/logout') } finally { setProfile(null); restore(); navigate('/login', { replace: true }) } }
  const selectTheme = (next: Theme) => { applyTheme(next); setTheme(next) }
  const navigation = <nav className="gf-sidebar-nav" aria-label="Primary navigation"><p className="gf-sidebar-eyebrow">Workspace</p>{grouped.map(({ group, routes }) => { const Icon = group.icon; const expanded = openGroups[group.name] ?? true; return <section key={group.name} className="gf-nav-group"><button type="button" className={`gf-nav-group-button${activeGroupName(location.pathname) === group.name ? ' is-active' : ''}`} title={`${expanded ? 'Collapse' : 'Expand'} ${group.name}`} aria-label={`${expanded ? 'Collapse' : 'Expand'} ${group.name}`} aria-expanded={expanded} onClick={() => setOpenGroups((current) => ({ ...current, [group.name]: !expanded }))}>{expanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}<Icon size={16} aria-hidden="true" /><span>{group.name}</span><small>{routes.length}</small></button>{expanded && <div className="gf-nav-children">{routes.map((route) => { const RouteIcon = routeIcon(route.path); const label = navigationLabel(route); const isCurrent = route.path === activePath; const end = !isCurrent || location.pathname === route.path; return <NavLink key={route.path} to={route.path} end={end} className={() => `gf-nav-link${isCurrent ? ' is-active' : ''}`} title={collapsed && !mobileOpen ? label : undefined} aria-label={label}><RouteIcon size={16} aria-hidden="true" /><span>{label}</span></NavLink> })}</div>}</section> })}</nav>
  const sidebar = <aside ref={sidebarRef} className={`gf-sidebar${collapsed ? ' is-collapsed' : ''}${mobileOpen ? ' is-mobile-open' : ''}`} aria-label="Glyphflow sidebar"><div className="gf-sidebar-brand"><BrandMark /><div className="gf-sidebar-brand-copy"><strong>Glyphflow <span className="gf-sidebar-version">v{appVersion}</span></strong><small>Scheduler console</small></div><Button className="gf-sidebar-collapse" variant="ghost" aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} onClick={() => setCollapsed((value) => !value)}>{collapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}</Button>{mobileOpen && <Button variant="ghost" aria-label="Close navigation" onClick={() => setMobileOpen(false)}><X size={18} /></Button>}</div><div className="gf-module-badge" title="Scheduler"><LayoutDashboard size={15} aria-hidden="true" /><span>Scheduler</span></div>{navigation}<div className="gf-sidebar-footer"><Link className="gf-user-card" to="/account"><Users size={18} aria-hidden="true" /><span><strong>{profile?.displayName ?? profile?.username}</strong><small>{profile?.username}</small></span></Link><div className="gf-sidebar-actions"><AppearanceChoices theme={theme} onSelect={selectTheme} /><Button variant="ghost" aria-label="Sign out" onClick={logout}><LogOut size={17} /></Button></div></div></aside>
  return <div className={`gf-app-shell${collapsed ? ' is-sidebar-collapsed' : ''}`}>{config.lockdownScheduler && <div className="gf-lockdown-banner" role="status">Scheduler in lockdown: Only read actions are allowed</div>}<Button ref={menuButtonRef} className="gf-mobile-menu" variant="secondary" aria-label="Open navigation" onClick={() => setMobileOpen(true)}><Menu size={18} /></Button>{mobileOpen && <button className="gf-drawer-scrim" title="Close navigation" aria-label="Close navigation" onClick={() => setMobileOpen(false)} />}{sidebar}<main id="app-main" className="gf-main" tabIndex={-1}>{children}</main></div>
}

export const allRoutes = ROUTES
