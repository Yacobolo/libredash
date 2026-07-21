import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'
import validateVisualizationEnvelope from '../../generated/visualization/validate'

let server: Server
let baseURL = ''
let browser: Browser
const projectRoot = process.cwd()
const root = join(projectRoot, '.tmp/dashboard-page-test')

test('dashboard fixtures satisfy the fail-closed visualization contract', () => {
  for (const [id, envelope] of Object.entries(testVisualizationEnvelopes())) {
    if (!validateVisualizationEnvelope(envelope)) {
      throw new Error(`${id}: ${JSON.stringify((validateVisualizationEnvelope as typeof validateVisualizationEnvelope & { errors?: unknown }).errors)}`)
    }
  }
})

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
    if (!file.startsWith(fileRoot)) { response.writeHead(404); response.end('not found'); return }
    try {
      response.setHeader('content-type', file.endsWith('.css') ? 'text/css' : 'text/javascript')
      response.end(await readFile(file))
    } catch { response.writeHead(404); response.end('not found') }
  })
  await new Promise<void>((resolve) => server.listen(0, resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

afterAll(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
}, 15_000)

for (const viewport of [{ name: 'desktop', width: 1280, height: 820 }, { name: 'mobile', width: 390, height: 820 }]) {
  test(`dashboard composes envelope-native visuals on ${viewport.name}`, async () => {
    const page = await browser.newPage({ viewport })
    try {
      await page.goto(baseURL)
      await page.waitForFunction(() => customElements.get('ld-dashboard-page') && customElements.get('ld-visualization-host'))
      await page.waitForFunction(() => (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
      await page.waitForFunction(() => {
        const dashboard = document.querySelector('ld-dashboard-page') as any
        const hosts = Array.from(dashboard?.shadowRoot?.querySelectorAll('ld-visualization-host') ?? []) as any[]
        const tableHost = hosts.find((host) => host.envelope?.visualID === 'orders')
        return Boolean(tableHost?.shadowRoot?.querySelector('ld-report-table'))
      })
      const state = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
        await element.updateComplete
        const root = element.shadowRoot
        const hosts = Array.from(root.querySelectorAll('ld-visualization-host')) as any[]
        await Promise.all(hosts.map((host) => host.updateComplete))
        const tableHost = hosts.find((host) => host.envelope?.visualID === 'orders')
        const table = tableHost?.shadowRoot?.querySelector('ld-report-table') as any
        await table?.updateComplete
        const canvas = root.querySelector('ld-report-canvas') as any
        await canvas.updateComplete
        const assigned = (canvas.shadowRoot.querySelector('slot') as HTMLSlotElement).assignedElements() as HTMLElement[]
        const visualFrame = (id: string) => assigned.find((item) => (item.querySelector('ld-visualization-host') as any)?.envelope?.visualID === id)?.getBoundingClientRect()
        const chart = visualFrame('orders_chart')
        const tableFrame = visualFrame('orders')
        return {
          title: root.querySelector('h1')?.textContent?.trim(), hostCount: hosts.length,
          legacyCount: root.querySelectorAll('ld-echart, ld-kpi-card, ld-report-table').length,
          kinds: hosts.map((host) => host.envelope?.spec?.kind).sort(),
          statuses: Object.fromEntries(hosts.map((host) => [host.envelope?.visualID, host.envelope?.status?.kind])),
          tableText: table?.shadowRoot?.textContent?.replace(/\s+/g, ' ').trim(),
          tableAlert: tableHost?.shadowRoot?.querySelector('[role="alert"]')?.textContent?.trim(),
          presentationMode: canvas.shadowRoot.querySelector('.surface')?.dataset.presentationMode,
          chartHeight: chart?.height ?? 0, tableHeight: tableFrame?.height ?? 0,
          tableAfterChart: (tableFrame?.top ?? 0) > (chart?.bottom ?? 0),
        }
      })
      expect(state.title).toBe('Executive Sales Dashboard')
      expect(state.hostCount).toBe(3)
      expect(state.legacyCount).toBe(0)
      expect(state.kinds).toEqual(['cartesian', 'kpi', 'table'])
      expect(state.statuses).toEqual({ orders_kpi: 'ready', orders_chart: 'loading', orders: 'error' })
      expect(state.tableAlert).toBe('Ratings query failed')
      expect(state.tableText).toContain('o1')
      if (viewport.name === 'mobile') {
        expect(state.presentationMode).toBe('responsive')
        expect(state.chartHeight).toBeGreaterThanOrEqual(280)
        expect(state.tableHeight).toBeLessThanOrEqual(700)
        expect(state.tableAfterChart).toBe(true)
      } else {
        expect(state.presentationMode).toBe('fit-width')
      }
    } finally { await page.close() }
  })
}

test('dashboard refresh progress is owned by the latest stream generation', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
    const states = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      const read = async () => {
        await element.updateComplete
        const progress = element.shadowRoot.querySelector('[data-dashboard-refresh-progress]')
        return { generation: progress?.getAttribute('data-generation'), now: progress?.getAttribute('aria-valuenow'), complete: progress?.getAttribute('data-complete') }
      }
      const initial = await read()
      mergePatch({ status: { generation: 4, refreshId: 'refresh-4', loading: true, progressPercent: 25 } })
      const active = await read()
      mergePatch({ status: { generation: 4, refreshId: 'refresh-4', loading: false, progressPercent: 100 } })
      const complete = await read()
      return { initial, active, complete }
    })
    expect(states).toEqual({
      initial: { generation: '3', now: '50', complete: 'false' },
      active: { generation: '4', now: '25', complete: 'false' },
      complete: { generation: '4', now: '100', complete: 'true' },
    })
  } finally { await page.close() }
})

