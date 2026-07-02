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

test('admin navigation remains pinned while content scrolls on desktop', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-admin-page') && customElements.get('ld-sub-sidebar') && customElements.get('ld-record-table'))

    const state = await page.evaluate(async () => {
      const element = document.createElement('ld-admin-page') as any
      element.style.minHeight = '1600px'
      element.page = {
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
      element.page = {
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
          csrfToken: 'csrf-token',
          updatePath: '/api/v1/admin/agent/config',
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
      element.agentPrompt = 'Signal prompt'
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
      element.page = {
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
          csrfToken: '',
          updatePath: '/api/v1/admin/agent/config',
          tools: [],
        },
        sections: [],
      }
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
    expect(state.detailMeta).toEqual(['6 required', 'dashboardId, filters.dateRange.start, filters.dateRange.end +10'])
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
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-bg-accent: #0969da; --ld-bg-accent-muted: #ddf4ff; --ld-sidebar-bg: #f1f3f5; --ld-report-rail-bg: #ffffff; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-accent: #0969da; --ld-fg-link: #0969da; --ld-fg-on-accent: #fff; --ld-icon-muted: #57606a; --ld-line-muted: #d8dee4; --ld-border-width: 1px; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-radius-default: 6px; --ld-radius-full: 999px; --ld-page-content-max-width: 72rem; --ld-workspace-detail-max-width: 72rem; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-12: 12px; --base-size-16: 16px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-body-md: 16px; --ld-font-size-title-sm: 18px; --ld-font-size-title-md: 22px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.2; --ld-line-height-compact: 1.3; --ld-line-height-normal: 1.5; }
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
