import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = process.cwd()
const tmpRoot = join(root, '.tmp/app-shell-test')

beforeAll(async () => {
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
    if (url.pathname === '/sidebar-history') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(true, false, true))
      return
    }
    if (url.pathname === '/sidebar-active-nav') {
      response.setHeader('content-type', 'text/html')
      response.end(testDocument(true, false, false, true))
      return
    }
    if (url.pathname === '/signal-shell') {
      response.setHeader('content-type', 'text/html')
      response.end(signalShellDocument())
      return
    }
    if (url.pathname === '/chat') {
      response.setHeader('content-type', 'text/html')
      response.end('<!doctype html><title>Chat list</title><main>Chat list</main>')
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

afterAll(async () => {
  await browser?.close()
  await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
}, 15_000)

test('global CSS reserves app shell geometry before custom elements upgrade', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/fallback`)

    const state = await shellGeometry(page)

    expect(state.shell.display).toBe('grid')
    expect(state.shell.x).toBe(0)
    expect(state.shell.width).toBe(1320)
    expect(state.shell.height).toBe(900)
    expect(state.route.display).toBe('block')
    expect(state.route.x).toBe(248)
    expect(state.route.width).toBe(1072)
    expect(state.route.height).toBe(900)
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

    expect(state.routeDefined).toBe(false)
    expect(state.shell.display).toBe('grid')
    expect(state.route.display).toBe('block')
    expect(state.route.x).toBe(248)
    expect(state.route.width).toBe(1072)
    expect(state.route.height).toBe(900)
  } finally {
    await page.close()
  }
})

test('upgraded compact app shell does not keep the fallback route grid column', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/upgraded-compact-shell`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.waitForFunction(() => (document.querySelector('ld-app-shell') as any)?.chrome?.sidebar?.compact === true)
    await page.waitForFunction(() => ((document.querySelector('ld-app-shell') as any)?.shadowRoot?.querySelector('ld-sidebar') as any)?.config?.compact === true)
    await page.waitForFunction(() => {
      const shell = document.querySelector('ld-app-shell') as HTMLElement | null
      const sidebar = shell?.shadowRoot?.querySelector('ld-sidebar') as HTMLElement | null
      return sidebar && Math.round(sidebar.getBoundingClientRect().width) === 48
    })
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await shellGeometry(page)

    expect(state.routeDefined).toBe(false)
    expect(state.sidebar.width).toBe(48)
    expect(state.shellMain.x).toBe(state.sidebar.right)
    expect(state.route.x).toBe(state.sidebar.right)
    expect(state.route.gridColumnStart).toBe('auto')
  } finally {
    await page.close()
  }
})

