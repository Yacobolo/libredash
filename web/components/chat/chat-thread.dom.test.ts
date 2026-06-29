import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let browser: Browser
let baseURL = ''

test.before(async () => {
  const root = join(process.cwd(), '.tmp/chat-thread-test')
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname !== '/') {
      const file = normalize(join(root, url.pathname))
      if (!file.startsWith(root)) {
        response.writeHead(404)
        response.end('not found')
        return
      }
      try {
        response.setHeader('content-type', 'text/javascript')
        response.end(await readFile(file))
        return
      } catch {
        response.writeHead(404)
        response.end('not found')
        return
      }
    }
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(`
      <!doctype html>
      <html>
        <head>
          <style>
            :root {
              --ld-chart-surface: rgb(1, 2, 3);
              --ld-border-default: 2px solid rgb(4, 5, 6);
            }
          </style>
          <script type="module" src="/chat-under-test.js"></script>
        </head>
        <body><ld-chat-thread></ld-chat-thread><ld-visual-modal></ld-visual-modal></body>
      </html>
    `)
    }
  })
  await new Promise<void>((resolve) => server.listen(0, resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('test server did not bind to a port')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

test.after(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
})

test('chat thread renders visual artifacts with dashboard web components', async () => {
  const page = await browser.newPage()
  await page.goto(baseURL)
  await page.evaluate(async () => {
    await customElements.whenDefined('ld-chat-thread')
    await customElements.whenDefined('ld-visual-modal')
    const thread = document.querySelector('ld-chat-thread') as any
    thread.visuals = {
      agent_chart_1: {
        version: 3,
        id: 'agent_chart_1',
        kind: 'chart',
        shape: 'category_value',
        renderer: 'echarts',
        type: 'bar',
        title: 'Orders',
        unit: '',
        interaction: {},
        dimensions: ['status'],
        measure: 'order_count',
        measures: ['order_count'],
        series: [],
        options: {},
        rendererOptions: {},
        selection: [],
        data: [{ label: 'delivered', value: 42 }],
      },
    }
    thread.tables = {
      agent_table_1: {
        version: 2,
        kind: 'data_table',
        title: 'Orders',
        style: { density: 'comfortable', zebra: true, grid: 'rows' },
        interaction: {},
        selection: [],
        columns: [{ key: 'order_id', label: 'Order', format: 'text' }],
        totalRows: 1,
        availableRows: 1,
        isCapped: false,
        rowCap: 50,
        chunkSize: 50,
        rowHeight: 34,
        resetVersion: 0,
        sort: { key: '', direction: '' },
        blocks: {
          a: { start: 0, requestSeq: 0, resetVersion: 0, sort: { key: '', direction: '' }, rows: [{ order_id: 'o1' }] },
        },
        loadingBlock: '',
        error: '',
      },
    }
    thread.transcript = [
      {
        id: 'tool-chart',
        kind: 'tool',
        name: 'query_visual',
        status: 'complete',
        resultJson: '{\n  "ok": true,\n  "kind": "chart",\n  "id": "agent_chart_1",\n  "signal": "visuals.agent_chart_1"\n}',
        artifact: {
          kind: 'chart',
          id: 'agent_chart_1',
          summary: 'Created chart.',
        },
      },
      {
        id: 'tool-table',
        kind: 'tool',
        name: 'query_visual',
        status: 'complete',
        resultJson: '{\n  "ok": true,\n  "kind": "table",\n  "id": "agent_table_1",\n  "signal": "tables.agent_table_1"\n}',
        artifact: {
          kind: 'table',
          id: 'agent_table_1',
          summary: 'Created table.',
        },
      },
    ]
    await thread.updateComplete
  })
  await page.waitForFunction(() => Boolean(
    document.querySelector('ld-chat-thread')!
      .shadowRoot!
      .querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]')
      ?.shadowRoot
      ?.querySelector('ld-echart[visual-id="agent_chart_1"]'),
  ))
  await page.waitForFunction(() => Boolean(
    document.querySelector('ld-chat-thread')!
      .shadowRoot!
      .querySelector('ld-visual-artifact[artifact-id="agent_table_1"]')
      ?.shadowRoot
      ?.querySelector('ld-data-table[table-id="agent_table_1"]'),
  ))

  const rendered = await page.evaluate(() => {
    const root = document.querySelector('ld-chat-thread')!.shadowRoot!
    return {
      chart: Boolean(root.querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]')?.shadowRoot?.querySelector('ld-echart[visual-id="agent_chart_1"]')),
      table: Boolean(root.querySelector('ld-visual-artifact[artifact-id="agent_table_1"]')?.shadowRoot?.querySelector('ld-data-table[table-id="agent_table_1"]')),
      jsonDetails: root.querySelectorAll('.tool-details').length,
      bodyText: root.textContent || '',
      artifactBackground: getComputedStyle(root.querySelector('ld-visual-artifact')!.shadowRoot!.querySelector('.artifact')!).backgroundColor,
      artifactBorderTopWidth: getComputedStyle(root.querySelector('ld-visual-artifact')!.shadowRoot!.querySelector('.artifact')!).borderTopWidth,
    }
  })
  assert.equal(rendered.chart, true)
  assert.equal(rendered.table, true)
  assert.equal(rendered.jsonDetails, 0)
  assert.equal(rendered.bodyText.includes('delivered'), false)
  assert.equal(rendered.artifactBackground, 'rgb(1, 2, 3)')
  assert.equal(rendered.artifactBorderTopWidth, '2px')

  await page.locator('ld-chat-thread').evaluate(async (element: any) => {
    const trigger = element.shadowRoot.querySelector('.tool-trigger') as HTMLButtonElement
    trigger.click()
    await element.updateComplete
  })
  const detailText = await page.locator('ld-chat-thread').evaluate((element: any) => element.shadowRoot.querySelector('.tool-details')?.textContent || '')
  assert.equal(detailText.includes('"signal": "visuals.agent_chart_1"'), true)
  assert.equal(detailText.includes('delivered'), false)

  await page.locator('ld-chat-thread').evaluate(async (element: any) => {
    const artifact = element.shadowRoot.querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]') as any
    const chart = artifact.shadowRoot.querySelector('ld-echart') as any
    await chart.updateComplete
    const summary = chart.shadowRoot.querySelector('.options summary') as HTMLElement
    summary.click()
    await chart.updateComplete
    const showData = chart.shadowRoot.querySelector('.menu button') as HTMLButtonElement
    showData.click()
  })
  const showDataState = await page.locator('ld-visual-modal').evaluate(async (modal: any) => {
    await modal.updateComplete
    return {
      hasDialog: Boolean(modal.shadowRoot.querySelector('.dialog')),
      text: modal.shadowRoot.textContent || '',
    }
  })
  assert.equal(showDataState.hasDialog, true)
  assert.equal(showDataState.text.includes('1 row from current visual data'), true)
  await page.locator('ld-visual-modal').evaluate(async (modal: any) => {
    modal.shadowRoot.querySelector('.close').click()
    await modal.updateComplete
  })

  await page.locator('ld-chat-thread').evaluate(async (element: any) => {
    const artifact = element.shadowRoot.querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]') as any
    const chart = artifact.shadowRoot.querySelector('ld-echart') as any
    await chart.updateComplete
    const expand = chart.shadowRoot.querySelector('.icon-action') as HTMLButtonElement
    expand.click()
  })
  const focusState = await page.locator('ld-visual-modal').evaluate(async (modal: any) => {
    await modal.updateComplete
    const chart = document.querySelector('ld-echart[visual-id="agent_chart_1"]') as HTMLElement
    const dialog = modal.shadowRoot.querySelector('.focus-dialog')
    return {
      chartParent: chart.parentElement?.localName,
      slot: chart.getAttribute('slot'),
      hasDialog: Boolean(dialog),
      dialogBackground: dialog ? getComputedStyle(dialog).backgroundColor : '',
    }
  })
  assert.deepEqual(focusState, {
    chartParent: 'ld-visual-modal',
    slot: 'focus-visual',
    hasDialog: true,
    dialogBackground: 'rgb(1, 2, 3)',
  })
  await page.locator('ld-visual-modal').evaluate(async (modal: any) => {
    modal.shadowRoot.querySelector('.focus-close').click()
    await modal.updateComplete
  })
  const restored = await page.locator('ld-chat-thread').evaluate((element: any) => Boolean(element.shadowRoot.querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]')?.shadowRoot?.querySelector('ld-echart[visual-id="agent_chart_1"]')))
  assert.equal(restored, true)
  await page.close()
})

