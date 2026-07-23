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
  await verifyEChartsFirstNavigation()
  await verifyDashboardCommandDoesNotReopenUpdates()
  await verifySpatialMapWindowing()
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

async function verifyEChartsFirstNavigation(): Promise<void> {
  const workspacePath = '/workspaces/sales'
  const dashboardHref = '/workspaces/sales/dashboards/executive-sales'
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  const messages = collectBlockingConsoleMessages(page)

  try {
    const response = await page.goto(new URL(workspacePath, baseURL).toString(), { waitUntil: 'domcontentloaded' })
    if (!response?.ok()) throw new Error(`${workspacePath}: status ${response?.status() ?? 'unknown'}`)
    await page.locator(`a[href="${dashboardHref}"]`).click()
    await page.waitForURL(`**${dashboardHref}/pages/overview`)
    try {
      await page.waitForFunction(() => {
        const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
        const hosts = Array.from(dashboard?.shadowRoot?.querySelectorAll('lv-visualization-host') ?? []) as Array<HTMLElement & { envelope?: any; shadowRoot: ShadowRoot }>
        const chart = hosts.find((host) => host.envelope?.visualID === 'revenue_by_month')
        const renderer = chart?.shadowRoot?.querySelector('.renderer')
        const canvas = renderer?.querySelector('canvas') as HTMLCanvasElement | null
        const context = canvas?.getContext('2d', { willReadFrequently: true })
        const pixels = context && canvas ? context.getImageData(0, 0, canvas.width, canvas.height).data : undefined
        let dataPixels = 0
        if (pixels) {
          for (let offset = 0; offset < pixels.length; offset += 16) {
            const red = pixels[offset] ?? 0
            const green = pixels[offset + 1] ?? 0
            const blue = pixels[offset + 2] ?? 0
            const alpha = pixels[offset + 3] ?? 0
            if (alpha > 0 && blue > red + 50 && blue > green + 35) dataPixels++
          }
        }
        return chart?.envelope?.status?.kind === 'ready'
          && renderer?.getAttribute('aria-hidden') === 'false'
          && canvas?.localName === 'canvas'
          && canvas.width > 0
          && canvas.height > 0
          && dataPixels > 500
      }, undefined, { timeout: 30_000, polling: 100 })
    } catch (error) {
      const state = await page.evaluate(() => {
        const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
        return Array.from(dashboard?.shadowRoot?.querySelectorAll('lv-visualization-host') ?? []).map((candidate) => {
          const host = candidate as HTMLElement & { envelope?: any; shadowRoot: ShadowRoot }
          const renderer = host.shadowRoot?.querySelector('.renderer')
          const canvas = renderer?.querySelector('canvas') as HTMLCanvasElement | null
          const context = canvas?.getContext('2d', { willReadFrequently: true })
          const pixels = context && canvas ? context.getImageData(0, 0, canvas.width, canvas.height).data : undefined
          let dataPixels = 0
          if (pixels) {
            for (let offset = 0; offset < pixels.length; offset += 16) {
              const red = pixels[offset] ?? 0
              const green = pixels[offset + 1] ?? 0
              const blue = pixels[offset + 2] ?? 0
              const alpha = pixels[offset + 3] ?? 0
              if (alpha > 0 && blue > red + 50 && blue > green + 35) dataPixels++
            }
          }
          return {
            visualID: host.envelope?.visualID,
            status: host.envelope?.status,
            dataRevision: host.envelope?.dataRevision,
            rows: host.envelope?.dataState?.datasets?.map((dataset: any) => dataset.rows?.length),
            ariaHidden: renderer?.getAttribute('aria-hidden'),
            canvas: canvas ? { width: canvas.width, height: canvas.height } : null,
            dataPixels,
            alert: host.shadowRoot?.querySelector('[role="alert"]')?.textContent,
          }
        })
      })
      throw new Error(`ECharts first navigation did not paint: ${JSON.stringify(state)}; ${String(error)}`)
    }
    assertNoBlockingConsoleMessages('ECharts first navigation', messages)
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

type SpatialWindowSnapshot = {
  status: string
  message: string
  dataRevision: number
  requestSeq: number
  windowID: string
  precision: string
  rows: number
  rowCap: number
  featureCap: number
  zoomControlWidth: number
  zoomControlHeight: number
}

async function verifySpatialMapWindowing(): Promise<void> {
  const path = '/workspaces/visuals/dashboards/visual-showcase/pages/chart-map-scale'
  const origin = new URL(baseURL).origin
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  const messages = collectBlockingConsoleMessages(page)
  const updates: string[] = []
  const spatialResponses: number[] = []
  const basemapResponses: Array<{ url: string; status: number; cacheControl: string; acceptRanges: string }> = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (url.pathname === '/updates') updates.push(request.url())
  })
  page.on('response', (response) => {
    const url = new URL(response.url())
    if (url.pathname.endsWith('/commands/visual-spatial-window')) spatialResponses.push(response.status())
    if (!url.pathname.endsWith('.pmtiles')) return
    const headers = response.headers()
    basemapResponses.push({ url: response.url(), status: response.status(), cacheControl: headers['cache-control'] ?? '', acceptRanges: headers['accept-ranges'] ?? '' })
  })

  try {
    const readyStarted = performance.now()
    const response = await page.goto(new URL(path, baseURL).toString(), { waitUntil: 'domcontentloaded', timeout: 120_000 })
    if (!response?.ok()) throw new Error(`${path}: status ${response?.status() ?? 'unknown'}`)
    await page.waitForSelector('lv-dashboard-page')
    await waitForUpdatesRequest(path, updates)
    await page.waitForFunction(() => {
      const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
      const host = dashboard?.shadowRoot?.querySelector('lv-visualization-host') as HTMLElement & { envelope?: any }
      const envelope = host?.envelope
      const zoom = host?.shadowRoot?.querySelector('button.maplibregl-ctrl-zoom-in') as HTMLButtonElement | null
      const zoomStyle = zoom ? getComputedStyle(zoom) : undefined
      return envelope?.dataState?.kind === 'spatial_windowed'
        && envelope.dataState.window?.rows?.length > 0
        && envelope.dataRevision >= 2
        && envelope.status?.kind !== 'loading'
        && !envelope.status?.message
        && Number.parseFloat(zoomStyle?.width ?? '0') === 30
        && Number.parseFloat(zoomStyle?.height ?? '0') === 30
    }, undefined, { timeout: 120_000 })
    const readyDurationMs = performance.now() - readyStarted
    const readyBudgetMs = 10_000
    if (readyDurationMs > readyBudgetMs) throw new Error(`${path}: initial map readiness ${Math.round(readyDurationMs)}ms exceeds ${readyBudgetMs}ms`)

    const initial = await spatialWindowSnapshot(page)
    assertSpatialWindow(path, initial)
    if (initial.rowCap !== 1_000_000 || initial.featureCap !== 5_000) {
      throw new Error(`${path}: budgets rowCap=${initial.rowCap}, featureCap=${initial.featureCap}; want 1000000 and 5000`)
    }
    if (initial.zoomControlWidth !== 30 || initial.zoomControlHeight !== 30) {
      throw new Error(`${path}: zoom control is ${initial.zoomControlWidth}x${initial.zoomControlHeight} CSS pixels; want 30x30`)
    }

    const zoomIn = page.locator('lv-dashboard-page').locator('lv-visualization-host').locator('button.maplibregl-ctrl-zoom-in')
    let current = initial
    let slowestViewportMs = 0
    const viewportBudgetMs = 5_000
    const rapidStarted = performance.now()
    await zoomIn.evaluate((button) => { button.click(); button.click(); button.click() })
    await waitForSpatialRevision(page, current)
    slowestViewportMs = Math.max(slowestViewportMs, performance.now() - rapidStarted)
    current = await spatialWindowSnapshot(page)
    assertSpatialWindow(path, current)
    for (let attempt = 0; attempt < 10 && current.precision !== 'raw'; attempt++) {
      const viewportStarted = performance.now()
      await zoomIn.click()
      await waitForSpatialRevision(page, current)
      slowestViewportMs = Math.max(slowestViewportMs, performance.now() - viewportStarted)
      current = await spatialWindowSnapshot(page)
      assertSpatialWindow(path, current)
    }
    if (slowestViewportMs > viewportBudgetMs) throw new Error(`${path}: viewport readiness ${Math.round(slowestViewportMs)}ms exceeds ${viewportBudgetMs}ms`)
    if (current.precision !== 'raw') throw new Error(`${path}: viewport never transitioned from aggregated to raw precision`)
    if (current.requestSeq <= initial.requestSeq || current.dataRevision <= initial.dataRevision) {
      throw new Error(`${path}: revisions did not advance from ${JSON.stringify(initial)} to ${JSON.stringify(current)}`)
    }

    const reset = page.locator('lv-dashboard-page').locator('lv-visualization-host').locator('button[aria-label="Reset map view"]')
    await reset.click()
    await waitForSpatialReset(page, current, initial)
    const restored = await spatialWindowSnapshot(page)
    assertSpatialWindow(path, restored)
    if (restored.windowID !== initial.windowID || restored.precision !== initial.precision) {
      throw new Error(`${path}: reset restored ${restored.windowID} (${restored.precision}), want ${initial.windowID} (${initial.precision})`)
    }
    if (updates.length !== 1) throw new Error(`${path}: /updates request count=${updates.length}, want 1`)
    if (spatialResponses.length === 0 || spatialResponses.some((status) => status !== 200)) {
      throw new Error(`${path}: spatial command statuses=${JSON.stringify(spatialResponses)}, want only 200 responses`)
    }
    if (basemapResponses.length === 0) throw new Error(`${path}: no PMTiles byte-range response observed`)
    for (const basemap of basemapResponses) {
      if (new URL(basemap.url).origin !== origin || basemap.status !== 206 || basemap.acceptRanges !== 'bytes' || !basemap.cacheControl.includes('immutable')) {
        throw new Error(`${path}: invalid basemap delivery ${JSON.stringify(basemap)}`)
      }
    }
    assertNoBlockingConsoleMessages(path, messages)
    console.log(`${path}: ready=${Math.round(readyDurationMs)}ms slowestViewport=${Math.round(slowestViewportMs)}ms`)
  } finally {
    await page.close()
  }
}

