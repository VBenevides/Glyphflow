import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Activity, ChevronDown, ChevronRight, FolderKanban, KeyRound, LayoutDashboard, LogOut, Menu, Moon, PanelLeftClose, PanelLeftOpen, Server, Shield, Sun, Users, X } from 'lucide-react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from './auth'
import { api } from './api'
import { Button } from './components'
import { applyTheme, currentTheme } from './theme'
import { ROUTES, visibleRoutes, type RouteRule } from './permissions'
import { BrandMark } from './feedback'

export const SIDEBAR_KEY = 'glyphflow:sidebar-collapsed'
type Group = { name: string; icon: typeof LayoutDashboard; paths: string[] }
const groups: Group[] = [
  { name: 'Operations', icon: LayoutDashboard, paths: ['/', '/tasks', '/schedules', '/runs'] },
  { name: 'Infrastructure', icon: Server, paths: ['/runners', '/runners/pools', '/resources'] },
  { name: 'Security', icon: Shield, paths: ['/audit'] },
  { name: 'Administration', icon: Users, paths: ['/admin/users', '/admin/roles', '/admin/sso', '/admin/auth'] },
]

export function groupedRoutes(routes: RouteRule[]): Array<{ group: Group; routes: RouteRule[] }> {
  return groups.map((group) => ({ group, routes: group.paths.map((path) => routes.find((route) => route.path === path)).filter((route): route is RouteRule => Boolean(route)) })).filter(({ routes }) => routes.length > 0)
}

export function activeGroupName(path: string): string | undefined {
  return groups.find((group) => group.paths.includes(path) || (path !== '/' && group.paths.some((candidate) => path.startsWith(`${candidate}/`))))?.name
}

function routeIcon(path: string) {
  if (path === '/runs') return Activity
  if (path.startsWith('/admin')) return path === '/admin/roles' ? KeyRound : Users
  if (path === '/tasks' || path === '/schedules') return FolderKanban
  return Server
}

export function Shell({ children }: { children: ReactNode }) {
  const { profile, permissions, setProfile } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem(SIDEBAR_KEY) === 'true')
  const [mobileOpen, setMobileOpen] = useState(false)
  const [theme, setTheme] = useState(currentTheme())
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({ Operations: true, Infrastructure: true, Security: true, Administration: true })
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const sidebarRef = useRef<HTMLElement>(null)
  const visible = useMemo(() => visibleRoutes(permissions), [permissions])
  const grouped = groupedRoutes(visible)
  useEffect(() => { window.localStorage.setItem(SIDEBAR_KEY, String(collapsed)) }, [collapsed])
  useEffect(() => { setMobileOpen(false) }, [location.pathname])
  useEffect(() => {
    const active = activeGroupName(location.pathname)
    if (active) setOpenGroups((current) => ({ ...current, [active]: true }))
  }, [location.pathname])
  useEffect(() => {
    if (!mobileOpen) return
    const previous = document.activeElement as HTMLElement | null
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
    menuButtonRef.current?.focus()
    return () => { document.body.style.overflow = ''; document.removeEventListener('keydown', onKeyDown); previous?.focus() }
  }, [mobileOpen])
  const logout = async () => { try { await api.post('/api/v1/auth/logout') } finally { setProfile(null); navigate('/login', { replace: true }) } }
  const toggleTheme = () => { const next = theme === 'dark' ? 'light' : 'dark'; applyTheme(next); setTheme(next) }
  const navigation = <nav className="gf-sidebar-nav" aria-label="Primary navigation">{grouped.map(({ group, routes }) => { const Icon = group.icon; const expanded = openGroups[group.name] ?? true; return <section key={group.name} className="gf-nav-group"><button className="gf-nav-group-button" title={`${expanded ? 'Collapse' : 'Expand'} ${group.name}`} aria-expanded={expanded} onClick={() => setOpenGroups((current) => ({ ...current, [group.name]: !expanded }))}><Icon size={16} aria-hidden="true" /><span>{group.name}</span>{expanded ? <ChevronDown size={15} aria-hidden="true" /> : <ChevronRight size={15} aria-hidden="true" />}</button>{expanded && routes.map((route) => { const RouteIcon = routeIcon(route.path); return <NavLink key={route.path} to={route.path} end={route.path === '/'} className={({ isActive }) => `gf-nav-link${isActive ? ' is-active' : ''}`} title={collapsed ? route.label : undefined}><RouteIcon size={16} aria-hidden="true" /><span>{route.label}</span></NavLink> })}</section> })}</nav>
  const sidebar = <aside ref={sidebarRef} className={`gf-sidebar${collapsed ? ' is-collapsed' : ''}${mobileOpen ? ' is-mobile-open' : ''}`} aria-label="Glyphflow sidebar"><div className="gf-sidebar-brand"><BrandMark /><div className="gf-sidebar-brand-copy"><strong>Glyphflow</strong><small>Scheduler console</small></div>{mobileOpen && <Button variant="ghost" aria-label="Close navigation" onClick={() => setMobileOpen(false)}><X size={18} /></Button>}</div>{navigation}<div className="gf-sidebar-footer"><Link className="gf-user-card" to="/account"><Users size={18} aria-hidden="true" /><span><strong>{profile?.displayName ?? profile?.username}</strong><small>{profile?.username}</small></span></Link><div className="gf-sidebar-actions"><Button variant="ghost" aria-label="Toggle theme" onClick={toggleTheme}>{theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}</Button><Button variant="ghost" aria-label="Sign out" onClick={logout}><LogOut size={17} /></Button></div></div></aside>
  return <div className="gf-app-shell"><Button ref={menuButtonRef} className="gf-mobile-menu" variant="secondary" aria-label="Open navigation" onClick={() => setMobileOpen(true)}><Menu size={18} /></Button>{mobileOpen && <button className="gf-drawer-scrim" title="Close navigation" aria-label="Close navigation" onClick={() => setMobileOpen(false)} />}{sidebar}<main id="app-main" className="gf-main" tabIndex={-1}>{children}</main><Button className="gf-collapse-button" variant="secondary" aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} onClick={() => setCollapsed((value) => !value)}>{collapsed ? <PanelLeftOpen size={18} /> : <PanelLeftClose size={18} />}</Button></div>
}

export const allRoutes = ROUTES
