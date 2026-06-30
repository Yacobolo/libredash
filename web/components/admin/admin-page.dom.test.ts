import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/admin-page-test')

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

for (const viewport of [
  { name: 'desktop', width: 1440, height: 820 },
  { name: 'mobile', width: 390, height: 820 },
]) {
  test(`admin page composes route UI on ${viewport.name}`, async () => {
    const page = await browser.newPage({ viewport })
    try {
      await page.goto(baseURL)
      await page.waitForFunction(() => (
        customElements.get('ld-admin-page')
          && customElements.get('ld-sub-sidebar')
          && customElements.get('ld-record-table')
      ))
      await page.locator('ld-admin-page').evaluate((element: any) => element.updateComplete)

      const state = await page.locator('ld-admin-page').evaluate((element: any) => {
        const root = element.shadowRoot
        const subSidebar = root.querySelector('ld-sub-sidebar') as HTMLElement
        const subSidebarAside = subSidebar?.shadowRoot?.querySelector('aside') as HTMLElement | null
        const main = root.querySelector('.main') as HTMLElement
        const mainRect = main.getBoundingClientRect()
        const routeRect = root.querySelector('.route')!.getBoundingClientRect()
        const sidebarRect = subSidebar.getBoundingClientRect()
        const availableLeft = window.innerWidth <= 640 ? routeRect.left : sidebarRect.right
        const availableRight = routeRect.right
        const availableCenter = availableLeft + (availableRight - availableLeft) / 2
        const isMobile = window.innerWidth <= 640
        return {
          title: root.querySelector('h1')?.textContent?.trim(),
          hasSidebar: Boolean(root.querySelector('ld-sub-sidebar')),
          sidebarBorderRight: subSidebar ? getComputedStyle(subSidebar).borderRight : '',
          sidebarBackground: subSidebarAside ? getComputedStyle(subSidebarAside).backgroundColor : '',
          mainCentered: isMobile || Math.abs((mainRect.left + mainRect.width / 2) - availableCenter) <= 1,
          mainConstrained: isMobile || Math.round(mainRect.width) < Math.round(availableRight - availableLeft),
          hasRecordTable: Boolean(root.querySelector('ld-record-table')),
          recordTableVariant: root.querySelector('ld-record-table')?.getAttribute('variant'),
          text: root.textContent,
        }
      })

      expect(state.title).toBe('Principals')
      expect(state.hasSidebar).toBe(true)
      if (viewport.width > 640) {
        expect(state.sidebarBorderRight).toContain('1px solid')
        expect(state.sidebarBackground).toBe('rgb(241, 243, 245)')
        expect(state.mainCentered).toBe(true)
        expect(state.mainConstrained).toBe(true)
      }
      expect(state.hasRecordTable).toBe(true)
      expect(state.recordTableVariant).toBe('compact')
      expect(state.text ?? '').toMatch(/analyst@example\.com/)
    } finally {
      await page.close()
    }
  })
}

