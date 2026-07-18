import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const projectRoot = process.cwd()
const root = join(projectRoot, '.tmp/admin-page-test')

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

test('admin navigation remains pinned while content scrolls on desktop', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-sub-sidebar') && customElements.get('ld-record-table'))

      const state = await page.evaluate(async () => {
        const element = document.createElement('ld-admin-page') as any
        element.style.minHeight = '1600px'
      const pageSignal = {
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
            { id: 'agent', title: 'Agent', href: '/admin/agent', active: false },
            { id: 'storage', title: 'Storage', href: '/admin/storage', active: false },
          ],
        },
        headerTitle: 'Principals',
        headerDetail: 'Users and service principals known to LibreDash.',
        sections: Array.from({ length: 40 }, (_, index) => ({
          title: `Section ${index + 1}`,
          facts: [
            { label: 'Principals', value: `${index + 1}` },
            { label: 'Groups', value: `${index + 2}` },
            { label: 'Roles', value: `${index + 3}` },
          ],
        })),
      }
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: pageSignal })
      const spacer = document.createElement('div')
      spacer.style.height = '1600px'
      document.body.replaceChildren(element, spacer)
      document.documentElement.style.minHeight = '2400px'
      document.body.style.minHeight = '2400px'
      await element.updateComplete
      const subSidebar = element.shadowRoot.querySelector('ld-sub-sidebar') as HTMLElement
      const before = subSidebar.getBoundingClientRect()
      window.scrollTo(0, 420)
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
      const after = subSidebar.getBoundingClientRect()
      return {
        scrollY: window.scrollY,
        beforeTop: Math.round(before.top),
        afterTop: Math.round(after.top),
        afterHeight: Math.round(after.height),
      }
    })

    expect(state.scrollY).toBeGreaterThan(300)
    expect(state.beforeTop).toBe(0)
    expect(state.afterTop).toBe(0)
    expect(state.afterHeight).toBe(720)
  } finally {
    await page.close()
  }
})

function queryAuditFixturePage() {
  const queryEvents = [
    {
      id: 'queryevent_1',
      workspaceId: 'sales',
      principalId: 'analyst',
      surface: 'api',
      operation: 'api_query',
      queryKind: 'semantic_aggregate',
      modelId: 'sales',
      target: 'orders',
      objectType: 'semantic_dataset',
      objectId: 'sales:orders',
      requestId: 'req_1',
      correlationId: 'corr_1',
      status: 'success',
      durationMs: 12,
      rowsReturned: 2,
      error: '',
      sql: 'select status from orders',
      planText: 'orders plan',
      queryJson: '{"workspaceId":"sales","target":"orders"}',
      createdAt: '2026-07-02T10:00:00Z',
    },
    {
      id: 'queryevent_2',
      workspaceId: 'operations',
      principalId: 'agent',
      surface: 'agent',
      operation: 'agent_query',
      queryKind: 'semantic_rows',
      modelId: 'operations',
      target: 'customers',
      objectType: 'agent_tool',
      objectId: 'query_semantic_dataset',
      requestId: 'call_1',
      correlationId: '',
      status: 'error',
      durationMs: 4,
      rowsReturned: 0,
      error: 'invalid field',
      sql: '',
      planText: '',
      queryJson: '{"workspaceId":"operations","target":"customers"}',
      createdAt: '2026-07-02T10:01:00Z',
    },
  ]
  return {
    kind: 'admin',
    title: 'Query History',
    active: 'queries',
    sidebar: {
      label: 'Admin',
      railLabel: 'Admin',
      ariaLabel: 'Admin navigation',
      storageKey: 'libredash-admin-sidebar-collapsed',
      activeId: 'queries',
      collapsible: false,
      numbered: false,
      items: [{ id: 'queries', title: 'Query History', href: '/admin/queries', active: true }],
    },
    headerTitle: 'Query History',
    headerDetail: 'Product query audit.',
    queryHistory: {
      table: queryAuditTableFixture(queryEvents),
      filterMenus: queryAuditFilterMenusFixture(),
      filters: {},
      nextCursor: 'cursor_next',
      loadedCountLabel: '2 queries loaded',
      hasMore: true,
      loading: false,
      error: '',
      limit: 50,
    },
    queryDetail: {
      eventId: 'queryevent_1',
      loading: false,
      error: '',
      status: 'success',
      statusLabel: 'Success',
      workspaceId: 'sales',
      principalId: 'analyst',
      surface: 'api',
      operation: 'api_query',
      queryKind: 'semantic_aggregate',
      modelId: 'sales',
      target: 'orders',
      objectType: 'semantic_dataset',
      objectId: 'sales:orders',
      requestId: 'req_1',
      correlationId: 'corr_1',
      durationMs: 12,
      rowsReturned: 2,
      queryError: '',
      sql: 'select status from orders',
      planText: 'orders plan',
      queryJson: '{"workspaceId":"sales","target":"orders"}',
      createdAt: '2026-07-02T10:00:00Z',
    },
  }
}

function queryAuditFilterMenusFixture() {
  return [
    {
      id: 'workspace',
      label: 'Workspace',
      summaryLabel: 'Workspace',
      mode: 'multi',
      search: '',
      selected: [],
      loading: false,
      error: '',
      placeholder: 'Search workspaces',
      emptyLabel: 'No workspaces found.',
      options: [
        { value: 'sales', label: 'sales', icon: 'workspace', countLabel: '1', selected: false, disabled: false },
        { value: 'operations', label: 'operations', icon: 'workspace', countLabel: '1', selected: false, disabled: false },
      ],
    },
    {
      id: 'principal',
      label: 'User',
      summaryLabel: 'User',
      mode: 'multi',
      search: '',
      selected: [],
      loading: false,
      error: '',
      placeholder: 'Search users',
      emptyLabel: 'No users found.',
      options: [
        { value: 'analyst', label: 'Me (analyst@example.com)', icon: 'user', countLabel: '1', selected: false, disabled: false },
        { value: 'agent', label: 'agent', icon: 'user', countLabel: '1', selected: false, disabled: false },
      ],
    },
    {
      id: 'surface',
      label: 'Source type',
      summaryLabel: 'Source type',
      mode: 'multi',
      search: '',
      selected: [],
      loading: false,
      error: '',
      placeholder: 'Search source types',
      emptyLabel: 'No source types found.',
      options: [
        { value: 'api', label: 'api', icon: 'source', countLabel: '1', selected: false, disabled: false },
        { value: 'agent', label: 'agent', icon: 'source', countLabel: '1', selected: false, disabled: false },
      ],
    },
    {
      id: 'kind',
      label: 'Kind',
      summaryLabel: 'Kind',
      mode: 'multi',
      search: '',
      selected: [],
      loading: false,
      error: '',
      placeholder: 'Search kinds',
      emptyLabel: 'No kinds found.',
      options: [
        { value: 'semantic_aggregate', label: 'semantic_aggregate', icon: 'kind', countLabel: '1', selected: false, disabled: false },
        { value: 'semantic_rows', label: 'semantic_rows', icon: 'kind', countLabel: '1', selected: false, disabled: false },
      ],
    },
    {
      id: 'status',
      label: 'Status',
      summaryLabel: 'Status',
      mode: 'multi',
      search: '',
      selected: [],
      loading: false,
      error: '',
      placeholder: 'Search statuses',
      emptyLabel: 'No statuses found.',
      options: [
        { value: 'success', label: 'success', icon: 'status', countLabel: '1', selected: false, disabled: false },
        { value: 'error', label: 'error', icon: 'status', countLabel: '1', selected: false, disabled: false },
      ],
    },
  ]
}

