import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const projectRoot = process.cwd()
const root = join(projectRoot, '.tmp/login-page-test')

beforeAll(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://127.0.0.1')
    if (url.pathname === '/') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument())
      return
    }
    if (url.pathname === '/loader-test') {
      response.setHeader('content-type', 'text/html')
      response.end(loaderTestDocument())
      return
    }
    if (url.pathname === '/fake-topology-background.js') {
      response.setHeader('content-type', 'text/javascript')
      response.end(`window.__loginBackgroundModuleLoaded = true`)
      return
    }
    const fileRoot = url.pathname.startsWith('/static/vendor/') || url.pathname === '/static/login-background-loader.js' ? projectRoot : root
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

test('login page composes route UI', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-login-page'))
    await page.locator('ld-login-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-login-page').evaluate((element: any) => {
      const root = element.shadowRoot
      const panel = root.querySelector('.panel') as HTMLElement
      const hostRect = element.getBoundingClientRect()
      const panelRect = panel.getBoundingClientRect()
      const visibleThemeIcon = root.querySelector('[data-theme-icon]:not([hidden])') as HTMLElement | null
      return {
        title: root.querySelector('h1')?.textContent?.trim(),
        hasBackground: Boolean(root.querySelector('ld-topology-background[data-login-background]')),
        backgroundRegistered: Boolean(customElements.get('ld-topology-background')),
        moduleSrc: root.querySelector('ld-topology-background')?.getAttribute('data-module-src'),
        hasThemeToggle: Boolean(root.querySelector('[data-theme-toggle]')),
        visibleThemeIcon: visibleThemeIcon?.getAttribute('data-theme-icon'),
        visibleThemeIconHasSvg: Boolean(visibleThemeIcon?.querySelector('svg')),
        panelCenteredX: Math.abs((panelRect.left + panelRect.width / 2) - (hostRect.left + hostRect.width / 2)) <= 1,
        panelCenteredY: Math.abs((panelRect.top + panelRect.height / 2) - (hostRect.top + hostRect.height / 2)) <= 1,
        hostHeight: Math.round(hostRect.height),
        provider: root.querySelector('.provider')?.textContent?.trim(),
      }
    })

    expect(state).toEqual({
      title: 'LibreDash',
      hasBackground: true,
      backgroundRegistered: false,
      moduleSrc: '/static/topology-background.js?v=dev',
      hasThemeToggle: true,
      visibleThemeIcon: 'system',
      visibleThemeIconHasSvg: true,
      panelCenteredX: true,
      panelCenteredY: true,
      hostHeight: 820,
      provider: 'Sign in with Azure Active Directory',
    })
  } finally {
    await page.close()
  }
})

test('login background loader imports shadow DOM background module during idle time', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 820 } })
  try {
    await page.goto(`${baseURL}/loader-test`)
    await page.waitForFunction(() => {
      const host = document.querySelector('ld-login-page')
      const background = host?.shadowRoot?.querySelector('[data-login-background]')
      return Boolean((window as any).__loginBackgroundModuleLoaded && background?.getAttribute('data-background-state') === 'loaded')
    })

    const state = await page.evaluate(() => {
      const host = document.querySelector('ld-login-page')
      const background = host?.shadowRoot?.querySelector('[data-login-background]')
      return {
        moduleLoaded: (window as any).__loginBackgroundModuleLoaded === true,
        backgroundState: background?.getAttribute('data-background-state'),
      }
    })

    expect(state).toEqual({
      moduleLoaded: true,
      backgroundState: 'loaded',
    })
  } finally {
    await page.close()
  }
})

test('login theme toggle cycles shadow DOM icon and dispatches theme change', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-login-page'))
    await page.locator('ld-login-page').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-login-page').evaluate(async (element: any) => {
      const root = element.shadowRoot
      const changes: string[] = []
      document.addEventListener('libredash-theme-change', (event: CustomEvent) => changes.push(event.detail?.mode), { once: true })
      const toggle = root.querySelector('[data-theme-toggle]') as HTMLButtonElement
      toggle.click()
      await element.updateComplete
      const visibleThemeIcon = root.querySelector('[data-theme-icon]:not([hidden])') as HTMLElement | null
      return {
        mode: toggle.getAttribute('data-theme-mode'),
        visibleThemeIcon: visibleThemeIcon?.getAttribute('data-theme-icon'),
        visibleThemeIconHasSvg: Boolean(visibleThemeIcon?.querySelector('svg')),
        changes,
      }
    })

    expect(state).toEqual({
      mode: 'light',
      visibleThemeIcon: 'light',
      visibleThemeIconHasSvg: true,
      changes: ['light'],
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
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-accent: #0969da; --bgColor-accent-emphasis: #0969da; --bgColor-inverse: #0d1117; --ld-topology-bg: #0d1117; --ld-border-default: 1px solid #d0d7de; --ld-radius-default: 6px; --base-size-12: 12px; --base-size-16: 16px; --base-size-20: 20px; --base-size-24: 24px; --control-medium-size: 32px; --control-xlarge-size: 40px; --ld-font-size-body-md: 16px; --ld-font-size-title-md: 20px; --ld-font-weight-medium: 500; --ld-font-weight-strong: 600; --ld-line-height-compact: 1.3; --shadow-resting-small: 0 1px 2px rgb(0 0 0 / .08); }
        </style>
      </head>
      <body>
        <main data-signals="${escapeHTML(JSON.stringify({ page }))}">
          <ld-login-page></ld-login-page>
        </main>
        <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script>
        <script type="module" src="/login-page-under-test.js"></script>
      </body>
    </html>
  `
}

function loaderTestDocument(): string {
  return `
    <!doctype html>
    <html>
      <body>
        <ld-login-page></ld-login-page>
        <script>
          customElements.define('ld-login-page', class extends HTMLElement {
            connectedCallback() {
              this.attachShadow({ mode: 'open' }).innerHTML = '<div data-login-background data-module-src="/fake-topology-background.js"></div>'
            }
          })
        </script>
        <script type="module" src="/static/login-background-loader.js"></script>
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
