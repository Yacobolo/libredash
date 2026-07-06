import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/workspace-page-test')

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
  { name: 'desktop', width: 1280, height: 820 },
  { name: 'mobile', width: 390, height: 820 },
]) {
  test(`workspace route roots compose UI on ${viewport.name}`, async () => {
    const page = await browser.newPage({ viewport })
    try {
      await page.goto(baseURL)
      await page.waitForFunction(() => (
        customElements.get('ld-workspace-page')
          && customElements.get('ld-connections-page')
          && customElements.get('ld-workspace-asset-page')
          && customElements.get('ld-record-table')
      ))
      await page.locator('ld-workspace-page').evaluate((element: any) => element.updateComplete)
      await page.locator('ld-connections-page').evaluate((element: any) => element.updateComplete)
      await page.locator('ld-workspace-asset-page').evaluate((element: any) => element.updateComplete)

      const state = await page.evaluate(() => {
        const workspace = document.querySelector('ld-workspace-page') as any
        const connections = document.querySelector('ld-connections-page') as any
        const asset = document.querySelector('ld-workspace-asset-page') as any
        const workspacePage = workspace.shadowRoot.querySelector('.page') as HTMLElement
        const workspaceToolbar = workspace.shadowRoot.querySelector('.toolbar') as HTMLElement
        const workspaceRecordTable = workspace.shadowRoot.querySelector('ld-record-table') as HTMLElement
        const workspaceGlyph = workspace.shadowRoot.querySelector('.record-entity-icon') as HTMLElement
        const workspaceDashboardGlyph = workspace.shadowRoot.querySelector('.record-icon-dashboard') as HTMLElement
        const workspaceRowActionIcon = workspace.shadowRoot.querySelector('.record-actions svg') as SVGElement
        const workspaceRowActionLink = workspace.shadowRoot.querySelector('.record-actions .record-icon-action') as HTMLElement
        const workspaceNameCell = workspace.shadowRoot.querySelector('tbody tr:first-child td:first-child') as HTMLElement
        const workspaceTypeCell = workspace.shadowRoot.querySelector('tbody tr:first-child td:nth-child(2)') as HTMLElement
        const workspaceAssetTitle = workspace.shadowRoot.querySelector('tbody tr:first-child .record-entity-label') as HTMLElement
        const workspaceAssetDescription = workspace.shadowRoot.querySelector('tbody tr:first-child .record-entity-description') as HTMLElement
        const connectionsPage = connections.shadowRoot.querySelector('.page') as HTMLElement
        const assetHeader = asset.shadowRoot.querySelector('.breadcrumb-header') as HTMLElement
        const assetTabs = asset.shadowRoot.querySelector('.asset-body > .tabs') as HTMLElement
        const assetFirstTab = asset.shadowRoot.querySelector('.asset-body > .tabs a') as HTMLElement
        const assetSectionBody = asset.shadowRoot.querySelector('.section-body') as HTMLElement
        const semanticGraph = asset.shadowRoot.querySelector('ld-semantic-model-graph') as HTMLElement
        const firstRecordTable = asset.shadowRoot.querySelector('ld-record-table') as HTMLElement
        const semanticGraphSection = asset.shadowRoot.querySelector('.semantic-model-section') as HTMLElement
        const nameCellRight = workspaceNameCell.getBoundingClientRect().right
        const typeCellLeft = workspaceTypeCell.getBoundingClientRect().left
        const workspacePageRect = workspacePage.getBoundingClientRect()
        const connectionsPageRect = connectionsPage.getBoundingClientRect()
        const isMobile = window.innerWidth <= 720
        return {
          workspaceTitle: workspace.shadowRoot.querySelector('h1')?.textContent?.trim(),
          workspaceHasAsset: Boolean(workspaceRecordTable && workspace.shadowRoot.querySelector('.record-entity-label')),
          workspaceTableVariant: workspaceRecordTable.getAttribute('variant'),
          workspaceTableHeaders: Array.from(workspaceRecordTable.querySelectorAll('thead th button span:first-child')).map((header) => header.textContent?.trim()),
          workspaceTableHeaderBackground: getComputedStyle(workspaceRecordTable.querySelector('thead th') as HTMLElement).backgroundColor,
          workspaceHasAccess: Boolean(workspace.shadowRoot.querySelector('ld-workspace-access-control')),
          workspaceIsStyled: getComputedStyle(workspacePage).paddingTop !== '0px',
          workspacePageCentered: isMobile || Math.abs((workspacePageRect.left + workspacePageRect.width / 2) - window.innerWidth / 2) <= 1,
          workspacePageConstrained: isMobile || Math.round(workspacePageRect.width) < window.innerWidth,
          workspaceToolbarDisplay: getComputedStyle(workspaceToolbar).display,
          workspaceGlyphText: workspaceGlyph.textContent?.trim(),
          workspaceGlyphBackground: getComputedStyle(workspaceGlyph).backgroundColor,
          workspaceGlyphHasIcon: Boolean(workspaceGlyph.querySelector('svg')),
          workspaceDashboardGlyphBorderColor: getComputedStyle(workspaceDashboardGlyph).borderTopColor,
          workspaceRowActionIconWidth: getComputedStyle(workspaceRowActionIcon).width,
          workspaceRowActionBorderColor: getComputedStyle(workspaceRowActionLink).borderTopColor,
          workspaceTitleFitsNameColumn: workspaceAssetTitle.getBoundingClientRect().right <= nameCellRight,
          workspaceDescriptionFitsNameColumn: workspaceAssetDescription.getBoundingClientRect().right <= nameCellRight,
          connectionsTitle: connections.shadowRoot.querySelector('h1')?.textContent?.trim(),
          connectionsHasSource: connections.shadowRoot.textContent?.includes('Orders source') ?? false,
          connectionsIsStyled: getComputedStyle(connectionsPage).paddingTop !== '0px',
          connectionsPageCentered: isMobile || Math.abs((connectionsPageRect.left + connectionsPageRect.width / 2) - window.innerWidth / 2) <= 1,
          connectionsPageConstrained: isMobile || Math.round(connectionsPageRect.width) < window.innerWidth,
          assetTitle: asset.shadowRoot.querySelector('h1 span:last-child')?.textContent?.trim(),
          assetHasOverview: asset.shadowRoot.textContent?.includes('Overview') ?? false,
          assetHasRecordTable: Boolean(asset.shadowRoot.querySelector('ld-record-table')),
          assetHasSemanticGraph: Boolean(semanticGraph),
          assetHasAccess: Boolean(asset.shadowRoot.querySelector('ld-workspace-access-control')),
          assetSemanticGraphBeforeRecordTable: Boolean(semanticGraph && firstRecordTable && semanticGraph.compareDocumentPosition(firstRecordTable) & Node.DOCUMENT_POSITION_FOLLOWING),
          assetHasDataModelHeading: Array.from(asset.shadowRoot.querySelectorAll('h2')).some((heading) => heading.textContent?.trim() === 'Data model'),
          assetGraphFlushLeft: semanticGraphSection ? Math.round(semanticGraphSection.getBoundingClientRect().left - assetSectionBody.getBoundingClientRect().left) : -1,
          assetHeaderDisplay: getComputedStyle(assetHeader).display,
          assetTabsPaddingLeft: getComputedStyle(assetTabs).paddingLeft,
          assetFirstTabInset: Math.round(assetFirstTab.getBoundingClientRect().left - assetTabs.getBoundingClientRect().left),
        }
      })

      expect(state).toEqual({
        workspaceTitle: 'LibreDash Workspace',
        workspaceHasAsset: true,
        workspaceTableVariant: 'primary',
        workspaceTableHeaders: ['Name', 'Type', 'Key', 'Actions'],
        workspaceTableHeaderBackground: 'rgb(246, 248, 250)',
        workspaceHasAccess: true,
        workspaceIsStyled: true,
        workspacePageCentered: true,
        workspacePageConstrained: true,
        workspaceToolbarDisplay: 'grid',
        workspaceGlyphText: '',
        workspaceGlyphBackground: 'rgb(221, 244, 255)',
        workspaceGlyphHasIcon: true,
        workspaceDashboardGlyphBorderColor: 'rgb(210, 191, 255)',
        workspaceRowActionIconWidth: '16px',
        workspaceRowActionBorderColor: 'rgba(0, 0, 0, 0)',
        workspaceTitleFitsNameColumn: true,
        workspaceDescriptionFitsNameColumn: true,
        connectionsTitle: 'Connections',
        connectionsHasSource: true,
        connectionsIsStyled: true,
        connectionsPageCentered: true,
        connectionsPageConstrained: true,
        assetTitle: 'Olist Commerce',
        assetHasOverview: true,
        assetHasRecordTable: true,
        assetHasSemanticGraph: true,
        assetHasAccess: true,
        assetSemanticGraphBeforeRecordTable: true,
        assetHasDataModelHeading: false,
        assetGraphFlushLeft: 0,
        assetHeaderDisplay: 'grid',
        assetTabsPaddingLeft: '16px',
        assetFirstTabInset: 16,
      })
    } finally {
      await page.close()
    }
  })
}