test('mobile navigation opens in an accessible drawer', async () => {
  const page = await browser.newPage({ viewport: { width: 553, height: 793 } })
  try {
    await page.goto(`${baseURL}/sidebar-history`)
    await page.evaluate(() => localStorage.setItem('libredash-sidebar-collapsed', 'true'))
    await page.reload()
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      const nav = root.querySelector('nav') as HTMLElement
      const main = element.shadowRoot.querySelector('main') as HTMLElement
      const menuButton = root.querySelector('.mobile-menu-button') as HTMLButtonElement
      const sidebarBox = sidebar.getBoundingClientRect()
      const mainBox = main.getBoundingClientRect()
      return {
        sidebarWidth: Math.round(sidebarBox.width),
        mainX: Math.round(mainBox.x),
        mainY: Math.round(mainBox.y),
        sidebarBottom: Math.round(sidebarBox.bottom),
        menu: {
          display: getComputedStyle(menuButton).display,
          expanded: menuButton.getAttribute('aria-expanded'),
        },
        navVisibility: getComputedStyle(nav).visibility,
        navInert: nav.inert,
      }
    })

    expect(state.sidebarWidth).toBe(553)
    expect(state.mainX).toBe(0)
    expect(state.mainY).toBe(state.sidebarBottom)
    expect(state.menu.display).not.toBe('none')
    expect(state.menu.expanded).toBe('false')
    expect(state.navVisibility).toBe('hidden')
    expect(state.navInert).toBe(true)

    await page.locator('ld-app-shell').evaluate(async (element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      ;(root.querySelector('.mobile-menu-button') as HTMLButtonElement).click()
      await sidebar.updateComplete
    })
    await page.waitForFunction(() => {
      const shell = document.querySelector('ld-app-shell') as HTMLElement
      const sidebar = shell.shadowRoot?.querySelector('ld-sidebar') as HTMLElement
      const nav = sidebar.shadowRoot?.querySelector('nav') as HTMLElement
      return getComputedStyle(nav).visibility === 'visible'
    })

    const openState = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      const nav = root.querySelector('nav') as HTMLElement
      const menuButton = root.querySelector('.mobile-menu-button') as HTMLButtonElement
      const backdrop = root.querySelector('.mobile-backdrop') as HTMLButtonElement
      const drawerHeader = root.querySelector('.mobile-drawer-header') as HTMLElement
      const drawer = root.querySelector('aside') as HTMLElement
      return {
        drawerOpen: root.querySelector('aside')?.hasAttribute('data-mobile-open'),
        expanded: menuButton.getAttribute('aria-expanded'),
        navVisibility: getComputedStyle(nav).visibility,
        navInert: nav.inert,
        backdropVisibility: getComputedStyle(backdrop).visibility,
        drawerBackground: getComputedStyle(drawer).backgroundColor,
        navBackground: getComputedStyle(nav).backgroundColor,
        headerBorderBottomWidth: getComputedStyle(drawerHeader).borderBottomWidth,
        navBoxShadow: getComputedStyle(nav).boxShadow,
      }
    })

    expect(openState.drawerOpen).toBe(true)
    expect(openState.expanded).toBe('true')
    expect(openState.navVisibility).toBe('visible')
    expect(openState.navInert).toBe(false)
    expect(openState.backdropVisibility).toBe('visible')
    expect(openState.navBackground).toBe(openState.drawerBackground)
    expect(openState.headerBorderBottomWidth).not.toBe('0px')
    expect(openState.navBoxShadow).not.toBe('none')

    await page.locator('ld-app-shell').evaluate(async (element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await sidebar.updateComplete
    })
    await page.waitForFunction(() => {
      const shell = document.querySelector('ld-app-shell') as HTMLElement
      const sidebar = shell.shadowRoot?.querySelector('ld-sidebar') as HTMLElement
      const nav = sidebar.shadowRoot?.querySelector('nav') as HTMLElement
      return getComputedStyle(nav).visibility === 'hidden'
    })

    const closedState = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      const nav = root.querySelector('nav') as HTMLElement
      const menuButton = root.querySelector('.mobile-menu-button') as HTMLButtonElement
      return {
        expanded: menuButton.getAttribute('aria-expanded'),
        navInert: nav.inert,
      }
    })

    expect(closedState.expanded).toBe('false')
    expect(closedState.navInert).toBe(true)
  } finally {
    await page.close()
  }
})