test('chat thread still renders legacy embedded artifact patches', async () => {
  const page = await browser.newPage()
  await page.goto(baseURL)
  await page.evaluate(async () => {
    await customElements.whenDefined('ld-chat-thread')
    const thread = document.querySelector('ld-chat-thread') as any
    thread.transcript = [{
      id: 'tool-chart',
      kind: 'tool',
      name: 'query_visual',
      status: 'complete',
      artifact: {
        kind: 'chart',
        id: 'legacy_chart_1',
        patch: {
          visuals: {
            legacy_chart_1: {
              version: 3,
              id: 'legacy_chart_1',
              kind: 'chart',
              shape: 'category_value',
              renderer: 'echarts',
              type: 'bar',
              title: 'Legacy Orders',
              unit: '',
              interaction: {},
              dimensions: ['status'],
              measure: 'order_count',
              measures: ['order_count'],
              series: [],
              options: {},
              rendererOptions: {},
              selection: [],
              data: [{ label: 'delivered', value: 42 }],
            },
          },
        },
      },
    }]
    await thread.updateComplete
  })
  await page.waitForFunction(() => Boolean(
    document.querySelector('ld-chat-thread')!
      .shadowRoot!
      .querySelector('ld-visual-artifact[artifact-id="legacy_chart_1"]')
      ?.shadowRoot
      ?.querySelector('ld-echart[visual-id="legacy_chart_1"]'),
  ))

  const hasChart = await page.evaluate(() => Boolean(document.querySelector('ld-chat-thread')!.shadowRoot!.querySelector('ld-visual-artifact[artifact-id="legacy_chart_1"]')?.shadowRoot?.querySelector('ld-echart[visual-id="legacy_chart_1"]')))
  assert.equal(hasChart, true)
  await page.close()
})
