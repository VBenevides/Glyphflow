import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import App from './App'

describe('application bootstrap gate', () => {
  it('renders only the branded startup state before session resolution', () => {
    const html = renderToStaticMarkup(<App />)
    expect(html).toContain('Restoring server session')
    expect(html).not.toContain('Overview')
    expect(html).not.toContain('Control plane session restored')
  })
})