test('sidebar renders global chat action and recent history', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/sidebar-history`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      return {
        links: Array.from(root.querySelectorAll('a')).map((link: any) => ({
          href: link.getAttribute('href'),
          text: link.textContent.trim(),
          current: link.getAttribute('aria-current'),
          ariaLabel: link.getAttribute('aria-label'),
          title: link.getAttribute('title'),
        })),
        spacing: (() => {
          const group = root.querySelector('.nav-group:not(.primary-action)') as HTMLElement
          const navItem = root.querySelector('a[href="/chat"]') as HTMLElement
          const historyList = root.querySelector('.history-list') as HTMLElement
          return {
            navGroupGap: getComputedStyle(group).gap,
            historyListGap: getComputedStyle(historyList).gap,
            navItemHeight: Math.round(navItem.getBoundingClientRect().height),
          }
        })(),
        primaryStyle: (() => {
          const link = root.querySelector('.primary-action .nav-item') as HTMLElement
          const icon = root.querySelector('.primary-action .nav-icon') as HTMLElement
          return {
            background: getComputedStyle(link).backgroundColor,
            color: getComputedStyle(link).color,
            iconBackground: getComputedStyle(icon).backgroundColor,
            iconRadius: getComputedStyle(icon).borderRadius,
          }
        })(),
        historyLabel: root.querySelector('.history-label')?.textContent?.trim(),
        hasHistorySearch: Boolean(root.querySelector('.history-search')),
        historyStyle: (() => {
          const history = root.querySelector('.history') as HTMLElement
          const style = getComputedStyle(history)
          return {
            borderTopWidth: style.borderTopWidth,
            paddingTop: style.paddingTop,
          }
        })(),
        historyItemMetrics: (() => {
          const item = root.querySelector('.history-item') as HTMLElement
          const title = item?.querySelector('.history-title') as HTMLElement
          const navIcon = root.querySelector('a[href="/chat"] .nav-icon') as HTMLElement
          const navText = root.querySelector('a[href="/chat"] .nav-text') as HTMLElement
          const label = root.querySelector('.history-label') as HTMLElement
          const mutedProbe = document.createElement('span')
          mutedProbe.style.color = 'var(--ld-fg-muted)'
          root.append(mutedProbe)
          const mutedColor = getComputedStyle(mutedProbe).color
          mutedProbe.remove()
          return {
            gridTemplateColumns: getComputedStyle(item).gridTemplateColumns,
            labelLeft: Math.round(label.getBoundingClientRect().left),
            titleLeft: Math.round(title.getBoundingClientRect().left),
            navIconLeft: Math.round(navIcon.getBoundingClientRect().left),
            navTextLeft: Math.round(navText.getBoundingClientRect().left),
            titleWidth: Math.round(title.getBoundingClientRect().width),
            titleScrollWidth: title.scrollWidth,
            labelColor: getComputedStyle(label).color,
            mutedColor,
          }
        })(),
      }
    })

    expect(state.historyLabel).toBe('Chats')
    expect(state.links).toContainEqual({ href: '/chat/new', text: 'New chat', current: 'false', ariaLabel: 'New chat', title: 'New chat' })
    expect(state.links).toContainEqual({ href: '/chat', text: 'Chats', current: 'page', ariaLabel: 'Chats', title: 'Chats' })
    expect(state.links).toContainEqual({ href: '/chat/c1', text: 'Revenue check', current: 'page', ariaLabel: 'Revenue check', title: 'Revenue check' })
    expect(state.spacing).toEqual({ navGroupGap: '2px', historyListGap: '2px', navItemHeight: 32 })
    expect(state.hasHistorySearch).toBe(false)
    expect(state.historyStyle).toEqual({ borderTopWidth: '0px', paddingTop: '8px' })
    expect(state.historyItemMetrics.gridTemplateColumns).not.toMatch(/^26px /)
    expect(state.historyItemMetrics.labelLeft).toBe(state.historyItemMetrics.navIconLeft)
    expect(state.historyItemMetrics.titleLeft).toBe(state.historyItemMetrics.navIconLeft)
    expect(state.historyItemMetrics.titleLeft).toBeLessThan(state.historyItemMetrics.navTextLeft)
    expect(state.historyItemMetrics.titleWidth).toBeGreaterThanOrEqual(state.historyItemMetrics.titleScrollWidth)
    expect(state.historyItemMetrics.labelColor).not.toBe(state.historyItemMetrics.mutedColor)
    expect(state.primaryStyle.background).toBe('rgba(0, 0, 0, 0)')
    expect(state.primaryStyle.iconBackground).not.toBe('rgba(0, 0, 0, 0)')
    expect(state.primaryStyle.iconRadius).not.toBe('0px')
  } finally {
    await page.close()
  }
})

test('sidebar active nav item uses a full-row highlight without selector rail', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/sidebar-active-nav`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const root = sidebar.shadowRoot
      const active = root.querySelector('a[href="/workspaces"]') as HTMLElement
      const icon = active.querySelector('.nav-icon') as HTMLElement
      const style = getComputedStyle(active)
      const iconStyle = getComputedStyle(icon)
      const before = getComputedStyle(active, '::before')
      return {
        text: active.textContent.trim(),
        label: active.getAttribute('aria-label'),
        title: active.getAttribute('title'),
        current: active.getAttribute('aria-current'),
        background: style.backgroundColor,
        controlHoverBackground: getComputedStyle(document.documentElement).getPropertyValue('--control-bgColor-hover').trim(),
        border: style.borderTopColor,
        iconBackground: iconStyle.backgroundColor,
        beforeContent: before.content,
        beforeWidth: before.width,
      }
    })

    expect(state.text).toBe('Workspaces')
    expect(state.label).toBe('Workspaces')
    expect(state.title).toBe('Workspaces')
    expect(state.current).toBe('page')
    expect(state.background).toBe('rgb(239, 242, 245)')
    expect(state.controlHoverBackground).toBe('#eff2f5')
    expect(state.border).toBe('rgba(0, 0, 0, 0)')
    expect(state.iconBackground).toBe('rgba(0, 0, 0, 0)')
    expect(state.beforeContent).toBe('none')
    expect(state.beforeWidth).toBe('auto')
  } finally {
    await page.close()
  }
})

test('active chat nav item navigates to the chat list href', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/sidebar-history`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const link = page.locator('ld-app-shell ld-sidebar a[href="/chat"]')
    expect(await link.count()).toBe(1)
    await link.click()
    await page.waitForURL(`${baseURL}/chat`)

    expect(new URL(page.url()).pathname).toBe('/chat')
  } finally {
    await page.close()
  }
})

test('app shell reads chrome from Datastar signals without a payload attribute', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/signal-shell`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.waitForFunction(() => (document.querySelector('ld-app-shell') as any)?.chrome?.sidebar?.active === 'chat')
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as any
      return {
        hasChromeAttr: element.hasAttribute('chrome'),
        active: element.chrome.sidebar.active,
        text: sidebar.shadowRoot.textContent.replace(/\s+/g, ' ').trim(),
      }
    })

    expect(state.hasChromeAttr).toBe(false)
    expect(state.active).toBe('chat')
    expect(state.text).toContain('Chats')
  } finally {
    await page.close()
  }
})

