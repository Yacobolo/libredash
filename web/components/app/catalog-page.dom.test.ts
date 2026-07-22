import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const projectRoot = process.cwd()
const root = join(projectRoot, '.tmp/catalog-page-test')

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
}, 15_000)

test('catalog page composes dashboard cards', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('lv-catalog-page'))
    await page.locator('lv-catalog-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('lv-catalog-page').evaluate((element: any) => {
      const root = element.shadowRoot
      const section = root.querySelector('section') as HTMLElement
      const rect = section.getBoundingClientRect()
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        card: root.querySelector('article h2')?.textContent?.trim(),
        href: root.querySelector('article a')?.getAttribute('href'),
        pages: root.querySelector('article footer span')?.textContent?.trim(),
        sectionWidth: Math.round(rect.width),
        centeredDelta: Math.round(Math.abs((rect.left + rect.width / 2) - window.innerWidth / 2)),
      }
    })

    expect(state.title).toBe('Dashboards')
    expect(state.card).toBe('Executive Sales Dashboard')
    expect(state.href).toBe('/dashboards/executive-sales')
    expect(state.pages).toBe('1 page')
    expect(state.sectionWidth).toBeLessThan(1280)
    expect(state.centeredDelta).toBeLessThanOrEqual(1)
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
      pageCount: 1,
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
          body { --fontStack-system: system-ui; --lv-bg-app: #f6f8fa; --lv-bg-panel: #fff; --lv-bg-panel-muted: #f6f8fa; --lv-fg-default: #24292f; --lv-fg-muted: #57606a; --lv-fg-link: #0969da; --lv-border-default: 1px solid #d0d7de; --lv-border-muted: 1px solid #d8dee4; --lv-radius-default: 6px; --lv-radius-full: 999px; --lv-page-content-max-width: 72rem; --base-size-4: 4px; --base-size-6: 6px; --base-size-8: 8px; --base-size-10: 10px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --lv-font-size-caption: 12px; --lv-font-size-body-sm: 14px; --lv-font-size-body-md: 16px; --lv-font-size-title-sm: 20px; --lv-font-weight-medium: 500; --lv-font-weight-strong: 600; --lv-line-height-tight: 1.1; --lv-line-height-snug: 1.35; --lv-line-height-compact: 1.25; }
        </style>
      </head>
      <body>
        <main data-signals="${escapeHTML(JSON.stringify({ page }))}">
          <lv-catalog-page></lv-catalog-page>
        </main>
        <script type="module" src="/catalog-page-under-test.js"></script>
        <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script>
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