function queryAuditTableFixture(events: any[]) {
  return {
    columns: [
      { id: 'query', header: 'Query', kind: 'query', width: '560px', toggleable: false },
      { id: 'started_at', header: 'Started', width: '150px' },
      { id: 'duration_ms', header: 'Duration', kind: 'number', align: 'right', width: '105px' },
      { id: 'source', header: 'Source type', width: '120px' },
      { id: 'runtime', header: 'Runtime', kind: 'code', width: '130px' },
      { id: 'principal_id', header: 'User', kind: 'code', width: '150px' },
      { id: 'rows_returned', header: 'Rows', kind: 'number', align: 'right', width: '90px' },
      { id: 'operation', header: 'Operation', kind: 'code', width: '145px' },
      { id: 'kind', header: 'Kind', kind: 'code', width: '170px' },
      { id: 'model', header: 'Model', kind: 'code', width: '130px' },
      { id: 'target', header: 'Target', kind: 'code', width: '150px' },
      { id: 'object', header: 'Object', kind: 'code', width: '220px' },
      { id: 'request_id', header: 'Request ID', kind: 'code', width: '170px' },
      { id: 'correlation_id', header: 'Correlation ID', kind: 'code', width: '170px' },
      { id: 'error', header: 'Error', kind: 'code', width: '220px' },
    ],
    rows: events.map((event) => ({
      id: event.id,
      query: {
        label: event.sql || `${event.operation} · ${event.queryKind} · ${event.modelId}.${event.target}`,
        statusLabel: event.status,
        tone: event.status === 'success' ? 'success' : 'danger',
        icon: event.status === 'success' ? 'check' : 'x',
        expandedContent: event.sql || `${event.operation} · ${event.queryKind}`,
      },
      started_at: event.createdAt,
      duration_ms: { label: `${event.durationMs ?? 0} ms`, value: event.durationMs ?? 0 },
      source: event.surface,
      runtime: event.workspaceId || '-',
      principal_id: event.principalId,
      rows_returned: event.rowsReturned,
      operation: event.operation,
      kind: event.queryKind,
      model: event.modelId,
      target: event.target,
      object: [event.objectType, event.objectId].filter(Boolean).join(':') || '-',
      request_id: event.requestId,
      correlation_id: event.correlationId,
      error: event.error,
    })),
    empty: 'No query events match these filters.',
    minWidth: '1305px',
    density: 'tight',
    rowAction: 'detail',
    columnSelector: {
      enabled: true,
      label: 'Columns',
      defaultColumns: ['started_at', 'duration_ms', 'source', 'runtime', 'principal_id', 'rows_returned'],
    },
  }
}

test('query audit page filters table rows and exposes optional metadata columns', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-record-table'))
    const state = await page.evaluate(async (fixture) => {
      localStorage.removeItem('libredash-admin-query-events-columns')
      const element = document.createElement('ld-admin-page') as any
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: fixture, adminQueryHistory: fixture.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
      ;(window as any).queryHistoryCommands = []
      element.addEventListener('ld-query-history-command', (event: CustomEvent) => {
        ;(window as any).queryHistoryCommands.push(event.detail)
      })
      document.body.replaceChildren(element)
      await element.updateComplete
      const root = element.shadowRoot
      const search = root.querySelector<HTMLInputElement>('#query-filter-search')!
      search.value = 'select status'
      search.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
      await new Promise((resolve) => setTimeout(resolve, 250))
      await element.updateComplete
      const commandAfterSearch = (window as any).queryHistoryCommands.at(-1)
      const menus = Array.from(root.querySelectorAll('ld-filter-menu')) as any[]
      menus[0]?.shadowRoot?.querySelector<HTMLButtonElement>('.trigger')?.click()
      await menus[0]?.updateComplete
      const workspaceMenuSearch = menus[0]?.shadowRoot?.querySelector<HTMLInputElement>('.search input')
      if (workspaceMenuSearch) {
        workspaceMenuSearch.value = 'oper'
        workspaceMenuSearch.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
      }
      await new Promise((resolve) => setTimeout(resolve, 250))
      await element.updateComplete
      const filterSearchCommand = (window as any).queryHistoryCommands.at(-1)
      menus[2]?.shadowRoot?.querySelector<HTMLButtonElement>('.trigger')?.click()
      await menus[2]?.updateComplete
      menus[2]?.shadowRoot?.querySelector<HTMLInputElement>('.option input')?.click()
      await element.updateComplete
      const filterToggleCommand = (window as any).queryHistoryCommands.at(-1)
      const table = root.querySelector('ld-record-table') as any
      const rowText = table?.textContent ?? ''
      table.querySelector('.record-table-column-selector summary')?.click()
      Array.from(table.querySelectorAll('label'))
        .find((label) => label.textContent?.includes('Runtime'))
        ?.querySelector('input')
        ?.click()
      await table.updateComplete
      const hiddenRuntimeText = table.textContent ?? ''
      const visibleHeaderLabels = (recordTable: Element) => Array.from(recordTable.querySelectorAll('thead th')).map((header: Element) => header.querySelector('.record-table-sort span:first-child')?.textContent?.trim() ?? '')
      const hiddenRuntimeHeaders = visibleHeaderLabels(table)
      table.querySelector<HTMLButtonElement>('.record-query-expand')?.click()
      await table.updateComplete
      const expandedCodeBlock = table.querySelector('.record-query-expanded-cell ld-code-block') as (HTMLElement & { updateComplete: Promise<boolean> }) | null
      await expandedCodeBlock?.updateComplete
      const expandedQueryText = expandedCodeBlock?.shadowRoot?.querySelector('code')?.textContent
        ?? table.querySelector('.record-query-expanded-cell')?.textContent
        ?? ''
      const drawerAfterExpand = root.querySelector('.query-detail-drawer')?.textContent ?? ''
      table.querySelector<HTMLButtonElement>('.record-query-expand')?.click()
      await table.updateComplete
      table.querySelector<HTMLElement>('tbody tr.record-row')?.click()
      await element.updateComplete
      const detailCommand = (window as any).queryHistoryCommands.at(-1)
      mergePatch({ adminQueryDetail: fixture.queryDetail })
      await element.updateComplete
      const drawer = root.querySelector('.query-detail-drawer') as HTMLElement | null
      const drawerText = drawer?.textContent ?? ''
      const drawerCodeBlock = drawer?.querySelector('ld-code-block') as (HTMLElement & { updateComplete: Promise<boolean> }) | null
      await drawerCodeBlock?.updateComplete
      const drawerCode = drawerCodeBlock?.shadowRoot?.querySelector('code')?.textContent ?? drawerCodeBlock?.querySelector('code')?.textContent ?? ''
      const drawerAnimationName = drawer ? getComputedStyle(drawer).animationName : ''
      const status = drawer?.querySelector('.query-detail-status') as HTMLElement | null
      const statusIcon = status?.querySelector('svg') as SVGElement | null
      const statusText = status?.querySelector('span') as HTMLElement | null
      const statusColor = status ? getComputedStyle(status).color : ''
      const statusTextColor = statusText ? getComputedStyle(statusText).color : ''
      const statusIconColor = statusIcon ? getComputedStyle(statusIcon).color : ''
      const hasSubtitle = Boolean(drawer?.querySelector('.query-detail-subtitle'))
      root.querySelector<HTMLButtonElement>('.query-detail-close')?.click()
      await element.updateComplete
      const closeCommand = (window as any).queryHistoryCommands.at(-1)
      mergePatch({ adminQueryDetail: { eventId: '', loading: false, error: '' } })
      await element.updateComplete
      const hasDrawerAfterClose = Boolean(root.querySelector('.query-detail-drawer'))
      table.querySelector<HTMLElement>('tbody tr.record-row')?.click()
      await element.updateComplete
      mergePatch({ adminQueryDetail: fixture.queryDetail })
      await element.updateComplete
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await element.updateComplete
      const escapeCommand = (window as any).queryHistoryCommands.at(-1)
      mergePatch({ adminQueryDetail: { eventId: '', loading: false, error: '' } })
      await element.updateComplete
      const hasDrawerAfterEscape = Boolean(root.querySelector('.query-detail-drawer'))
      Array.from(table.querySelectorAll('label'))
        .find((label) => label.textContent?.includes('Operation'))
        ?.querySelector('input')
        ?.click()
      await table.updateComplete
      const operationHeaders = visibleHeaderLabels(table)
      const operationText = table.textContent ?? ''
      const hasDetailAction = Boolean(table.querySelector('.record-icon-action[aria-label="Details"]'))
      const recreated = document.createElement('ld-admin-page') as any
      document.body.replaceChildren(recreated)
      await recreated.updateComplete
      const recreatedTable = recreated.shadowRoot.querySelector('ld-record-table') as any
      const refreshedHeaders = visibleHeaderLabels(recreatedTable)
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        hasFilters: root.querySelectorAll('ld-filter-menu').length === 5,
        firstMenuText: menus[0]?.shadowRoot?.textContent ?? '',
        hasMetrics: Boolean(root.querySelector('.metrics')),
        hasColumnSelector: Boolean(table.querySelector('.record-table-column-selector')),
        queryStatusLabel: table.querySelector('.record-query-status')?.getAttribute('aria-label'),
        hasStatusHeader: visibleHeaderLabels(table).includes('Status'),
        sourceBadgeCount: table.querySelectorAll('.record-badge').length,
        rowHeight: Math.round(table.querySelector('tbody tr:first-child')?.getBoundingClientRect().height ?? 0),
        rowText,
        commandAfterSearch,
        filterSearchCommand,
        filterToggleCommand,
        hiddenRuntimeText,
        hiddenRuntimeHeaders,
        expandedQueryText,
        drawerAfterExpand,
        drawerText,
        detailCommand,
        closeCommand,
        escapeCommand,
        drawerHasCodeBlock: Boolean(drawerCodeBlock),
        drawerCode,
        drawerAnimationName,
        statusColor,
        statusTextColor,
        statusIconColor,
        hasSubtitle,
        hasDrawerAfterClose,
        hasDrawerAfterEscape,
        operationHeaders,
        operationText,
        hasDetailAction,
        refreshedHeaders,
      }
    }, queryAuditFixturePage())

    expect(state.title).toBe('Query History')
    expect(state.hasFilters).toBe(true)
    expect(state.firstMenuText).toMatch(/Workspace/)
    expect(state.hasMetrics).toBe(false)
    expect(state.rowText).toMatch(/Query/)
    expect(state.rowText).not.toMatch(/Status/)
    expect(state.rowText).toMatch(/Started/)
    expect(state.rowText).toMatch(/Source type/)
    expect(state.rowText).toMatch(/Runtime/)
    expect(state.rowText).toMatch(/User/)
    expect(state.rowText).toMatch(/analyst/)
    expect(state.rowText).toMatch(/select status from orders/)
    expect(state.rowText).toMatch(/orders/)
    expect(state.rowText).toMatch(/customers/)
    expect(state.rowText).not.toMatch(/stale_page_event/)
    expect(state.commandAfterSearch).toMatchObject({ action: 'reset', limit: 50, filters: { search: 'select status' } })
    expect(state.filterSearchCommand).toMatchObject({ action: 'filter_search', filterMenu: { menuId: 'workspace', action: 'search', search: 'oper' } })
    expect(state.filterToggleCommand).toMatchObject({ action: 'filter_toggle', filterMenu: { menuId: 'surface', action: 'toggle', value: 'api' } })
    expect(state.hasColumnSelector).toBe(true)
    expect(state.hasStatusHeader).toBe(false)
    expect(state.sourceBadgeCount).toBe(0)
    expect(state.queryStatusLabel).toBe('success')
    expect(state.rowHeight).toBeLessThanOrEqual(44)
    expect(state.hiddenRuntimeHeaders).not.toContain('Runtime')
    expect(state.hiddenRuntimeHeaders).not.toContain('Status')
    expect(state.hiddenRuntimeHeaders[0]).toBe('Query')
    expect(state.hiddenRuntimeText).toMatch(/select status from orders/)
    expect(state.expandedQueryText).toMatch(/SELECT\s+status\s+FROM\s+orders/i)
    expect(state.drawerAfterExpand).toBe('')
    expect(state.detailCommand).toMatchObject({ action: 'select_detail', eventId: 'queryevent_1', limit: 50 })
    expect(state.drawerText).toMatch(/Finished|Success|success/i)
    expect(state.drawerText).toMatch(/analyst/)
    expect(state.drawerText).toMatch(/api/)
    expect(state.drawerText).toMatch(/sales/)
    expect(state.hasSubtitle).toBe(false)
    expect(state.statusTextColor).toBe(state.statusColor)
    expect(state.statusIconColor).not.toBe(state.statusColor)
    expect(state.drawerText).toMatch(/queryevent_1/)
    expect(state.drawerText).toMatch(/req_1/)
    expect(state.drawerText).toMatch(/corr_1/)
    expect(state.drawerHasCodeBlock).toBe(true)
    expect(state.drawerCode).toContain('SELECT')
    expect(state.drawerCode).toMatch(/\nFROM\n\s+orders/)
    expect(state.drawerText).toMatch(/12 ms/)
    expect(state.drawerText).toMatch(/semantic_aggregate/)
    expect(state.drawerText).toMatch(/semantic_dataset:sales:orders/)
    expect(state.drawerText).toMatch(/Rows returned/)
    expect(state.drawerAnimationName).toContain('query-detail-slide-in')
    expect(state.closeCommand).toMatchObject({ action: 'close_detail' })
    expect(state.escapeCommand).toMatchObject({ action: 'close_detail' })
    expect(state.hasDrawerAfterClose).toBe(false)
    expect(state.hasDrawerAfterEscape).toBe(false)
    expect(state.operationHeaders).toContain('Operation')
    expect(state.operationText).toMatch(/api_query/)
    expect(state.hasDetailAction).toBe(false)
    expect(state.refreshedHeaders).toContain('Runtime')
    expect(state.refreshedHeaders).not.toContain('Operation')
    expect(state.refreshedHeaders).not.toContain('Status')
  } finally {
    await page.close()
  }
})