test('workspace catalog cards keep Open links visible with long descriptions', async () => {
  const page = await browser.newPage({ viewport: { width: 1420, height: 1155 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-page'))
    await page.locator('ld-workspace-page').evaluate((element: any) => {
      element.page = {
        kind: 'workspace',
        title: 'Workspaces',
        description: 'View published BI workspaces.',
        cards: [
          { id: 'operations', title: 'Operations Workspace', description: 'Fulfillment and delivery analysis.', href: '/workspaces/operations', servingLabel: 'Serving' },
          { id: 'sales', title: 'Sales Workspace', description: 'Revenue, orders, and product category analysis.', href: '/workspaces/sales', servingLabel: 'Serving' },
          { id: 'visuals', title: 'Visuals Workspace', description: 'Developer QA workspace for exhaustive dashboard visual and table renderer coverage.', href: '/workspaces/visuals', servingLabel: 'Serving' },
        ],
      }
    })
    await page.locator('ld-workspace-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-workspace-page').evaluate((element: any) => {
      const cards = Array.from(element.shadowRoot.querySelectorAll('article.card')) as HTMLElement[]
      const visualCard = cards[2]
      const open = visualCard.querySelector('a.primary-link') as HTMLAnchorElement
      const cardRect = visualCard.getBoundingClientRect()
      const openRect = open.getBoundingClientRect()
      return {
        href: open.getAttribute('href'),
        text: open.textContent?.trim(),
        display: getComputedStyle(open).display,
        visibleWithinCard: openRect.bottom <= cardRect.bottom && openRect.top >= cardRect.top,
      }
    })

    expect(state).toEqual({
      href: '/workspaces/visuals',
      text: 'Open',
      display: 'grid',
      visibleWithinCard: true,
    })
  } finally {
    await page.close()
  }
})

test('workspace asset search filters the current asset rows', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-page'))
    await page.locator('ld-workspace-page').evaluate((element: any) => element.updateComplete)

    const state = await page.evaluate(async () => {
      const workspace = document.querySelector('ld-workspace-page') as any
      const root = workspace.shadowRoot
      const input = root.querySelector('.toolbar .search input[type="search"]') as HTMLInputElement
      const form = root.querySelector('.toolbar .search') as HTMLFormElement
      const before = Array.from(root.querySelectorAll('.record-entity-label')).map((link) => link.textContent?.trim())
      input.value = 'customer'
      input.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
      await workspace.updateComplete
      input.focus()
      const focusedStyle = getComputedStyle(input)
      const after = Array.from(root.querySelectorAll('.record-entity-label')).map((link) => link.textContent?.trim())
      return {
        before,
        after,
        focusedBorderColor: focusedStyle.borderTopColor,
        focusedOutlineStyle: focusedStyle.outlineStyle,
        hasSubmitButton: Boolean(root.querySelector('.toolbar .search button[type="submit"]')),
        formAction: form.getAttribute('action'),
        inputAutocomplete: input.getAttribute('autocomplete'),
      }
    })

    expect(state.before).toEqual(['Executive Sales Dashboard', 'Customer Segments'])
    expect(state.after).toEqual(['Customer Segments'])
    expect(state.focusedBorderColor).toBe('rgb(9, 105, 218)')
    expect(state.focusedOutlineStyle).toBe('solid')
    expect(state.hasSubmitButton).toBe(false)
    expect(state.formAction).toBeNull()
    expect(state.inputAutocomplete).toBe('off')
  } finally {
    await page.close()
  }
})

