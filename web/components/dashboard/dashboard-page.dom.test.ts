import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const projectRoot = process.cwd()
const root = join(projectRoot, '.tmp/dashboard-page-test')

beforeAll(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument())
      return
    }
    const fileRoot = url.pathname.startsWith('/static/vendor/') ? projectRoot : root
    const file = normalize(join(fileRoot, url.pathname))
    if (!file.startsWith(fileRoot)) {
      response.writeHead(404)
      response.end('not found')
      return
    }
    try {
      response.setHeader('content-type', file.endsWith('.css') ? 'text/css' : 'text/javascript')
      response.end(await readFile(file))
    } catch {
      response.writeHead(404)
      response.end('not found')
    }
  })
  await new Promise<void>((resolve) => server.listen(0, resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind to a port')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

afterAll(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
}, 15_000)

for (const viewport of [
  { name: 'desktop', width: 1280, height: 820 },
  { name: 'mobile', width: 390, height: 820 },
]) {
  test(`dashboard page composes route UI on ${viewport.name}`, async () => {
    const page = await browser.newPage({ viewport })
    try {
      await page.goto(baseURL)
      await page.waitForFunction(() => (
        customElements.get('ld-dashboard-page')
          && customElements.get('ld-filter-dock')
          && customElements.get('ld-filter-panel')
          && customElements.get('ld-filter-card')
          && customElements.get('ld-kpi-card')
          && customElements.get('ld-echart')
          && customElements.get('ld-report-table')
      ))
      await page.waitForFunction(() => (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
      await page.locator('ld-dashboard-page').evaluate((element: any) => element.updateComplete)

      const state = await page.locator('ld-dashboard-page').evaluate((element: any) => {
        const root = element.shadowRoot
        const tags = Array.from(root.querySelectorAll('*')).map((node: Element) => node.localName)
        const filterDock = root.querySelector('ld-filter-dock')
        const rect = root.querySelector('ld-report-canvas')?.getBoundingClientRect()
        return {
          title: root.querySelector('h1')?.textContent?.trim(),
          hasSubSidebar: tags.includes('ld-sub-sidebar'),
          hasCanvas: tags.includes('ld-report-canvas'),
          hasFilterCard: tags.includes('ld-filter-card'),
          hasKpi: tags.includes('ld-kpi-card'),
          hasChart: tags.includes('ld-echart'),
          hasTable: tags.includes('ld-report-table'),
          hasFilterDock: tags.includes('ld-filter-dock'),
          hasFilterPanel: Boolean(filterDock?.shadowRoot?.querySelector('ld-filter-panel')),
          hasFooter: tags.includes('ld-report-footer'),
          hasModal: tags.includes('ld-visual-modal'),
          canvasVisible: Boolean(rect && rect.width > 40 && rect.height > 40),
        }
      })

      expect(state).toEqual({
        title: 'Executive Sales Dashboard',
        hasSubSidebar: true,
        hasCanvas: true,
        hasFilterCard: true,
        hasKpi: true,
        hasChart: true,
        hasTable: true,
        hasFilterDock: true,
        hasFilterPanel: true,
        hasFooter: true,
        hasModal: true,
        canvasVisible: true,
      })

      const tableState = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
        const table = element.shadowRoot.querySelector('ld-report-table') as any
        await table.updateComplete
        const root = table.shadowRoot
        return {
          text: root.textContent.replace(/\s+/g, ' ').trim(),
          rows: root.querySelectorAll('[role="row"]').length,
          cells: root.querySelectorAll('[role="cell"]').length,
        }
      })

      expect(tableState.text).toContain('Orders')
      expect(tableState.text).toContain('Order')
      expect(tableState.text).toContain('o1')
      expect(tableState.rows).toBeGreaterThan(0)
      expect(tableState.cells).toBeGreaterThan(0)

      if (viewport.name === 'desktop') {
        const dockState = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
          const dock = element.shadowRoot.querySelector('ld-filter-dock') as HTMLElement
          const root = dock.shadowRoot
          const beforeAside = root.querySelector('aside') as HTMLElement
          const beforeRail = root.querySelector('.rail') as HTMLElement
          const rect = (node: HTMLElement) => {
            const box = node.getBoundingClientRect()
            return {
              width: Math.round(box.width),
              height: Math.round(box.height),
              right: Math.round(box.right),
            }
          }
          const closedAside = rect(beforeAside)
          const closedRail = rect(beforeRail)
          beforeRail.click()
          await dock.updateComplete
          await new Promise((resolve) => setTimeout(resolve, 220))
          const afterAside = root.querySelector('aside') as HTMLElement
          const afterRail = root.querySelector('.rail') as HTMLElement
          const panel = root.querySelector('.panel') as HTMLElement
          return {
            closedAside,
            closedRail,
            openAside: rect(afterAside),
            openRail: rect(afterRail),
            openRailDisplay: getComputedStyle(afterRail).display,
            panelDisplay: getComputedStyle(panel).display,
          }
        })

        expect(dockState.closedRail.width <= dockState.closedAside.width).toBe(true)
        expect(dockState.openAside.width >= 300).toBe(true)
        expect(dockState.openRailDisplay).toBe('none')
        expect(dockState.openRail.height).toBe(0)
        expect(dockState.panelDisplay).toBe('block')
      }
    } finally {
      await page.close()
    }
  })
}