test('query audit emits load more commands from backend-driven history state', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-record-table'))
      const state = await page.evaluate(async (fixture) => {
        const element = document.createElement('ld-admin-page') as any
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: fixture, adminQueryHistory: fixture.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
      ;(window as any).queryHistoryCommands = []
      element.addEventListener('ld-query-history-command', (event: CustomEvent) => {
        ;(window as any).queryHistoryCommands.push(event.detail)
      })
      document.body.replaceChildren(element)
      await element.updateComplete
      const root = element.shadowRoot
      const footerText = root.querySelector('.query-history-footer')?.textContent ?? ''
      root.querySelector<HTMLButtonElement>('.query-history-load-more')?.click()
      await element.updateComplete
      const command = (window as any).queryHistoryCommands.at(-1)
      mergePatch({ adminQueryHistory: {
        ...fixture.queryHistory,
        table: {
          ...fixture.queryHistory.table,
          rows: [fixture.queryHistory.table.rows[1]],
        },
        filterMenus: fixture.queryHistory.filterMenus.map((menu: any) => menu.id === 'workspace' ? {
          ...menu,
          summaryLabel: 'operations',
          selected: ['operations'],
          options: menu.options.map((option: any) => ({ ...option, selected: option.value === 'operations' })),
        } : menu),
        filters: { workspaces: ['operations'] },
        nextCursor: '',
        hasMore: false,
        loadedCountLabel: '1 query loaded',
      } })
      await element.updateComplete
      const updatedText = root.textContent ?? ''
      const workspaceMenu = root.querySelector('ld-filter-menu') as HTMLElement | null
      return {
        footerText,
        command,
        updatedText,
        hasLoadMoreAfterPatch: Boolean(root.querySelector('.query-history-load-more')),
        workspaceFilterText: workspaceMenu?.shadowRoot?.textContent ?? '',
      }
    }, queryAuditFixturePage())

    expect(state.footerText).toMatch(/2 queries loaded/)
    expect(state.command).toMatchObject({ action: 'load_more', pageToken: 'cursor_next', limit: 50 })
    expect(state.updatedText).toMatch(/customers/)
    expect(state.updatedText).not.toMatch(/orders/)
    expect(state.hasLoadMoreAfterPatch).toBe(false)
    expect(state.workspaceFilterText).toMatch(/operations/)
  } finally {
    await page.close()
  }
})