test('workspace access drawer normalizes Go-shaped access signals', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-page'))
    await page.locator('ld-workspace-page').evaluate((element: any) => element.updateComplete)

    const state = await page.evaluate(async () => {
      const workspace = document.querySelector('ld-workspace-page') as any
      const accessControl = workspace.shadowRoot.querySelector('ld-workspace-access-control') as any
      accessControl.shadowRoot.querySelector('.trigger').click()
      await accessControl.updateComplete
      const drawer = accessControl.shadowRoot.querySelector('ld-drawer') as any
      await drawer.updateComplete
      const dialog = drawer.shadowRoot.querySelector('[role="dialog"]')
      const roleOptions = Array.from(accessControl.shadowRoot.querySelectorAll('.composer-grant-role option')).map((option) => ({
        value: (option as HTMLOptionElement).value,
        label: option.textContent?.trim(),
      }))
      const rowRole = accessControl.shadowRoot.querySelector('.row select') as HTMLSelectElement | null
      return {
        hasDialog: Boolean(dialog),
        hasDrawer: Boolean(drawer),
        title: accessControl.shadowRoot.querySelector('.subtitle')?.textContent?.trim(),
        roleOptions,
        rowRoleValue: rowRole?.value,
        principal: accessControl.shadowRoot.querySelector('.name')?.textContent?.trim(),
      }
    })

    expect(state).toEqual({
      hasDialog: true,
      hasDrawer: true,
      title: 'LibreDash Workspace roles apply to every published asset in this workspace.',
      roleOptions: [
        { value: 'viewer', label: 'Viewer' },
        { value: 'workspace_admin', label: 'Workspace Admin' },
      ],
      rowRoleValue: 'viewer',
      principal: 'analyst@example.com',
    })
  } finally {
    await page.close()
  }
})

