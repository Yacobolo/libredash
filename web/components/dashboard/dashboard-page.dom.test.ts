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

      const layout = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
        const canvas = element.shadowRoot.querySelector('ld-report-canvas') as any
        await canvas.updateComplete
        const root = canvas.shadowRoot
        const surface = root.querySelector('.surface') as HTMLElement
        const viewport = root.querySelector('.viewport') as HTMLElement
        const frame = root.querySelector('.frame') as HTMLElement
        const assigned = (root.querySelector('slot') as HTMLSlotElement).assignedElements() as HTMLElement[]
        const kpi = assigned.find((item) => item.querySelector('ld-kpi-card'))
        const chart = assigned.find((item) => item.querySelector('ld-echart'))
        const table = assigned.find((item) => item.querySelector('ld-report-table'))
        const rect = (item?: HTMLElement) => item ? item.getBoundingClientRect() : null
        return {
          mode: surface.dataset.presentationMode,
          scale: Number(surface.dataset.scale),
          viewportScrollable: viewport.scrollHeight > viewport.clientHeight,
          framePosition: getComputedStyle(frame).position,
          kpi: rect(kpi),
          chart: rect(chart),
          table: rect(table),
        }
      })

      if (viewport.name === 'mobile') {
        expect(layout.mode).toBe('responsive')
        expect(layout.scale).toBe(1)
        expect(layout.framePosition).toBe('relative')
        expect(layout.kpi?.width ?? 0).toBeGreaterThanOrEqual(300)
        expect(layout.kpi?.height ?? 0).toBeLessThan(300)
        expect(layout.chart?.height ?? 0).toBeGreaterThanOrEqual(280)
        expect(layout.table?.height ?? 0).toBeLessThanOrEqual(700)
        expect(layout.table?.top ?? 0).toBeGreaterThan(layout.chart?.bottom ?? 0)
      } else {
        expect(layout.mode).toBe('fit-width')
        expect(layout.viewportScrollable).toBe(true)
      }

      const footerState = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
        const footer = element.shadowRoot.querySelector('ld-report-footer') as any
        await footer.updateComplete
        const initial = footer.shadowRoot.querySelector('.status')?.textContent?.replace(/\s+/g, ' ').trim()
        footer.status = { ...footer.status, loading: true }
        await footer.updateComplete
        const loading = footer.shadowRoot.querySelector('.status')?.textContent?.replace(/\s+/g, ' ').trim()
        footer.status = { ...footer.status, error: 'query failed' }
        await footer.updateComplete
        const failed = footer.shadowRoot.querySelector('.status')?.textContent?.replace(/\s+/g, ' ').trim()
        return { initial, loading, failed }
      })
      expect(footerState.initial).toStartWith('Data refreshed ')
      expect(footerState.initial).not.toContain('2026-07-18T10:00:00Z')
      expect(footerState.loading).toBe(footerState.initial)
      expect(footerState.failed).toBe('Unable to update visuals')

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

      const progressiveState = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
        const root = element.shadowRoot
        const chartFrame = root.querySelector('[data-component-status-key="visual:orders_chart"]') as any
        const tableFrame = root.querySelector('[data-component-status-key="visual:orders"]') as any
        const kpiFrame = root.querySelector('[data-component-status-key="visual:orders_kpi"]') as any
        const chart = chartFrame?.querySelector('ld-echart') as any
        const table = tableFrame?.querySelector('ld-report-table') as any
        await Promise.all([chartFrame?.updateComplete, tableFrame?.updateComplete, chart?.updateComplete, table?.updateComplete])
        return {
          chartBusy: chartFrame?.shadowRoot?.querySelector('article')?.getAttribute('aria-busy'),
          chartLoadingLabel: chartFrame?.shadowRoot?.querySelector('[role="status"]')?.getAttribute('aria-label'),
          tableAlert: tableFrame?.shadowRoot?.querySelector('[role="alert"]')?.textContent?.replace(/\s+/g, ' ').trim(),
          tableRetainedRow: table?.shadowRoot?.textContent?.includes('o1'),
          kpiHasOverlay: Boolean(kpiFrame?.shadowRoot?.querySelector('.refresh-overlay')),
          chartSelection: chart?.chart?.selection,
          tableSelection: table?.table?.selection,
        }
      })

      expect(progressiveState).toEqual({
        chartBusy: 'true',
        chartLoadingLabel: 'Refreshing component',
        tableAlert: 'Could not refresh this component Ratings query failed',
        tableRetainedRow: true,
        kpiHasOverlay: false,
        chartSelection: [{ mappings: [{ field: 'orders.status', value: 'delivered', label: 'Delivered' }], label: 'Delivered' }],
        tableSelection: [{ mappings: [{ field: 'orders.order_id', value: 'o1', label: 'o1' }], label: 'o1' }],
      })

      if (viewport.name === 'desktop') {
        const updateIsolation = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
          const chart = element.shadowRoot.querySelector('ld-echart') as any
          const direct = document.createElement('ld-echart') as any
          direct.chart = structuredClone(chart.chart)
          document.body.append(direct)
          await direct.updateComplete
          direct.rendererHandle?.dispose()
          let directRendererUpdates = 0
          direct.rendererHandle = {
            update: () => { directRendererUpdates += 1 },
            resize: () => {},
            clear: () => {},
            dispose: () => {},
          }
          direct.rendererName = 'echarts'
          direct.chart = structuredClone(direct.chart)
          await direct.updateComplete
          const afterEquivalentClone = directRendererUpdates
          direct.chart = { ...direct.chart, data: [{ label: 'delivered', value: 99 }] }
          await direct.updateComplete
          const afterDirectDataChange = directRendererUpdates
          direct.remove()

          let rendererUpdates = 0
          chart.renderChart = () => { rendererUpdates += 1 }
          const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')

          mergePatch({ componentStatus: { 'visual:orders': { generation: 3, loading: true, error: '' } } })
          await element.updateComplete
          await chart.updateComplete
          const afterUnrelatedPatch = rendererUpdates

          mergePatch({ visuals: { orders_chart: { data: [{ label: 'delivered', value: 43 }, { label: 'shipped', value: 7 }] } } })
          await element.updateComplete
          await chart.updateComplete
          return { afterEquivalentClone, afterDirectDataChange, afterUnrelatedPatch, afterChartPatch: rendererUpdates }
        })

        expect(updateIsolation).toEqual({
          afterEquivalentClone: 0,
          afterDirectDataChange: 1,
          afterUnrelatedPatch: 0,
          afterChartPatch: 1,
        })
      }

      if (viewport.name === 'desktop') {
        const optimistic = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
          const root = element.shadowRoot
          const chart = root.querySelector('ld-echart') as any
          const detail = (value: string) => ({
            sourceKind: 'visual',
            sourceId: 'orders_chart',
            interactionKind: 'point_selection',
            action: 'replace',
            toggle: true,
            mappings: [{ field: 'orders.status', value, label: value }],
          })
          chart.dispatchEvent(new CustomEvent('ld-interaction-select', { bubbles: true, composed: true, detail: detail('processing') }))
          chart.dispatchEvent(new CustomEvent('ld-interaction-select', { bubbles: true, composed: true, detail: detail('complete') }))
          await element.updateComplete
          await chart.updateComplete

          const kpiFrame = root.querySelector('[data-component-status-key="visual:orders_kpi"]') as any
          const tableFrame = root.querySelector('[data-component-status-key="visual:orders"]') as any
          await Promise.all([kpiFrame.updateComplete, tableFrame.updateComplete])
          const pending = {
            selection: chart.chart.selection,
            kpiBusy: kpiFrame.shadowRoot.querySelector('article')?.getAttribute('aria-busy'),
            tableBusy: tableFrame.shadowRoot.querySelector('article')?.getAttribute('aria-busy'),
          }

          const beforeForged = JSON.stringify(chart.chart.selection)
          chart.dispatchEvent(new CustomEvent('ld-interaction-select', {
            bubbles: true,
            composed: true,
            detail: { ...detail('forged'), mappings: [{ field: 'orders.secret', value: 'forged' }] },
          }))
          await element.updateComplete

          const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
          mergePatch({
            status: { generation: 4, refreshId: 'refresh-4', loading: true },
            filters: {
              selections: [{
                id: 'visual:orders_chart:point_selection',
                sourceKind: 'visual',
                sourceId: 'orders_chart',
                interactionKind: 'point_selection',
                label: 'complete',
                order: 1,
                entries: [{ mappings: [{ field: 'orders.status', value: 'complete', label: 'complete' }], label: 'complete' }],
              }],
            },
          })
          await element.updateComplete
          await chart.updateComplete
          return {
            pending,
            forgedRolledBack: JSON.stringify(chart.chart.selection) === beforeForged,
            canonical: chart.chart.selection,
          }
        })

        expect(optimistic).toEqual({
          pending: {
            selection: [{ mappings: [{ field: 'orders.status', value: 'complete', label: 'complete' }], label: 'complete' }],
            kpiBusy: 'true',
            tableBusy: 'true',
          },
          forgedRolledBack: true,
          canonical: [{ mappings: [{ field: 'orders.status', value: 'complete', label: 'complete' }], label: 'complete' }],
        })
      }

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