test('query audit detail drawer behaves as a mobile overlay', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-record-table'))
      const state = await page.evaluate(async (fixture) => {
        const element = document.createElement('ld-admin-page') as any
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: fixture, adminQueryHistory: fixture.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
      document.body.replaceChildren(element)
      await element.updateComplete
      const root = element.shadowRoot
      const table = root.querySelector('ld-record-table') as any
      table.querySelector<HTMLElement>('tbody tr.record-row')?.click()
      await element.updateComplete
      mergePatch({ adminQueryDetail: fixture.queryDetail })
      await element.updateComplete
      const drawer = root.querySelector('.query-detail-drawer') as HTMLElement
      const drawerRect = drawer.getBoundingClientRect()
      const tableRect = table.getBoundingClientRect()
      return {
        drawerText: drawer.textContent ?? '',
        drawerPosition: getComputedStyle(drawer).position,
        drawerWidth: Math.round(drawerRect.width),
        viewportWidth: window.innerWidth,
        drawerCoversTableHorizontally: drawerRect.left <= tableRect.left && drawerRect.right >= tableRect.right,
      }
    }, queryAuditFixturePage())

    expect(state.drawerText).toMatch(/queryevent_1/)
    expect(state.drawerPosition).toBe('fixed')
    expect(state.drawerWidth).toBe(state.viewportWidth)
    expect(state.drawerCoversTableHorizontally).toBe(true)
  } finally {
    await page.close()
  }
})

test('query audit drawer does not block selecting another row', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-record-table'))
      const state = await page.evaluate(async (fixture) => {
        const element = document.createElement('ld-admin-page') as any
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: fixture, adminQueryHistory: fixture.queryHistory, adminQueryDetail: { eventId: '', loading: false, error: '' } })
      document.body.replaceChildren(element)
      await element.updateComplete
      const root = element.shadowRoot
      const table = root.querySelector('ld-record-table') as any
      const rows = Array.from(table.querySelectorAll<HTMLElement>('tbody tr.record-row'))
      rows[0]?.click()
      await element.updateComplete
      mergePatch({ adminQueryDetail: fixture.queryDetail })
      await element.updateComplete
      const firstDrawerText = root.querySelector('.query-detail-drawer')?.textContent ?? ''
      const hasBackdrop = Boolean(root.querySelector('.query-detail-backdrop'))
      rows[1]?.click()
      await element.updateComplete
      mergePatch({ adminQueryDetail: {
        ...fixture.queryDetail,
        eventId: 'queryevent_2',
        status: 'error',
        statusLabel: 'Error',
        workspaceId: 'operations',
        principalId: 'agent',
        surface: 'agent',
        operation: 'agent_query',
        queryKind: 'semantic_rows',
        modelId: 'operations',
        target: 'customers',
        objectType: 'agent_tool',
        objectId: 'query_semantic_dataset',
        requestId: 'call_1',
        correlationId: '',
        durationMs: 4,
        rowsReturned: 0,
        queryError: 'invalid field',
        sql: '',
        planText: '',
        queryJson: '{"workspaceId":"operations","target":"customers"}',
        createdAt: '2026-07-02T10:01:00Z',
      } })
      await element.updateComplete
      const secondDrawerText = root.querySelector('.query-detail-drawer')?.textContent ?? ''
      return {
        hasBackdrop,
        firstDrawerText,
        secondDrawerText,
      }
    }, queryAuditFixturePage())

    expect(state.hasBackdrop).toBe(false)
    expect(state.firstDrawerText).toMatch(/queryevent_1/)
    expect(state.firstDrawerText).toMatch(/analyst/)
    expect(state.secondDrawerText).toMatch(/queryevent_2/)
    expect(state.secondDrawerText).toMatch(/agent/)
    expect(state.secondDrawerText).toMatch(/invalid field/)
  } finally {
    await page.close()
  }
})

test('admin storage route renders storage explorer from typed signal data', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-storage-explorer'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-admin-page') as any
      const table = {
        key: 'ducklake-catalog\u0000model\u0000orders',
        databaseId: 'ducklake-catalog',
        databaseName: 'DuckLake catalog',
        databasePath: '/tmp/libredash/libredash.db',
        modelId: 'ducklake',
        modelName: 'DuckLake',
        schema: 'model',
        name: 'orders',
        type: 'table',
        tableId: 42,
        tableUuid: 'table-uuid',
        duckLakePath: 'model/orders/',
        beginSnapshot: 7,
        endSnapshot: 0,
        rowCount: 32000204,
        rowCountLabel: '32,000,204',
        columnCount: 1,
        fileCount: 1,
        sizeBytes: 12288,
        sizeLabel: '12 KiB',
        columns: [{ id: 91, name: 'order_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '', initialDefault: '', defaultValueType: 'literal', defaultValueDialect: 'duckdb', beginSnapshot: 7, containsNull: 'No', containsNan: '-', minValue: 'o_001', maxValue: 'o_999', extraStats: '' }],
        files: [{ id: 9, path: 'model/orders/file.parquet', format: 'parquet', recordCount: 32000204, recordCountLabel: '32,000,204', sizeBytes: 12288, sizeLabel: '12 KiB', beginSnapshot: 7, endSnapshot: 0 }],
        history: [{ snapshotId: 7, time: '2026-07-03T10:00:00Z', schemaVersion: 1, source: 'table,data_file', changes: 'tables_inserted_into', author: 'tester', message: 'materialize orders', extraInfo: '{}' }],
        servingStates: [{ workspaceId: 'sales', environment: 'dev', servingStateId: 'state_1', status: 'active', snapshotId: 7, digest: 'digest', active: true, activatedAt: 'now' }],
      }
      const storage = {
        summary: {
          catalogPath: '/tmp/libredash/libredash.db',
          dataPath: '/tmp/libredash/data',
          catalogSizeLabel: '32 KiB',
          dataSizeLabel: '12 KiB',
          totalSizeLabel: '44 KiB',
          totalDataSizeLabel: '12 KiB',
          databaseCount: 1,
          tableCount: 1,
          snapshotCount: 1,
          dataFileCount: 1,
        },
        status: '',
        warnings: ['Storage warning'],
        selectedKey: 'ducklake-catalog\u0000model\u0000orders',
        tables: [table],
        snapshots: [{ id: 7, time: '2026-07-03T10:00:00Z', schemaVersion: 1, author: 'tester', message: 'materialize', changes: 'tables_inserted_into', extraInfo: '{}', protected: true, servingStateCount: 1 }],
        servingStates: [{ workspaceId: 'sales', environment: 'dev', servingStateId: 'state_1', status: 'active', snapshotId: 7, digest: 'digest', active: true, activatedAt: 'now' }],
        selectedTable: table,
      }
      const pageSignal = {
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
        headerDetail: 'Read-only DuckLake catalog and table metadata.',
        metrics: [{ label: 'Tables', value: '1' }],
        storage,
      }
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: pageSignal, adminStorage: storage })
      document.body.append(element)
      await element.updateComplete
      const explorer = element.shadowRoot.querySelector('ld-storage-explorer') as any
      await explorer.updateComplete
      const schemaText = explorer.shadowRoot.textContent
      const filesTab = Array.from(explorer.shadowRoot.querySelectorAll<HTMLButtonElement>('.storage-tab')).find((button) => button.textContent?.includes('Data files'))
      filesTab?.click()
      await explorer.updateComplete
      const filesText = explorer.shadowRoot.textContent
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
        databaseTreeBadges: Array.from(explorer.shadowRoot.querySelectorAll('.storage-db > summary em')).map((badge) => badge.textContent?.trim()),
        schemaTreeBadges: Array.from(explorer.shadowRoot.querySelectorAll('.storage-schema > summary em')).map((badge) => badge.textContent?.trim()),
        tableListSizes: Array.from(explorer.shadowRoot.querySelectorAll('.storage-table-size')).map((size) => size.textContent?.trim()),
        searchBorder: getComputedStyle(explorer.shadowRoot.querySelector('.storage-search input')!).border,
        metricsOverflow: getComputedStyle(explorer.shadowRoot.querySelector('.storage-metrics')!).overflowX,
        metricsWrap: getComputedStyle(explorer.shadowRoot.querySelector('.storage-metrics')!).flexWrap,
        explorerText: explorer.shadowRoot.textContent,
        schemaText,
        filesText,
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
    expect(state.hasGlobalSummary).toBe(true)
    expect(state.detailBadges).toBe(0)
    expect(state.databaseTreeBadges).toEqual([])
    expect(state.schemaTreeBadges).toEqual([])
    expect(state.tableListSizes).toEqual(['12 KiB'])
    expect(state.searchBorder).toContain('0px')
    expect(state.metricsOverflow).toBe('hidden')
    expect(state.metricsWrap).toBe('wrap')
    expect(state.explorerText ?? '').toMatch(/orders/)
    expect(state.explorerText ?? '').toMatch(/DuckLake catalog/)
    expect(state.explorerText ?? '').toMatch(/\/tmp\/libredash\/libredash\.db/)
    expect(state.explorerText ?? '').toMatch(/Table UUID/)
    expect(state.explorerText ?? '').toMatch(/table-uuid/)
    expect(state.explorerText ?? '').toMatch(/DuckLake path/)
    expect(state.explorerText ?? '').toMatch(/model\/orders\//)
    expect(state.schemaText ?? '').toMatch(/Column ID/)
    expect(state.schemaText ?? '').toMatch(/literal/)
    expect(state.schemaText ?? '').toMatch(/duckdb/)
    expect(state.schemaText ?? '').toMatch(/Nulls/)
    expect(state.schemaText ?? '').toMatch(/o_001/)
    expect(state.schemaText ?? '').toMatch(/o_999/)
    expect(state.filesText ?? '').toMatch(/model\/orders\/file\.parquet/)
    expect(state.explorerText ?? '').toMatch(/32,000,204/)
    expect(state.explorerText ?? '').not.toMatch(/32000204/)
    expect(state.explorerText ?? '').not.toMatch(/dep_1/)
    expect(state.explorerText ?? '').toMatch(/12 KiB/)
  } finally {
    await page.close()
  }
})

