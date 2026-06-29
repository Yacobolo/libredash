import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/login-page-test')

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

test('login page composes route UI', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-login-page'))
    await page.locator('ld-login-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-login-page').evaluate((element: any) => {
      const root = element.shadowRoot
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        hasBackground: Boolean(root.querySelector('ld-topology-background[data-login-background]')),
        moduleSrc: root.querySelector('ld-topology-background')?.getAttribute('data-module-src'),
        hasThemeToggle: Boolean(root.querySelector('[data-theme-toggle]')),
        provider: root.querySelector('.provider')?.textContent?.trim(),
      }
    })

    assert.deepEqual(state, {
      title: 'LibreDash',
      hasBackground: true,
      moduleSrc: '/static/topology-background.js?v=dev',
      hasThemeToggle: true,
      provider: 'Sign in with Azure Active Directory',
    })
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  const page = {
    kind: 'login',
    title: 'LibreDash',
    providerLabel: 'Sign in with Azure Active Directory',
    backgroundModuleSrc: '/static/topology-background.js?v=dev',
  }
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-border-default: 1px solid #d0d7de; --ld-radius-default: 6px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --control-medium-size: 32px; --control-xlarge-size: 40px; --ld-font-size-body-md: 16px; --ld-font-size-title-md: 20px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-compact: 1.3; --shadow-resting-small: 0 1px 2px rgb(0 0 0 / .08); }
        </style>
      </head>
      <body>
        <ld-login-page page="${escapeHTML(JSON.stringify(page))}"></ld-login-page>
        <script type="module" src="/login-page-under-test.js"></script>
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
