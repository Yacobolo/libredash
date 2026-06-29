import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/workspace-page-test')

test.before(async () => {
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

test.after(async () => {
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
          && customElements.get('ld-data-grid')
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
        const workspaceGlyph = workspace.shadowRoot.querySelector('.asset-glyph') as HTMLElement
        const workspaceRowActionIcon = workspace.shadowRoot.querySelector('.row-actions svg') as SVGElement
        const workspaceRowActionLink = workspace.shadowRoot.querySelector('.row-actions .icon-link') as HTMLElement
        const workspaceNameCell = workspace.shadowRoot.querySelector('tbody tr:first-child .name-col') as HTMLElement
        const workspaceTypeCell = workspace.shadowRoot.querySelector('tbody tr:first-child .type-col') as HTMLElement
        const workspaceAssetTitle = workspace.shadowRoot.querySelector('tbody tr:first-child .asset-title') as HTMLElement
        const workspaceAssetDescription = workspace.shadowRoot.querySelector('tbody tr:first-child .name-col p') as HTMLElement
        const connectionsPage = connections.shadowRoot.querySelector('.page') as HTMLElement
        const assetHeader = asset.shadowRoot.querySelector('.breadcrumb-header') as HTMLElement
        const assetTabs = asset.shadowRoot.querySelector('.asset-body > .tabs') as HTMLElement
        const assetFirstTab = asset.shadowRoot.querySelector('.asset-body > .tabs a') as HTMLElement
        const nameCellRight = workspaceNameCell.getBoundingClientRect().right
        const typeCellLeft = workspaceTypeCell.getBoundingClientRect().left
        return {
          workspaceTitle: workspace.shadowRoot.querySelector('h1')?.textContent?.trim(),
          workspaceHasAsset: Boolean(workspace.shadowRoot.querySelector('.asset-title')),
          workspaceHasAccess: Boolean(workspace.shadowRoot.querySelector('ld-workspace-access-control')),
          workspaceIsStyled: getComputedStyle(workspacePage).paddingTop !== '0px',
          workspaceToolbarDisplay: getComputedStyle(workspaceToolbar).display,
          workspaceGlyphText: workspaceGlyph.textContent?.trim(),
          workspaceGlyphBackground: getComputedStyle(workspaceGlyph).backgroundColor,
          workspaceGlyphHasIcon: Boolean(workspaceGlyph.querySelector('svg')),
          workspaceRowActionIconWidth: getComputedStyle(workspaceRowActionIcon).width,
          workspaceRowActionBorderColor: getComputedStyle(workspaceRowActionLink).borderTopColor,
          workspaceTitleFitsNameColumn: workspaceAssetTitle.getBoundingClientRect().right <= nameCellRight && workspaceAssetTitle.getBoundingClientRect().right <= typeCellLeft,
          workspaceDescriptionFitsNameColumn: workspaceAssetDescription.getBoundingClientRect().right <= nameCellRight && workspaceAssetDescription.getBoundingClientRect().right <= typeCellLeft,
          connectionsTitle: connections.shadowRoot.querySelector('h1')?.textContent?.trim(),
          connectionsHasSource: connections.shadowRoot.textContent?.includes('Orders source') ?? false,
          connectionsIsStyled: getComputedStyle(connectionsPage).paddingTop !== '0px',
          assetTitle: asset.shadowRoot.querySelector('h1 span:last-child')?.textContent?.trim(),
          assetHasOverview: asset.shadowRoot.textContent?.includes('Overview') ?? false,
          assetHasGrid: Boolean(asset.shadowRoot.querySelector('ld-data-grid')),
          assetHeaderDisplay: getComputedStyle(assetHeader).display,
          assetTabsPaddingLeft: getComputedStyle(assetTabs).paddingLeft,
          assetFirstTabInset: Math.round(assetFirstTab.getBoundingClientRect().left - assetTabs.getBoundingClientRect().left),
        }
      })

      assert.deepEqual(state, {
        workspaceTitle: 'LibreDash Workspace',
        workspaceHasAsset: true,
        workspaceHasAccess: true,
        workspaceIsStyled: true,
        workspaceToolbarDisplay: 'grid',
        workspaceGlyphText: '',
        workspaceGlyphBackground: 'rgb(221, 244, 255)',
        workspaceGlyphHasIcon: true,
        workspaceRowActionIconWidth: '16px',
        workspaceRowActionBorderColor: 'rgba(0, 0, 0, 0)',
        workspaceTitleFitsNameColumn: true,
        workspaceDescriptionFitsNameColumn: true,
        connectionsTitle: 'Connections',
        connectionsHasSource: true,
        connectionsIsStyled: true,
        assetTitle: 'Olist Commerce',
        assetHasOverview: true,
        assetHasGrid: true,
        assetHeaderDisplay: 'grid',
        assetTabsPaddingLeft: '16px',
        assetFirstTabInset: 16,
      })
    } finally {
      await page.close()
    }
  })
}