test('admin agent route renders prompt editor, tools catalog, and emits save command', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-agent-prompt-editor') && customElements.get('ld-agent-tools'))

    const state = await page.evaluate(async () => {
      const waitFor = async (predicate: () => boolean, timeoutMs = 5000): Promise<void> => {
        const started = performance.now()
        while (!predicate()) {
          if (performance.now() - started > timeoutMs) throw new Error('timed out waiting for condition')
          await new Promise((resolve) => setTimeout(resolve, 20))
        }
      }
      const element = document.createElement('ld-admin-page') as any
      const pageSignal = {
        kind: 'admin',
        title: 'Agent',
        active: 'agent',
        sidebar: {
          label: 'Admin',
          railLabel: 'Admin',
          ariaLabel: 'Admin navigation',
          storageKey: 'libredash-admin-sidebar-collapsed',
          activeId: 'agent',
          collapsible: false,
          numbered: false,
          items: [{ id: 'agent', title: 'Agent', href: '/admin/agent', active: true }],
        },
        headerTitle: 'Agent',
        headerDetail: 'Platform agent prompt and read-only tool inventory.',
        metrics: [{ label: 'Tools', value: '1' }],
        agent: {
          enabled: true,
          model: 'fake-model',
          systemPrompt: 'Initial prompt',
          canWrite: true,
          updatePath: '/admin/agent/config',
          tools: [{
            name: 'query_visual',
            description: 'Query visual data.',
            inputSchema: {
              type: 'object',
              required: ['dashboardId'],
              properties: {
                dashboardId: { type: 'string', description: 'Dashboard identifier.' },
                mode: { enum: ['summary', 'detail'], description: 'Result detail level.' },
              },
              additionalProperties: false,
            },
          }],
        },
        sections: [{
          title: 'Tools',
          table: {
            columns: [{ id: 'name', header: 'Name', kind: 'code' }],
            rows: [{ name: 'query_visual' }],
            empty: 'No tools configured.',
          },
        }],
      }
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: pageSignal, adminAgentCommand: { systemPrompt: 'Signal prompt' } })
      document.body.append(element)
      await element.updateComplete
      let command: unknown = null
      element.addEventListener('ld-agent-system-prompt-save', (event: CustomEvent) => { command = event.detail })
      const root = element.shadowRoot
      const editor = root.querySelector('ld-agent-prompt-editor') as any
      const toolsCatalog = root.querySelector('ld-agent-tools') as any
      await editor.updateComplete
      await toolsCatalog.updateComplete
      const editorRoot = editor.shadowRoot
      await customElements.whenDefined('ld-code-editor')
      await waitFor(() => Boolean(editorRoot.querySelector('ld-code-editor')))
      const controlRow = editorRoot.querySelector('.prompt-control-row')!
      const actions = editorRoot.querySelector('.prompt-actions')!
      const body = editorRoot.querySelector('.prompt-body')!
      const markdownView = editorRoot.querySelector('ld-markdown-view') as any
      const preSwitchState = {
        hasCodeEditor: Boolean(editorRoot.querySelector('ld-code-editor')),
        hasMarkdownView: Boolean(markdownView),
        markdownViewCompact: markdownView?.compact,
        markdownValue: markdownView?.value,
        hasLoading: Boolean(editorRoot.querySelector('.editor-loading')),
        hasTextarea: Boolean(editorRoot.querySelector('textarea')),
        hasSaveButton: Boolean(editorRoot.querySelector('.save-button')),
        status: editorRoot.querySelector('.prompt-status')?.textContent?.trim() ?? '',
      }
      const editButton = editorRoot.querySelector<HTMLButtonElement>('.mode-toggle button[aria-label="Edit"]')!
      editButton.click()
      await editor.updateComplete
      const immediateSwitchState = {
        hasCodeEditor: Boolean(editorRoot.querySelector('ld-code-editor')),
        hasLoading: Boolean(editorRoot.querySelector('.editor-loading')),
        hasTextarea: Boolean(editorRoot.querySelector('textarea')),
      }
      await editor.updateComplete
      const codeEditor = editorRoot.querySelector('ld-code-editor') as any
      await codeEditor.updateComplete
      await waitFor(() => Boolean(codeEditor.shadowRoot.querySelector('.view-line')))
      const editorFontSize = getComputedStyle(codeEditor.shadowRoot.querySelector('.view-line')!).fontSize
      const seededEditorValue = codeEditor.value
      codeEditor.value = 'Updated prompt'
      codeEditor.dispatchEvent(new CustomEvent('ld-code-editor-change', {
        bubbles: true,
        composed: true,
        detail: { value: 'Updated prompt' },
      }))
      await codeEditor.updateComplete
      await editor.updateComplete
      const dirtyState = {
        hasSaveButton: Boolean(editorRoot.querySelector('.save-button')),
        saveText: editorRoot.querySelector('.save-button')?.textContent?.trim(),
        status: editorRoot.querySelector('.prompt-status')?.textContent?.trim(),
      }
      codeEditor.value = 'Signal prompt'
      codeEditor.dispatchEvent(new CustomEvent('ld-code-editor-change', {
        bubbles: true,
        composed: true,
        detail: { value: 'Signal prompt' },
      }))
      await codeEditor.updateComplete
      await editor.updateComplete
      const revertedState = {
        hasSaveButton: Boolean(editorRoot.querySelector('.save-button')),
        status: editorRoot.querySelector('.prompt-status')?.textContent?.trim() ?? '',
      }
      codeEditor.value = 'Updated prompt'
      codeEditor.dispatchEvent(new CustomEvent('ld-code-editor-change', {
        bubbles: true,
        composed: true,
        detail: { value: 'Updated prompt' },
      }))
      await codeEditor.updateComplete
      await editor.updateComplete
      editorRoot.querySelector<HTMLButtonElement>('.save-button')?.click()
      await editor.updateComplete
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        hasEditor: Boolean(editor),
        hasToolsCatalog: Boolean(toolsCatalog),
        hasGenericToolsRecordTable: Boolean(root.querySelector('section[aria-label="Tools"] ld-record-table')),
        toolsCatalogText: toolsCatalog.shadowRoot.textContent,
        hasCodeEditor: Boolean(codeEditor),
        preSwitchState,
        immediateSwitchState,
        actionsInControlRow: actions.parentElement === controlRow,
        actionsBeforeBody: Boolean(actions.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING),
        actionsAfterBody: Boolean(actions.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_PRECEDING),
        dirtyState,
        revertedState,
        editorFontSize,
        seededEditorValue,
        editorValue: codeEditor.value,
        hasSaveAfterSave: Boolean(editorRoot.querySelector('.save-button')),
        activeMode: editorRoot.querySelector('.mode-toggle button[aria-pressed="true"]')?.getAttribute('aria-label'),
        status: editorRoot.querySelector('.prompt-status')?.textContent?.trim(),
        command,
      }
    })

    expect(state.title).toBe('Agent')
    expect(state.hasEditor).toBe(true)
    expect(state.hasToolsCatalog).toBe(true)
    expect(state.hasGenericToolsRecordTable).toBe(false)
    expect(state.toolsCatalogText ?? '').toMatch(/query_visual/)
    expect(state.toolsCatalogText ?? '').toMatch(/dashboardId/)
    expect(state.hasCodeEditor).toBe(true)
    expect(state.preSwitchState).toEqual({
      hasCodeEditor: true,
      hasMarkdownView: true,
      markdownViewCompact: true,
      markdownValue: 'Signal prompt',
      hasLoading: false,
      hasTextarea: false,
      hasSaveButton: false,
      status: '',
    })
    expect(state.immediateSwitchState).toEqual({ hasCodeEditor: true, hasLoading: false, hasTextarea: false })
    expect(state.actionsInControlRow).toBe(true)
    expect(state.actionsBeforeBody).toBe(true)
    expect(state.actionsAfterBody).toBe(false)
    expect(state.editorFontSize).toBe('12px')
    expect(state.seededEditorValue).toBe('Signal prompt')
    expect(state.editorValue).toBe('Updated prompt')
    expect(state.dirtyState).toEqual({ hasSaveButton: true, saveText: 'Save', status: 'Unsaved changes' })
    expect(state.revertedState).toEqual({ hasSaveButton: false, status: '' })
    expect(state.hasSaveAfterSave).toBe(false)
    expect(state.activeMode).toBe('Edit')
    expect(state.status).toBe('Saved')
    expect(state.command).toEqual({ systemPrompt: 'Updated prompt' })
  } finally {
    await page.close()
  }
})