test('asset access drawer renders object grants and privilege labels', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-asset-page'))
    await page.locator('ld-workspace-asset-page').evaluate((element: any) => element.updateComplete)

    const state = await page.evaluate(async () => {
      const asset = document.querySelector('ld-workspace-asset-page') as any
      const accessControl = asset.shadowRoot.querySelector('ld-workspace-access-control') as any
      accessControl.shadowRoot.querySelector('.trigger').click()
      await accessControl.updateComplete
      const drawer = accessControl.shadowRoot.querySelector('ld-drawer') as any
      await drawer.updateComplete
      const roleOptions = Array.from(accessControl.shadowRoot.querySelectorAll('.composer-grant-role option')).map((option) => ({
        value: (option as HTMLOptionElement).value,
        label: option.textContent?.trim(),
      }))
      return {
        hasDialog: Boolean(drawer.shadowRoot.querySelector('[role="dialog"]')),
        subtitle: accessControl.shadowRoot.querySelector('.subtitle')?.textContent?.trim(),
        sectionTitle: accessControl.shadowRoot.querySelector('.section-title')?.textContent?.trim(),
        roleOptions,
        rowRoleValue: (accessControl.shadowRoot.querySelector('.row select') as HTMLSelectElement | null)?.value,
        principal: accessControl.shadowRoot.querySelector('.name')?.textContent?.trim(),
      }
    })

    expect(state).toEqual({
      hasDialog: true,
      subtitle: 'Olist Commerce grants apply only to this asset.',
      sectionTitle: 'Direct grants',
      roleOptions: [
        { value: 'VIEW_ITEM', label: 'VIEW ITEM' },
        { value: 'QUERY_DATA', label: 'QUERY DATA' },
        { value: 'MANAGE_GRANTS', label: 'MANAGE GRANTS' },
      ],
      rowRoleValue: 'VIEW_ITEM',
      principal: 'analyst@example.com',
    })
  } finally {
    await page.close()
  }
})

