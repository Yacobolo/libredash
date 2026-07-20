import { chromium, type Page } from '@playwright/test'

type RouteExpectation = {
  path: string
  root: string
  shell: boolean
}

const baseURL = Bun.env.LEAPVIEW_BASE_URL ?? 'http://localhost:8195'
const dashboardPath = '/workspaces/visuals/dashboards/visual-showcase/pages/overview'
const routes: RouteExpectation[] = [
  { path: '/', root: 'lv-catalog-page', shell: true },
  { path: dashboardPath, root: 'lv-dashboard-page', shell: true },
  { path: '/data', root: 'lv-data-explorer', shell: true },
  { path: '/workspaces', root: 'lv-workspace-page', shell: true },
  { path: '/connections', root: 'lv-connections-page', shell: true },
  { path: '/admin', root: 'lv-admin-page', shell: true },
  { path: '/chat', root: 'lv-chat-page', shell: true },
  { path: '/login', root: 'lv-login-page', shell: false },
]

const browser = await chromium.launch()
try {
  for (const route of routes) {
    await verifyRoute(route)
  }
  await verifyDashboardCommandDoesNotReopenUpdates()
  console.log(`DatastarLit route QA passed for ${routes.length} routes at ${baseURL}`)
} finally {
  await browser.close()
}

async function verifyRoute(route: RouteExpectation): Promise<void> {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  const messages = collectBlockingConsoleMessages(page)
  const updates: string[] = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (url.pathname === '/updates') updates.push(request.url())
  })

  try {
    const response = await page.goto(new URL(route.path, baseURL).toString(), { waitUntil: 'domcontentloaded' })
    if (!response?.ok()) {
      throw new Error(`${route.path}: status ${response?.status() ?? 'unknown'}`)
    }
    await page.waitForSelector(route.root)
    await page.waitForFunction((expectedRoot) => {
      if (expectedRoot === 'lv-chat-page') return true
      const root = document.querySelector(expectedRoot)
      return (root?.shadowRoot?.textContent?.replace(/\s+/g, ' ').trim().length ?? 0) > 0
    }, route.root, { timeout: 5000 })
    await waitForUpdatesRequest(route.path, updates)
    const state = await page.evaluate((expectedRoot) => {
      const root = document.querySelector(expectedRoot)
      return {
        root: root?.localName ?? '',
        shell: Boolean(document.querySelector('lv-app-shell')),
        shadowText: root?.shadowRoot?.textContent?.replace(/\s+/g, ' ').trim() ?? '',
        datastarScriptCount: document.querySelectorAll('script[src*="datastar-1.0.2"]').length,
      }
    }, route.root)

    if (state.root !== route.root) throw new Error(`${route.path}: mounted ${state.root || 'no root'}, want ${route.root}`)
    if (state.shell !== route.shell) throw new Error(`${route.path}: shell=${state.shell}, want ${route.shell}`)
    if (state.shadowText.length === 0 && route.root !== 'lv-chat-page') throw new Error(`${route.path}: route root rendered no shadow text`)
    if (state.datastarScriptCount !== 1) throw new Error(`${route.path}: Datastar script count=${state.datastarScriptCount}, want 1`)
    if (updates.length !== 1) throw new Error(`${route.path}: /updates request count=${updates.length}, want 1`)
    assertNoBlockingConsoleMessages(route.path, messages)
  } finally {
    await page.close()
  }
}

async function waitForUpdatesRequest(label: string, updates: string[]): Promise<void> {
  const deadline = Date.now() + 5000
  while (updates.length === 0) {
    if (Date.now() > deadline) throw new Error(`${label}: timed out waiting for /updates request`)
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
}

async function verifyDashboardCommandDoesNotReopenUpdates(): Promise<void> {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  const messages = collectBlockingConsoleMessages(page)
  const updates: string[] = []
  const commands: string[] = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (url.pathname === '/updates') updates.push(request.url())
    if (url.pathname.includes('/commands/')) commands.push(`${request.method()} ${url.pathname}`)
  })

  try {
    await page.goto(new URL(dashboardPath, baseURL).toString(), { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('lv-dashboard-page')
    await page.waitForTimeout(1000)
    const beforeUpdates = updates.length
    await page.evaluate(() => {
      document.querySelector('lv-dashboard-page')?.dispatchEvent(new CustomEvent('lv-filters-refresh', { bubbles: true, composed: true }))
    })
    await page.waitForTimeout(1000)

    if (beforeUpdates !== 1) throw new Error(`dashboard command: initial /updates count=${beforeUpdates}, want 1`)
    if (updates.length !== 1) throw new Error(`dashboard command reopened /updates: count=${updates.length}`)
    if (!commands.includes('POST /workspaces/visuals/commands/reload')) {
      throw new Error(`dashboard command requests=${JSON.stringify(commands)}, want reload POST`)
    }
    assertNoBlockingConsoleMessages('dashboard command', messages)
  } finally {
    await page.close()
  }
}

function collectBlockingConsoleMessages(page: Page): string[] {
  const messages: string[] = []
  page.on('console', (message) => {
    if (message.type() !== 'warning' && message.type() !== 'error') return
    const text = message.text()
    if (text.includes('Failed to load resource')) messages.push(text)
    if (text.includes('[LeapView]')) messages.push(text)
    if (text.includes('Multiple versions of Lit loaded')) messages.push(text)
    if (text.includes('Lit is in dev mode')) messages.push(text)
  })
  return messages
}

function assertNoBlockingConsoleMessages(label: string, messages: string[]): void {
  if (messages.length === 0) return
  throw new Error(`${label}: blocking console messages:\n${messages.join('\n')}`)
}