test('dashboard keeps the source visualization selected through canonicalization and clearing', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
    const selections = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      const readSelection = async () => {
        await element.updateComplete
        await Promise.resolve()
        await element.updateComplete
        const host = Array.from(element.shadowRoot.querySelectorAll('ld-visualization-host') as NodeListOf<any>)
          .find((candidate: any) => candidate.envelope?.visualID === 'orders_chart')
        return host.envelope.selection
      }
      await element.updateComplete
      const source = Array.from(element.shadowRoot.querySelectorAll('ld-visualization-host') as NodeListOf<any>)
        .find((host: any) => host.envelope?.visualID === 'orders_chart')
      source.dispatchEvent(new CustomEvent('ld-interaction-select', { bubbles: true, composed: true, detail: {
        sourceKind: 'visual', sourceId: 'orders_chart', interactionKind: 'selection', action: 'set', toggle: true,
        mappings: [{ field: 'orders.status', fact: 'orders', value: 'delivered', label: 'Delivered' }],
      } }))
      const optimistic = await readSelection()

      mergePatch({
        filters: { selections: [{
          sourceKind: 'visual', sourceId: 'orders_chart', interactionKind: 'selection',
          entries: [{ label: 'Delivered', mappings: [{ field: 'orders.status', fact: 'orders', value: 'delivered' }] }],
        }] },
        status: { generation: 4, refreshId: 'refresh-4', loading: false, progressPercent: 100 },
      })
      const canonical = await readSelection()

      mergePatch({
        filters: { selections: [] },
        status: { generation: 5, refreshId: 'refresh-5', loading: false, progressPercent: 100 },
      })
      const cleared = await readSelection()
      return { optimistic, canonical, cleared }
    })
    const selected = [{
      datum: { dataset: 'primary', dataRevision: 1, identity: { label: 'delivered' } }, label: 'Delivered',
    }]
    expect(selections).toEqual({ optimistic: selected, canonical: selected, cleared: [] })
  } finally { await page.close() }
})

