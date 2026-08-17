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
    expect(css).toContain('.gf-filter-bar > label, .gf-filter-bar > .gf-task-picker { min-width: 10rem; flex: 1 1 10rem; }')
    expect(css).toContain(".gf-filter-bar > label:has(> input[type='datetime-local']) { min-width: 11rem; flex-basis: 11rem; }")
    expect(css).toContain('.gf-state.gf-empty { min-height: 7rem; padding: 1.25rem; }')
  })

  it('keeps the mobile task editor inside the viewport', () => {
    expect(css).toContain('.gf-app-shell, .gf-main { width: 100%; max-width: 100%; overflow-x: hidden; }')
    expect(css).toContain('.gf-content, .gf-editor-form { width: 100%; max-width: none; }')
    expect(css).toContain('.gf-editor-form .gf-table { width: 100%; min-width: 0; }')
  })
})
