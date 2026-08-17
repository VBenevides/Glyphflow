import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { ApiError } from './api'
import { DangerousAction, dangerousActionError, dangerousWarning } from './actions'

describe('dangerous actions', () => {
  it('warns about external effects before retrying', () => {
    expect(dangerousWarning('Retry run')).toContain('external side effects')
    expect(renderToStaticMarkup(<DangerousAction label="Cancel" onConfirm={() => undefined} />)).toContain('Cancel')
  })

  it('explains runner lifecycle actions in the hover tooltip', () => {
    const html = renderToStaticMarkup(<DangerousAction label="Drain" variant="secondary" onConfirm={() => undefined} />)
    expect(html).toContain('title="Stops this runner from receiving new work while existing work can finish."')
  })

  it('shows server conflict details unless the action opted into stale-data refresh', () => {
    expect(dangerousActionError(new ApiError(409, { error: 'runner pool is still in use' }))).toBe('runner pool is still in use')
    let refreshed = false
    expect(dangerousActionError(new ApiError(409, { error: 'changed' }), () => { refreshed = true })).toBe('This resource changed. Reload it before trying again.')
    expect(refreshed).toBe(true)
  })
})