test('visualization host renders the shared title and expands without moving the live source', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
    const initial = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
      await element.updateComplete
      const hosts = Array.from(element.shadowRoot.querySelectorAll('ld-visualization-host') as NodeListOf<any>)
      const host = hosts.find((candidate: any) => candidate.envelope?.visualID === 'orders_chart')
      await host.updateComplete
      const title = host.shadowRoot.querySelector('[data-visualization-title]')?.textContent?.trim()
      const expand = host.shadowRoot.querySelector('button[aria-label="Expand chart"]') as HTMLButtonElement | null
      return { title, expand: expand?.title }
    })
    expect(initial).toEqual({
      title: 'Orders by status',
      expand: 'Expand chart',
    })

    await page.locator('[data-visualization-id="orders_chart"][data-visualization-expand]').click()
    await page.waitForFunction(() => {
      const dashboard = document.querySelector('ld-dashboard-page')
      return Boolean(dashboard?.shadowRoot?.querySelector('ld-visual-modal')?.shadowRoot?.querySelector('[role="dialog"]'))
    })
    const focused = await page.locator('ld-dashboard-page').evaluate((dashboard: any) => {
      const host = Array.from(dashboard.shadowRoot.querySelectorAll('ld-visualization-host') as NodeListOf<any>)
        .find((candidate: any) => candidate.envelope?.visualID === 'orders_chart') as HTMLElement | undefined
      const modal = dashboard.shadowRoot.querySelector('ld-visual-modal') as HTMLElement
      const clone = modal.querySelector('[data-visual-focus-clone]') as HTMLElement | null
      return {
        dialog: modal.shadowRoot?.querySelector('[role="dialog"]')?.getAttribute('aria-label'),
        sourceParent: host?.parentElement?.localName,
        sourceSlot: host?.getAttribute('slot'),
        cloneParent: clone?.parentElement?.localName,
        cloneSlot: clone?.getAttribute('slot'),
        cloneTitle: clone?.shadowRoot?.querySelector('[data-visualization-title]')?.textContent?.trim(),
      }
    })
    expect(focused).toEqual({
      dialog: 'Orders by status',
      sourceParent: 'ld-dashboard-visual-frame',
      sourceSlot: null,
      cloneParent: 'ld-visual-modal',
      cloneSlot: 'focus-visual',
      cloneTitle: 'Orders by status',
    })

    const mirroredStatus = await page.locator('ld-dashboard-page').evaluate(async (dashboard: any) => {
      const source = Array.from(dashboard.shadowRoot.querySelectorAll('ld-visualization-host') as NodeListOf<any>)
        .find((candidate: any) => candidate.envelope?.visualID === 'orders_chart') as any
      const modal = dashboard.shadowRoot.querySelector('ld-visual-modal') as HTMLElement
      const clone = modal.querySelector('[data-visual-focus-clone]') as any
      source.envelope = { ...source.envelope, status: { kind: 'partial', message: 'Focused refresh' } }
      await source.updateComplete
      await clone.updateComplete
      return clone.envelope?.status
    })
    expect(mirroredStatus).toEqual({ kind: 'partial', message: 'Focused refresh' })

    await page.locator('button[aria-label="Close visual modal"]').click()
    await page.waitForFunction(() => {
      const dashboard = document.querySelector('ld-dashboard-page')
      const modal = dashboard?.shadowRoot?.querySelector('ld-visual-modal')
      return !modal?.shadowRoot?.querySelector('[role="dialog"]') && !modal?.querySelector('[data-visual-focus-clone]')
    })
  } finally { await page.close() }
})

