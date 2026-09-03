import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it } from 'vitest'
import { shouldWarnUnsaved, useUnsavedChanges } from './unsaved'

function UnsavedChangesHarness() {
  useUnsavedChanges(true, 'Leave this page?')
  return null
}

describe('unsaved changes', () => {
  it('warns only for dirty, unconfirmed navigation', () => {
    expect(shouldWarnUnsaved(true, false)).toBe(true)
    expect(shouldWarnUnsaved(true, true)).toBe(false)
    expect(shouldWarnUnsaved(false, false)).toBe(false)
  })

  it('cancels beforeunload navigation for dirty pages', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    act(() => root.render(createElement(UnsavedChangesHarness)))
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    act(() => root.unmount())
    host.remove()
  })
})
