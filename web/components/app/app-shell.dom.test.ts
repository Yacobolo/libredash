import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = process.cwd()
const tmpRoot = join(root, '.tmp/app-shell-test')

test.before(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/fallback') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(false))
      return
    }
    if (url.pathname === '/upgraded-shell') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(true))
      return
    }
    if (url.pathname === '/upgraded-compact-shell') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(true, true))
      return
    }

    const fileRoot = url.pathname.startsWith('/tmp/') ? tmpRoot : root
    const path = url.pathname.startsWith('/tmp/') ? url.pathname.replace('/tmp/', '/') : url.pathname
    const file = normalize(join(fileRoot, path))
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

test.after(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
})

test('global CSS reserves app shell geometry before custom elements upgrade', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/fallback`)

    const state = await shellGeometry(page)

    assert.equal(state.shell.display, 'grid')
    assert.equal(state.shell.x, 0)
    assert.equal(state.shell.width, 1320)
    assert.equal(state.shell.height, 900)
    assert.equal(state.route.display, 'block')
    assert.equal(state.route.x, 248)
    assert.equal(state.route.width, 1072)
    assert.equal(state.route.height, 900)
  } finally {
    await page.close()
  }
})

test('app shell preserves slotted route geometry before route component upgrade', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/upgraded-shell`)
    await page.waitForFunction(() => customElements.get('ld-app-shell'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await shellGeometry(page)

    assert.equal(state.routeDefined, false)
    assert.equal(state.shell.display, 'grid')
    assert.equal(state.route.display, 'block')
    assert.equal(state.route.x, 248)
    assert.equal(state.route.width, 1072)
    assert.equal(state.route.height, 900)
  } finally {
    await page.close()
  }
})

test('upgraded compact app shell does not keep the fallback route grid column', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/upgraded-compact-shell`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await shellGeometry(page)

    assert.equal(state.routeDefined, false)
    assert.equal(state.sidebar.width, 48)
    assert.equal(state.shellMain.x, state.sidebar.right)
    assert.equal(state.route.x, state.sidebar.right)
    assert.equal(state.route.gridColumnStart, 'auto')
  } finally {
    await page.close()
  }
})

async function shellGeometry(page: any) {
  return await page.evaluate(() => {
    const shell = document.querySelector('ld-app-shell') as HTMLElement
    const route = document.querySelector('ld-workspace-page') as HTMLElement
    const sidebar = shell.shadowRoot?.querySelector('ld-sidebar') as HTMLElement
    const shellMain = shell.shadowRoot?.querySelector('main') as HTMLElement
    const box = (element?: HTMLElement | null) => {
      if (!element) return null
      const rect = element.getBoundingClientRect()
      const style = getComputedStyle(element)
      return {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
        right: Math.round(rect.right),
        display: style.display,
        gridColumnStart: style.gridColumnStart,
      }
    }
    return {
      routeDefined: Boolean(customElements.get('ld-workspace-page')),
      shell: box(shell),
      sidebar: box(sidebar),
      shellMain: box(shellMain),
      route: box(route),
    }
  })
}

function testDocument(includeShellScript: boolean, compact = false): string {
  const chrome = compact ? ` chrome="${escapeHTML(JSON.stringify({
    sidebar: {
      workspaceTitle: 'LibreDash Workspace',
      active: 'workspaces',
      dashboardId: '',
      dashboardTitle: '',
      pageTitle: '',
      modelId: '',
      modelTitle: '',
      compact: true,
      groups: [],
    },
  }))}"` : ''
  return `
    <!doctype html>
    <html>
      <head>
        <link rel="stylesheet" href="/static/app.css">
      </head>
      <body>
        <main class="min-h-svh bg-app text-fg-default">
          <ld-app-shell${chrome}>
            <ld-workspace-page slot="page"></ld-workspace-page>
          </ld-app-shell>
        </main>
        ${includeShellScript ? '<script type="module" src="/tmp/app-shell-under-test.js"></script>' : ''}
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
