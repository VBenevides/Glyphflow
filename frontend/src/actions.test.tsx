import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { DangerousAction, dangerousWarning } from './actions'

describe('dangerous actions', () => {
  it('warns about external effects before retrying', () => {
    expect(dangerousWarning('Retry run')).toContain('external side effects')
    expect(renderToStaticMarkup(<DangerousAction label="Cancel" onConfirm={() => undefined} />)).toContain('Cancel')
  })
})
