import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { AuditValue, LogOutput, SafeLink, prefixLogLines, redactAuditValue, safeOutput } from './safe'

describe('safe output rendering', () => {
  it('keeps markup as text, removes controls, and bounds output', () => {
    const html = renderToStaticMarkup(<LogOutput stream="stdout" value={'<script>alert(1)</script>\u0000'} />)
    expect(html).toContain('&lt;script&gt;')
    expect(html).not.toContain('<script>')
    expect(safeOutput('abcdef', 3)).toBe('abc')
    expect(safeOutput('a\u0000b')).toContain('�')
  })

  it('prefixes each displayed log line', () => {
    expect(prefixLogLines('first\n\nsecond\n')).toBe('0 $ first\n$ \n$ second\n')
    expect(prefixLogLines(`${Array.from({ length: 51 }, (_, index) => `line ${index}`).join('\n')}`)).toContain('\n50 $ line 50')
  })

  it('redacts secret-shaped audit keys and rejects external links', () => {
    const html = renderToStaticMarkup(<><AuditValue value={{ token: 'hidden', nested: { password: 'hidden' }, passwordLoginEnabled: true, name: 'visible' }} /><SafeLink href="https://evil.example">open</SafeLink></>)
    expect(html).toContain('[REDACTED]')
    expect(redactAuditValue({ passwordLoginEnabled: true })).toEqual({ passwordLoginEnabled: true })
    expect(html).toContain('visible')
    expect(html).not.toContain('href="https://evil.example"')
  })
})
