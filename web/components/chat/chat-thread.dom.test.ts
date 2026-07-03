import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let browser: Browser
let baseURL = ''

beforeAll(async () => {
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
              --fontStack-system: system-ui;
              --fontStack-monospace: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
              --ld-bg-page: #fff;
              --ld-bg-panel: #fff;
              --ld-bg-panel-muted: #f6f8fa;
              --ld-bg-control: #f6f8fa;
              --ld-fg-default: #24292f;
              --ld-fg-muted: #57606a;
              --ld-fg-accent: #0969da;
              --ld-line-muted: #d8dee4;
              --ld-border-width: 1px;
              --ld-border-muted: 1px solid #d8dee4;
              --ld-radius-default: 6px;
              --base-size-4: 4px;
              --base-size-8: 8px;
              --base-size-12: 12px;
              --base-size-16: 16px;
              --base-size-20: 20px;
              --ld-space-sm: 8px;
              --ld-font-size-caption: 12px;
              --ld-font-size-body-sm: 14px;
              --ld-font-size-body-md: 16px;
              --ld-font-size-title-sm: 18px;
              --ld-font-size-title-md: 22px;
              --ld-font-weight-strong: 600;
              --ld-line-height-compact: 1.3;
              --ld-line-height-normal: 1.5;
              --ld-line-height-relaxed: 1.55;
              --ld-chat-thread-padding: 16px;
              --ld-chat-stack-width: 760px;
              --ld-chat-stack-gap: 16px;
              --ld-chat-message-width: 760px;
              --ld-chat-message-gap: 8px;
              --ld-chat-agent-item-gap: 8px;
              --ld-chat-empty-min-height: 180px;
              --ld-chat-bubble-padding-block: 12px;
              --ld-chat-bubble-padding-inline: 16px;
              --ld-chat-markdown-block-gap: 10px;
              --ld-chat-markdown-list-indent: 20px;
              --ld-chat-markdown-list-item-gap: 2px;
              --ld-chat-code-radius: 4px;
              --ld-chat-code-padding-block: 1px;
              --ld-chat-code-padding-inline: 4px;
              --ld-chat-code-font-scale: 0.92em;
              --ld-chat-pre-padding-block: 9px;
              --ld-chat-pre-padding-inline: 10px;
              --ld-chat-quote-border-width: 2px;
              --ld-chat-link-underline-thickness: 1px;
              --ld-chat-link-underline-offset: 2px;
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

afterAll(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
})

test('chat thread preserves plain user message text without template whitespace', async () => {
  const page = await browser.newPage()
  await page.goto(baseURL)
  await page.evaluate(async () => {
    await customElements.whenDefined('ld-chat-thread')
    const thread = document.querySelector('ld-chat-thread') as any
    thread.transcript = [{
      id: 'user-1',
      kind: 'user',
      text: '# nice!',
    }]
    await thread.updateComplete
  })

  const state = await page.locator('ld-chat-thread').evaluate((element: any) => {
    const bubble = element.shadowRoot.querySelector('.message.user .bubble.plain') as HTMLElement
    const rect = bubble.getBoundingClientRect()
    return {
      text: bubble.textContent,
      width: Math.round(rect.width),
      whiteSpace: getComputedStyle(bubble).whiteSpace,
    }
  })

  expect(state.text).toBe('# nice!')
  expect(state.whiteSpace).toBe('pre-wrap')
  expect(state.width).toBeLessThan(140)
  await page.close()
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
      ?.querySelector('ld-report-table[table-id="agent_table_1"]'),
  ))

  const rendered = await page.evaluate(() => {
    const root = document.querySelector('ld-chat-thread')!.shadowRoot!
    return {
      chart: Boolean(root.querySelector('ld-visual-artifact[artifact-id="agent_chart_1"]')?.shadowRoot?.querySelector('ld-echart[visual-id="agent_chart_1"]')),
      table: Boolean(root.querySelector('ld-visual-artifact[artifact-id="agent_table_1"]')?.shadowRoot?.querySelector('ld-report-table[table-id="agent_table_1"]')),
      jsonDetails: root.querySelectorAll('.tool-details').length,
      bodyText: root.textContent || '',
      artifactBackground: getComputedStyle(root.querySelector('ld-visual-artifact')!.shadowRoot!.querySelector('.artifact')!).backgroundColor,
      artifactBorderTopWidth: getComputedStyle(root.querySelector('ld-visual-artifact')!.shadowRoot!.querySelector('.artifact')!).borderTopWidth,
    }
  })
  expect(rendered.chart).toBe(true)
  expect(rendered.table).toBe(true)
  expect(rendered.jsonDetails).toBe(0)
  expect(rendered.bodyText.includes('delivered')).toBe(false)
  expect(rendered.artifactBackground).toBe('rgb(1, 2, 3)')
  expect(rendered.artifactBorderTopWidth).toBe('2px')

  await page.locator('ld-chat-thread').evaluate(async (element: any) => {
    const trigger = element.shadowRoot.querySelector('.tool-trigger') as HTMLButtonElement
    trigger.click()
    await element.updateComplete
  })
  const detailText = await page.locator('ld-chat-thread').evaluate((element: any) => element.shadowRoot.querySelector('.tool-details')?.textContent || '')
  expect(detailText.includes('"signal": "visuals.agent_chart_1"')).toBe(true)
  expect(detailText.includes('delivered')).toBe(false)

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
  expect(showDataState.hasDialog).toBe(true)
  expect(showDataState.text.includes('1 row from current visual data')).toBe(true)
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
  expect(focusState).toEqual({
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
  expect(restored).toBe(true)
  await page.close()
})

test('chat thread renders tool details with compact json and toon code blocks', async () => {
  const page = await browser.newPage()
  await page.goto(baseURL)
  await page.evaluate(async () => {
    await customElements.whenDefined('ld-chat-thread')
    const thread = document.querySelector('ld-chat-thread') as any
    thread.transcript = [
      {
        id: 'tool-toon',
        kind: 'tool',
        name: 'list_workspaces',
        status: 'complete',
        inputJson: '{\n  "name": "list_workspaces",\n  "arguments": "{}"\n}',
        inputFormat: 'json',
        resultJson: 'items[2]{id,title}:\n  sales,Sales\n  ops,Operations\ncount: 2\nhasMore: false',
        resultFormat: 'toon',
      },
      {
        id: 'tool-json',
        kind: 'tool',
        name: 'query_visual',
        status: 'complete',
        resultJson: '{\n  "ok": true,\n  "kind": "chart"\n}',
      },
    ]
    await thread.updateComplete
    for (const trigger of Array.from(thread.shadowRoot.querySelectorAll('.tool-trigger')) as HTMLButtonElement[]) {
      trigger.click()
    }
    await thread.updateComplete
  })

  const state = await page.locator('ld-chat-thread').evaluate((element: any) => {
    const blocks = Array.from(element.shadowRoot.querySelectorAll('ld-code-block')) as any[]
    return {
      blockCount: blocks.length,
      languages: blocks.map((block) => block.language),
      compact: blocks.map((block) => block.compact),
      text: element.shadowRoot.querySelector('.tool-details')?.textContent || '',
      hasRawPre: Boolean(element.shadowRoot.querySelector('.tool-detail-block > pre')),
    }
  })

  expect(state.blockCount).toBe(3)
  expect(state.languages).toEqual(['json', 'toon', 'json'])
  expect(state.compact).toEqual([true, true, true])
  expect(state.text).toContain('items[2]{id,title}:')
  expect(state.hasRawPre).toBe(false)
  await page.close()
})

test('chat thread renders assistant markdown through shared markdown view', async () => {
  const page = await browser.newPage()
  await page.goto(baseURL)
  await page.evaluate(async () => {
    await customElements.whenDefined('ld-chat-thread')
    await customElements.whenDefined('ld-markdown-view')
    const thread = document.querySelector('ld-chat-thread') as any
    thread.transcript = [{
      id: 'assistant-1',
      kind: 'assistant',
      markdown: [
        '# Assistant heading',
        '',
        'A paragraph with **strong** text and `code`.',
        '',
        '- One',
        '- Two',
      ].join('\n'),
    }]
    await thread.updateComplete
  })

  const state = await page.locator('ld-chat-thread').evaluate(async (element: any) => {
    const markdownView = element.shadowRoot.querySelector('ld-markdown-view') as any
    await markdownView.updateComplete
    return {
      hasMarkdownView: Boolean(markdownView),
      value: markdownView.value,
      h1Text: markdownView.shadowRoot.querySelector('h1')?.textContent,
      hasStrong: Boolean(markdownView.shadowRoot.querySelector('strong')),
      hasCode: Boolean(markdownView.shadowRoot.querySelector('code')),
      hasList: Boolean(markdownView.shadowRoot.querySelector('ul')),
    }
  })

  expect(state.hasMarkdownView).toBe(true)
  expect(state.value).toMatch(/^# Assistant heading/)
  expect(state.h1Text).toBe('Assistant heading')
  expect(state.hasStrong).toBe(true)
  expect(state.hasCode).toBe(true)
  expect(state.hasList).toBe(true)
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
  expect(hasChart).toBe(true)
  await page.close()
})