test('app shell routes retargeted sidebar clicks to the visual link', async () => {
  const page = await browser.newPage({ viewport: { width: 1320, height: 900 } })
  try {
    await page.goto(`${baseURL}/sidebar-history`)
    await page.waitForFunction(() => customElements.get('ld-app-shell') && customElements.get('ld-sidebar'))
    await page.locator('ld-app-shell').evaluate((element: any) => element.updateComplete)

    await page.locator('ld-app-shell').evaluate((element: any) => {
      const sidebar = element.shadowRoot.querySelector('ld-sidebar') as HTMLElement
      const link = sidebar.shadowRoot.querySelector('a[href="/chat"]') as HTMLElement
      const rect = link.getBoundingClientRect()
      element.dispatchEvent(new MouseEvent('click', {
        bubbles: true,
        composed: true,
        button: 0,
        clientX: rect.left + rect.width / 2,
        clientY: rect.top + rect.height / 2,
      }))
    })
    await page.waitForURL(`${baseURL}/chat`)

    expect(new URL(page.url()).pathname).toBe('/chat')
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

function signalShellDocument(): string {
  const signals = {
    chrome: {
      sidebar: {
        workspaceTitle: 'LibreDash Workspace',
        active: 'chat',
        dashboardId: '',
        dashboardTitle: '',
        pageTitle: '',
        modelId: '',
        modelTitle: '',
        compact: false,
        groups: [{
          label: 'Navigation',
          items: [
            { id: 'dashboards', label: 'Dashboards', href: '/', icon: 'dashboard' },
            { id: 'chat', label: 'Chats', href: '/chat', icon: 'chat' },
            { id: 'workspaces', label: 'Workspaces', href: '/workspaces', icon: 'catalog' },
          ],
        }],
      },
    },
  }
  return `
    <!doctype html>
    <html>
      <head>
        <link rel="stylesheet" href="/static/app.css">
      </head>
      <body>
        <main class="min-h-svh bg-app text-fg-default" data-signals="${escapeHTML(JSON.stringify(signals))}">
          <ld-app-shell>
            <ld-workspace-page slot="page"></ld-workspace-page>
          </ld-app-shell>
        </main>
        <script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script>
        <script type="module" src="/tmp/app-shell-under-test.js"></script>
      </body>
    </html>
  `
}

function testDocument(includeShellScript: boolean, compact = false, history = false, nav = false): string {
  const chromeConfig = compact || history || nav ? {
    sidebar: {
      workspaceTitle: 'LibreDash Workspace',
      active: history ? 'chat' : 'workspaces',
      dashboardId: '',
      dashboardTitle: '',
      pageTitle: '',
      modelId: '',
      modelTitle: '',
      compact,
      primaryAction: history ? { label: 'New chat', href: '/chat/new', icon: 'plus' } : undefined,
      history: history ? {
        label: 'Chats',
        emptyText: 'No conversations yet.',
        items: [
          { id: 'c1', title: 'Revenue check', href: '/chat/c1', active: true },
          { id: 'c2', title: 'Inventory status', href: '/chat/c2' },
        ],
      } : undefined,
      groups: history || nav ? [{
        label: 'Navigation',
        items: [
          { id: 'dashboards', label: 'Dashboards', href: '/', icon: 'dashboard' },
          { id: 'chat', label: 'Chats', href: '/chat', icon: 'chat' },
          { id: 'workspaces', label: 'Workspaces', href: '/workspaces', icon: 'catalog' },
        ],
      }] : [],
    },
  } : null
  const signals = chromeConfig ? ` data-signals="${escapeHTML(JSON.stringify({ chrome: chromeConfig }))}"` : ''
  return `
    <!doctype html>
    <html>
      <head>
        <link rel="stylesheet" href="/static/app.css">
        <style>
          :root {
            --control-bgColor-hover: #eff2f5;
            --ld-border-transparent: 1px solid transparent;
            --ld-border-muted: 1px solid #d8dee4;
            --ld-border-width: 1px;
            --ld-fg-muted: #57606a;
            --ld-shadow-floating: 0 8px 24px rgb(0 0 0 / 12%);
          }
        </style>
      </head>
      <body>
        <main class="min-h-svh bg-app text-fg-default"${signals}>
          <ld-app-shell>
            <ld-workspace-page slot="page"></ld-workspace-page>
          </ld-app-shell>
        </main>
        ${includeShellScript ? '<script type="module" src="/static/vendor/datastar-1.0.2.js?v=dev"></script><script type="module" src="/tmp/app-shell-under-test.js"></script>' : ''}
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