test('admin agent prompt editor disables saves for read-only users', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-agent-prompt-editor'))

    const state = await page.evaluate(async () => {
      const waitFor = async (predicate: () => boolean, timeoutMs = 5000): Promise<void> => {
        const started = performance.now()
        while (!predicate()) {
          if (performance.now() - started > timeoutMs) throw new Error('timed out waiting for condition')
          await new Promise((resolve) => setTimeout(resolve, 20))
        }
      }
      const element = document.createElement('ld-admin-page') as any
      const pageSignal = {
        kind: 'admin',
        title: 'Agent',
        active: 'agent',
        sidebar: {
          label: 'Admin',
          railLabel: 'Admin',
          ariaLabel: 'Admin navigation',
          storageKey: 'libredash-admin-sidebar-collapsed',
          activeId: 'agent',
          collapsible: false,
          numbered: false,
          items: [{ id: 'agent', title: 'Agent', href: '/admin/agent', active: true }],
        },
        headerTitle: 'Agent',
        headerDetail: 'Platform agent prompt and read-only tool inventory.',
        agent: {
          enabled: true,
          model: 'fake-model',
          systemPrompt: 'Initial prompt',
          canWrite: false,
          updatePath: '/admin/agent/config',
          tools: [],
        },
        sections: [],
      }
      const { mergePatch } = await import('/static/vendor/datastar-1.0.2.js?v=dev') as any
      mergePatch({ page: pageSignal, adminAgentCommand: { systemPrompt: '' } })
      document.body.append(element)
      await element.updateComplete
      let command: unknown = null
      element.addEventListener('ld-agent-system-prompt-save', (event: CustomEvent) => { command = event.detail })
      const editor = element.shadowRoot.querySelector('ld-agent-prompt-editor') as any
      await editor.updateComplete
      const editorRoot = editor.shadowRoot
      const editButton = editorRoot.querySelector<HTMLButtonElement>('.mode-toggle button[aria-label="Edit"]')!
      editButton.click()
      await customElements.whenDefined('ld-code-editor')
      await waitFor(() => Boolean(editorRoot.querySelector('ld-code-editor')))
      await editor.updateComplete
      const codeEditor = editorRoot.querySelector('ld-code-editor') as any
      await codeEditor.updateComplete
      const saveButton = editorRoot.querySelector<HTMLButtonElement>('.save-button')
      return {
        codeEditorDisabled: codeEditor.disabled,
        hasSaveButton: Boolean(saveButton),
        status: editorRoot.querySelector('.prompt-status')?.textContent?.trim(),
        command,
      }
    })

    expect(state.codeEditorDisabled).toBe(true)
    expect(state.hasSaveButton).toBe(false)
    expect(state.status).toBe('Read-only')
    expect(state.command).toBeNull()
  } finally {
    await page.close()
  }
})