test('admin storage route renders storage explorer from typed signal data', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-storage-explorer'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-admin-page') as any
      const table = {
        key: 'db\u0000main\u0000orders',
        databaseId: 'db',
        databaseName: 'libredash.duckdb',
        databasePath: '/tmp/duckdb/libredash.duckdb',
        modelId: 'olist',
        modelName: 'Olist Commerce',
        schema: 'main',
        name: 'orders',
        type: 'table',
        rowCountLabel: '10',
        columnCount: 1,
        sizeLabel: '12 KiB',
        columns: [{ name: 'order_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '' }],
      }
      const storage = {
        summary: { duckdbDir: '/tmp/duckdb', databaseCount: 1, totalSizeLabel: '12 KiB', tableCount: 1 },
        status: '',
        warnings: ['Storage warning'],
        selectedKey: 'db\u0000main\u0000orders',
        tables: [table],
        selectedTable: table,
      }
      element.page = {
        kind: 'admin',
        title: 'Storage',
        active: 'storage',
        sidebar: {
          label: 'Admin',
          railLabel: 'Admin',
          ariaLabel: 'Admin navigation',
          storageKey: 'libredash-admin-sidebar-collapsed',
          activeId: 'storage',
          collapsible: false,
          numbered: false,
          items: [{ id: 'storage', title: 'Storage', href: '/admin/storage', active: true }],
        },
        headerTitle: 'Storage',
        headerDetail: 'Read-only DuckDB database and table inventory.',
        metrics: [{ label: 'Tables and views', value: '1' }],
        storage,
      }
      element.storage = storage
      document.body.append(element)
      await element.updateComplete
      const explorer = element.shadowRoot.querySelector('ld-storage-explorer') as any
      await explorer.updateComplete
      return {
        hasPageTitle: Boolean(element.shadowRoot.querySelector('h1')),
        explorerTitle: explorer.shadowRoot.querySelector('h2')?.textContent?.trim(),
        hasGenericMetrics: Boolean(element.shadowRoot.querySelector('.metrics')),
        warning: explorer.shadowRoot.textContent?.includes('Storage warning'),
        hasExplorer: Boolean(explorer),
        explorerHeight: Math.round(explorer.shadowRoot.querySelector('.storage-explorer')?.getBoundingClientRect().height ?? 0),
        searchInBrowserMenu: Boolean(explorer.shadowRoot.querySelector('.storage-browser-menu .storage-search input')),
        searchInPageHeader: Boolean(explorer.shadowRoot.querySelector('.storage-explorer-header .storage-search input')),
        hasGlobalSummary: Boolean(explorer.shadowRoot.querySelector('.storage-summary')),
        detailBadges: explorer.shadowRoot.querySelectorAll('.storage-detail-header > span, .storage-columns-header > span').length,
        databaseTreeCounts: explorer.shadowRoot.querySelectorAll('.storage-db > summary em').length,
        schemaTreeCounts: explorer.shadowRoot.querySelectorAll('.storage-schema > summary em').length,
        tableListSizes: Array.from(explorer.shadowRoot.querySelectorAll('.storage-table-size')).map((size) => size.textContent?.trim()),
        searchBorder: getComputedStyle(explorer.shadowRoot.querySelector('.storage-search input')!).border,
        explorerText: explorer.shadowRoot.textContent,
      }
    })

    expect(state.hasPageTitle).toBe(false)
    expect(state.explorerTitle).toBe('Storage')
    expect(state.hasGenericMetrics).toBe(false)
    expect(state.warning).toBe(true)
    expect(state.hasExplorer).toBe(true)
    expect(state.explorerHeight).toBeGreaterThan(500)
    expect(state.searchInBrowserMenu).toBe(true)
    expect(state.searchInPageHeader).toBe(false)
    expect(state.hasGlobalSummary).toBe(false)
    expect(state.detailBadges).toBe(0)
    expect(state.databaseTreeCounts).toBe(0)
    expect(state.schemaTreeCounts).toBe(1)
    expect(state.tableListSizes).toEqual(['12 KiB'])
    expect(state.searchBorder).toContain('0px')
    expect(state.explorerText ?? '').toMatch(/orders/)
    expect(state.explorerText ?? '').toMatch(/Olist Commerce/)
    expect(state.explorerText ?? '').toMatch(/\/tmp\/duckdb/)
    expect(state.explorerText ?? '').toMatch(/12 KiB/)
  } finally {
    await page.close()
  }
})

