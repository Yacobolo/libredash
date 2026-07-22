import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser
const signalQueries: string[] = []
const root = join(process.cwd(), '.tmp/datastar-inspector-test')

beforeAll(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument())
      return
    }
    if (url.pathname === '/__dev/pagestream/signals') {
      signalQueries.push(url.search)
      const selected = url.searchParams.get('path') === '/status/progressPercent'
      const incremental = url.searchParams.has('after')
      if (!selected) await new Promise((resolve) => setTimeout(resolve, 100))
      response.setHeader('content-type', 'application/json')
      response.end(JSON.stringify({
        streamId: 'dashboard:ratings:tab-1',
        streams: [{
          streamId: 'dashboard:ratings:tab-1',
          lastEventId: 4,
          lastTimestamp: '2026-07-14T12:00:00.300Z',
        }, {
          streamId: 'dashboard:ratings:tab-2',
          lastEventId: 3,
          lastTimestamp: '2026-07-14T11:59:59.300Z',
        }],
        state: { status: { loading: false, progressPercent: 50 } },
        leaves: [
          { path: '/status/loading', displayPath: 'status.loading', value: false },
          { path: '/status/progressPercent', displayPath: 'status.progressPercent', value: 50 },
        ],
        history: selected && !incremental ? [
          {
            id: 1,
            traceEventId: 1,
            timestamp: '2026-07-14T12:00:00Z',
            streamId: 'dashboard:ratings:tab-1',
            path: '/status/progressPercent',
            displayPath: 'status.progressPercent',
            operation: 'set',
            value: 0,
            generation: 4,
            sequence: 1,
            origin: 'dashboard.refresh',
            correlationId: 'refresh-4',
          },
          {
            id: 2,
            traceEventId: 2,
            timestamp: '2026-07-14T12:00:00.120Z',
            streamId: 'dashboard:ratings:tab-1',
            path: '/status/progressPercent',
            displayPath: 'status.progressPercent',
            operation: 'set',
            value: 25,
            generation: 4,
            sequence: 2,
            origin: 'dashboard.refresh',
            correlationId: 'refresh-4',
          },
          {
            id: 3,
            traceEventId: 2,
            timestamp: '2026-07-14T12:00:00.300Z',
            streamId: 'dashboard:ratings:tab-1',
            path: '/status/progressPercent',
            displayPath: 'status.progressPercent',
            operation: 'set',
            value: 50,
            generation: 4,
            sequence: 3,
            origin: 'dashboard.refresh',
            correlationId: 'refresh-4',
          },
        ] : [],
        nextAfter: selected ? 3 : 0,
      }))
      return
    }
    const file = normalize(join(root, url.pathname))
    if (!file.startsWith(root)) {
      response.writeHead(404)
      response.end('not found')
      return
    }
    try {
      response.setHeader('content-type', 'text/javascript')
      response.end(await readFile(file))
    } catch {
      response.writeHead(404)
      response.end('not found')
    }
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

test('inspector selects a delivered signal and shows its effective history', async () => {
  const page = await browser.newPage({ viewport: { width: 900, height: 650 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('datastar-inspector'))
    const state = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      const toggleStyle = getComputedStyle(element.shadowRoot.querySelector('.toggle'))
      const launcher = {
        bottom: toggleStyle.bottom,
        width: toggleStyle.width,
        height: toggleStyle.height,
        opacity: toggleStyle.opacity,
      }
      element.shadowRoot.querySelector<HTMLButtonElement>('.toggle')!.click()
      await element.updateComplete
      const deadline = Date.now() + 3_000
      while (!element.shadowRoot.querySelector('[data-signal-path="/status/progressPercent"]') && Date.now() < deadline) {
        const branch = element.shadowRoot.querySelector<HTMLButtonElement>('[data-signal-branch="/status"]')
        if (branch && branch.getAttribute('aria-expanded') !== 'true') branch.click()
        await new Promise((resolve) => setTimeout(resolve, 25))
        await element.updateComplete
      }
      element.shadowRoot.querySelector<HTMLButtonElement>('[data-signal-path="/status/progressPercent"]')!.click()
      await element.updateComplete
      const historyDeadline = Date.now() + 3_000
      while (!element.shadowRoot.textContent.includes('refresh-4') && Date.now() < historyDeadline) {
        await new Promise((resolve) => setTimeout(resolve, 25))
        await element.updateComplete
      }
      return {
        text: element.shadowRoot.textContent,
        selected: element.shadowRoot.querySelector('[data-signal-path="/status/progressPercent"]')?.getAttribute('aria-selected'),
        sparkline: Boolean(element.shadowRoot.querySelector('[data-signal-sparkline]')),
        changes: element.shadowRoot.querySelectorAll('[data-signal-change]').length,
        historyValues: [...element.shadowRoot.querySelectorAll('[data-signal-change] .signal-change-value')].map((node: Element) => node.textContent?.trim()),
        transportTabs: element.shadowRoot.querySelectorAll('[data-view="transport"]').length,
        streamSelectors: element.shadowRoot.querySelectorAll('.signal-stream-select').length,
        hasDeliveredStream: element.shadowRoot.textContent.includes('Delivered stream'),
        storedSelection: JSON.parse(sessionStorage.getItem('ds-inspector') ?? '{}').selectedSignalPath,
        launcher,
      }
    })

    expect(state.text).toMatch(/status\.progressPercent/)
    expect(state.text).toMatch(/Current value/)
    expect(state.text).toMatch(/refresh-4/)
    expect(state.text).toMatch(/25/)
    expect(state.selected).toBe('true')
    expect(state.sparkline).toBe(true)
    expect(state.changes).toBe(3)
    expect(state.historyValues).toEqual(['50', '25', '0'])
    expect(state.transportTabs).toBe(0)
    expect(state.streamSelectors).toBe(0)
    expect(state.hasDeliveredStream).toBe(false)
    expect(state.storedSelection).toBeUndefined()
    expect(state.launcher).toEqual({ bottom: '16px', width: '38px', height: '38px', opacity: '1' })
    expect(signalQueries.some((query) => query.includes('path=%2Fstatus%2FprogressPercent'))).toBe(true)

    await page.reload()
    await page.waitForFunction(() => customElements.get('datastar-inspector'))
    const refreshed = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      await element.updateComplete
      return {
        selected: element.shadowRoot.querySelector('[aria-selected="true"]')?.getAttribute('data-signal-path'),
        hasCurrentValue: element.shadowRoot.textContent.includes('Current value'),
      }
    })
    expect(refreshed.selected).toBeUndefined()
    expect(refreshed.hasCurrentValue).toBe(false)
  } finally {
    await page.close()
  }
})

