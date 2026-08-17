import { mkdir, writeFile } from 'node:fs/promises'

const baseURL = (process.env.GLYPHFLOW_URL ?? 'http://localhost:5173').replace(/\/$/, '')
const cdpURL = process.env.GLYPHFLOW_CDP_URL ?? 'http://127.0.0.1:9222'
const email = process.env.GLYPHFLOW_ADMIN_EMAIL ?? 'admin@example_domain.com'
const password = process.env.GLYPHFLOW_ADMIN_PASSWORD ?? 'admin-password-123'
const outputDir = new URL('.', `file://${process.cwd()}/internal/v2-ui-test/`).pathname

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds))

async function json(url, options) {
  const response = await fetch(url, options)
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`)
  return response.json()
}

class Page {
  constructor(target) {
    this.target = target
    this.nextID = 1
    this.pending = new Map()
    this.consoleErrors = []
  }

  async connect() {
    this.socket = new WebSocket(this.target.webSocketDebuggerUrl)
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      if (message.method === 'Runtime.consoleAPICalled' && ['error', 'assert'].includes(message.params.type)) this.consoleErrors.push(message.params.args?.map((item) => item.value ?? item.description ?? '').join(' '))
      if (message.method === 'Runtime.exceptionThrown') this.consoleErrors.push(message.params.exceptionDetails?.text ?? 'Unhandled browser exception')
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
    const result = await this.command('Runtime.evaluate', { expression: `(${fn.toString()})(...${JSON.stringify(args)})`, awaitPromise: true, returnByValue: true })
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? 'Browser evaluation failed')
    return result.result?.value
  }

  async waitFor(fn, ...args) {
    const deadline = Date.now() + 10_000
    while (Date.now() < deadline) {
      if (await this.evaluate(fn, ...args)) return
      await sleep(100)
    }
    throw new Error('Timed out waiting for browser state')
  }

  async setViewport(width, height) {
    await this.command('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false })
  }

  async navigate(path) {
    await this.evaluate((url) => { location.href = url }, `${baseURL}${path}`)
    await this.waitFor((expected) => location.pathname === expected, path)
    await this.waitFor(() => document.readyState === 'complete')
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

  async summary(route) {
    return this.evaluate((path) => {
      const visible = (element) => {
        const style = getComputedStyle(element)
        const box = element.getBoundingClientRect()
        return style.display !== 'none' && style.visibility !== 'hidden' && box.width > 0 && box.height > 0
      }
      const text = (element) => element?.textContent?.replace(/\s+/g, ' ').trim() ?? ''
      const box = (element) => { const rect = element.getBoundingClientRect(); return { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) } }
      return {
        path,
        heading: text(document.querySelector('h1, h2')),
        title: document.title,
        body: text(document.body).slice(0, 1000),
        buttons: [...document.querySelectorAll('button')].filter(visible).map((element) => ({ text: text(element), disabled: element.disabled, box: box(element) })),
        links: [...document.querySelectorAll('a')].filter(visible).map((element) => ({ text: text(element), href: element.getAttribute('href'), box: box(element) })).slice(0, 40),
        controls: [...document.querySelectorAll('input, textarea, select')].filter(visible).map((element) => ({ tag: element.tagName.toLowerCase(), id: element.id, name: element.getAttribute('aria-label') ?? element.getAttribute('name'), type: element.getAttribute('type'), box: box(element) })),
        dialogs: [...document.querySelectorAll('[role="dialog"]')].filter(visible).map((element) => ({ text: text(element).slice(0, 500), box: box(element) })),
        alerts: [...document.querySelectorAll('[role="alert"]')].filter(visible).map(text),
        overflow: { viewport: window.innerWidth, document: document.documentElement.scrollWidth, body: document.body.scrollWidth },
      }
    }, route)
  }

  async screenshot(name) {
    const result = await this.command('Page.captureScreenshot', { format: 'png' })
    await writeFile(`${outputDir}/${name}.png`, Buffer.from(result.data, 'base64'))
  }

  async close() { this.socket?.close() }
}

function csrf() {
  return decodeURIComponent(document.cookie.split('; ').find((part) => part.startsWith('glyphflow_csrf='))?.split('=').slice(1).join('=') ?? '')
}

async function api(page, path, options = {}) {
  return page.evaluate(async ([url, request]) => {
    const response = await fetch(url, { credentials: 'include', ...request, headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': decodeURIComponent(document.cookie.split('; ').find((part) => part.startsWith('glyphflow_csrf='))?.split('=').slice(1).join('=') ?? ''), ...(request.headers ?? {}) } })
    const body = await response.text()
    let parsed
    try { parsed = body ? JSON.parse(body) : null } catch { parsed = body }
    return { status: response.status, body: parsed }
  }, [path, { ...options, body: options.body && JSON.stringify(options.body) }])
}

async function main() {
  await mkdir(outputDir, { recursive: true })
  const targets = await json(`${cdpURL}/json`)
  const target = targets.find((item) => item.type === 'page')
  if (!target) throw new Error('Chrome DevTools has no page target')
  const page = new Page(target)
  await page.connect()
  const report = { runtime: { baseURL, target: target.url }, setup: {}, pages: [], interactions: [], browserErrors: page.consoleErrors }
  try {
    await page.navigate('/login')
    if (await page.evaluate(() => Boolean(document.querySelector('#email')))) {
      await page.fill('#email', email)
      await page.fill('#password', password)
      await page.clickText('Sign in')
    }
    await page.waitFor(() => Boolean(document.querySelector('.gf-sidebar')))
    report.setup.login = await page.evaluate(async () => ({ me: (await fetch('/api/v1/me', { credentials: 'include' })).status, csrf: Boolean(document.cookie.includes('glyphflow_csrf=')) }))

    const suffix = Date.now()
    report.setup.data = {}
    report.setup.role = await api(page, '/api/v1/admin/roles', { method: 'POST', body: { name: `ui-audit-${suffix}`, permissions: ['tasks.read', 'runs.read'] } })
    report.setup.user = await api(page, '/api/v1/users', { method: 'POST', body: { email: `ui-audit-${suffix}@example.com`, password: 'ui-audit-password-123' } })
    const variables = await api(page, '/api/v1/global-variables')
    const variableItems = Array.isArray(variables.body) ? variables.body : variables.body?.items ?? []
    if (variables.status === 200 && !variableItems.some((item) => item.name === 'PYTHON_PATH_LINUX')) {
      await api(page, '/api/v1/global-variables', { method: 'POST', body: { name: 'PYTHON_PATH_LINUX', value: '/usr/sbin/python' } })
    }
    report.setup.admin = { note: 'Bootstrap admin used for this audit; the UI exposes no create-admin or role-assignment flow.' }

    const routes = ['/', '/tasks', '/tasks/new', '/schedules', '/schedules/new', '/runs', '/runs/execute', '/runners', '/runners/pools', '/runners/enroll', '/resources', '/audit', '/global-variables', '/admin/users', '/admin/roles', '/admin/sso', '/admin/auth', '/admin/execution-status', '/account', '/account/password', '/account/identities', '/account/sessions', '/tasks/missing', '/runs/missing', '/runners/missing', '/resources/missing', '/admin/users/missing']
    await page.evaluate(() => window.localStorage.removeItem('glyphflow:sidebar-collapsed'))
    await page.setViewport(1440, 1000)
    for (const route of routes) {
      try {
        await page.navigate(route)
        await sleep(route.endsWith('/missing') ? 2_500 : 400)
        const name = route.replaceAll('/', '_').replace(/^_/, 'home') || 'home'
        const summary = await page.summary(route)
        await page.screenshot(name)
        report.pages.push(summary)
      } catch (error) {
        report.pages.push({ path: route, error: error.message })
      }
    }

    await page.navigate('/admin/roles')
    await page.clickText('Create role')
    report.interactions.push({ name: 'open custom role editor', result: await page.summary('/admin/roles#create') })
    await page.fill('#role-name', `ui-audit-role-${suffix}`)
    await page.click('input[type="checkbox"]')
    await page.click('form button[type="submit"]')
    await sleep(300)
    report.interactions.push({ name: 'create custom role', result: await page.summary('/admin/roles#created') })

    await page.navigate('/account')
    await page.click('button[aria-label="Appearance"]')
    report.interactions.push({ name: 'appearance dialog', result: await page.summary('/account#appearance') })
    await page.clickText('Done')

    await page.navigate('/audit')
    await page.clickText('Details')
    report.interactions.push({ name: 'audit details dialog', result: await page.summary('/audit#details') })
    await page.click('button[aria-label="Close dialog"]')

    await page.navigate('/admin/auth')
    await page.clickText('Save settings')
    report.interactions.push({ name: 'dangerous confirmation dialog', result: await page.summary('/admin/auth#confirm') })
    await page.clickText('Cancel')

    await page.navigate('/admin/roles')
    await page.clickText('Delete')
    report.interactions.push({ name: 'role deletion confirmation', result: await page.summary('/admin/roles#delete') })
    await page.clickText('Cancel')

    await page.navigate('/global-variables')
    await page.clickText('Delete')
    report.interactions.push({ name: 'global variable deletion has no confirmation', result: await page.summary('/global-variables#deleted') })
    await api(page, '/api/v1/global-variables', { method: 'POST', body: { name: 'PYTHON_PATH_LINUX', value: '/usr/sbin/python' } })

    await page.navigate('/account/sessions')
    await page.clickText('Revoke')
    report.interactions.push({ name: 'session revocation confirmation', result: await page.summary('/account/sessions#revoke') })
    await page.clickText('Cancel')

    await page.navigate('/runs')
    const runHref = await page.evaluate(() => [...document.querySelectorAll('a')].map((link) => link.getAttribute('href')).find((href) => href?.startsWith('/runs/') && href !== '/runs/execute'))
    if (runHref) {
      await page.navigate(runHref)
      const action = await page.evaluate(() => ['Cancel', 'Retry', 'Reconcile unknown'].find((label) => [...document.querySelectorAll('button')].some((button) => button.textContent?.trim() === label)))
      if (action) {
        await page.clickText(action)
        report.interactions.push({ name: 'run action confirmation', result: await page.summary(`${runHref}#${action}`) })
        await page.clickText('Cancel')
      }
    }

    await page.navigate('/tasks/new')
    await page.clickText('Add variable')
    await page.fill('[aria-label="Environment variable name 1"]', '$ENV:')
    await page.evaluate(() => {
      const input = document.querySelector('[aria-label="Environment variable name 1"]')
      input?.focus()
      input?.setSelectionRange(input.value.length, input.value.length)
      input?.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await page.fill('#task-command', '$ENV:PYTHON_PATH_LINUX\nasd')
    const environmentPoint = await page.evaluate(() => {
      const input = document.querySelector('[aria-label="Environment variable name 1"]')
      input?.focus()
      input?.scrollIntoView({ block: 'center' })
      const box = input?.getBoundingClientRect()
      return box ? { x: box.x + box.width / 2, y: box.y + box.height / 2 } : null
    })
    if (environmentPoint) await page.command('Input.dispatchMouseEvent', { type: 'mouseMoved', ...environmentPoint })
    await sleep(200)
    const autocompleteOptions = await page.evaluate(() => [...document.querySelectorAll('.gf-task-option')].map((option) => ({ text: option.textContent?.replace(/\s+/g, ' ').trim(), title: option.getAttribute('title'), box: option.getBoundingClientRect().toJSON() })))
    await page.click('.gf-task-option')
    await page.fill('[aria-label="Environment variable name 1"]', '$ENV:PYTHON_PATH_LINUX')
    const selectedEnvironmentPoint = await page.evaluate(() => {
      const input = document.querySelector('[aria-label="Environment variable name 1"]')
      input?.focus()
      const box = input?.getBoundingClientRect()
      return box ? { x: box.x + box.width / 2, y: box.y + box.height / 2 } : null
    })
    if (selectedEnvironmentPoint) await page.command('Input.dispatchMouseEvent', { type: 'mouseMoved', ...selectedEnvironmentPoint })
    await sleep(100)
    report.interactions.push({
      name: 'environment autocomplete and final command',
      result: {
        page: await page.summary('/tasks/new#environment'),
        options: autocompleteOptions,
        tooltips: await page.evaluate(() => ({ active: document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.id, focusWithin: Boolean(document.querySelector('[aria-label="Environment variable name 1"]')?.closest('.gf-env-variable-picker')?.matches(':focus-within')), values: [...document.querySelectorAll('.gf-env-variable-tooltip')].map((tooltip) => ({ field: tooltip.parentElement?.querySelector('input, textarea')?.getAttribute('aria-label') ?? tooltip.parentElement?.querySelector('input, textarea')?.id, text: tooltip.textContent, visibility: getComputedStyle(tooltip).visibility, opacity: getComputedStyle(tooltip).opacity })) })),
      },
    })
    await page.screenshot('task-env-autocomplete')
    await page.setViewport(320, 900)
    report.interactions.push({ name: 'task editor narrow layout', result: await page.summary('/tasks/new#320') })
    await page.screenshot('task-new-320')
    await page.setViewport(1440, 1000)

    report.browserErrors = page.consoleErrors
    await writeFile(`${outputDir}/runtime-report.json`, JSON.stringify(report, null, 2))
    console.log(JSON.stringify({ setup: report.setup, pages: report.pages.length, interactions: report.interactions.length, browserErrors: report.browserErrors.length }, null, 2))
  } finally {
    await page.close()
  }
}

main().catch((error) => { console.error(error.stack ?? error); process.exitCode = 1 })
