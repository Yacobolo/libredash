import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/data-explorer-test')

beforeAll(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument())
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
  if (!address || typeof address === 'string') throw new Error('test server did not bind to a port')
  baseURL = `http://127.0.0.1:${address.port}`
  browser = await chromium.launch()
})

afterAll(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
})

test('data explorer renders object browser and emits preview commands', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-data-explorer') && customElements.get('ld-data-preview-table') && customElements.get('ld-windowed-table'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-data-explorer') as any
      element.page = {
        kind: 'data',
        title: 'Data Explorer',
        description: 'Inspect rows.',
        workspaceId: 'sales',
        selectedWorkspaceId: 'sales',
        selectedObject: 'model_table:model_table:olist.orders',
        workspaces: [{ id: 'sales', title: 'Sales', href: '/data?workspace=sales', objectCount: 3, active: true }],
        tabs: [],
      }
      element.dataExplorer = {
        objects: [
          {
            key: 'source:source:olist.orders',
            workspaceId: 'sales',
            workspaceTitle: 'Sales',
            assetId: 'source:olist.orders',
            layer: 'source',
            modelId: 'olist',
            source: 'orders',
            title: 'orders source',
            columnCount: 2,
            rowCountLabel: '10',
            columns: [{ key: 'order_id', label: 'order_id', type: 'VARCHAR' }],
          },
          {
            key: 'model_table:model_table:olist.orders',
            workspaceId: 'sales',
            workspaceTitle: 'Sales',
            assetId: 'model_table:olist.orders',
            layer: 'model_table',
            modelId: 'olist',
            table: 'orders',
            title: 'orders',
            columnCount: 2,
            rowCountLabel: '10',
            columns: [
              { key: 'order_id', label: 'order_id', type: 'VARCHAR' },
              { key: 'status', label: 'status', type: 'VARCHAR' },
            ],
          },
          {
            key: 'semantic_view:olist.orders',
            workspaceId: 'sales',
            workspaceTitle: 'Sales',
            assetId: 'model_table:olist.orders',
            layer: 'semantic_view',
            modelId: 'olist',
            table: 'orders',
            title: 'orders semantic view',
            columnCount: 1,
            rowCountLabel: 'Unknown',
            columns: [{ key: 'status', label: 'Status', type: 'string' }],
          },
        ],
        selectedKey: 'model_table:model_table:olist.orders',
        selectedWorkspaceId: 'sales',
        selectedObject: {
          key: 'model_table:model_table:olist.orders',
          workspaceId: 'sales',
          workspaceTitle: 'Sales',
          assetId: 'model_table:olist.orders',
          layer: 'model_table',
          modelId: 'olist',
          table: 'orders',
          title: 'orders',
          columnCount: 2,
          rowCountLabel: '10',
          columns: [
            { key: 'order_id', label: 'order_id', type: 'VARCHAR' },
            { key: 'status', label: 'status', type: 'VARCHAR' },
          ],
        },
        preview: {
          columns: [
            { key: 'order_id', label: 'order_id', type: 'VARCHAR' },
            { key: 'status', label: 'status', type: 'VARCHAR' },
          ],
          totalRows: 500,
          availableRows: 500,
          chunkSize: 100,
          rowHeight: 34,
          resetVersion: 0,
          blocks: {
            a: { start: 0, requestSeq: 0, resetVersion: 0, sort: {}, rows: [
              { order_id: 'o1', status: 'delivered' },
              { order_id: 'o2', status: 'a very long status value that should truncate inside the cell without changing layout' },
            ] },
            b: { start: 100, requestSeq: 0, resetVersion: 0, sort: {}, rows: [{ order_id: 'o100', status: 'processing' }] },
            c: { start: 200, requestSeq: 0, resetVersion: 0, sort: {}, rows: [] },
          },
          totalRowLabel: '500',
          sort: {},
          sql: 'SELECT * FROM model.orders',
          error: '',
        },
        command: { workspaceId: 'sales', objectKey: 'model_table:model_table:olist.orders', offset: 0, limit: 100, block: 'all', start: 0, count: 100, requestSeq: 0, resetVersion: 0, sort: {}, visibleColumns: [], columnWidths: {} },
        warnings: [],
      }
      const commands: any[] = []
      element.addEventListener('ld-data-explorer-command', (event: CustomEvent) => commands.push(event.detail))
      document.body.append(element)
      await element.updateComplete
      const root = element.shadowRoot
      const previewTable = root.querySelector('ld-data-preview-table') as any
      await previewTable.updateComplete
      const grid = previewTable.renderRoot.querySelector('ld-windowed-table') as any
      await grid.updateComplete
      const firstSemantic = Array.from(root.querySelectorAll<HTMLButtonElement>('.object-button')).find((button) => button.textContent?.includes('semantic view'))!
      firstSemantic.click()
      await element.updateComplete
      await previewTable.updateComplete
      await grid.updateComplete
      const firstHeader = grid.shadowRoot.querySelector('.header-cell button') as HTMLButtonElement
      firstHeader.click()
      const resizer = grid.shadowRoot.querySelector('.column-resizer') as HTMLElement
      resizer.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, clientX: 160 }))
      document.dispatchEvent(new MouseEvent('mousemove', { bubbles: true, clientX: 230 }))
      await new Promise((resolve) => requestAnimationFrame(resolve))
      document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, clientX: 230 }))
      const scrollport = grid.shadowRoot.querySelector('.scrollport') as HTMLDivElement
      scrollport.scrollTop = 9000
      scrollport.dispatchEvent(new Event('scroll'))
      await new Promise((resolve) => setTimeout(resolve, 80))
      const cellRect = grid.shadowRoot.querySelector('.cell')!.getBoundingClientRect()
      const tableRect = grid.shadowRoot.querySelector('.plane')!.getBoundingClientRect()
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        groups: Array.from(root.querySelectorAll('summary')).map((item) => item.textContent?.trim()),
        selectedTitle: root.querySelector('h2')?.textContent?.trim(),
        hasSearch: Boolean(root.querySelector('.search input')),
        hasSQLButton: Boolean(root.querySelector('.icon-button')),
        hasPreviewTable: Boolean(previewTable),
        hasWindowedTable: Boolean(grid),
        tableKey: grid.table?.tableKey,
        rowCount: grid.shadowRoot.querySelectorAll('.row[role="row"]').length,
        firstCellWidth: Math.round(cellRect.width),
        tableWidth: Math.round(tableRect.width),
        commands,
      }
    })

    expect(state.title).toBe('Data Explorer')
    expect(state.groups.join(' ')).toContain('Sources')
    expect(state.groups.join(' ')).toContain('Sales')
    expect(state.groups.join(' ')).toContain('Model tables')
    expect(state.groups.join(' ')).toContain('Semantic views')
    expect(state.selectedTitle).toBe('orders')
    expect(state.hasSearch).toBe(true)
    expect(state.hasSQLButton).toBe(true)
    expect(state.hasPreviewTable).toBe(true)
    expect(state.hasWindowedTable).toBe(true)
    expect(state.tableKey).toBe('sales:model_table:model_table:olist.orders')
    expect(state.rowCount).toBeGreaterThan(0)
    expect(state.tableWidth).toBeGreaterThan(700)
    expect(state.firstCellWidth).toBeGreaterThan(100)
    expect(state.commands.some((command) => command.workspaceId === 'sales' && command.objectKey === 'semantic_view:olist.orders')).toBe(true)
    expect(state.commands.some((command) => command.workspaceId === 'sales' && command.sort?.column === 'order_id')).toBe(true)
    expect(state.commands.some((command) => command.workspaceId === 'sales' && command.objectKey === 'model_table:model_table:olist.orders' && command.columnWidths?.order_id > 200)).toBe(true)
    expect(state.commands.some((command) => command.workspaceId === 'sales' && command.block && command.start > 0 && command.count === 100 && command.requestSeq > 0)).toBe(true)
  } finally {
    await page.close()
  }
})

function testDocument() {
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { font-family: Inter, system-ui, sans-serif; }
          ld-data-explorer { display: block; min-height: 720px; }
        </style>
      </head>
      <body>
        <script type="module" src="/data-explorer-under-test.js"></script>
      </body>
    </html>
  `
}