test('inspector launcher and panel can be dragged and keep their positions', async () => {
  const page = await browser.newPage({ viewport: { width: 900, height: 650 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('datastar-inspector'))
    const initialToggle = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      await element.updateComplete
      const toggle = element.shadowRoot.querySelector('.toggle') as HTMLElement
      const rect = toggle.getBoundingClientRect()
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height }
    })

    await page.mouse.move(initialToggle.x + initialToggle.width / 2, initialToggle.y + initialToggle.height / 2)
    await page.mouse.down()
    await page.mouse.move(initialToggle.x - 180, initialToggle.y - 120, { steps: 5 })
    await page.mouse.up()

    const movedToggle = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      await element.updateComplete
      const toggle = element.shadowRoot.querySelector('.toggle') as HTMLElement
      const rect = toggle.getBoundingClientRect()
      return {
        expanded: Boolean(element.shadowRoot.querySelector('.panel')),
        x: rect.x,
        y: rect.y,
        stylePosition: { x: Number.parseFloat(toggle.style.left), y: Number.parseFloat(toggle.style.top) },
        stored: JSON.parse(sessionStorage.getItem('ds-inspector') ?? '{}').togglePosition,
      }
    })
    expect(movedToggle.expanded).toBe(false)
    expect(movedToggle.x).toBeLessThan(initialToggle.x - 100)
    expect(movedToggle.y).toBeLessThan(initialToggle.y - 70)
    expect(movedToggle.stored).toEqual(movedToggle.stylePosition)

    await page.locator('datastar-inspector').evaluate(async (element: any) => {
      element.shadowRoot.querySelector<HTMLButtonElement>('.toggle')!.click()
      await element.updateComplete
    })
    const initialPanel = await page.locator('datastar-inspector').evaluate((element: any) => {
      const panel = element.shadowRoot.querySelector('.panel') as HTMLElement
      const handle = element.shadowRoot.querySelector('.drag-handle') as HTMLElement
      const panelRect = panel.getBoundingClientRect()
      const handleRect = handle.getBoundingClientRect()
      return {
        x: panelRect.x,
        y: panelRect.y,
        handleX: handleRect.x + handleRect.width / 2,
        handleY: handleRect.y + handleRect.height / 2,
      }
    })

    await page.mouse.move(initialPanel.handleX, initialPanel.handleY)
    await page.mouse.down()
    await page.mouse.move(initialPanel.handleX - 80, initialPanel.handleY + 45, { steps: 5 })
    await page.mouse.up()

    const movedPanel = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      await element.updateComplete
      const rect = element.shadowRoot.querySelector('.panel').getBoundingClientRect()
      return {
        x: rect.x,
        y: rect.y,
        stored: JSON.parse(sessionStorage.getItem('ds-inspector') ?? '{}').panelPosition,
      }
    })
    expect(movedPanel.x).toBeLessThan(initialPanel.x - 50)
    expect(movedPanel.y).toBeGreaterThan(initialPanel.y + 5)
    expect(movedPanel.stored).toEqual({ x: movedPanel.x, y: movedPanel.y })

    await page.reload()
    await page.waitForFunction(() => customElements.get('datastar-inspector'))
    const restoredPanel = await page.locator('datastar-inspector').evaluate(async (element: any) => {
      await element.updateComplete
      const rect = element.shadowRoot.querySelector('.panel').getBoundingClientRect()
      return { x: rect.x, y: rect.y }
    })
    expect(restoredPanel.x).toBe(movedPanel.x)
    expect(restoredPanel.y).toBe(movedPanel.y)
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  return `
    <!doctype html>
    <html>
      <body>
        <datastar-inspector signals-url="/__dev/pagestream/signals">
          <pre data-json-signals>{"status":{"loading":false,"progressPercent":50}}</pre>
        </datastar-inspector>
        <script type="module" src="/datastar-inspector-under-test.js"></script>
      </body>
    </html>
  `
}
