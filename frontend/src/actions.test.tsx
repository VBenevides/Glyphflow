import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { DangerousAction, dangerousWarning } from './actions'

describe('dangerous actions', () => {
  it('warns about external effects before retrying', () => {
    expect(dangerousWarning('Retry run')).toContain('external side effects')
    expect(renderToStaticMarkup(<DangerousAction label="Cancel" onConfirm={() => undefined} />)).toContain('Cancel')
  })

  it('explains runner lifecycle actions in the hover tooltip', () => {
    const html = renderToStaticMarkup(<DangerousAction label="Drain" variant="secondary" onConfirm={() => undefined} />)
    expect(html).toContain('title="Stops this runner from receiving new work while existing work can finish."')
  })
})
