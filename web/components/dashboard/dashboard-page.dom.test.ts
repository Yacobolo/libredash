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
        customElements.get('lv-dashboard-page')
          && customElements.get('lv-filter-dock')
          && customElements.get('lv-filter-panel')
          && customElements.get('lv-filter-card')
          && customElements.get('lv-kpi-card')
          && customElements.get('lv-echart')
          && customElements.get('lv-report-table')
      ))
      await page.waitForFunction(() => (document.querySelector('lv-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
      await page.locator('lv-dashboard-page').evaluate((element: any) => element.updateComplete)

      const state = await page.locator('lv-dashboard-page').evaluate((element: any) => {
        const root = element.shadowRoot
        const tags = Array.from(root.querySelectorAll('*')).map((node: Element) => node.localName)
        const filterDock = root.querySelector('lv-filter-dock')
        const rect = root.querySelector('lv-report-canvas')?.getBoundingClientRect()
        return {
          title: root.querySelector('h1')?.textContent?.trim(),
          hasSubSidebar: tags.includes('lv-sub-sidebar'),
          hasCanvas: tags.includes('lv-report-canvas'),
          hasFilterCard: tags.includes('lv-filter-card'),
          hasKpi: tags.includes('lv-kpi-card'),
          hasChart: tags.includes('lv-echart'),
          hasTable: tags.includes('lv-report-table'),
          hasFilterDock: tags.includes('lv-filter-dock'),
          hasFilterPanel: Boolean(filterDock?.shadowRoot?.querySelector('lv-filter-panel')),
          hasFooter: tags.includes('lv-report-footer'),
          hasModal: tags.includes('lv-visual-modal'),
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

      const layout = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
        const canvas = element.shadowRoot.querySelector('lv-report-canvas') as any
        await canvas.updateComplete
        const root = canvas.shadowRoot
        const surface = root.querySelector('.surface') as HTMLElement
        const viewport = root.querySelector('.viewport') as HTMLElement
        const frame = root.querySelector('.frame') as HTMLElement
        const assigned = (root.querySelector('slot') as HTMLSlotElement).assignedElements() as HTMLElement[]
        const kpi = assigned.find((item) => item.querySelector('lv-kpi-card'))
        const chart = assigned.find((item) => item.querySelector('lv-echart'))
        const table = assigned.find((item) => item.querySelector('lv-report-table'))
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

      const footerState = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
        const footer = element.shadowRoot.querySelector('lv-report-footer') as any
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

      const tableState = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
        const table = element.shadowRoot.querySelector('lv-report-table') as any
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

      const progressiveState = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
        const root = element.shadowRoot
        const chartFrame = root.querySelector('[data-component-status-key="visual:orders_chart"]') as any
        const tableFrame = root.querySelector('[data-component-status-key="visual:orders"]') as any
        const kpiFrame = root.querySelector('[data-component-status-key="visual:orders_kpi"]') as any
        const chart = chartFrame?.querySelector('lv-echart') as any
        const table = tableFrame?.querySelector('lv-report-table') as any
        await Promise.all([chartFrame?.updateComplete, tableFrame?.updateComplete, chart?.updateComplete, table?.updateComplete])
        const chartSpinner = chartFrame?.shadowRoot?.querySelector('lv-loading-spinner') as any
        await chartSpinner?.updateComplete
        const chartSpinnerSvg = chartSpinner?.shadowRoot?.querySelector('svg') as SVGElement | null
        const chartIndicator = chartFrame?.shadowRoot?.querySelector('.loading-indicator.header') as HTMLElement | null
        return {
          chartBusy: chartFrame?.shadowRoot?.querySelector('article')?.getAttribute('aria-busy'),
          chartLoadingLabel: chartFrame?.shadowRoot?.querySelector('[role="status"]')?.getAttribute('aria-label'),
          chartIndicatorPlacement: chartIndicator?.className ?? '',
          chartIndicatorDelay: chartIndicator ? getComputedStyle(chartIndicator).animationDelay : '',
          chartHasFullOverlay: Boolean(chartFrame?.shadowRoot?.querySelector('.refresh-overlay.loading')),
          chartSpinnerDuration: chartSpinnerSvg ? getComputedStyle(chartSpinnerSvg).animationDuration : '',
          chartSpinnerIsNeutral: Boolean(chartSpinner && chartIndicator && getComputedStyle(chartSpinner).color === getComputedStyle(chartIndicator).color),
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
        chartIndicatorPlacement: 'loading-indicator header',
        chartIndicatorDelay: '0.5s',
        chartHasFullOverlay: false,
        chartSpinnerDuration: '1.8s',
        chartSpinnerIsNeutral: true,
        tableAlert: 'Could not refresh this component Ratings query failed',
        tableRetainedRow: true,
        kpiHasOverlay: false,
        chartSelection: [{ mappings: [{ field: 'orders.status', value: 'delivered', label: 'Delivered' }], label: 'Delivered' }],
        tableSelection: [{ mappings: [{ field: 'orders.order_id', value: 'o1', label: 'o1' }], label: 'o1' }],
      })

      if (viewport.name === 'desktop') {
        const updateIsolation = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
          const chart = element.shadowRoot.querySelector('lv-echart') as any
          const direct = document.createElement('lv-echart') as any
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
        const optimistic = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
          const root = element.shadowRoot
          const chart = root.querySelector('lv-echart') as any
          const detail = (value: string) => ({
            sourceKind: 'visual',
            sourceId: 'orders_chart',
            interactionKind: 'point_selection',
            action: 'replace',
            toggle: true,
            mappings: [{ field: 'orders.status', value, label: value }],
          })
          chart.dispatchEvent(new CustomEvent('lv-interaction-select', { bubbles: true, composed: true, detail: detail('processing') }))
          chart.dispatchEvent(new CustomEvent('lv-interaction-select', { bubbles: true, composed: true, detail: detail('complete') }))
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
          chart.dispatchEvent(new CustomEvent('lv-interaction-select', {
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
        const dockState = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
          const dock = element.shadowRoot.querySelector('lv-filter-dock') as HTMLElement
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

test('embed presentation keeps page navigation and removes non-navigation chrome', async () => {
  const page = await browser.newPage({ viewport: { width: 760, height: 620 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (document.querySelector('lv-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard')
    const state = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      element.presentation = 'embed'
      await element.updateComplete
      const root = element.shadowRoot
      const visible = (selector: string) => {
        const node = root.querySelector(selector) as HTMLElement | null
        return Boolean(node && getComputedStyle(node).display !== 'none')
      }
      const canvas = root.querySelector('lv-report-canvas') as HTMLElement
      return {
        reflected: element.getAttribute('presentation'),
        sidebarVisible: visible('lv-sub-sidebar'),
        headerVisible: visible('.header'),
        footerVisible: visible('lv-report-footer'),
        hasAgentToggle: Boolean(root.querySelector('.agent-toggle')),
        hasAgentDrawer: Boolean(root.querySelector('lv-chat-drawer')),
        agentActionCount: root.querySelectorAll('.ask-visual').length,
        canvasWidth: canvas.getBoundingClientRect().width,
        documentOverflow: document.documentElement.scrollWidth - window.innerWidth,
      }
    })
    expect(state.reflected).toBe('embed')
    expect(state.sidebarVisible).toBe(true)
    expect(state.headerVisible).toBe(false)
    expect(state.footerVisible).toBe(false)
    expect(state.hasAgentToggle).toBe(false)
    expect(state.hasAgentDrawer).toBe(false)
    expect(state.agentActionCount).toBe(0)
    expect(state.canvasWidth).toBeGreaterThan(500)
    expect(state.documentOverflow).toBe(0)
  } finally {
    await page.close()
  }
})

test('visual frame delays and distinguishes initial loading from background refresh', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('lv-dashboard-visual-frame'))
    const state = await page.evaluate(async () => {
      const frame = document.createElement('lv-dashboard-visual-frame') as any
      frame.refreshStatus = { generation: 4, loading: true, error: '' }
      frame.loadingPresentation = 'center'
      document.body.append(frame)
      await frame.updateComplete
      const center = frame.shadowRoot.querySelector('.loading-indicator.center') as HTMLElement
      const centerSpinner = center.querySelector('lv-loading-spinner') as HTMLElement
      const initial = {
        delay: getComputedStyle(center).animationDelay,
        background: getComputedStyle(center).backgroundColor,
        spinnerSize: getComputedStyle(centerSpinner).width,
      }

      frame.loadingPresentation = 'header'
      await frame.updateComplete
      const header = frame.shadowRoot.querySelector('.loading-indicator.header') as HTMLElement
      const headerSpinner = header.querySelector('lv-loading-spinner') as HTMLElement
      const refresh = {
        delay: getComputedStyle(header).animationDelay,
        background: getComputedStyle(header).backgroundColor,
        spinnerSize: getComputedStyle(headerSpinner).width,
      }
      frame.remove()
      return { initial, refresh }
    })

    expect(state).toEqual({
      initial: { delay: '0.25s', background: 'rgb(255, 255, 255)', spinnerSize: '24px' },
      refresh: { delay: '0.5s', background: 'rgba(0, 0, 0, 0)', spinnerSize: '12px' },
    })
  } finally {
    await page.close()
  }
})

test('dashboard refresh progress follows only the latest generation', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && (document.querySelector('lv-dashboard-page') as any)?.page?.title === 'Executive Sales Dashboard'
    ))

    const states = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
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
        fadeDelay: '0.25s',
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
        fadeDelay: '0.25s',
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
        fadeDelay: '0.25s',
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
        fadeDelay: '0.25s',
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

test('dashboard agent drawer carries page context and explicit visual references', async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && customElements.get('lv-chat-drawer')
        && customElements.get('lv-chat-composer')
    ))
    await page.locator('lv-dashboard-page').evaluate((element: any) => element.updateComplete)

    const initial = await page.locator('lv-dashboard-page').evaluate((element: any) => {
      const root = element.shadowRoot
      const drawer = root.querySelector('lv-chat-drawer') as any
      const toggle = root.querySelector('.agent-toggle') as HTMLButtonElement
      const toggleStyle = getComputedStyle(toggle)
      return {
        hasToggle: Boolean(toggle),
        toggleHasVisibleSurface: toggleStyle.borderColor !== 'rgba(0, 0, 0, 0)'
          && toggleStyle.backgroundColor !== 'rgba(0, 0, 0, 0)',
        open: drawer?.open,
        drawerWidth: Math.round(drawer?.getBoundingClientRect().width ?? 0),
      }
    })
    expect(initial).toEqual({ hasToggle: true, toggleHasVisibleSurface: true, open: false, drawerWidth: 0 })

    const visualActionsAtRest = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const root = element.shadowRoot
      const frame = root.querySelector('[data-component-status-key="visual:orders_chart"]') as any
      const chart = frame?.querySelector('lv-echart') as any
      const kpi = root.querySelector('lv-kpi-card') as any
      const table = root.querySelector('lv-report-table') as any
      await Promise.all([frame?.updateComplete, chart?.updateComplete, kpi?.updateComplete, table?.updateComplete])
      const ask = chart.querySelector('.ask-visual') as HTMLElement
      const kpiAsk = kpi.querySelector('.ask-visual') as HTMLElement
      const tableAsk = table.querySelector('.ask-visual') as HTMLElement
      const askStyle = getComputedStyle(ask)
      const expand = chart.shadowRoot.querySelector('[aria-label="Expand visual"]') as HTMLElement
      const options = chart.shadowRoot.querySelector('[aria-label="Visual options"]') as HTMLElement
      const agentIconMarkup = root.querySelector('.agent-toggle svg')?.innerHTML
      const drawer = root.querySelector('lv-chat-drawer') as any
      return {
        askOpacity: askStyle.opacity,
        askPointerEvents: askStyle.pointerEvents,
        askBackground: askStyle.backgroundColor,
        askBoxShadow: askStyle.boxShadow,
        askRight: ask.getBoundingClientRect().right,
        expandLeft: expand.getBoundingClientRect().left,
        askActionRow: ask.assignedSlot?.parentElement?.className,
        kpiAskActionRow: kpiAsk.assignedSlot?.parentElement?.className,
        tableAskActionRow: tableAsk.assignedSlot?.parentElement?.className,
        askPressed: ask.getAttribute('aria-pressed'),
        askUsesAgentIcon: ask.querySelector('svg')?.innerHTML === agentIconMarkup
          && drawer.shadowRoot.querySelector('.title svg')?.innerHTML === agentIconMarkup,
        chartActions: [expand, options].map((control) => control.getAttribute('aria-label')),
        chartHasExport: Array.from(chart.shadowRoot.querySelectorAll('[role="menuitem"]'))
          .some((item: any) => item.textContent?.trim() === 'Export CSV'),
        tableHasExpand: Boolean(table.shadowRoot.querySelector('[aria-label="Expand table"]')),
        tableHasExport: Array.from(table.shadowRoot.querySelectorAll('[role="menuitem"]'))
          .some((item: any) => item.textContent?.trim() === 'Export CSV'),
      }
    })
    expect(visualActionsAtRest).toMatchObject({
      askOpacity: '0',
      askPointerEvents: 'none',
      askBackground: 'rgba(0, 0, 0, 0)',
      askBoxShadow: 'none',
      askActionRow: 'visual-actions',
      kpiAskActionRow: 'visual-actions',
      tableAskActionRow: 'visual-actions',
      askPressed: 'false',
      askUsesAgentIcon: true,
      chartActions: ['Expand visual', 'Visual options'],
      chartHasExport: true,
      tableHasExpand: true,
      tableHasExport: true,
    })

    await page.locator('lv-dashboard-visual-frame[data-component-status-key="visual:orders_chart"]').hover()
    const visualActionsOnHover = await page.locator('lv-dashboard-page').evaluate((element: any) => {
      const frame = element.shadowRoot.querySelector('[data-component-status-key="visual:orders_chart"]') as any
      const chart = frame.querySelector('lv-echart') as any
      const ask = chart.querySelector('.ask-visual') as HTMLElement
      const expand = chart.shadowRoot.querySelector('[aria-label="Expand visual"]') as HTMLElement
      const askStyle = getComputedStyle(ask)
      return {
        askOpacity: askStyle.opacity,
        askPointerEvents: askStyle.pointerEvents,
        askRight: ask.getBoundingClientRect().right,
        expandLeft: expand.getBoundingClientRect().left,
      }
    })
    expect(visualActionsOnHover.askOpacity).toBe('1')
    expect(visualActionsOnHover.askPointerEvents).toBe('auto')
    expect(visualActionsOnHover.askRight).toBeLessThanOrEqual(visualActionsOnHover.expandLeft)

    await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      element.shadowRoot.querySelector('.agent-toggle').click()
      await element.updateComplete
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer')
      await drawer.updateComplete
    })
    await page.waitForFunction(() => {
      const dashboard = document.querySelector('lv-dashboard-page') as any
      const drawer = dashboard?.shadowRoot?.querySelector('lv-chat-drawer')
      return (drawer?.getBoundingClientRect().width ?? 0) >= 419.9
    })

    const opened = await page.locator('lv-dashboard-page').evaluate((element: any) => {
      const root = element.shadowRoot
      const drawer = root.querySelector('lv-chat-drawer') as any
      const drawerRoot = drawer.shadowRoot
      const drawerSurface = drawerRoot.querySelector('.drawer') as HTMLElement
      const header = drawerRoot.querySelector('.header') as HTMLElement
      const context = drawerRoot.querySelector('.context') as HTMLElement
      const toolbarAction = drawerRoot.querySelector('.toolbar-actions button') as HTMLElement
      const thread = drawerRoot.querySelector('lv-chat-thread') as any
      const composer = drawerRoot.querySelector('lv-chat-composer') as any
      const toggle = root.querySelector('.agent-toggle') as HTMLButtonElement
      const toggleRect = toggle.getBoundingClientRect()
      const toggleIconRect = toggle.querySelector('svg')!.getBoundingClientRect()
      return {
        open: drawer.open,
        drawerWidth: Math.round(drawer.getBoundingClientRect().width),
        pageContext: drawerRoot.querySelector('.page-context')?.textContent?.replace(/\s+/g, ' ').trim(),
        filterContext: drawerRoot.querySelector('.filter-context')?.textContent?.replace(/\s+/g, ' ').trim(),
        hasThread: Boolean(thread),
        hasComposer: Boolean(composer),
        contextInHeader: header.contains(context),
        contextBorder: getComputedStyle(context).borderBottomStyle,
        contextSharesSurface: getComputedStyle(context).backgroundColor === getComputedStyle(drawerSurface).backgroundColor,
        toolbarActionBorder: toolbarAction ? getComputedStyle(toolbarAction).borderStyle : 'missing',
        threadSharesSurface: getComputedStyle(thread.shadowRoot.querySelector('.thread')).backgroundColor === getComputedStyle(drawerSurface).backgroundColor,
        composerDockBorder: getComputedStyle(composer).borderTopStyle,
        composerShadow: getComputedStyle(composer.shadowRoot.querySelector('.composer-surface')).boxShadow,
        composerHeight: Math.round(composer.shadowRoot.querySelector('.composer-surface').getBoundingClientRect().height),
        toggleIconCenterOffset: Math.abs((toggleRect.left + toggleRect.width / 2) - (toggleIconRect.left + toggleIconRect.width / 2)),
      }
    })
    expect(opened).toMatchObject({
      open: true,
      pageContext: 'Overview',
      filterContext: '1 filter · 2 selections',
      hasThread: true,
      hasComposer: true,
      contextInHeader: true,
      contextBorder: 'none',
      contextSharesSurface: true,
      toolbarActionBorder: 'none',
      threadSharesSurface: true,
      toggleIconCenterOffset: 0,
      composerDockBorder: 'none',
      composerShadow: 'none',
    })
    expect(opened.composerHeight).toBeLessThan(80)
    expect(opened.drawerWidth).toBeGreaterThanOrEqual(360)
    expect(opened.drawerWidth).toBeLessThanOrEqual(520)

    const groupedSearch = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      mergePatch({ agentReferenceSearch: {
        query: 'orders', requestId: 1,
        results: [
		  { reference: { workspaceId: 'sales', type: 'visual', id: 'executive-sales.orders_chart' }, name: 'Orders by status', workspace: { id: 'sales', name: 'Sales' }, hierarchy: ['Sales', 'Executive Sales', 'Overview'], href: '/orders', locations: [{ dashboardId: 'executive-sales', pageId: 'overview', href: '/orders' }], context: ['current_page'] },
		  { reference: { workspaceId: 'finance', type: 'visual', id: 'executive-sales.foreign_orders' }, name: 'Finance orders', description: 'From another workspace', workspace: { id: 'finance', name: 'Finance' }, hierarchy: ['Finance', 'Executive Sales', 'Overview'], href: '/finance', locations: [{ dashboardId: 'executive-sales', pageId: 'overview', href: '/finance' }], context: [] },
		  { reference: { workspaceId: 'sales', type: 'measure', id: 'olist.order_count' }, name: 'Orders count', description: 'Across the sales workspace', workspace: { id: 'sales', name: 'Sales' }, hierarchy: ['Sales', 'Olist'], href: '/measure', locations: [], context: ['current_workspace'] },
        ],
      } })
      await element.updateComplete
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      await drawer.updateComplete
      const composer = drawer.shadowRoot.querySelector('lv-chat-composer') as any
      const textarea = composer.shadowRoot.querySelector('textarea') as HTMLTextAreaElement
      textarea.value = '@orders'
      textarea.setSelectionRange(textarea.value.length, textarea.value.length)
      textarea.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true }))
      await composer.updateComplete
      return {
        labels: Array.from(composer.shadowRoot.querySelectorAll('.mention-section-label')).map((node: any) => node.textContent.trim()),
        options: Array.from(composer.shadowRoot.querySelectorAll('.mention-option')).map((node: any) => node.textContent.replace(/\s+/g, ' ').trim()),
        onPage: Array.from(composer.shadowRoot.querySelector('[aria-label="On this page"]')?.querySelectorAll('.mention-option') ?? []).map((node: any) => node.textContent.replace(/\s+/g, ' ').trim()),
        accessible: Array.from(composer.shadowRoot.querySelector('[aria-label="All accessible"]')?.querySelectorAll('.mention-option') ?? []).map((node: any) => node.textContent.replace(/\s+/g, ' ').trim()),
      }
    })
    expect(groupedSearch.labels).toEqual(['On this page', 'All accessible'])
    expect(groupedSearch.options[0]).toContain('Orders')
	expect(groupedSearch.onPage).not.toContain('Finance orders Finance › Executive Sales › Overview Visual')
	expect(groupedSearch.accessible).toContain('Finance orders Finance › Executive Sales › Overview Visual')
	expect(groupedSearch.options.at(-1)).toBe('Orders count Sales › Olist Measure')

    await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      mergePatch({ agentContext: { referenceLimit: 1 } })
      await element.updateComplete
    })

    await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const frame = Array.from(element.shadowRoot.querySelectorAll('lv-dashboard-visual-frame'))
        .find((candidate: any) => candidate.getAttribute('data-component-status-key') === 'visual:orders_chart') as any
      frame.querySelector('.ask-visual').click()
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      await drawer.updateComplete
    })

    const referenced = await page.locator('lv-dashboard-page').evaluate((element: any) => {
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      const drawerRoot = drawer.shadowRoot
      const composerRoot = drawerRoot.querySelector('lv-chat-composer')?.shadowRoot
      return {
        chip: composerRoot?.querySelector('.reference-chip')?.textContent?.replace(/\s+/g, ' ').trim(),
        highlighted: Boolean(element.shadowRoot.querySelector('lv-dashboard-visual-frame[data-agent-referenced]')),
		pressed: element.shadowRoot.querySelector('[data-component-status-key="visual:orders_chart"] .ask-visual')?.getAttribute('aria-pressed'),
      }
    })
    expect(referenced).toEqual({ chip: 'Orders by status', highlighted: true, pressed: 'true' })

    const limitReached = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const frame = Array.from(element.shadowRoot.querySelectorAll('lv-dashboard-visual-frame'))
        .find((candidate: any) => candidate.getAttribute('data-component-status-key') === 'visual:orders_kpi') as any
      frame.querySelector('.ask-visual').click()
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      await drawer.updateComplete
      const composer = drawer.shadowRoot.querySelector('lv-chat-composer') as any
      await composer.updateComplete
      return {
        chips: Array.from(composer.shadowRoot.querySelectorAll('.reference-chip')).map((node: any) => node.textContent?.replace(/\s+/g, ' ').trim()),
        status: drawer.shadowRoot.querySelector('[data-reference-limit-status]')?.textContent?.replace(/\s+/g, ' ').trim(),
      }
    })
    expect(limitReached).toEqual({ chips: ['Orders by status'], status: 'Up to 1 item can be attached' })

    const submitted = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const received: any[] = []
      element.addEventListener('lv-chat-submit', (event: CustomEvent) => received.push(event.detail), { once: true })
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      const composer = drawer.shadowRoot.querySelector('lv-chat-composer') as any
      const textarea = composer.shadowRoot.querySelector('textarea') as HTMLTextAreaElement
      textarea.value = 'Why did this decline?'
      textarea.dispatchEvent(new InputEvent('input', { bubbles: true }))
      composer.shadowRoot.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await new Promise((resolve) => setTimeout(resolve, 0))
      return received[0]
    })
    expect(submitted).toEqual({
      input: 'Why did this decline?',
      references: [{
        reference: { workspaceId: 'sales', type: 'visual', id: 'executive-sales.orders_chart' },
        name: 'Orders by status',
        visualType: 'bar',
        workspace: { id: 'sales', name: 'sales' },
        hierarchy: ['sales', 'Executive Sales Dashboard', 'Overview'],
        href: '/workspaces/sales/dashboards/executive-sales/pages/overview',
        locations: [{ dashboardId: 'executive-sales', dashboardName: 'Executive Sales Dashboard', pageId: 'overview', pageName: 'Overview', href: '/workspaces/sales/dashboards/executive-sales/pages/overview' }],
        context: ['current_page', 'current_dashboard', 'current_workspace'],
      }],
    })

	const accepted = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
	  const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
	  mergePatch({ agent: {
		activeConversationId: 'agentconv_1',
		transcript: [{
		  id: 'user_1', kind: 'user', runId: 'run_1', text: 'Why did this decline?',
		  references: [{
			reference: { workspaceId: 'sales', type: 'visual', id: 'executive-sales.orders_chart' },
			name: 'Orders by status', workspace: { id: 'sales', name: 'Sales' },
			hierarchy: ['Sales', 'Executive Sales Dashboard', 'Overview'],
			href: '/workspaces/sales/dashboards/executive-sales/pages/overview', locations: [], context: ['current_page'],
		  }],
		}],
		status: { enabled: true, running: true },
		composer: { value: '', disabled: true, placeholder: 'Agent is working…' },
	  } })
	  await element.updateComplete
	  const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
	  await drawer.updateComplete
	  const composer = drawer.shadowRoot.querySelector('lv-chat-composer') as any
	  const thread = drawer.shadowRoot.querySelector('lv-chat-thread') as any
	  await Promise.all([composer.updateComplete, thread.updateComplete])
	  return {
		composerReferences: composer.references.length,
		draft: composer.shadowRoot.querySelector('textarea').value,
		bubble: thread.shadowRoot.querySelector('.message.user .bubble')?.textContent?.replace(/\s+/g, ' ').trim(),
		highlighted: Boolean(element.shadowRoot.querySelector('lv-dashboard-visual-frame[data-agent-referenced]')),
	  }
	})
	expect(accepted).toEqual({
	  composerReferences: 0,
	  draft: '',
	  bubble: 'Orders by status Why did this decline?',
	  highlighted: false,
	})
  } finally {
    await page.close()
  }
})

