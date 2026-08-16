#!/usr/bin/env node

const baseURL = (process.env.GLYPHFLOW_URL ?? 'http://localhost:5173').replace(/\/$/, '')
const cdpURL = process.env.GLYPHFLOW_CDP_URL ?? 'http://127.0.0.1:9222'
const email = process.env.GLYPHFLOW_ADMIN_EMAIL ?? 'admin@example_domain.com'
const password = process.env.GLYPHFLOW_ADMIN_PASSWORD ?? 'admin-password-123'

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds))

async function json(url, options) {
  const response = await fetch(url, options)
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`)
  return response.json()
}

class CDPPage {
  constructor(target) {
    this.target = target
    this.nextID = 1
    this.pending = new Map()
  }

  async connect() {
    this.socket = new WebSocket(this.target.webSocketDebuggerUrl)
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      const pending = this.pending.get(message.id)
      if (!pending) return
      this.pending.delete(message.id)
      if (message.error) pending.reject(new Error(message.error.message))
      else pending.resolve(message.result ?? {})
    })
    await new Promise((resolve, reject) => {
      this.socket.addEventListener('open', resolve, { once: true })
      this.socket.addEventListener('error', () => reject(new Error('Chrome DevTools connection failed')), { once: true })
    })
    await this.command('Runtime.enable')
    await this.command('Page.enable')
    await this.command('Accessibility.enable')
  }

  command(method, params = {}) {
    const id = this.nextID++
    this.socket.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      setTimeout(() => {
        if (!this.pending.has(id)) return
        this.pending.delete(id)
        reject(new Error(`CDP command timed out: ${method}`))
      }, 15_000)
    })
  }

  async evaluate(fn, ...args) {
    const expression = `(${fn.toString()})(...${JSON.stringify(args)})`
    const result = await this.command('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? 'Browser evaluation failed')
    return result.result?.value
  }

  async navigate(path) {
    await this.command('Page.navigate', { url: `${baseURL}${path}` })
    await this.waitFor(() => document.readyState === 'complete' && location.origin === new URL(baseURL).origin)
  }

  async setViewport(width, height = 900) {
    await this.command('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false })
  }

  async waitFor(fn, timeout = 10_000) {
    const deadline = Date.now() + timeout
    let lastError
    while (Date.now() < deadline) {
      try {
        if (await this.evaluate(fn)) return
      } catch (error) {
        lastError = error
      }
      await sleep(100)
    }
    throw new Error(lastError ? `Timed out: ${lastError.message}` : 'Timed out waiting for browser state')
  }

  async click(selector) {
    const clicked = await this.evaluate((value) => {
      const element = document.querySelector(value)
      if (!element) return false
      element.scrollIntoView({ block: 'center' })
      element.click()
      return true
    }, selector)
    if (!clicked) throw new Error(`Element not found: ${selector}`)
  }

  async clickText(text, selector = 'button, a') {
    const clicked = await this.evaluate(([value, elementSelector]) => {
      const element = [...document.querySelectorAll(elementSelector)].find((candidate) => candidate.textContent?.trim() === value)
      if (!element) return false
      element.scrollIntoView({ block: 'center' })
      element.click()
      return true
    }, [text, selector])
    if (!clicked) throw new Error(`Control not found: ${text}`)
  }

  async fill(selector, value) {
    const filled = await this.evaluate(([elementSelector, nextValue]) => {
      const element = document.querySelector(elementSelector)
      if (!element) return false
      const prototype = element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
      const setter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set
      setter?.call(element, nextValue)
      element.dispatchEvent(new Event('input', { bubbles: true }))
      element.dispatchEvent(new Event('change', { bubbles: true }))
      return true
    }, [selector, value])
    if (!filled) throw new Error(`Input not found: ${selector}`)
  }

  async close() {
    this.socket?.close()
  }
}

async function createPage() {
  let targets
  try {
    targets = await json(`${cdpURL}/json`)
  } catch (error) {
    throw new Error(`Cannot reach Chrome DevTools at ${cdpURL}. Start Chromium with --remote-debugging-port=9222 (${error.message})`)
  }
  const target = targets.find((item) => item.type === 'page')
  if (!target) throw new Error('Chrome DevTools has no page target. Open the local frontend in Chromium before running this suite.')
  const page = new CDPPage(target)
  await page.connect()
  return page
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function login(page) {
  await page.navigate('/login')
  await page.waitFor(() => Boolean(document.querySelector('#email')))
  await page.fill('#email', email)
  await page.fill('#password', password)
  await page.clickText('Sign in')
  await page.waitFor(() => Boolean(document.querySelector('.gf-sidebar')))
  const profileStatus = await page.evaluate(async () => (await fetch('/api/v1/me', { credentials: 'include' })).status)
  assert(profileStatus === 200, `session bootstrap returned HTTP ${profileStatus}`)
}

async function logoutLogin(page) {
  await page.click('button[aria-label="Sign out"]')
  await page.waitFor(() => location.pathname === '/login')
  await page.fill('#email', email)
  await page.fill('#password', password)
  await page.clickText('Sign in')
  await page.waitFor(() => Boolean(document.querySelector('.gf-sidebar')), 8_000)
  const alert = await page.evaluate(() => document.querySelector('[role="alert"]')?.textContent?.trim() ?? '')
  assert(!alert, `login after logout displayed an error: ${alert}`)
}

async function roleRoutes(page) {
  await page.navigate('/admin/users')
  await page.waitFor(() => document.querySelector('h1')?.textContent?.includes('Users'))
  const denied = await page.evaluate(() => document.body.textContent?.includes('Access denied') ?? false)
  assert(!denied, 'administrator route was denied')
}

async function mobileMenu(page) {
  await page.setViewport(390)
  await page.navigate('/')
  await page.waitFor(() => Boolean(document.querySelector('.gf-mobile-menu')))
  const visible = await page.evaluate(() => {
    const element = document.querySelector('.gf-mobile-menu')
    if (!element) return false
    const style = getComputedStyle(element)
    const box = element.getBoundingClientRect()
    return style.display !== 'none' && box.width > 0 && box.height > 0
  })
  assert(visible, 'mobile navigation button is hidden at 390px')
  await page.click('button[aria-label="Open navigation"]')
  await page.waitFor(() => Boolean(document.querySelector('.gf-sidebar.is-mobile-open')))
  assert(await page.evaluate(() => Boolean(document.querySelector('.gf-drawer-scrim'))), 'mobile drawer scrim is missing')
}

async function taskEditor(page) {
  await page.setViewport(320)
  await page.navigate('/tasks/new')
  await page.waitFor(() => Boolean(document.querySelector('.gf-editor-form')))
  const overflow = await page.evaluate(() => ({ width: window.innerWidth, scrollWidth: document.documentElement.scrollWidth }))
  assert(overflow.scrollWidth <= overflow.width, `task editor overflows narrow viewport (${overflow.scrollWidth}px > ${overflow.width}px)`)
}

async function scheduleNames(page) {
  await page.setViewport(1440)
  await page.navigate('/schedules/new')
  await page.waitFor(() => Boolean(document.querySelector('.gf-editor-form')))
  const tree = await page.command('Accessibility.getFullAXTree', { depth: -1 })
  const names = new Set((tree.nodes ?? []).filter((node) => ['textbox', 'combobox', 'spinbutton'].includes(node.role?.value)).map((node) => node.name?.value).filter(Boolean))
  const required = ['Task', 'Name', 'UTC offset', 'Cron expression', 'Misfire policy', 'Start deadline seconds', 'Concurrency']
  const missing = required.filter((name) => !names.has(name))
  assert(!missing.length, `schedule controls have no accessible names: ${missing.join(', ')}`)
}

async function runLogs(page, state) {
  const run = await page.evaluate(async () => {
    const response = await fetch('/api/v1/runs?limit=1', { credentials: 'include' })
    if (!response.ok) throw new Error(`run list returned HTTP ${response.status}`)
    const page = await response.json()
    if (page.items?.[0]?.id) return { id: page.items[0].id, taskID: '' }
    return { id: '', taskID: '' }
  })
  let runID = run.id
  if (!runID) {
    state.createdTaskID = await page.evaluate(async () => {
      const csrf = decodeURIComponent(document.cookie.split('; ').find((part) => part.startsWith('glyphflow_csrf='))?.split('=').slice(1).join('=') ?? '')
      const response = await fetch('/api/v1/tasks', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, body: JSON.stringify({ name: `browser-acceptance-${Date.now()}`, command: ['/bin/echo', 'browser acceptance'], runner_pool: 'default', timeout_seconds: 60, max_output_bytes: 1024, max_attempts: 1 }) })
      if (!response.ok) throw new Error(`/api/v1/tasks returned HTTP ${response.status}`)
      return (await response.json()).id
    })
    runID = await page.evaluate(async (taskID) => {
      const csrf = decodeURIComponent(document.cookie.split('; ').find((part) => part.startsWith('glyphflow_csrf='))?.split('=').slice(1).join('=') ?? '')
      const response = await fetch('/api/v1/runs/execute', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, body: JSON.stringify({ task_id: taskID, idempotency_key: `browser-acceptance-${Date.now()}` }) })
      if (!response.ok) throw new Error(`/api/v1/runs/execute returned HTTP ${response.status}`)
      return (await response.json()).id
    }, state.createdTaskID)
  }
  await page.navigate(`/runs/${encodeURIComponent(runID)}`)
  await page.waitFor(() => document.querySelectorAll('.gf-log-toolbar').length === 2)
  await sleep(1_000)
  const errors = await page.evaluate(() => [...document.querySelectorAll('[role="alert"]')].map((element) => element.textContent?.trim()).filter((value) => value?.startsWith('Log stream failed')))
  assert(!errors.length, `run log streams failed: ${errors.join('; ')}`)
}

async function main() {
  try {
    const response = await fetch(`${baseURL}/login`)
    if (!response.ok) throw new Error(`local frontend returned HTTP ${response.status}`)
  } catch (error) {
    throw new Error(`Cannot reach ${baseURL}. Start ./dev_run.sh first (${error.message})`)
  }
  const page = await createPage()
  const state = { createdTaskID: '' }
  const checks = [
    ['login and session bootstrap', () => login(page)],
    ['logout then login CSRF regression', () => logoutLogin(page)],
    ['administrator role route', () => roleRoutes(page)],
    ['mobile navigation at 390px', () => mobileMenu(page)],
    ['task editor at 320px', () => taskEditor(page)],
    ['schedule control accessible names', () => scheduleNames(page)],
    ['run log panels', () => runLogs(page, state)],
  ]
  let failed = 0
  try {
    for (const [name, check] of checks) {
      try {
        await check()
        console.log(`PASS ${name}`)
      } catch (error) {
        failed += 1
        console.error(`FAIL ${name}: ${error.message}`)
      }
    }
  } finally {
    if (state.createdTaskID) {
      await page.evaluate(async (taskID) => {
        const csrf = decodeURIComponent(document.cookie.split('; ').find((part) => part.startsWith('glyphflow_csrf='))?.split('=').slice(1).join('=') ?? '')
        await fetch(`/api/v1/tasks/${encodeURIComponent(taskID)}`, { method: 'DELETE', credentials: 'include', headers: { 'X-CSRF-Token': csrf } })
      }, state.createdTaskID).catch((error) => console.error(`WARN cleanup failed: ${error.message}`))
    }
    await page.close()
  }
  if (failed) throw new Error(`${failed} browser acceptance check(s) failed`)
}

main().catch((error) => {
  console.error(`Browser acceptance stopped: ${error.message}`)
  process.exitCode = 1
})
