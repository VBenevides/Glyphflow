import { describe, expect, it } from 'vitest'
import { sessionDeviceLabel } from './session-device'

describe('session device labels', () => {
  it('summarizes common desktop and mobile agents', () => {
    expect(sessionDeviceLabel('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36', '10.0.0.1')).toBe('Desktop · Chrome · 10.0.0.1')
    expect(sessionDeviceLabel('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1')).toBe('Mobile · Safari')
  })

  it('uses safe fallbacks for unknown or missing agents', () => {
    expect(sessionDeviceLabel('Custom/1.0', '127.0.0.1')).toBe('Custom/1.0 · 127.0.0.1')
    expect(sessionDeviceLabel(undefined, '127.0.0.1')).toBe('Unknown device · 127.0.0.1')
    expect(sessionDeviceLabel()).toBe('Unknown device')
  })
})
