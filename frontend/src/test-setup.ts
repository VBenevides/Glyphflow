import { afterAll, beforeAll } from 'vitest'

const originalError = console.error

beforeAll(() => {
  console.error = (...args: Parameters<typeof console.error>) => {
    if (typeof args[0] === 'string' && args[0].includes('useLayoutEffect does nothing on the server')) return
    originalError(...args)
  }
})

afterAll(() => {
  console.error = originalError
})