test('collapsed filters and page navigation use the same rail width', async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  try {
    await page.addInitScript(() => {
      localStorage.setItem('leapview-report-sidebar-collapsed', 'true')
      localStorage.setItem('leapview:filters-open', 'closed')
    })
    await page.goto(baseURL)
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && customElements.get('lv-sub-sidebar')
        && customElements.get('lv-filter-dock')
        && (document.querySelector('lv-dashboard-page') as any)?.page
    ))

    const widths = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      await element.updateComplete
      const root = element.shadowRoot
      const pageSidebar = root.querySelector('lv-sub-sidebar') as any
      const filterDock = root.querySelector('lv-filter-dock') as any
      await Promise.all([pageSidebar.updateComplete, filterDock.updateComplete])
      return {
        pageSidebar: Math.round(pageSidebar.getBoundingClientRect().width),
        filters: Math.round(filterDock.shadowRoot.querySelector('aside').getBoundingClientRect().width),
      }
    })

    expect(widths.pageSidebar).toBeGreaterThan(0)
    expect(widths.filters).toBe(widths.pageSidebar)
  } finally {
    await page.close()
  }
})

test('dashboard agent drawer folds out with the dashboard motion contract', async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  try {
    await page.addInitScript(() => localStorage.removeItem('leapview-dashboard-agent-state'))
    await page.goto(baseURL)
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && customElements.get('lv-chat-drawer')
        && (document.querySelector('lv-dashboard-page') as any)?.page
    ))

    const motion = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      await element.updateComplete
      const root = element.shadowRoot
      const route = root.querySelector('.route') as HTMLElement
      const drawer = root.querySelector('lv-chat-drawer') as HTMLElement
      const toggle = root.querySelector('.agent-toggle') as HTMLButtonElement
      const before = getComputedStyle(route)
      const closedWidth = drawer.getBoundingClientRect().width
      toggle.click()
      await element.updateComplete
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
      return {
        transitionProperty: before.transitionProperty,
        transitionDuration: before.transitionDuration,
        animatedProperties: route.getAnimations().map((animation) => (
          'transitionProperty' in animation ? (animation as CSSTransition).transitionProperty : ''
        )),
        closedWidth: Math.round(closedWidth),
        openingWidth: Math.round(drawer.getBoundingClientRect().width),
      }
    })

    expect(motion.transitionProperty).toContain('grid-template-columns')
    expect(motion.transitionDuration).toBe('0.16s')
    expect(motion.animatedProperties).toContain('grid-template-columns')
    expect(motion.closedWidth).toBe(0)
    expect(motion.openingWidth).toBeGreaterThan(0)
    expect(motion.openingWidth).toBeLessThan(420)

    await page.waitForFunction(() => {
      const dashboard = document.querySelector('lv-dashboard-page') as any
      const drawer = dashboard?.shadowRoot?.querySelector('lv-chat-drawer')
      return (drawer?.getBoundingClientRect().width ?? 0) >= 419.9
    })
    const openWidth = await page.locator('lv-dashboard-page').evaluate((element: any) => (
      Math.round(element.shadowRoot.querySelector('lv-chat-drawer')?.getBoundingClientRect().width ?? 0)
    ))
    expect(openWidth).toBe(420)

    await page.emulateMedia({ reducedMotion: 'reduce' })
    const reducedMotionDuration = await page.locator('lv-dashboard-page').evaluate((element: any) => (
      getComputedStyle(element.shadowRoot.querySelector('.route')).transitionDuration
    ))
    expect(reducedMotionDuration).toBe('0s')
  } finally {
    await page.close()
  }
})