test('admin storage explorer keeps table, schema, and breadcrumb selection coherent', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-storage-explorer'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-storage-explorer') as any
      const customers = {
        key: 'db\u0000model\u0000customers',
        databaseId: 'db',
        databaseName: 'libredash-olist.duckdb',
        databasePath: '/tmp/duckdb/libredash-olist.duckdb',
        modelId: 'olist',
        modelName: 'Olist Commerce',
        schema: 'model',
        name: 'customers',
        type: 'table',
        rowCountLabel: '10',
        columnCount: 1,
        sizeLabel: '12 KiB',
        columns: [{ name: 'customer_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '' }],
      }
      const orders = {
        ...customers,
        key: 'db\u0000model\u0000orders',
        name: 'orders',
        rowCountLabel: '20',
        columns: [{ name: 'order_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '' }],
      }
      element.storage = {
        summary: { duckdbDir: '/tmp/duckdb', databaseCount: 1, totalSizeLabel: '24 KiB', tableCount: 2 },
        status: '',
        warnings: [],
        selectedKey: customers.key,
        tables: [customers, orders],
        selectedTable: customers,
      }
      const commands: unknown[] = []
      element.addEventListener('ld-storage-table-select', (event: CustomEvent) => commands.push(event.detail))
      document.body.append(element)
      await element.updateComplete

      const root = element.shadowRoot
      const selectedNames = () => Array.from(root.querySelectorAll('.storage-table-button.is-selected')).map((button) => button.textContent?.trim())
      const tableSizes = () => Array.from(root.querySelectorAll('.storage-table-size')).map((size) => size.textContent?.trim())
      const detailText = () => root.querySelector('.storage-detail')?.textContent ?? ''
      const ordersButton = Array.from(root.querySelectorAll<HTMLButtonElement>('.storage-table-button')).find((button) => button.textContent?.includes('orders'))!
      const schemaSummary = root.querySelector<HTMLElement>('.storage-schema > summary')!

      ordersButton.click()
      await element.updateComplete
      const afterOrders = {
        selectedNames: selectedNames(),
        tableSizes: tableSizes(),
        detail: detailText(),
        commands: [...commands],
      }

      schemaSummary.click()
      await element.updateComplete
      const afterSchema = {
        selectedNames: selectedNames(),
        detail: detailText(),
      }

      const schemaRowsBeforeBreadcrumb = root.querySelectorAll('ld-record-table tbody tr').length
      ordersButton.click()
      await element.updateComplete
      const schemaBreadcrumb = root.querySelector<HTMLButtonElement>('button[data-breadcrumb-kind="schema"]')!
      schemaBreadcrumb.click()
      await element.updateComplete
      const databaseBreadcrumb = root.querySelector<HTMLButtonElement>('button[data-breadcrumb-kind="database"]')!
      databaseBreadcrumb.click()
      await element.updateComplete
      const afterBreadcrumb = {
        selectedNames: selectedNames(),
        detail: detailText(),
        schemaRows: root.querySelectorAll('ld-record-table tbody tr').length,
        schemaRowsBeforeBreadcrumb,
      }

      return { afterOrders, afterSchema, afterBreadcrumb }
    })

    expect(state.afterOrders.selectedNames).toHaveLength(1)
    expect(state.afterOrders.selectedNames[0]).toContain('orders')
    expect(state.afterOrders.tableSizes).toEqual(['12 KiB', '12 KiB'])
    expect(state.afterOrders.detail).toContain('order_id')
    expect(state.afterOrders.commands).toEqual([{ databaseId: 'db', schema: 'model', table: 'orders' }])

    expect(state.afterSchema.selectedNames).toHaveLength(0)
    expect(state.afterSchema.detail).toContain('Tables')
    expect(state.afterSchema.detail).toContain('customers')
    expect(state.afterSchema.detail).toContain('orders')

    expect(state.afterBreadcrumb.selectedNames).toHaveLength(0)
    expect(state.afterBreadcrumb.detail).toContain('Schemas')
    expect(state.afterBreadcrumb.detail).toContain('model')
    expect(state.afterBreadcrumb.schemaRows).toBe(1)
    expect(state.afterBreadcrumb.schemaRowsBeforeBreadcrumb).toBe(2)
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  const page = {
    kind: 'admin',
    title: 'Principals',
    active: 'principals',
    sidebar: {
      label: 'Admin',
      railLabel: 'Admin',
      ariaLabel: 'Admin navigation',
      storageKey: 'libredash-admin-sidebar-collapsed',
      activeId: 'principals',
      collapsible: false,
      numbered: false,
      items: [
        { id: 'general', title: 'General', href: '/admin', active: false },
        { id: 'principals', title: 'Principals', href: '/admin/principals', active: true },
        { id: 'groups', title: 'Groups', href: '/admin/groups', active: false },
      ],
    },
    headerTitle: 'Principals',
    headerDetail: 'Users and service principals known to LibreDash.',
    sections: [{
      title: 'Principals',
      table: {
        columns: [
          { id: 'name', header: 'Name', kind: 'link', hrefKey: 'name_href' },
          { id: 'email', header: 'Email' },
          { id: 'roles', header: 'Direct roles', kind: 'tags' },
        ],
        rows: [{ name: 'Analyst', name_href: '/admin/principals/p1', email: 'analyst@example.com', roles: ['viewer'] }],
        empty: 'No principals found.',
      },
    }],
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-sidebar-bg: #f1f3f5; --ld-report-rail-bg: #ffffff; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-radius-default: 6px; --ld-radius-full: 999px; --base-size-4: 4px; --base-size-8: 8px; --base-size-12: 12px; --base-size-16: 16px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-title-sm: 16px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; }
          ld-admin-page { min-height: 720px; }
        </style>
      </head>
      <body>
        <ld-admin-page page="${attr(page)}"></ld-admin-page>
        <script type="module" src="/admin-page-under-test.js"></script>
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