test('admin agent tools catalog renders payload fields, JSON, empty, unsupported, and search', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-agent-tools'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-agent-tools') as any
      element.tools = [{
        name: 'query_visual',
        description: 'Query visual data.',
        effect: 'read',
        defaults: { mode: 'summary' },
        inputSchema: {
          type: 'object',
          required: ['dashboardId', 'mode'],
          properties: {
            dashboardId: { type: 'string', description: 'Dashboard identifier.' },
            filters: {
              type: 'object',
              properties: {
                dateRange: {
                  type: 'object',
                  required: ['start'],
                  properties: {
                    start: { type: 'string', description: 'Start date.' },
                    end: { type: 'string', description: 'End date.' },
                  },
                },
              },
            },
            metrics: { type: 'array', items: { type: 'string' }, description: 'Metric IDs.' },
            mode: { enum: ['summary', 'detail'], description: 'Result detail level.' },
            dimensions: { type: 'array', items: { $ref: '#/$defs/fieldRef' }, description: 'Dimension fields.' },
            series: { $ref: '#/$defs/fieldRef', description: 'Series field.' },
            sort: { type: 'array', items: { $ref: '#/$defs/sort' } },
            options: { type: 'object', additionalProperties: true, description: 'Renderer options.' },
            rendererOptions: {
              type: 'object',
              additionalProperties: { type: 'object', additionalProperties: true },
              description: 'Renderer-specific options.',
            },
          },
          $defs: {
            fieldRef: {
              type: 'object',
              additionalProperties: false,
              required: ['field'],
              properties: {
                field: { type: 'string', minLength: 1, description: 'Semantic field ID.' },
                alias: { type: 'string', description: 'Display alias.' },
              },
            },
            sort: {
              type: 'object',
              additionalProperties: false,
              required: ['field'],
              properties: {
                field: { type: 'string', minLength: 1 },
                direction: { type: 'string', enum: ['asc', 'desc'] },
              },
            },
          },
          additionalProperties: false,
        },
        outputSchema: {
          type: 'object',
          properties: {
            rows: { type: 'array', items: { type: 'object' } },
          },
          additionalProperties: false,
        },
      }, {
        name: 'no_input',
        description: 'No payload required.',
        inputSchema: { type: 'object', additionalProperties: false },
      }, {
        name: 'unsupported_input',
        description: 'Composition schema.',
        inputSchema: { oneOf: [{ type: 'string' }, { type: 'number' }] },
      }]
      document.body.append(element)
      await element.updateComplete
      const root = element.shadowRoot
      const firstText = root.textContent ?? ''
      const catalogHeight = Math.round(root.querySelector('.catalog')!.getBoundingClientRect().height)
      const listOverflow = getComputedStyle(root.querySelector('.list')!).overflowY
      const detailBodyOverflow = getComputedStyle(root.querySelector('.detail-body')!).overflowY
      const toolButtons = Array.from(root.querySelectorAll('.tool-button')).map((button) => button.textContent?.trim())
      const listText = root.querySelector('.list')?.textContent ?? ''
      const firstRows = Array.from(root.querySelectorAll('.fields tbody tr')).map((row) => Array.from(row.querySelectorAll('td')).map((cell) => cell.textContent?.trim()))
      const detailMeta = Array.from(root.querySelectorAll('.detail-meta .required-count')).map((item) => item.textContent?.trim())

      const jsonButton = root.querySelector<HTMLButtonElement>('.tabs button:nth-child(2)')!
      jsonButton.click()
      await element.updateComplete
      const jsonText = root.querySelector('.json')?.textContent ?? ''

      const outputButton = root.querySelector<HTMLButtonElement>('.tabs button:nth-child(3)')!
      outputButton.click()
      await element.updateComplete
      const outputText = root.querySelector('.json')?.textContent ?? ''

      const noInputButton = Array.from(root.querySelectorAll<HTMLButtonElement>('.tool-button')).find((button) => button.textContent?.includes('no_input'))!
      noInputButton.click()
      await element.updateComplete
      const noInputText = root.textContent ?? ''

      const unsupportedButton = Array.from(root.querySelectorAll<HTMLButtonElement>('.tool-button')).find((button) => button.textContent?.includes('unsupported_input'))!
      unsupportedButton.click()
      await element.updateComplete
      const unsupportedText = root.textContent ?? ''

      const search = root.querySelector<HTMLInputElement>('input[type="search"]')!
      search.value = 'filters.dateRange.start'
      search.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true }))
      await element.updateComplete
      const searchRows = Array.from(root.querySelectorAll('.tool-button')).map((button) => button.textContent?.trim())
      return {
        firstText,
        catalogHeight,
        listOverflow,
        detailBodyOverflow,
        toolButtons,
        listText,
        firstRows,
        detailMeta,
        jsonText,
        outputText,
        noInputText,
        unsupportedText,
        searchRows,
      }
    })

    expect(state.firstText).toMatch(/query_visual/)
    expect(state.firstText).toMatch(/dashboardId, filters\.dateRange\.start, filters\.dateRange\.end \+10/)
    expect(state.catalogHeight).toBeGreaterThan(440)
    expect(state.listOverflow).toBe('auto')
    expect(state.detailBodyOverflow).toBe('auto')
    expect(state.toolButtons).toEqual(['query_visual', 'no_input', 'unsupported_input'])
    expect(state.listText).not.toMatch(/Query visual data/)
    expect(state.detailMeta).toEqual(['read', '6 required', 'dashboardId, filters.dateRange.start, filters.dateRange.end +10', 'Defaults: mode=summary'])
    expect(state.firstRows).toContainEqual(['dashboardId', 'string', 'Yes', 'Dashboard identifier.'])
    expect(state.firstRows).toContainEqual(['filters.dateRange.start', 'string', 'Yes', 'Start date.'])
    expect(state.firstRows).toContainEqual(['filters.dateRange.end', 'string', 'No', 'End date.'])
    expect(state.firstRows).toContainEqual(['metrics', 'array<string>', 'No', 'Metric IDs.'])
    expect(state.firstRows).toContainEqual(['mode', 'enum: summary | detail', 'Yes', 'Result detail level.'])
    expect(state.firstRows).toContainEqual(['dimensions[].field', 'string', 'Yes', 'Semantic field ID.'])
    expect(state.firstRows).toContainEqual(['dimensions[].alias', 'string', 'No', 'Display alias.'])
    expect(state.firstRows).toContainEqual(['series.field', 'string', 'Yes', 'Semantic field ID.'])
    expect(state.firstRows).toContainEqual(['sort[].direction', 'enum: asc | desc', 'No', '-'])
    expect(state.firstRows).toContainEqual(['options', 'object<string, any>', 'No', 'Renderer options.'])
    expect(state.firstRows).toContainEqual(['rendererOptions', 'object<string, object>', 'No', 'Renderer-specific options.'])
    expect(state.jsonText).toMatch(/"dashboardId"/)
    expect(state.outputText).toMatch(/"rows"/)
    expect(state.noInputText).toMatch(/No input/)
    expect(state.unsupportedText).toMatch(/Schema is only available as JSON/)
    expect(state.searchRows).toHaveLength(1)
    expect(state.searchRows[0] ?? '').toMatch(/query_visual/)
  } finally {
    await page.close()
  }
})

test('agent prompt editor seeds edit mode from value attribute', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-agent-prompt-editor'))

    const state = await page.evaluate(async () => {
      const waitFor = async (predicate: () => boolean, timeoutMs = 5000): Promise<void> => {
        const started = performance.now()
        while (!predicate()) {
          if (performance.now() - started > timeoutMs) throw new Error('timed out waiting for condition')
          await new Promise((resolve) => setTimeout(resolve, 20))
        }
      }
      const element = document.createElement('ld-agent-prompt-editor') as any
      element.setAttribute('value', 'Attribute prompt')
      document.body.append(element)
      await element.updateComplete
      const root = element.shadowRoot
      const editButton = root.querySelector<HTMLButtonElement>('.mode-toggle button[aria-label="Edit"]')!
      editButton.click()
      await customElements.whenDefined('ld-code-editor')
      await waitFor(() => Boolean(root.querySelector('ld-code-editor')))
      await element.updateComplete
      const codeEditor = root.querySelector('ld-code-editor') as any
      await codeEditor.updateComplete
      return {
        activeMode: root.querySelector('.mode-toggle button[aria-pressed="true"]')?.getAttribute('aria-label'),
        codeEditorValue: codeEditor.value,
      }
    })

    expect(state.activeMode).toBe('Edit')
    expect(state.codeEditorValue).toBe('Attribute prompt')
  } finally {
    await page.close()
  }
})

