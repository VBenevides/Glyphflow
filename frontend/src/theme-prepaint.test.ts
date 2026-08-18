import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import vm from 'node:vm'
import { describe, expect, it } from 'vitest'

const script = readFileSync(resolve(process.cwd(), 'public/theme-prepaint.js'), 'utf8')

function prepaint(saved: string | null, prefersDark: boolean, readStorage = () => saved) {
  const document = { documentElement: { dataset: {} as Record<string, string> } }
  vm.runInNewContext(script, {
    document,
    localStorage: { getItem: readStorage },
    matchMedia: () => ({ matches: prefersDark }),
  })
  return document.documentElement.dataset.theme
}

describe('theme prepaint', () => {
  it('prefers a supported saved theme', () => {
    expect(prepaint('dark', false)).toBe('dark')
    expect(prepaint('neon', true)).toBe('dark')
  })

  it('falls back to system preference when storage is unavailable', () => {
    expect(prepaint(null, true, () => { throw new Error('storage disabled') })).toBe('dark')
  })
})