function testDocument(): string {
  const page = {
    kind: 'dashboard',
    title: 'Executive Sales Dashboard',
    dashboardId: 'executive-sales',
    dashboardTitle: 'Executive Sales Dashboard',
    pageId: 'overview',
    pageTitle: 'Overview',
    headerDetail: '1. Overview',
    modelId: 'olist',
    modelTitle: 'Olist',
    canvas: { width: 1024, height: 720 },
    grid: { columns: 12, rowHeight: 48, gap: 16, padding: 16 },
    pages: [{ id: 'overview', title: 'Overview', href: '/dashboards/executive-sales/pages/overview', active: true }],
    components: [
      { id: 'title', kind: 'header', x: 16, y: 16, width: 456, height: 88, title: 'Executive Sales', eyebrow: 'LibreDash report', badges: ['Sales'] },
      { id: 'state-filter', kind: 'filter_card', filter: 'state', x: 488, y: 16, width: 216, height: 88 },
      { id: 'orders-kpi', kind: 'kpi_card', visual: 'orders_kpi', x: 720, y: 16, width: 240, height: 88 },
      { id: 'orders-chart', kind: 'bar_chart', visual: 'orders_chart', x: 16, y: 128, width: 456, height: 280 },
      { id: 'orders-table', kind: 'table', table: 'orders', x: 488, y: 128, width: 472, height: 280 },
    ],
  }
  const filterConfig = [{
    id: 'state',
    type: 'multi_select',
    label: 'State',
    dimension: 'orders.state',
    default: { values: [] },
    operator: 'in',
    urlParam: 'state',
  }]
  const filters = { controls: { state: { type: 'multi_select', operator: 'in', values: [] } }, selections: [] }
  const visuals = {
    orders_kpi: {
      version: 3,
      id: 'orders_kpi',
      kind: 'kpi',
      shape: 'single_value',
      renderer: 'html',
      type: 'kpi',
      title: 'Orders',
      unit: '',
      interaction: { kind: 'point_selection', toggle: false, mappings: [] },
      dimensions: [],
      measure: 'order_count',
      measures: ['order_count'],
      series: [],
      options: { tone: 'ink', note: 'Filtered' },
      rendererOptions: {},
      selection: [],
      data: [{ label: 'Orders', value: 42 }],
    },
    orders_chart: {
      version: 3,
      id: 'orders_chart',
      kind: 'visual',
      shape: 'category_value',
      renderer: 'echarts',
      type: 'bar',
      title: 'Orders by status',
      unit: 'orders',
      interaction: { kind: 'point_selection', toggle: true, mappings: [{ field: 'orders.status', value: 'label' }] },
      dimensions: ['status'],
      measure: 'order_count',
      measures: ['order_count'],
      series: [],
      options: {},
      rendererOptions: {},
      selection: [],
      data: [{ label: 'delivered', value: 42 }, { label: 'shipped', value: 7 }],
    },
  }
  const tables = {
    orders: {
      version: 2,
      kind: 'data_table',
      title: 'Orders',
      style: { density: 'compact', zebra: true, grid: 'full' },
      interaction: { kind: 'row_selection', toggle: false, mappings: [] },
      selection: [],
      columns: [{ key: 'order_id', label: 'Order', width: 180 }],
      totalRows: 1,
      availableRows: 1,
      isCapped: false,
      rowCap: 1000,
      chunkSize: 100,
      rowHeight: 28,
      resetVersion: 0,
      sort: { key: 'order_id', direction: 'asc' },
      blocks: {
        a: { start: 0, requestSeq: 0, resetVersion: 0, sort: { key: 'order_id', direction: 'asc' }, rows: [{ order_id: 'o1' }] },
        b: { start: 100, requestSeq: 0, resetVersion: 0, sort: { key: 'order_id', direction: 'asc' }, rows: [] },
        c: { start: 200, requestSeq: 0, resetVersion: 0, sort: { key: 'order_id', direction: 'asc' }, rows: [] },
      },
      loadingBlock: '',
      error: '',
    },
  }
	const status = { loading: false, error: '', lastUpdated: '12:00:00', setupRequired: false }
  const signals = {
    page,
    filterConfig,
    filters,
    filterOptions: { state: [{ value: 'SP', label: 'SP' }] },
    visuals,
    tables,
    status,
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-chart-surface: #fff; --ld-report-page-bg: #fff; --ld-report-canvas-bg: #eaeef2; --ld-report-rail-bg: #fff; --ld-bg-overlay: #fff; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-line-muted: #d8dee4; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-border-transparent: 1px solid transparent; --ld-radius-default: 6px; --ld-radius-full: 999px; --ld-dashboard-filter-width: 44px; --ld-dashboard-filter-open-width: 320px; --base-size-2: 2px; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --control-medium-size: 32px; --control-xlarge-size: 40px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-title-sm: 16px; --ld-font-size-title-lg: 28px; --ld-font-size-display: 32px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-none: 1; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; --zIndex-dropdown: 100; --zIndex-modal: 200; --zIndex-sticky: 50; --shadow-resting-small: 0 1px 2px rgb(0 0 0 / .08); --shadow-floating-small: 0 8px 24px rgb(0 0 0 / .12); --ld-duration-fast: 160ms; --motion-easing-move: ease; --motion-transition-stateChange: 160ms ease; }
          ld-dashboard-page { min-height: 720px; }
        </style>
      </head>
      <body>
        <main data-signals="${attr(signals)}">
          <ld-dashboard-page></ld-dashboard-page>
        </main>
        <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script>
        <script type="module" src="/dashboard-page-under-test.js"></script>
      </body>
    </html>
  `
}

function escapeHTML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
}
