import { describe, expect, it } from 'vitest'
import { accountDirty, sessionMetadata } from './account-pages'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const source = readFileSync(resolve(process.cwd(), 'src/account-pages.tsx'), 'utf8')

describe('account dirty baseline', () => {
  it('keeps email locked while leaving display name editable', () => {
    expect(source).toContain('value={profile.email ?? profile.username} readOnly disabled')
    expect(source).toContain('value={displayName} onChange=')
  })

  it('updates shared profile state after saving', () => {
    expect(source).toContain("const saved = await api.put<Profile>('/api/v1/me'")
    expect(source).toContain('setProfile(saved)')
  })

  it('does not mark the loaded profile dirty', () => {
    expect(accountDirty('Ada', 'Ada', { current: '', next: '', confirm: '' })).toBe(false)
  })

  it('marks profile edits and password edits dirty', () => {
    expect(accountDirty('Grace', 'Ada', { current: '', next: '', confirm: '' })).toBe(true)
    expect(accountDirty('Ada', 'Ada', { current: 'old', next: '', confirm: '' })).toBe(true)
  })

  it('exposes session identity and lifecycle metadata with fallbacks', () => {
    const metadata = sessionMetadata({ id: 'session-1', userAgent: 'Chrome on Linux', ipAddress: '127.0.0.1', lastSeenAt: '2026-08-17T12:00:00Z', expiresAt: '2026-08-18T12:00:00Z' })
    expect(metadata).toEqual(expect.arrayContaining([{ label: 'User agent', value: 'Chrome on Linux' }, { label: 'IP address', value: '127.0.0.1' }]))
    expect(metadata.find((item) => item.label === 'Last seen')?.value).toContain('2026')
    expect(metadata.find((item) => item.label === 'Expires')?.value).toContain('2026')
    expect(sessionMetadata({ id: 'session-2' })).toEqual(expect.arrayContaining([{ label: 'User agent', value: 'Unknown' }, { label: 'IP address', value: 'Unknown' }, { label: 'Last seen', value: '—' }, { label: 'Created', value: '—' }, { label: 'Expires', value: '—' }]))
  })
})