async function spatialWindowSnapshot(page: Page): Promise<SpatialWindowSnapshot> {
  return page.evaluate(() => {
    const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
    const host = dashboard?.shadowRoot?.querySelector('lv-visualization-host') as HTMLElement & { envelope?: any; shadowRoot: ShadowRoot }
    const envelope = host?.envelope
    const state = envelope?.dataState
    const window = state?.window
    const zoom = host?.shadowRoot?.querySelector('button.maplibregl-ctrl-zoom-in') as HTMLButtonElement | null
    const style = zoom ? getComputedStyle(zoom) : undefined
    return {
      status: String(envelope?.status?.kind ?? ''),
      message: String(envelope?.status?.message ?? ''),
      dataRevision: Number(envelope?.dataRevision ?? 0),
      requestSeq: Number(window?.requestSeq ?? 0),
      windowID: String(window?.id ?? ''),
      precision: String(window?.precision ?? ''),
      rows: Array.isArray(window?.rows) ? window.rows.length : 0,
      rowCap: Number(state?.rowCap ?? 0),
      featureCap: Number(state?.featureCap ?? 0),
      zoomControlWidth: Number.parseFloat(style?.width ?? '0'),
      zoomControlHeight: Number.parseFloat(style?.height ?? '0'),
    }
  })
}