test('workspace asset refresh page renders refresh tab and emits refresh events', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-asset-page') && customElements.get('ld-record-table'))

    const state = await page.evaluate(async () => {
      const asset = document.querySelector('ld-workspace-asset-page') as any
      let refreshEvents = 0
      asset.addEventListener('ld-refresh-asset', () => { refreshEvents += 1 })
      asset.page = {
        kind: 'workspace_asset',
        title: 'Olist Commerce',
        workspaceId: 'libredash',
        assetId: 'semantic_model:olist',
        activeSection: 'refreshes',
        asset: {
          id: 'semantic_model:olist',
          title: 'Olist Commerce',
          description: 'Brazilian ecommerce model.',
          type: 'semantic_model',
          typeLabel: 'Semantic model',
          key: 'olist',
          detailHref: '/workspaces/libredash/assets/semantic_model:olist/details',
          openHref: '/workspaces/libredash/assets/semantic_model:olist/details',
        },
        breadcrumbs: [
          { label: 'Workspaces', href: '/workspaces' },
          { label: 'LibreDash Workspace', href: '/workspaces/libredash' },
          { label: 'Olist Commerce', current: true },
        ],
        actions: [
          { label: 'Refresh data', icon: 'refresh', command: 'refresh' },
          { label: 'Back to workspace', href: '/workspaces/libredash', icon: 'back' },
        ],
        tabs: [
          { id: 'details', label: 'Details', href: '/workspaces/libredash/assets/semantic_model:olist/details', active: false },
          { id: 'refreshes', label: 'Refreshes', href: '/workspaces/libredash/assets/semantic_model:olist/refreshes', active: true },
          { id: 'lineage', label: 'Lineage', href: '/workspaces/libredash/assets/semantic_model:olist/lineage', active: false, count: 1 },
        ],
        refresh: {
          status: 'succeeded',
          running: false,
          lastSuccessful: '2026-06-26 10:00:12',
          runsTable: {
            columns: [
              { id: 'status', header: 'Status', kind: 'status' },
              { id: 'started', header: 'Started' },
              { id: 'run', header: 'Run ID', kind: 'code' },
            ],
            rows: [{ status: { label: 'succeeded', tone: 'success' }, started: '2026-06-26 10:00:00', run: 'matrun_123' }],
            empty: 'No refresh runs.',
          },
        },
      }
      await asset.updateComplete
      const button = asset.shadowRoot.querySelector('button[aria-label="Refresh data"]') as HTMLButtonElement
      button.click()
      return {
        activeTab: asset.shadowRoot.querySelector('.tabs a.active')?.textContent?.trim(),
        hasRefreshButton: Boolean(button),
        recordTableText: asset.shadowRoot.querySelector('ld-record-table')?.textContent,
        refreshEvents,
      }
    })

    expect(state.activeTab).toBe('Refreshes')
    expect(state.hasRefreshButton).toBe(true)
    expect(state.recordTableText ?? '').toMatch(/matrun_123/)
    expect(state.refreshEvents).toBe(1)
  } finally {
    await page.close()
  }
})

