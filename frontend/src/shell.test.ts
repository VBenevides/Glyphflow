import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { createElement } from 'react'
import { AppearanceChoices, activeGroupName, activeRoutePath, groupedRoutes, navigationLabel } from './shell'
import { ROUTES, visibleRoutes } from './permissions'

describe('application shell navigation', () => {
  it('groups the visible route catalog into product areas', () => {
    const groups = groupedRoutes(ROUTES)
    expect(groups.map(({ group }) => group.name)).toEqual(['Operations', 'Infrastructure', 'Security', 'Administration'])
    expect(groups[0].routes.map((route) => route.path)).toContain('/runs')
    expect(activeGroupName('/admin/roles')).toBe('Administration')
    expect(activeGroupName('/unknown')).toBeUndefined()
  })

  it('activates only the most specific route for nested pages', () => {
    expect(activeRoutePath('/runners/pools', ROUTES)).toBe('/runners/pools')
    expect(activeRoutePath('/runners/runner-1', ROUTES)).toBe('/runners')
    expect(activeRoutePath('/tasks/new', ROUTES)).toBe('/tasks')
  })

  it('uses contextual labels for compact navigation items', () => {
    expect(navigationLabel(ROUTES.find((route) => route.path === '/runners/pools')!)).toBe('Runner pools')
    expect(navigationLabel(ROUTES.find((route) => route.path === '/admin/sso')!)).toBe('Single sign-on')
  })

  it('counts only permitted routes in each group', () => {
    const groups = groupedRoutes(visibleRoutes(['users.read']))
    expect(groups.find(({ group }) => group.name === 'Administration')?.routes.map((route) => route.path)).toEqual(['/admin/users'])
    expect(groups.find(({ group }) => group.name === 'Operations')?.routes.map((route) => route.path)).toEqual(['/'])
  })

  it('renders all appearance choices with accessible pressed state', () => {
    const html = renderToStaticMarkup(createElement(AppearanceChoices, { theme: 'dark', onSelect: () => undefined }))
    expect(html).toContain('>Light<')
    expect(html).toContain('>Dark<')
    expect(html).not.toContain('>Neon<')
    expect(html).toContain('aria-pressed="true"')
  })
})