test('dashboard refresh progress follows only the latest generation', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (
      customElements.get('ld-dashboard-page')
        && (document.querySelector('ld-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard'
    ))

    const states = await page.locator('ld-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      const read = async () => {
        await element.updateComplete
        const progress = element.shadowRoot.querySelector('[data-dashboard-refresh-progress]')
        const value = progress?.querySelector('.dashboard-refresh-progress-value')
        return {
          active: progress?.getAttribute('data-active'),
          complete: progress?.getAttribute('data-complete'),
          generation: progress?.getAttribute('data-generation'),
          now: progress?.getAttribute('aria-valuenow'),
          max: progress?.getAttribute('aria-valuemax'),
          text: progress?.getAttribute('aria-valuetext'),
          indeterminate: progress?.hasAttribute('data-indeterminate'),
          width: value?.getAttribute('style'),
          animationName: getComputedStyle(value).animationName,
          fadeDelay: getComputedStyle(progress).transitionDelay,
        }
      }

      const initial = await read()
      mergePatch({
        status: {
          generation: 4,
          refreshId: 'refresh-4',
          loading: true,
          progressPercent: null,
        },
        componentStatus: {
          'visual:orders_chart': { generation: 4, loading: true, error: '' },
        },
      })
      const planning = await read()
      mergePatch({
        status: {
          generation: 4,
          refreshId: 'refresh-4',
          loading: true,
          progressPercent: 0,
        },
        componentStatus: {
          'visual:orders_chart': { generation: 4, loading: true, error: '' },
        },
      })
      const started = await read()
      mergePatch({
        status: { progressPercent: 33.33333333333333 },
        componentStatus: {
          'visual:orders_chart': { generation: 4, loading: false, error: '' },
          'visual:stale': { generation: 3, loading: false, error: '' },
        },
      })
      const progressive = await read()
      mergePatch({
        componentStatus: {
          'visual:orders_kpi': { generation: 4, loading: false, error: 'Query failed' },
          'visual:orders': { generation: 4, loading: false, error: '' },
        },
        status: {
          generation: 4,
          refreshId: 'refresh-4',
          loading: false,
          progressPercent: 100,
        },
      })
      const complete = await read()
      return { initial, planning, started, progressive, complete }
    })

    expect(states).toEqual({
      initial: {
        active: 'true',
        complete: 'false',
        generation: '3',
        now: '50',
        max: '100',
        text: '50% of dashboard refresh complete',
        indeterminate: false,
        width: 'width:50%',
        animationName: 'none',
        fadeDelay: '0s',
      },
      planning: {
        active: 'true',
        complete: 'false',
        generation: '4',
        now: '0',
        max: '100',
        text: '0% of dashboard refresh complete',
        indeterminate: false,
        width: 'width:0%',
        animationName: 'none',
        fadeDelay: '0s',
      },
      started: {
        active: 'true',
        complete: 'false',
        generation: '4',
        now: '0',
        max: '100',
        text: '0% of dashboard refresh complete',
        indeterminate: false,
        width: 'width:0%',
        animationName: 'none',
        fadeDelay: '0s',
      },
      progressive: {
        active: 'true',
        complete: 'false',
        generation: '4',
        now: '33.33333333333333',
        max: '100',
        text: '33% of dashboard refresh complete',
        indeterminate: false,
        width: 'width:33.33333333333333%',
        animationName: 'none',
        fadeDelay: '0s',
      },
      complete: {
        active: 'false',
        complete: 'true',
        generation: '4',
        now: '100',
        max: '100',
        text: '100% of dashboard refresh complete',
        indeterminate: false,
        width: 'width:100%',
        animationName: 'none',
        fadeDelay: '0.18s',
      },
    })
  } finally {
    await page.close()
  }
})

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
      { id: 'state-filter', kind: 'filter', filter: 'state', x: 488, y: 16, width: 216, height: 88 },
      { id: 'orders-kpi', kind: 'visual', visual: 'orders_kpi', x: 720, y: 16, width: 240, height: 88 },
      { id: 'orders-chart', kind: 'visual', visual: 'orders_chart', x: 16, y: 128, width: 456, height: 280 },
      { id: 'orders-table', kind: 'visual', visual: 'orders', x: 16, y: 760, width: 944, height: 280 },
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
  const filters = {
    controls: { state: { type: 'multi_select', operator: 'in', values: [] } },
    selections: [
      {
        id: 'visual:orders_chart:point_selection',
        sourceKind: 'visual',
        sourceId: 'orders_chart',
        interactionKind: 'point_selection',
        label: 'Delivered',
        order: 1,
        entries: [{ mappings: [{ field: 'orders.status', value: 'delivered', label: 'Delivered' }], label: 'Delivered' }],
      },
      {
        id: 'visual:orders:row_selection',
        sourceKind: 'visual',
        sourceId: 'orders',
        interactionKind: 'row_selection',
        label: 'o1',
        order: 2,
        entries: [{ mappings: [{ field: 'orders.order_id', value: 'o1', label: 'o1' }], label: 'o1' }],
      },
    ],
  }
  const visuals = {
    orders_kpi: {
      version: 3,
      id: 'orders_kpi',
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
      shape: 'category_value',
      renderer: 'echarts',
      type: 'bar',
      title: 'Orders by status',
      unit: 'orders',
      interaction: { kind: 'point_selection', toggle: true, mappings: [{ field: 'orders.status', value: 'label' }], targets: ['orders_kpi', 'orders'] },
      dimensions: ['status'],
      measure: 'order_count',
      measures: ['order_count'],
      series: [],
      options: {},
      rendererOptions: {},
      selection: [{ mappings: [{ field: 'orders.status', value: 'shipped', label: 'Shipped' }], label: 'Shipped' }],
      data: [{ label: 'delivered', value: 42 }, { label: 'shipped', value: 7 }],
    },
    orders: {
      version: 2,
      id: 'orders',
      type: 'table',
      title: 'Orders',
      style: { density: 'compact', zebra: true, grid: 'full' },
      interaction: { kind: 'row_selection', toggle: false, mappings: [{ field: 'orders.order_id', value: 'order_id' }] },
      selection: [{ mappings: [{ field: 'orders.order_id', value: 'server-value', label: 'Server value' }] }],
      columns: [{ key: 'order_id', label: 'Order', width: 180 }],
		cardinality: { kind: 'exact', value: 1 },
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
  const status = {
    loading: true,
    error: '',
    refreshId: 'refresh-3',
    generation: 3,
    lastUpdated: '2026-07-18T10:00:00Z',
    setupRequired: false,
    progressPercent: 50,
  }
  const componentStatus = {
    'visual:orders_chart': { generation: 3, loading: true, error: '' },
    'visual:orders': { generation: 3, loading: false, error: 'Ratings query failed' },
  }
  const signals = {
    page,
    filterConfig,
    filters,
    filterOptions: { state: [{ value: 'SP', label: 'SP' }] },
    componentStatus,
    visuals,
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