test('workspace asset page renders versions as config history', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-asset-page') && customElements.get('ld-record-table'))

    const state = await page.evaluate(async () => {
      const asset = document.querySelector('ld-workspace-asset-page') as any
      asset.page = {
        kind: 'workspace_asset',
        title: 'Executive Sales Dashboard',
        workspaceId: 'libredash',
        assetId: 'dashboard:executive-sales',
        activeSection: 'versions',
        asset: {
          id: 'dashboard:executive-sales',
          title: 'Executive Sales Dashboard',
          type: 'dashboard',
          typeLabel: 'Dashboard',
          key: 'executive-sales',
          detailHref: '/workspaces/libredash/assets/dashboard:executive-sales/details',
          openHref: '/dashboards/executive-sales',
        },
        breadcrumbs: [
          { label: 'Workspaces', href: '/workspaces' },
          { label: 'LibreDash Workspace', href: '/workspaces/libredash' },
          { label: 'Executive Sales Dashboard', current: true },
        ],
        actions: [],
        tabs: [
          { id: 'details', label: 'Details', href: '/workspaces/libredash/assets/dashboard:executive-sales/details', active: false },
          { id: 'lineage', label: 'Lineage', href: '/workspaces/libredash/assets/dashboard:executive-sales/lineage', active: false, count: 1 },
          { id: 'versions', label: 'Versions', href: '/workspaces/libredash/assets/dashboard:executive-sales/versions', active: true },
        ],
        versions: {
          currentContentHash: 'hash-current',
          table: {
            columns: [
              { id: 'version', header: 'Version', kind: 'code' },
              { id: 'published', header: 'Published' },
              { id: 'status', header: 'Status', kind: 'badge' },
              { id: 'config_hash', header: 'Config hash', kind: 'code' },
              { id: 'source_file', header: 'Source file', kind: 'code' },
              { id: 'published_by', header: 'Published by' },
            ],
            rows: [
              {
                version: 'hash-cur',
                published: '2026-07-05',
                status: { label: 'current', tone: 'success' },
                config_hash: 'hash-cur',
                source_file: 'dashboards/sales.yaml',
                published_by: 'Local Developer',
              },
            ],
            empty: 'No config versions recorded for this asset yet.',
          },
        },
        details: {
          overview: [
            { label: 'Type', value: 'Dashboard' },
          ],
          sections: [],
        },
      }
      await asset.updateComplete
      const table = asset.shadowRoot.querySelector('ld-record-table') as HTMLElement | null
      return {
        tabText: asset.shadowRoot.querySelector('.tabs')?.textContent ?? '',
        sectionTitle: asset.shadowRoot.querySelector('.detail-section h2')?.textContent?.trim(),
        tableText: table?.textContent ?? '',
        bodyText: asset.shadowRoot.textContent ?? '',
      }
    })

    expect(state.tabText).toMatch(/Versions/)
    expect(state.sectionTitle).toBe('Versions')
    expect(state.tableText).toMatch(/Config hash/)
    expect(state.tableText).toMatch(/Source file/)
    expect(state.tableText).toMatch(/Published by/)
    expect(state.bodyText).not.toMatch(/Deployment digest/)
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  const assetList = {
    workspaceId: 'libredash',
    searchHref: '/workspaces/libredash',
    tabs: [
      { id: '', label: 'All', href: '/workspaces/libredash', active: true },
      { id: 'dashboard', label: 'Dashboard', href: '/workspaces/libredash?type=dashboard', active: false },
    ],
    assets: [
      {
        id: 'semantic_model:olist',
        title: 'Executive Sales Dashboard',
        description: 'Sales, order, category, and delivery overview with deliberately long text for table fitting.',
        type: 'semantic_model',
        typeLabel: 'Semantic model',
        key: 'olist',
        parentTitle: '-',
        detailHref: '/workspaces/libredash/assets/semantic_model:olist/details',
        openHref: '/workspaces/libredash/assets/semantic_model:olist/details',
      },
      {
        id: 'dashboard:customers',
        title: 'Customer Segments',
        description: 'Customer cohort report.',
        type: 'dashboard',
        typeLabel: 'Dashboard',
        key: 'customers',
        parentTitle: '-',
        detailHref: '/workspaces/libredash/assets/dashboard:customers/details',
        openHref: '/dashboards/customers',
      },
    ],
    empty: 'No assets match this view.',
  }
  const workspacePage = {
    kind: 'workspace',
    title: 'LibreDash Workspace',
    description: 'Published BI assets.',
    workspaceId: 'libredash',
    assetList,
  }
  const connectionsPage = {
    kind: 'connections',
    title: 'Connections',
    description: 'Connection-scoped data assets.',
    workspaceId: 'libredash',
    assetList: {
      ...assetList,
      searchHref: '/connections',
      assets: [{ ...assetList.assets[0], title: 'Orders source', type: 'source', typeLabel: 'Source', detailHref: '/connections/connection:olist/sources/source:orders/details' }],
    },
  }
  const assetPage = {
    kind: 'workspace_asset',
    title: 'Olist Commerce',
    workspaceId: 'libredash',
    assetId: 'semantic_model:olist',
    activeSection: 'details',
    asset: assetList.assets[0],
    breadcrumbs: [
      { label: 'Workspaces', href: '/workspaces' },
      { label: 'LibreDash Workspace', href: '/workspaces/libredash' },
      { label: 'Olist Commerce', current: true },
    ],
    actions: [],
    tabs: [
      { id: 'details', label: 'Details', href: '/workspaces/libredash/assets/semantic_model:olist/details', active: true },
      { id: 'lineage', label: 'Lineage', href: '/workspaces/libredash/assets/semantic_model:olist/lineage', active: false, count: 1 },
    ],
    details: {
      overview: [
        { label: 'Type', value: 'Semantic model' },
        { label: 'Key', value: 'olist', code: true },
      ],
      semanticModelGraph: {
        baseTable: 'orders',
        nodes: [{
          id: 'orders',
          title: 'orders',
          primaryKey: 'order_id',
          fields: [
            { name: 'order_id', label: 'Order ID', primaryKey: true },
            { name: 'customer_id', label: 'Customer ID', join: true, relationships: ['orders_customers'] },
          ],
        }, {
          id: 'customers',
          title: 'customers',
          primaryKey: 'customer_id',
          fields: [{ name: 'customer_id', label: 'Customer ID', primaryKey: true, join: true, relationships: ['orders_customers'] }],
        }],
        edges: [{
          id: 'orders_customers',
          source: 'orders',
          target: 'customers',
          sourceField: 'customer_id',
          targetField: 'customer_id',
          cardinality: 'many_to_one',
          label: '*:1',
          active: true,
        }],
      },
      sections: [{
        title: 'Model tables (1)',
        table: {
          columns: [{ id: 'name', header: 'Name', kind: 'link', hrefKey: 'nameHref' }],
          rows: [{ name: 'orders', nameHref: '/workspaces/libredash/assets/model_table:olist.orders/details' }],
          empty: 'No model tables.',
        },
      }],
    },
  }
  const access = {
    workspace: { ID: 'libredash', Title: 'LibreDash Workspace' },
    roles: [{ Name: 'viewer' }, { Name: 'workspace_admin' }],
    bindings: [{
      PrincipalID: 'principal:analyst@example.com',
      Email: 'analyst@example.com',
      DisplayName: '',
      Role: 'viewer',
    }],
    canManage: true,
    status: { loading: false, error: '', message: '' },
    csrfToken: 'token',
    command: { email: '', role: '', principalId: '', bindingId: '', subjectType: '', subjectId: '' },
    search: '',
  }
  const assetAccess = {
    workspace: { ID: 'libredash', Title: 'LibreDash Workspace' },
    objectType: 'semantic_model',
    objectId: 'olist',
    objectTitle: 'Olist Commerce',
    mode: 'object',
    roles: [{ Name: 'VIEW_ITEM' }, { Name: 'QUERY_DATA' }, { Name: 'MANAGE_GRANTS' }],
    bindings: [{
      ID: 'grant_1',
      SubjectType: 'principal',
      SubjectID: 'email_analyst',
      PrincipalID: 'email_analyst',
      Email: 'analyst@example.com',
      DisplayName: '',
      Role: 'VIEW_ITEM',
    }],
    canManage: true,
    status: { loading: false, error: '', message: '' },
    csrfToken: 'token',
    command: { email: '', role: '', principalId: '', bindingId: '', subjectType: '', subjectId: '' },
    search: '',
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-accent: #0969da; --ld-accent-fg: #fff; --ld-line-muted: #d8dee4; --ld-line-accent: #0969da; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-border-transparent: 1px solid transparent; --ld-radius-default: 6px; --ld-radius-tight: 4px; --ld-radius-full: 999px; --ld-page-content-max-width: 72rem; --ld-workspace-detail-max-width: 72rem; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --ld-space-control: 10px; --control-medium-size: 32px; --control-xlarge-size: 40px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-title-sm: 16px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; --ld-asset-dashboard-bg: #fbefff; --ld-asset-dashboard-accent: #8250df; --ld-asset-dashboard-border: #d2bfff; --ld-asset-semantic-model-bg: #ddf4ff; --ld-asset-semantic-model-accent: #0969da; --ld-asset-semantic-model-border: #b6e3ff; --z-index-inspector: 1000; --ld-modal-backdrop: rgb(0 0 0 / .28); }
          ld-workspace-page, ld-connections-page, ld-workspace-asset-page { display: block; min-height: 720px; }
        </style>
      </head>
      <body>
        <ld-workspace-page page="${attr(workspacePage)}" workspaceaccess="${attr(access)}"></ld-workspace-page>
        <ld-connections-page page="${attr(connectionsPage)}"></ld-connections-page>
        <ld-workspace-asset-page page="${attr(assetPage)}" workspaceaccess="${attr(assetAccess)}"></ld-workspace-asset-page>
        <script type="module" src="/workspace-page-under-test.js"></script>
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