async function waitForSpatialRevision(page: Page, previous: SpatialWindowSnapshot): Promise<void> {
  await page.waitForFunction(({ dataRevision, requestSeq }) => {
    const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
    const host = dashboard?.shadowRoot?.querySelector('lv-visualization-host') as HTMLElement & { envelope?: any }
    const envelope = host?.envelope
    return envelope?.dataRevision > dataRevision
      && envelope?.dataState?.window?.requestSeq > requestSeq
      && envelope?.status?.kind !== 'loading'
      && !envelope?.status?.message
  }, { dataRevision: previous.dataRevision, requestSeq: previous.requestSeq }, { timeout: 120_000 })
}

async function waitForSpatialReset(page: Page, previous: SpatialWindowSnapshot, initial: SpatialWindowSnapshot): Promise<void> {
  await page.waitForFunction(({ dataRevision, requestSeq, windowID, precision }) => {
    const dashboard = document.querySelector('lv-dashboard-page') as HTMLElement & { shadowRoot: ShadowRoot }
    const host = dashboard?.shadowRoot?.querySelector('lv-visualization-host') as HTMLElement & { envelope?: any }
    const envelope = host?.envelope
    const window = envelope?.dataState?.window
    return envelope?.dataRevision > dataRevision
      && window?.requestSeq > requestSeq
      && window?.id === windowID
      && window?.precision === precision
      && envelope?.status?.kind !== 'loading'
  }, {
    dataRevision: previous.dataRevision,
    requestSeq: previous.requestSeq,
    windowID: initial.windowID,
    precision: initial.precision,
  }, { timeout: 120_000 })
}

function assertSpatialWindow(path: string, snapshot: SpatialWindowSnapshot): void {
  if (!['partial', 'ready'].includes(snapshot.status) || snapshot.message) {
    throw new Error(`${path}: invalid spatial status ${JSON.stringify(snapshot)}`)
  }
  if (!snapshot.windowID || snapshot.dataRevision < 1 || snapshot.requestSeq < 1 || snapshot.rows < 1) {
    throw new Error(`${path}: incomplete spatial window ${JSON.stringify(snapshot)}`)
  }
  if (snapshot.rows > snapshot.featureCap) {
    throw new Error(`${path}: rendered rows=${snapshot.rows} exceeds featureCap=${snapshot.featureCap}`)
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