test('agent prompt preview delegates to compact markdown view', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-agent-prompt-editor') && customElements.get('ld-markdown-view'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-agent-prompt-editor') as any
      element.value = [
        '# Hello darkness',
        '',
        'A paragraph with **strong**, _emphasis_, ~~strike~~, `inline code`, and https://example.com.',
        '',
        '## Section',
        '',
        '- One',
        '- Two',
        '  - Nested',
        '',
        '> Quoted guidance',
        '',
        '---',
        '',
        '| Name | Value |',
        '| --- | --- |',
        '| Tool | Enabled |',
        '',
        '```json',
        '{"enabled": true}',
        '```',
        '',
        '![Alt text](https://example.com/image.png)',
      ].join('\n')
      document.body.append(element)
      await element.updateComplete
      const root = element.shadowRoot
      const markdownView = root.querySelector('ld-markdown-view') as any
      await markdownView.updateComplete
      const h1 = markdownView.shadowRoot.querySelector('h1')!
      return {
        hasMarkdownView: Boolean(markdownView),
        compact: markdownView.compact,
        value: markdownView.value,
        emptyText: markdownView.emptyText,
        h1Text: h1.textContent,
      }
    })

    expect(state.hasMarkdownView).toBe(true)
    expect(state.compact).toBe(true)
    expect(state.value).toMatch(/^# Hello darkness/)
    expect(state.emptyText).toBe('No system prompt configured.')
    expect(state.h1Text).toBe('Hello darkness')
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
        key: 'ducklake-catalog\u0000model\u0000customers',
        databaseId: 'ducklake-catalog',
        databaseName: 'DuckLake catalog',
        databasePath: '/tmp/libredash/libredash.db',
        modelId: 'ducklake',
        modelName: 'DuckLake',
        schema: 'model',
        name: 'customers',
        type: 'table',
        tableId: 41,
        tableUuid: 'customers-uuid',
        duckLakePath: 'model/customers/',
        beginSnapshot: 6,
        endSnapshot: 0,
        rowCount: 10,
        rowCountLabel: '10',
        columnCount: 1,
        fileCount: 1,
        sizeBytes: 12288,
        sizeLabel: '12 KiB',
        columns: [{ id: 81, name: 'customer_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '', initialDefault: '', defaultValueType: 'literal', defaultValueDialect: 'duckdb', beginSnapshot: 6, containsNull: 'No', containsNan: '-', minValue: 'c_001', maxValue: 'c_999', extraStats: '' }],
        files: [{ id: 1, path: 'model/customers/file.parquet', format: 'parquet', recordCount: 10, recordCountLabel: '10', sizeBytes: 12288, sizeLabel: '12 KiB', beginSnapshot: 6, endSnapshot: 0 }],
        history: [{ snapshotId: 6, time: '2026-07-03T10:00:00Z', schemaVersion: 1, source: 'table,data_file', changes: 'tables_inserted_into', author: 'tester', message: 'materialize customers', extraInfo: '{}' }],
        servingStates: [{ workspaceId: 'olist', environment: 'dev', servingStateId: 'state_1', status: 'active', snapshotId: 6, digest: 'digest', active: true, activatedAt: 'now' }],
      }
      const orders = {
        ...customers,
        key: 'ducklake-catalog\u0000model\u0000orders',
        name: 'orders',
        tableId: 42,
        tableUuid: 'orders-uuid',
        duckLakePath: 'model/orders/',
        rowCount: 20,
        rowCountLabel: '20',
        columns: [{ id: 82, name: 'order_id', type: 'VARCHAR', ordinal: 1, nullable: 'No', default: '', initialDefault: '', defaultValueType: 'literal', defaultValueDialect: 'duckdb', beginSnapshot: 6, containsNull: 'No', containsNan: '-', minValue: 'o_001', maxValue: 'o_999', extraStats: '' }],
        files: [{ id: 2, path: 'model/orders/file.parquet', format: 'parquet', recordCount: 20, recordCountLabel: '20', sizeBytes: 12288, sizeLabel: '12 KiB', beginSnapshot: 6, endSnapshot: 0 }],
        history: [{ snapshotId: 6, time: '2026-07-03T10:00:00Z', schemaVersion: 1, source: 'table,data_file', changes: 'tables_inserted_into', author: 'tester', message: 'materialize orders', extraInfo: '{}' }],
      }
      const storage = {
        summary: {
          catalogPath: '/tmp/libredash/libredash.db',
          dataPath: '/tmp/libredash/data',
          catalogSizeLabel: '32 KiB',
          dataSizeLabel: '24 KiB',
          totalSizeLabel: '56 KiB',
          totalDataSizeLabel: '24 KiB',
          databaseCount: 1,
          tableCount: 2,
          snapshotCount: 1,
          dataFileCount: 2,
        },
        status: '',
        warnings: [],
        selectedKey: customers.key,
        tables: [customers, orders],
        snapshots: [{ id: 6, time: '2026-07-03T10:00:00Z', schemaVersion: 1, author: 'tester', message: 'materialize', changes: 'tables_inserted_into', extraInfo: '{}', protected: true, servingStateCount: 1 }],
        servingStates: [{ workspaceId: 'olist', environment: 'dev', servingStateId: 'state_1', status: 'active', snapshotId: 6, digest: 'digest', active: true, activatedAt: 'now' }],
        selectedTable: customers,
      }
      element.storage = storage
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
      const tabText = (tab: Element | null) => tab?.textContent?.replace(/\s+/g, ' ').trim()
      const defaultTabLabels = Array.from(root.querySelectorAll('.storage-tab')).map((tab) => tabText(tab))
      const activeTabBefore = tabText(root.querySelector('.storage-tab.is-active'))
      const schemaDetail = detailText()
      const filesTab = Array.from(root.querySelectorAll<HTMLButtonElement>('.storage-tab')).find((button) => button.textContent?.includes('Data files'))!
      filesTab.click()
      await element.updateComplete
      const filesDetail = detailText()
      const historyTab = Array.from(root.querySelectorAll<HTMLButtonElement>('.storage-tab')).find((button) => button.textContent?.includes('History'))!
      historyTab.click()
      await element.updateComplete
      const historyDetail = detailText()
      const afterOrders = {
        selectedNames: selectedNames(),
        tableSizes: tableSizes(),
        detail: detailText(),
        defaultTabLabels,
        activeTabBefore,
        schemaDetail,
        filesDetail,
        historyDetail,
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
      const catalogTabs = Array.from(root.querySelectorAll('.storage-tab')).map((tab) => tabText(tab))
      const catalogActiveTab = tabText(root.querySelector('.storage-tab.is-active'))
      const catalogDefaultDetail = detailText()
      const catalogServingStatesTab = Array.from(root.querySelectorAll<HTMLButtonElement>('.storage-tab')).find((button) => button.textContent?.includes('Serving states'))!
      catalogServingStatesTab.click()
      await element.updateComplete
      const catalogServingStatesDetail = detailText()
      const catalogSnapshotsTab = Array.from(root.querySelectorAll<HTMLButtonElement>('.storage-tab')).find((button) => button.textContent?.includes('Snapshots'))!
      catalogSnapshotsTab.click()
      await element.updateComplete
      const catalogSnapshotsDetail = detailText()
      const afterBreadcrumb = {
        selectedNames: selectedNames(),
        detail: detailText(),
        schemaRows: root.querySelectorAll('ld-record-table tbody tr').length,
        schemaRowsBeforeBreadcrumb,
        catalogTabs,
        catalogActiveTab,
        catalogDefaultDetail,
        catalogServingStatesDetail,
        catalogSnapshotsDetail,
      }

      return { afterOrders, afterSchema, afterBreadcrumb }
    })

    expect(state.afterOrders.selectedNames).toHaveLength(1)
    expect(state.afterOrders.selectedNames[0]).toContain('orders')
    expect(state.afterOrders.tableSizes).toEqual(['12 KiB', '12 KiB'])
    expect(state.afterOrders.activeTabBefore).toContain('Schema')
    expect(state.afterOrders.defaultTabLabels).toEqual(['Schema 1', 'Data files 1', 'History 1'])
    expect(state.afterOrders.schemaDetail).toContain('order_id')
    expect(state.afterOrders.filesDetail).toContain('model/orders/file.parquet')
    expect(state.afterOrders.historyDetail).toContain('materialize orders')
    expect(state.afterOrders.historyDetail).toContain('tables_inserted_into')
    expect(state.afterOrders.commands).toEqual([{ databaseId: 'ducklake-catalog', schema: 'model', table: 'orders' }])

    expect(state.afterSchema.selectedNames).toHaveLength(0)
    expect(state.afterSchema.detail).toContain('Tables')
    expect(state.afterSchema.detail).toContain('customers')
    expect(state.afterSchema.detail).toContain('orders')

    expect(state.afterBreadcrumb.selectedNames).toHaveLength(0)
    expect(state.afterBreadcrumb.catalogActiveTab).toContain('Schemas')
    expect(state.afterBreadcrumb.catalogTabs).toEqual(['Schemas 1', 'Serving states 1', 'Snapshots 1'])
    expect(state.afterBreadcrumb.catalogDefaultDetail).toContain('Schemas')
    expect(state.afterBreadcrumb.catalogDefaultDetail).toContain('model')
    expect(state.afterBreadcrumb.catalogDefaultDetail).not.toContain('state_1')
    expect(state.afterBreadcrumb.catalogServingStatesDetail).toContain('state_1')
    expect(state.afterBreadcrumb.catalogSnapshotsDetail).toContain('materialize')
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
  const signals = escapeHTML(JSON.stringify({ page }))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-bg-accent: #0969da; --ld-bg-accent-muted: #ddf4ff; --ld-sidebar-bg: #f1f3f5; --ld-report-rail-bg: #ffffff; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-accent: #0969da; --ld-fg-link: #0969da; --ld-fg-success: #1a7f37; --ld-fg-warning: #9a6700; --ld-fg-danger: #d1242f; --ld-fg-on-accent: #fff; --ld-icon-muted: #57606a; --ld-line-muted: #d8dee4; --ld-border-width: 1px; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-radius-default: 6px; --ld-radius-full: 999px; --ld-page-content-max-width: 72rem; --ld-workspace-detail-max-width: 72rem; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-12: 12px; --base-size-16: 16px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-body-md: 16px; --ld-font-size-title-sm: 18px; --ld-font-size-title-md: 22px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; --ld-line-height-normal: 1.5; }
          ld-admin-page { min-height: 720px; }
        </style>
      </head>
      <body>
        <main data-signals="${signals}">
          <ld-admin-page></ld-admin-page>
        </main>
        <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script>
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
