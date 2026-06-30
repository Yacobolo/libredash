import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser, type Page } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/record-table-test')

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

test('record table renders cells and sorts through TanStack headers', async () => {
  const page = await browser.newPage({ viewport: { width: 900, height: 620 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-record-table'))
    await page.locator('ld-record-table').evaluate((element: any) => {
      element.table = {
        columns: [
          { id: 'name', header: 'Name', kind: 'entity', width: '260px' },
          { id: 'status', header: 'Status', kind: 'status', width: '150px' },
          { id: 'score', header: 'Score', kind: 'number', align: 'right', width: '110px' },
          { id: 'key', header: 'Key', kind: 'code', width: '160px' },
          { id: 'kind', header: 'Kind', kind: 'badge', width: '130px' },
          { id: 'roles', header: 'Roles', kind: 'tags', width: '180px' },
          { id: 'sql', header: 'SQL', kind: 'expression', width: '220px' },
          { id: 'actions', header: 'Actions', kind: 'actions', align: 'right', sortable: false, width: '96px' },
        ],
        rows: [
          {
            name: { label: 'Orders', description: 'Fact table', href: '/orders', icon: 'table' },
            status: { label: 'succeeded', tone: 'success' },
            score: 2,
            key: 'orders',
            kind: { label: 'table', tone: 'muted' },
            roles: ['sales', 'ops'],
            sql: 'select * from orders',
            actions: [{ label: 'Open', href: '/orders/open', icon: 'open' }],
          },
          {
            name: { label: 'Customers', description: 'Dimension table', href: '/customers', icon: 'semantic_model' },
            status: { label: 'queued', tone: 'attention' },
            score: 1,
            key: 'customers',
            kind: { label: 'view', tone: 'accent' },
            roles: ['crm'],
            sql: 'select * from customers',
            actions: [{ label: 'Open', href: '/customers/open', icon: 'open' }],
          },
        ],
        empty: 'No records.',
        minWidth: '1300px',
      }
    })
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)

    const initial = await tableState(page)
    expect(initial.firstNames).toEqual(['Orders', 'Customers'])
    expect(initial.hasEntityLink).toBe(true)
    expect(initial.hasStatusIcon).toBe(true)
    expect(initial.hasCode).toBe(true)
    expect(initial.hasExpression).toBe(true)
    expect(initial.badges).toEqual(['table', 'view'])
    expect(initial.tags).toEqual(['sales', 'ops', 'crm'])
    expect(initial.actionHref).toBe('/orders/open')
    expect(initial.minWidth).toBe('1300px')
    expect(initial.headerPosition).toBe('sticky')

    await page.locator('ld-record-table th:nth-child(3) button').click()
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    expect((await tableState(page)).firstNames).toEqual(['Customers', 'Orders'])

    await page.locator('ld-record-table th:nth-child(3) button').click()
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    expect((await tableState(page)).firstNames).toEqual(['Orders', 'Customers'])
  } finally {
    await page.close()
  }
})

test('primary record table gives entity cells the full column width', async () => {
  const page = await browser.newPage({ viewport: { width: 760, height: 520 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-record-table'))
    await page.locator('ld-record-table').evaluate((element: any) => {
      element.setAttribute('variant', 'primary')
      element.table = {
        columns: [
          { id: 'name', header: 'Name', kind: 'entity', width: '380px' },
          { id: 'type', header: 'Type', width: '160px' },
        ],
        rows: [{
          name: {
            label: 'Executive Sales Dashboard',
            description: 'Sales, order, category, and delivery overview.',
            href: '/dashboards/executive-sales',
            icon: 'dashboard',
          },
          type: 'Dashboard',
        }],
        empty: 'No records.',
        minWidth: '620px',
      }
    })
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    const state = await page.locator('ld-record-table').evaluate((element) => {
      const cell = element.querySelector('tbody td:first-child') as HTMLElement
      const link = element.querySelector('.record-entity-link') as HTMLElement
      const title = element.querySelector('.record-entity-label') as HTMLElement
      const description = element.querySelector('.record-entity-description') as HTMLElement
      const cellStyle = getComputedStyle(cell)
      const cellPadding = Number.parseFloat(cellStyle.paddingLeft) + Number.parseFloat(cellStyle.paddingRight)
      return {
        variant: element.getAttribute('variant'),
        linkWidth: Math.round(link.getBoundingClientRect().width),
        cellInnerWidth: Math.round(cell.getBoundingClientRect().width - cellPadding),
        titleFits: title.getBoundingClientRect().right <= cell.getBoundingClientRect().right,
        descriptionFits: description.getBoundingClientRect().right <= cell.getBoundingClientRect().right,
        iconBackground: getComputedStyle(element.querySelector('.record-entity-icon')!).backgroundColor,
      }
    })
    expect(state.variant).toBe('primary')
    expect(state.linkWidth).toBeGreaterThanOrEqual(state.cellInnerWidth - 4)
    expect(state.titleFits).toBe(true)
    expect(state.descriptionFits).toBe(true)
    expect(state.iconBackground).not.toBe('rgba(0, 0, 0, 0)')
  } finally {
    await page.close()
  }
})

test('compact record table keeps metadata dense and scalar placeholders muted', async () => {
  const page = await browser.newPage({ viewport: { width: 760, height: 520 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-record-table'))
    await page.locator('ld-record-table').evaluate((element: any) => {
      element.setAttribute('variant', 'compact')
      element.table = {
        columns: [
          { id: 'name', header: 'Name', width: '220px' },
          { id: 'email', header: 'Email', kind: 'link', hrefKey: 'emailHref', width: '260px' },
          { id: 'roles', header: 'Roles', kind: 'number', width: '100px' },
          { id: 'key', header: 'Key', kind: 'code', width: '160px' },
        ],
        rows: [
          { name: 'Analyst', email: 'analyst@example.com', emailHref: 'mailto:analyst@example.com', roles: 3, key: 'analyst' },
          { name: '', email: '', roles: null, key: '' },
        ],
        empty: 'No records.',
        minWidth: '740px',
      }
    })
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    const initial = await page.locator('ld-record-table').evaluate((element) => {
      const wrap = element.querySelector('.record-table-wrap') as HTMLElement
      const firstHeader = element.querySelector('thead th') as HTMLElement
      const firstCell = element.querySelector('tbody td') as HTMLElement
      const numberHeader = element.querySelector('thead th:nth-child(3)') as HTMLElement
      const numberCell = element.querySelector('tbody tr:first-child td:nth-child(3)') as HTMLElement
      const unsortedIndicator = element.querySelector('thead th:first-child .record-table-sort-indicator') as HTMLElement
      return {
        variant: element.getAttribute('variant'),
        hasCompactClass: wrap.classList.contains('variant-compact'),
        headerPaddingTop: getComputedStyle(firstHeader).paddingTop,
        cellPaddingTop: getComputedStyle(firstCell).paddingTop,
        headerBackground: getComputedStyle(firstHeader).backgroundColor,
        numberHeaderAlign: getComputedStyle(numberHeader).textAlign,
        numberCellAlign: getComputedStyle(numberCell).textAlign,
        mutedCount: element.querySelectorAll('.record-muted').length,
        unsortedIndicatorOpacity: getComputedStyle(unsortedIndicator).opacity,
      }
    })
    expect(initial.variant).toBe('compact')
    expect(initial.hasCompactClass).toBe(true)
    expect(initial.headerPaddingTop).toBe('8px')
    expect(initial.cellPaddingTop).toBe('8px')
    expect(initial.headerBackground).toBe('rgb(246, 248, 250)')
    expect(initial.numberHeaderAlign).toBe('right')
    expect(initial.numberCellAlign).toBe('right')
    expect(initial.mutedCount).toBe(4)
    expect(initial.unsortedIndicatorOpacity).toBe('0')

    await page.locator('ld-record-table th:nth-child(3) button').click()
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    const sorted = await page.locator('ld-record-table').evaluate((element) => {
      const sortedIndicator = element.querySelector('thead th:nth-child(3) .record-table-sort-indicator') as HTMLElement
      return {
        sortedIndicatorOpacity: getComputedStyle(sortedIndicator).opacity,
        sortedIndicatorText: sortedIndicator.textContent?.trim(),
      }
    })
    expect(sorted.sortedIndicatorOpacity).toBe('1')
    expect(sorted.sortedIndicatorText).toBe('↑')
  } finally {
    await page.close()
  }
})

test('record table renders configured empty state', async () => {
  const page = await browser.newPage({ viewport: { width: 500, height: 360 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-record-table'))
    await page.locator('ld-record-table').evaluate((element: any) => {
      element.table = { columns: [{ id: 'name', header: 'Name' }], rows: [], empty: 'No records.' }
    })
    await page.locator('ld-record-table').evaluate((element: any) => element.updateComplete)
    const empty = await page.locator('ld-record-table .record-table-empty').textContent()
    expect(empty).toBe('No records.')
  } finally {
    await page.close()
  }
})

async function tableState(page: Page) {
  return page.locator('ld-record-table').evaluate((element) => {
    const rows = Array.from(element.querySelectorAll('tbody tr'))
    const firstNames = rows.map((row) => row.querySelector('.record-entity-label')?.textContent?.trim() ?? '')
    const table = element.querySelector('table') as HTMLTableElement
    const header = element.querySelector('th') as HTMLTableCellElement
    return {
      firstNames,
      hasEntityLink: Boolean(element.querySelector('a.record-entity-link[href="/orders"]')),
      hasStatusIcon: Boolean(element.querySelector('.record-status-icon svg')),
      hasCode: Boolean(element.querySelector('code.record-code')),
      hasExpression: Boolean(element.querySelector('code.record-expression')),
      badges: Array.from(element.querySelectorAll('.record-badge')).map((badge) => badge.textContent?.trim()),
      tags: Array.from(element.querySelectorAll('.record-tags span')).map((tag) => tag.textContent?.trim()),
      actionHref: element.querySelector<HTMLAnchorElement>('.record-icon-action')?.getAttribute('href'),
      minWidth: getComputedStyle(table).minWidth,
      headerPosition: getComputedStyle(header).position,
    }
  })
}

function testDocument(): string {
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          body { --fontStack-system: system-ui; --fontStack-monospace: monospace; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-line-muted: #d8dee4; --ld-border-muted: 1px solid #d8dee4; --ld-border-transparent: 1px solid transparent; --ld-radius-default: 6px; --ld-radius-full: 999px; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --control-medium-size: 32px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-body-md: 14px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-font-weight-regular: 400; --ld-line-height-normal: 1.5; --ld-line-height-compact: 1.3; }
        </style>
      </head>
      <body>
        <ld-record-table></ld-record-table>
        <script type="module" src="/record-table-under-test.js"></script>
      </body>
    </html>
  `
}