test('dashboard agent restores its open state and active conversation after reload', async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  try {
    await page.addInitScript(() => {
      ;(window as any).__agentRestoreRequests = []
      window.addEventListener('lv-chat-restore', (event: Event) => {
        ;(window as any).__agentRestoreRequests.push((event as CustomEvent).detail)
      })
    })
    await page.goto(baseURL)
    await page.evaluate(() => {
      localStorage.setItem('leapview-dashboard-agent-state', JSON.stringify({
        open: true,
        conversationId: 'agentconv_saved',
      }))
    })
    await page.reload()
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && (window as any).__agentRestoreRequests?.length === 1
    ))

    const restoredShell = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      await element.updateComplete
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      return {
        open: drawer.open,
        request: (window as any).__agentRestoreRequests[0],
      }
    })
    expect(restoredShell).toEqual({
      open: true,
      request: { conversationId: 'agentconv_saved' },
    })

    await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev')
      mergePatch({ agent: {
        activeConversationId: 'agentconv_saved',
        transcript: [{ id: 'user_saved', kind: 'user', text: 'Persisted question' }],
      } })
      await element.updateComplete
      const drawer = element.shadowRoot.querySelector('lv-chat-drawer') as any
      await drawer.updateComplete
      drawer.shadowRoot.querySelector('[aria-label="Close agent"]').click()
      await element.updateComplete
    })

    const closedState = await page.locator('lv-dashboard-page').evaluate((element: any) => ({
      open: element.shadowRoot.querySelector('lv-chat-drawer')?.open,
      persisted: JSON.parse(localStorage.getItem('leapview-dashboard-agent-state') || '{}'),
    }))
    expect(closedState).toEqual({
      open: false,
      persisted: { open: false, conversationId: 'agentconv_saved' },
    })

    await page.reload()
    await page.waitForFunction(() => (
      customElements.get('lv-dashboard-page')
        && (window as any).__agentRestoreRequests?.length === 1
    ))
    const reloadedClosedState = await page.locator('lv-dashboard-page').evaluate(async (element: any) => {
      await element.updateComplete
      return {
        open: element.shadowRoot.querySelector('lv-chat-drawer')?.open,
        request: (window as any).__agentRestoreRequests[0],
      }
    })
    expect(reloadedClosedState).toEqual({
      open: false,
      request: { conversationId: 'agentconv_saved' },
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
      { id: 'title', kind: 'header', x: 16, y: 16, width: 456, height: 88, title: 'Executive Sales', eyebrow: 'LeapView report', badges: ['Sales'] },
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
    agent: {
      conversations: [],
      activeConversationId: '',
      transcript: [],
      status: { enabled: true, running: false },
      composer: { value: '', disabled: false, placeholder: 'Ask about this dashboard...' },
    },
    agentContext: {
      surface: 'dashboard',
      workspaceId: 'sales',
      dashboardId: 'executive-sales',
      dashboardTitle: 'Executive Sales Dashboard',
      pageId: 'overview',
      pageTitle: 'Overview',
      modelId: 'olist',
      generation: 3,
      filters,
      references: [],
    },
    agentVisuals: {},
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --lv-bg-app: #f6f8fa; --lv-bg-panel: #fff; --lv-bg-panel-muted: #f6f8fa; --lv-bg-control-hover: #f3f4f6; --lv-chart-surface: #fff; --lv-report-page-bg: #fff; --lv-report-canvas-bg: #eaeef2; --lv-report-rail-bg: #fff; --lv-bg-overlay: #fff; --lv-fg-default: #24292f; --lv-fg-muted: #57606a; --lv-fg-link: #0969da; --lv-line-muted: #d8dee4; --lv-border-default: 1px solid #d0d7de; --lv-border-muted: 1px solid #d8dee4; --lv-border-transparent: 1px solid transparent; --lv-radius-default: 6px; --lv-radius-full: 999px; --lv-page-rail-width-collapsed: 38px; --lv-dashboard-filter-open-width: 320px; --lv-dashboard-agent-width: 420px; --base-size-2: 2px; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --control-medium-size: 32px; --control-xlarge-size: 40px; --lv-font-size-caption: 12px; --lv-font-size-body-sm: 14px; --lv-font-size-title-sm: 16px; --lv-font-size-title-lg: 28px; --lv-font-size-display: 32px; --lv-font-weight-medium: 500; --lv-font-weight-strong: 600; --lv-line-height-none: 1; --lv-line-height-tight: 1.2; --lv-line-height-compact: 1.3; --zIndex-dropdown: 100; --zIndex-modal: 200; --zIndex-sticky: 50; --shadow-resting-small: 0 1px 2px rgb(0 0 0 / .08); --shadow-floating-small: 0 8px 24px rgb(0 0 0 / .12); --lv-duration-fast: 160ms; --lv-spinner-size-md: 16px; --lv-spinner-duration: 1800ms; --motion-easing-move: ease; --motion-transition-stateChange: 160ms ease; }
          body { --lv-loading-delay-short: 250ms; --lv-loading-delay-long: 500ms; }
          lv-dashboard-page { min-height: 720px; }
        </style>
      </head>
      <body>
        <main data-signals="${attr(signals)}">
          <lv-dashboard-page></lv-dashboard-page>
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
