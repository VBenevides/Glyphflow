import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { GlobalVariableInput } from './global-variable-input'

describe('global variable input', () => {
  it('renders the variable value in a hover tooltip', () => {
    const markup = renderToStaticMarkup(createElement(GlobalVariableInput, { value: '$ENV:CACHE_PATH', variables: [{ id: '1', name: 'CACHE_PATH', value: '/tmp/cache' }], onChange: () => undefined }))
    expect(markup).toContain('role="tooltip"')
    expect(markup).toContain('CACHE_PATH: /tmp/cache')
  })
})