function testDocument(): string {
  const page = {
    kind: 'dashboard', title: 'Executive Sales Dashboard', dashboardId: 'executive-sales', dashboardTitle: 'Executive Sales Dashboard',
    pageId: 'overview', pageTitle: 'Overview', headerDetail: '1. Overview', modelId: 'olist', modelTitle: 'Olist',
    canvas: { width: 1024, height: 720 }, grid: { columns: 12, rowHeight: 48, gap: 16, padding: 16 },
    pages: [{ id: 'overview', title: 'Overview', href: '/dashboards/executive-sales/pages/overview', active: true }],
    components: [
      { id: 'title', kind: 'header', x: 16, y: 16, width: 456, height: 88, title: 'Executive Sales' },
      { id: 'state-filter', kind: 'filter', filter: 'state', x: 488, y: 16, width: 216, height: 88 },
      { id: 'orders-kpi', kind: 'visual', visual: 'orders_kpi', x: 720, y: 16, width: 240, height: 88 },
      { id: 'orders-chart', kind: 'visual', visual: 'orders_chart', x: 16, y: 128, width: 456, height: 280 },
      { id: 'orders-table', kind: 'visual', visual: 'orders', x: 16, y: 760, width: 944, height: 280 },
    ],
  }
  const signals = {
    page,
    filterConfig: [{ id: 'state', type: 'multi_select', label: 'State', dimension: 'orders.state', default: { values: [] }, operator: 'in', urlParam: 'state' }],
    filters: { controls: { state: { type: 'multi_select', operator: 'in', values: [] } }, selections: [] },
    filterOptions: { state: [{ value: 'SP', label: 'SP' }] }, visuals: testVisualizationSignals(),
    status: { loading: true, error: '', refreshId: 'refresh-3', generation: 3, lastUpdated: '2026-07-18T10:00:00Z', setupRequired: false, progressPercent: 50 },
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `<!doctype html><html><head><style>
    html, body { margin: 0; min-height: 100%; }
    body { --fontStack-system: system-ui; --ld-bg-app:#f6f8fa; --ld-bg-panel:#fff; --ld-bg-panel-muted:#f6f8fa; --ld-bg-control-hover:#f3f4f6; --ld-chart-surface:#fff; --ld-report-page-bg:#fff; --ld-report-canvas-bg:#eaeef2; --ld-report-rail-bg:#fff; --ld-bg-overlay:#fff; --ld-fg-default:#24292f; --ld-fg-muted:#57606a; --ld-fg-danger:#cf222e; --ld-fg-link:#0969da; --ld-line-muted:#d8dee4; --ld-border-default:1px solid #d0d7de; --ld-border-muted:1px solid #d8dee4; --ld-border-transparent:1px solid transparent; --ld-radius-default:6px; --ld-radius-full:999px; --ld-dashboard-filter-width:44px; --ld-dashboard-filter-open-width:320px; --base-size-2:2px; --base-size-4:4px; --base-size-6:6px; --base-size-8:8px; --base-size-10:10px; --base-size-12:12px; --base-size-16:16px; --base-size-20:20px; --base-size-24:24px; --control-medium-size:32px; --control-xlarge-size:40px; --ld-font-size-caption:12px; --ld-font-size-body-sm:14px; --ld-font-size-title-sm:16px; --ld-font-size-title-lg:28px; --ld-font-size-display:32px; --ld-font-weight-medium:500; --ld-font-weight-strong:600; --ld-line-height-none:1; --ld-line-height-tight:1.2; --ld-line-height-compact:1.3; --zIndex-dropdown:100; --zIndex-modal:200; --zIndex-sticky:50; --shadow-resting-small:0 1px 2px rgb(0 0 0/.08); --shadow-floating-small:0 8px 24px rgb(0 0 0/.12); --ld-duration-fast:160ms; --motion-easing-move:ease; --motion-transition-stateChange:160ms ease; }
    ld-dashboard-page { min-height: 720px; }
  </style></head><body><main data-signals="${attr(signals)}"><ld-dashboard-page></ld-dashboard-page></main>
  <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script><script type="module" src="/dashboard-page-under-test.js"></script></body></html>`
}

function testVisualizationEnvelopes() {
  const kpiRevision = `sha256:${'1'.repeat(64)}`
  const chartRevision = `sha256:${'2'.repeat(64)}`
  const tableRevision = `sha256:${'3'.repeat(64)}`
  const field = (id: string, role: string, dataType: string, label: string) => ({ id, role, dataType, nullable: false, label })
  const base = (title: string, fields: unknown[]) => ({ title, datasets: [{ id: 'primary', fields }], dataBudget: { maxRows: 1000, requiredCompleteness: 'complete' }, accessibility: { title, description: title }, interactions: [] })
  const inline = (revision: string, columns: string[], rows: unknown[][]) => ({ kind: 'inline', specRevision: revision, dataRevision: 1, generation: 3, datasets: [{ id: 'primary', specRevision: revision, dataRevision: 1, generation: 3, columns, rows, completeness: 'complete' }] })
  return {
    orders_kpi: { schemaVersion: 3, visualID: 'orders_kpi', rendererID: 'html', specRevision: kpiRevision, dataRevision: 1, spec: { ...base('Orders', [field('value', 'measure', 'decimal', 'Orders')]), kind: 'kpi', value: { dataset: 'primary', field: 'value' }, presentation: { trend: 'neutral', tone: 'ink', note: 'Filtered' } }, dataState: inline(kpiRevision, ['value'], [[42]]), selection: [], status: { kind: 'ready' }, diagnostics: [] },
    orders_chart: { schemaVersion: 3, visualID: 'orders_chart', rendererID: 'echarts', specRevision: chartRevision, dataRevision: 1, spec: { ...base('Orders by status', [field('label', 'identity', 'string', 'Status'), field('value', 'measure', 'decimal', 'Orders')]), kind: 'cartesian', mark: 'bar', interactions: [{ id: 'selection', kind: 'select', mappings: [{ source: { dataset: 'primary', field: 'label' }, targetFieldID: 'orders.status', targetFactID: 'orders' }], targets: ['orders_kpi', 'orders'], mode: 'multiple', requiresStableIdentity: true }], x: { dataset: 'primary', field: 'label' }, y: [{ dataset: 'primary', field: 'value' }], presentation: { legend: 'hidden', showLabels: false, smooth: false, stacked: false, showSymbols: true, dataZoom: false, area: false, step: false } }, dataState: inline(chartRevision, ['label', 'value'], [['delivered', 42], ['shipped', 7]]), selection: [], status: { kind: 'loading', message: 'Refreshing' }, diagnostics: [] },
    orders: { schemaVersion: 3, visualID: 'orders', rendererID: 'tanstack', specRevision: tableRevision, dataRevision: 1, spec: { ...base('Orders', [field('order_id', 'identity', 'string', 'Order')]), kind: 'table', dataBudget: { maxRows: 1000, requiredCompleteness: 'partial' }, columns: [{ field: { dataset: 'primary', field: 'order_id' }, label: 'Order', width: 180, formatting: [] }], defaultSort: [{ field: { dataset: 'primary', field: 'order_id' }, direction: 'ascending' }], presentation: { rowHeight: 28, striped: true, showHeader: true } }, dataState: { kind: 'windowed', specRevision: tableRevision, dataRevision: 1, generation: 3, schema: { id: 'primary', fields: [field('order_id', 'identity', 'string', 'Order')] }, cardinality: { kind: 'exact', count: 1 }, availableRows: 1, rowCap: 1000, chunkSize: 100, resetVersion: 0, sort: [{ field: { dataset: 'primary', field: 'order_id' }, direction: 'ascending' }], blocks: { a: { id: 'a', start: 0, rows: [['o1']], requestSeq: 0, resetVersion: 0, sort: [{ field: { dataset: 'primary', field: 'order_id' }, direction: 'ascending' }] } } }, selection: [], status: { kind: 'error', message: 'Ratings query failed' }, diagnostics: [{ code: 'query_failed', severity: 'error', message: 'Ratings query failed' }] },
  }
}

function testVisualizationSignals() {
  return Object.fromEntries(Object.entries(testVisualizationEnvelopes()).map(([id, envelope]) => {
    const { dataState, ...signal } = envelope
    return [id, { ...signal, dataState: { schemaVersion: 1, encoding: 'json', kind: dataState.kind, specRevision: dataState.specRevision, dataRevision: dataState.dataRevision, generation: dataState.generation, payload: JSON.stringify(dataState) } }]
  }))
}

function escapeHTML(value: string): string { return value.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;') }
