import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/catalog-page-test')

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

test('catalog page composes dashboard cards', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-catalog-page'))
    await page.locator('ld-catalog-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-catalog-page').evaluate((element: any) => {
      const root = element.shadowRoot
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        card: root.querySelector('article h2')?.textContent?.trim(),
        href: root.querySelector('article a')?.getAttribute('href'),
        pages: root.querySelector('article footer span')?.textContent?.trim(),
      }
    })

    assert.deepEqual(state, {
      title: 'Dashboards',
      card: 'Executive Sales Dashboard',
      href: '/dashboards/executive-sales',
      pages: '2 pages',
    })
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  const page = {
    kind: 'catalog',
    title: 'Dashboards',
    description: 'Reports backed by semantic models.',
    dashboards: [{
      id: 'executive-sales',
      title: 'Executive Sales Dashboard',
      description: 'Fixture report',
      semanticModel: 'olist',
      pageCount: 2,
      tags: ['sales'],
      href: '/dashboards/executive-sales',
    }],
  }
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-panel-muted: #f6f8fa; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-radius-default: 6px; --ld-radius-full: 999px; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-body-md: 16px; --ld-font-size-title-sm: 20px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-tight: 1.1; --ld-line-height-snug: 1.35; --ld-line-height-compact: 1.25; }
        </style>
      </head>
      <body>
        <ld-catalog-page page="${escapeHTML(JSON.stringify(page))}"></ld-catalog-page>
        <script type="module" src="/catalog-page-under-test.js"></script>
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