test('workspace access modal normalizes Go-shaped access signals', async () => {
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
      const dialog = accessControl.shadowRoot.querySelector('[role="dialog"]')
      const roleOptions = Array.from(accessControl.shadowRoot.querySelectorAll('.composer-role option')).map((option) => ({
        value: (option as HTMLOptionElement).value,
        label: option.textContent?.trim(),
      }))
      const rowRole = accessControl.shadowRoot.querySelector('.row select') as HTMLSelectElement | null
      return {
        hasDialog: Boolean(dialog),
        title: accessControl.shadowRoot.querySelector('.subtitle')?.textContent?.trim(),
        roleOptions,
        rowRoleValue: rowRole?.value,
        principal: accessControl.shadowRoot.querySelector('.name')?.textContent?.trim(),
      }
    })

    assert.deepEqual(state, {
      hasDialog: true,
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

test('workspace asset refresh page renders refresh tab and emits refresh events', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-workspace-asset-page') && customElements.get('ld-data-grid'))

    const state = await page.evaluate(async () => {
      const asset = document.querySelector('ld-workspace-asset-page') as any
      let refreshEvents = 0
      asset.addEventListener('ld-refresh-materializations', () => { refreshEvents += 1 })
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
          { label: 'Refresh materializations', icon: 'refresh', command: 'refresh-materializations' },
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
          runsGrid: {
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
      const button = asset.shadowRoot.querySelector('button[aria-label="Refresh materializations"]') as HTMLButtonElement
      button.click()
      return {
        activeTab: asset.shadowRoot.querySelector('.tabs a.active')?.textContent?.trim(),
        hasRefreshButton: Boolean(button),
        gridText: asset.shadowRoot.querySelector('ld-data-grid')?.textContent,
        refreshEvents,
      }
    })

    assert.equal(state.activeTab, 'Refreshes')
    assert.equal(state.hasRefreshButton, true)
    assert.match(state.gridText ?? '', /matrun_123/)
    assert.equal(state.refreshEvents, 1)
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
    assets: [{
      id: 'semantic_model:olist',
      title: 'Executive Sales Dashboard',
      description: 'Sales, order, category, and delivery overview with deliberately long text for table fitting.',
      type: 'semantic_model',
      typeLabel: 'Semantic model',
      key: 'olist',
      parentTitle: '-',
      detailHref: '/workspaces/libredash/assets/semantic_model:olist/details',
      openHref: '/workspaces/libredash/assets/semantic_model:olist/details',
    }],
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
      sections: [{
        title: 'Model tables (1)',
        grid: {
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
    command: { email: '', role: '', principalId: '' },
    search: '',
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-accent: #0969da; --ld-accent-fg: #fff; --ld-line-muted: #d8dee4; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-border-transparent: 1px solid transparent; --ld-radius-default: 6px; --ld-radius-tight: 4px; --ld-radius-full: 999px; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --control-medium-size: 32px; --control-xlarge-size: 40px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-title-sm: 16px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; --ld-asset-semantic-model-bg: #ddf4ff; --ld-asset-semantic-model-accent: #0969da; --ld-asset-semantic-model-border: #b6e3ff; --z-index-inspector: 1000; --ld-modal-backdrop: rgb(0 0 0 / .28); }
          ld-workspace-page, ld-connections-page, ld-workspace-asset-page { display: block; min-height: 720px; }
        </style>
      </head>
      <body>
        <ld-workspace-page page="${attr(workspacePage)}" workspaceaccess="${attr(access)}"></ld-workspace-page>
        <ld-connections-page page="${attr(connectionsPage)}"></ld-connections-page>
        <ld-workspace-asset-page page="${attr(assetPage)}"></ld-workspace-asset-page>
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
