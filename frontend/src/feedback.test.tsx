import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { FatalErrorPage, ForbiddenPage, StartupPage } from './feedback'

describe('startup and error pages', () => {
  it('renders branded startup status', () => {
    const html = renderToStaticMarkup(<StartupPage status="Restoring session…" />)
    expect(html).toContain('Glyphflow')
    expect(html).toContain('Restoring session…')
    expect(html).toContain('aria-busy="true"')
  })

  it('renders retryable and forbidden states', () => {
    expect(renderToStaticMarkup(<FatalErrorPage message="Missing configuration" onRetry={() => undefined} />)).toContain('Retry')
    expect(renderToStaticMarkup(<ForbiddenPage />)).toContain('Access denied')
  })
})
