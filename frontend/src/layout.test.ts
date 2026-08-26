import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf8')

describe('UI layout contracts', () => {
  it('keeps authentication controls compact and aligned', () => {
    expect(css).toContain(".gf-editor-form label:has(> input[type='checkbox']) { display: inline-flex; align-items: center;")
    expect(css).toContain('.gf-editor-form label:has(> select#default-role) > select { width: 12rem; flex: 0 0 12rem; }')
    expect(css).toContain(".gf-editor-form label:has(> select#default-role) { align-items: stretch; flex-direction: column;")
  })

  it('keeps run filters flexible and empty states compact', () => {
    expect(css).toContain('.gf-filter-bar > label, .gf-filter-bar > .gf-filter-field, .gf-filter-bar > .gf-task-picker { min-width: 10rem; max-width: 18rem; flex: 0 1 16rem; }')
    expect(css).toContain(".gf-filter-bar > label:has(> input[data-utc-datetime]) { min-width: 18rem; max-width: 22rem; flex-basis: 18rem; }")
    expect(css).toContain('.gf-state.gf-empty { min-height: 7rem; padding: 1.25rem; }')
  })

  it('keeps the mobile task editor inside the viewport', () => {
    expect(css).toContain('.gf-app-shell, .gf-main { width: 100%; max-width: 100%; overflow-x: hidden; }')
    expect(css).toContain('.gf-content, .gf-editor-form { width: 100%; max-width: none; }')
    expect(css).toContain('.gf-editor-form .gf-table { width: 100%; min-width: 0; }')
  })

  it('keeps light-theme logs readable on the dark log surface', () => {
    expect(css).toContain('.gf-log, .gf-audit-value { max-width: 100%; overflow: auto; padding: 1rem; color: #f8fafc; background: #07101f;')
  })

  it('keeps collapsed sidebar controls visible and aligned', () => {
    expect(css).toContain('.gf-sidebar.is-collapsed .gf-sidebar-brand .gf-brand-mark { display: none; }')
    expect(css).toContain('.gf-sidebar.is-collapsed .gf-sidebar-collapse { width: 100%; min-width: 0; margin: 0; padding-inline: 0; }')
    expect(css).toContain('.gf-sidebar.is-collapsed .gf-sidebar-actions { display: grid; justify-items: center; }')
    expect(css).not.toContain(":root[data-theme='neon']")
  })

  it('keeps shared surfaces hierarchical and usable on small screens', () => {
    expect(css).toContain('--gf-page-glow:')
    expect(css).toContain('.gf-card-panel, .gf-dashboard-widget, .gf-editor-form, .gf-table-wrap { box-shadow: var(--gf-shadow-soft); }')
    expect(css).toContain('.gf-table tbody tr:hover td')
    expect(css).toContain('.gf-dialog-backdrop { background: rgb(5 12 25 / 68%); backdrop-filter: blur(4px); }')
    expect(css).toContain('.gf-page-header-actions { margin-left: 0; justify-content: flex-start; }')
  })

  it('keeps the registry theme compact and lavender', () => {
    expect(css).toContain('--gf-page: oklch(0.985 0.004 280);')
    expect(css).toContain('.gf-app-shell { grid-template-columns: 18rem 1fr; }')
    expect(css).toContain('.gf-sidebar.is-collapsed .gf-nav-link.is-active')
    expect(css).toContain('.gf-role-table .gf-table { min-width: 58rem; }')
    expect(css).toContain('border-radius: 999px; font-size: 0.72rem; line-height: 1.25;')
  })
})
