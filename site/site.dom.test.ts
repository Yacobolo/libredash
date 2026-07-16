import { afterAll, beforeAll, expect, test } from 'bun:test'
import { chromium, type Browser } from '@playwright/test'

const sitePort = 20000 + (process.pid % 10000)
const baseURL = `http://127.0.0.1:${sitePort}`
let browser: Browser
let siteProcess: ReturnType<typeof Bun.spawn>

beforeAll(async () => {
  siteProcess = Bun.spawn(['go', 'run', './cmd/libredash-site', '-addr', `127.0.0.1:${sitePort}`], {
    cwd: process.cwd(),
    env: process.env,
    stdout: 'ignore',
    stderr: 'ignore',
  })
  await waitForSite()
  browser = await chromium.launch()
}, 15_000)

afterAll(async () => {
  await browser?.close()
  siteProcess?.kill()
  await siteProcess?.exited
})

test('site streams an initial chart and switches its metric through PageStream', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => Boolean(customElements.get('ld-topology-background')))
    expect(await page.locator('ld-topology-background.site-hero-background').count()).toBe(1)
    const header = page.locator('.site-header')
    expect(await header.isVisible()).toBe(true)
    expect(await header.getAttribute('aria-hidden')).toBeNull()
    expect(await header.evaluate((element) => getComputedStyle(element).position)).toBe('sticky')
    const hero = await page.locator('.site-hero').evaluate((element) => ({
      height: element.getBoundingClientRect().height,
      width: element.getBoundingClientRect().width,
      viewportHeight: window.innerHeight,
      viewportWidth: window.innerWidth,
    }))
    expect(hero.width).toBe(hero.viewportWidth)
    expect(hero.height).toBeGreaterThanOrEqual(hero.viewportHeight)
    expect(await page.locator('.site-hero-proof .site-proof-item').count()).toBe(3)
    expect(await page.locator('.site-principles .site-principle').count()).toBe(6)
    expect(await page.locator('ld-site-feature-icon').count()).toBe(9)
    expect(await page.getByRole('contentinfo').count()).toBe(1)
    expect(await page.locator('.site-product-proof ld-site-chart-demo').count()).toBe(1)
    expect(await page.getByRole('heading', { name: 'Start with the model. End with a dashboard.' }).isVisible()).toBe(true)
    await page.evaluate(() => {
      document.documentElement.style.scrollBehavior = 'auto'
      window.scrollTo(0, 64)
    })
    expect(await header.isVisible()).toBe(true)
    expect(await header.evaluate((element) => Math.round(element.getBoundingClientRect().top))).toBe(0)
    await page.waitForFunction(() => {
      const demo = document.querySelector('ld-site-chart-demo') as HTMLElement & { shadowRoot: ShadowRoot }
      const chart = demo?.shadowRoot?.querySelector('ld-echart') as HTMLElement & { chart?: { title?: string } }
      return chart?.chart?.title === 'Monthly revenue'
    })
    expect(await page.getByRole('heading', { name: 'Monthly revenue' }).isVisible()).toBe(true)

    await page.getByRole('button', { name: 'Orders' }).click()
    await page.waitForFunction(() => {
      const demo = document.querySelector('ld-site-chart-demo') as HTMLElement & { shadowRoot: ShadowRoot }
      const chart = demo?.shadowRoot?.querySelector('ld-echart') as HTMLElement & { chart?: { title?: string } }
      return chart?.chart?.title === 'Monthly orders'
    })
    expect(await page.getByRole('heading', { name: 'Monthly orders' }).isVisible()).toBe(true)
  } finally {
    await page.close()
  }
})

test('site supports system, light, and dark color modes', async () => {
  const page = await browser.newPage()
  try {
    await page.addInitScript(() => localStorage.setItem('libredash-color-mode', 'system'))
    await page.goto(baseURL)
    await page.waitForFunction(() => document.documentElement.dataset.colorMode === 'auto')

    await page.waitForFunction(() => Boolean(customElements.get('ld-site-theme-toggle')))
    await page.evaluate(() => {
      document.documentElement.style.scrollBehavior = 'auto'
      window.scrollTo(0, 64)
    })
    const toggle = page.locator('ld-site-theme-toggle').locator('button[data-theme-toggle]')
    expect(await toggle.getAttribute('data-theme-mode')).toBe('system')
    expect(await page.locator('ld-site-theme-toggle').evaluate((element) => element.shadowRoot?.querySelectorAll('svg[data-lucide="icon"]').length)).toBe(3)
    await toggle.click()
    await page.waitForFunction(() => document.documentElement.dataset.colorMode === 'light')
    expect(await toggle.getAttribute('data-theme-mode')).toBe('light')

    await toggle.click()
    await page.waitForFunction(() => document.documentElement.dataset.colorMode === 'dark')
    expect(await toggle.getAttribute('data-theme-mode')).toBe('dark')
    expect(await page.locator('html').evaluate((element) => getComputedStyle(element).colorScheme)).toBe('dark')
  } finally {
    await page.close()
  }
})

test('mobile landing page uses a compact menu and proof cards', async () => {
  const page = await browser.newPage()
  try {
    await page.setViewportSize({ width: 320, height: 900 })
    await page.goto(baseURL)

    expect(await page.locator('.site-nav-links').evaluate((element) => getComputedStyle(element).display)).toBe('none')
    const headerHeight = await page.locator('.site-header').evaluate((element) => element.getBoundingClientRect().height)
    expect(headerHeight).toBeLessThanOrEqual(45)
    const menu = page.locator('ld-site-mobile-menu')
    const menuButton = menu.locator('button')
    expect(await menuButton.count()).toBe(1)
    expect(await menuButton.evaluate((element) => element.getBoundingClientRect().height)).toBeLessThanOrEqual(30)

    const principleColumns = await page.locator('.site-principles').evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(' ').length)
    expect(principleColumns).toBe(2)
    expect(await menuButton.getAttribute('aria-expanded')).toBe('false')

    await menuButton.click()
    expect(await menuButton.getAttribute('aria-expanded')).toBe('true')
    expect(await menu.getByRole('link', { name: 'Docs' }).count()).toBe(1)

    const proofHeights = await page.locator('.site-hero-proof .site-proof-item').evaluateAll((items) => items.map((item) => item.getBoundingClientRect().height))
    expect(proofHeights).toHaveLength(3)
    expect(Math.max(...proofHeights)).toBeLessThan(180)

    await page.setViewportSize({ width: 533, height: 900 })
    const mobileHeroTitleSize = await page.locator('.site-hero h1').evaluate((element) => Number.parseFloat(getComputedStyle(element).fontSize))
    expect(mobileHeroTitleSize).toBeLessThanOrEqual(40)
    expect(await page.locator('.site-principles-heading').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  } finally {
    await page.close()
  }
})

test('getting started route gives users a code-native first path', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(`${baseURL}/docs/getting-started`)
    await page.evaluate(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (value: string) => {
            document.documentElement.dataset.copiedMarkdown = value
          },
        },
      })
    })
    expect(await page.getByRole('article').count()).toBe(1)
    expect(await page.getByRole('heading', { name: 'Get started with LibreDash' }).isVisible()).toBe(true)
    const sidebar = page.locator('.site-docs-sidebar')
    expect(await sidebar.count()).toBe(1)
    expect(await sidebar.evaluate((element) => getComputedStyle(element).position)).toBe('sticky')
    const docsNavigation = page.getByRole('navigation', { name: 'Documentation' })
    expect(await docsNavigation.getByRole('link', { name: 'Get started with LibreDash' }).getAttribute('aria-current')).toBe('page')
    const configurationGroup = sidebar.locator('details[data-site-docs-group="configuration"]')
    expect(await configurationGroup.count()).toBe(1)
    expect(await configurationGroup.getAttribute('open')).toBeNull()
    await configurationGroup.locator('summary').click()
    expect(await configurationGroup.getByRole('link', { name: 'Environment' }).count()).toBe(1)
    expect(await docsNavigation.getByRole('link', { name: 'Enterprise auth' }).count()).toBe(1)
    expect(await docsNavigation.getByRole('link', { name: 'Storage architecture' }).count()).toBe(1)
    expect(await docsNavigation.getByRole('link', { name: 'Dashboard demo' }).count()).toBe(0)
    const documentationGroup = sidebar.locator('details[data-site-docs-group="documentation"]')
    expect(await documentationGroup.count()).toBe(1)
    expect(await documentationGroup.getAttribute('open')).not.toBeNull()
    const chartGroup = sidebar.locator('details[data-site-docs-group="charts"]')
    expect(await chartGroup.count()).toBe(1)
    expect(await chartGroup.getAttribute('open')).toBeNull()
    await chartGroup.locator('summary').click()
    expect(await chartGroup.getAttribute('open')).not.toBeNull()
    expect(await chartGroup.getByRole('link', { name: 'Overview' }).getAttribute('href')).toBe('/docs/charts/overview')
    expect(await chartGroup.getByRole('link', { name: 'Line chart' }).count()).toBe(1)
    const apiGroup = sidebar.locator('details[data-site-docs-group="api-reference"]')
    expect(await apiGroup.count()).toBe(1)
    expect(await apiGroup.locator('a[href="/docs/api"]').getAttribute('href')).toBe('/docs/api')
    expect(await apiGroup.locator('a[href="/docs/api/workspaces"]').count()).toBe(1)
    const breadcrumb = page.getByRole('navigation', { name: 'Breadcrumb' })
    expect(await breadcrumb.getByRole('link', { name: 'Documentation' }).count()).toBe(1)
    expect(await breadcrumb.getByRole('link', { name: 'LibreDash' }).count()).toBe(0)
    expect(await breadcrumb.getByText('Getting started', { exact: true }).getAttribute('aria-current')).toBe('page')

    const markdownCopy = page.locator('ld-site-markdown-copy')
    expect(await markdownCopy.getAttribute('markdown')).toStartWith('# Get started with LibreDash')
    expect(await markdownCopy.evaluate((element) => (element as HTMLElement & { markdown?: string }).markdown)).toStartWith('# Get started with LibreDash')
    const copyMarkdown = page.getByRole('button', { name: 'Copy Markdown' })
    await copyMarkdown.click()
    await page.waitForFunction(() => document.querySelector('ld-site-markdown-copy')?.shadowRoot?.querySelector('button')?.getAttribute('aria-label') === 'Markdown copied')
    expect(await markdownCopy.evaluate((element) => element.shadowRoot?.querySelector('button')?.getAttribute('aria-label'))).toBe('Markdown copied')
    expect(await page.locator('html').getAttribute('data-copied-markdown')).toStartWith('# Get started with LibreDash')

    expect(await page.locator('.site-guide-step').count()).toBe(0)
    expect((await page.locator('.site-docs-article pre code').allTextContents()).map((content) => content.trim())).toEqual([
      'task bootstrap',
      'task dev',
      'dashboards/\n  catalog.yaml\n  olist/\n    model.yaml\n    executive-sales.yaml',
    ])
    expect(await page.getByRole('link', { name: 'Visual gallery' }).count()).toBeGreaterThan(0)
  } finally {
    await page.close()
  }
})

test('documentation index links to every article', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(`${baseURL}/docs`)
    expect(await page.getByRole('heading', { name: 'Documentation' }).isVisible()).toBe(true)
    const articleNavigation = page.getByRole('navigation', { name: 'Documentation articles' })
    for (const title of ['Get started with LibreDash', 'Configuration reference', 'Enterprise auth', 'Storage architecture']) {
      expect(await articleNavigation.getByRole('heading', { name: title }).isVisible()).toBe(true)
    }
    expect(await articleNavigation.getByRole('link', { name: /Chart types/ }).count()).toBe(1)
    expect(await articleNavigation.getByRole('link', { name: /Line chart/ }).count()).toBe(1)
  } finally {
    await page.close()
  }
})

test('chart documentation exposes a chart-specific configuration block', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(`${baseURL}/docs/charts/line`)
    const sidebar = page.locator('.site-docs-sidebar')
    const documentationGroup = sidebar.locator('details[data-site-docs-group="documentation"]')
    const chartGroup = sidebar.locator('details[data-site-docs-group="charts"]')
    const apiGroup = sidebar.locator('details[data-site-docs-group="api-reference"]')
    expect(await documentationGroup.count()).toBe(1)
    expect(await chartGroup.getAttribute('open')).not.toBeNull()
    expect(await apiGroup.getAttribute('open')).toBeNull()
    const breadcrumb = page.getByRole('navigation', { name: 'Breadcrumb' })
    expect(await breadcrumb.getByRole('link', { name: 'Charts' }).getAttribute('href')).toBe('/docs/charts/overview')
    expect(await breadcrumb.getByRole('link', { name: 'Documentation' }).count()).toBe(0)
    expect(await page.getByRole('heading', { name: 'Line chart' }).isVisible()).toBe(true)
    expect(await page.getByRole('heading', { name: 'Configuration' }).isVisible()).toBe(true)
    await page.waitForFunction(() => {
      const chart = document.querySelector('ld-site-doc-chart') as HTMLElement & { shadowRoot: ShadowRoot }
      const visual = chart?.shadowRoot?.querySelector('ld-echart') as HTMLElement & { chart?: { title?: string } }
      return visual?.chart?.title === 'Line'
    })
    expect(await page.locator('ld-site-doc-chart').getAttribute('chart-id')).toBe('line')
    expect(await page.locator('.site-docs-article pre code').allTextContents()).toContain('visuals:\n  revenue_by_month:\n    title: Revenue by month\n    type: line\n    query:\n      dimensions:\n        purchase_month: orders.purchase_month\n      measures:\n        revenue: null\n      sort:\n      - field: purchase_month\n        direction: asc\n      limit: 30\n')
  } finally {
    await page.close()
  }
})

test('documentation articles apply the shared Markdown treatment', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(`${baseURL}/docs/charts/line`)
    const codeBlock = await page.locator('.site-docs-article pre').evaluate((element) => {
      const style = getComputedStyle(element)
      return { borderTopWidth: style.borderTopWidth, borderRadius: style.borderRadius }
    })
    expect(codeBlock.borderTopWidth).toBe('1px')
    expect(codeBlock.borderRadius).not.toBe('0px')

    await page.goto(`${baseURL}/docs/configuration`)
    const tableHeader = await page.locator('.site-docs-article th').first().evaluate((element) => getComputedStyle(element).backgroundColor)
    expect(tableHeader).not.toBe('rgba(0, 0, 0, 0)')
  } finally {
    await page.close()
  }
})

test('documentation header keeps the Markdown copy action in the viewport', async () => {
  const page = await browser.newPage()
  try {
    await page.setViewportSize({ width: 559, height: 793 })
    await page.goto(`${baseURL}/docs/configuration`)
    const layout = await page.locator('.site-docs-article').evaluate((article) => {
      const button = document.querySelector('ld-site-markdown-copy')?.shadowRoot?.querySelector('button')
      const title = article.querySelector('h1')
      return {
        buttonLeft: button?.getBoundingClientRect().left ?? 0,
        buttonRight: button?.getBoundingClientRect().right ?? 0,
        pageWidth: document.documentElement.scrollWidth,
        titleLeft: title?.getBoundingClientRect().left ?? 0,
        viewportWidth: window.innerWidth,
      }
    })
    expect(layout.buttonLeft).toBe(layout.titleLeft)
    expect(layout.buttonRight).toBeLessThanOrEqual(layout.viewportWidth)
    expect(layout.pageWidth).toBeLessThanOrEqual(layout.viewportWidth)
  } finally {
    await page.close()
  }
})

test('compact documentation navigation opens in a drawer', async () => {
  const page = await browser.newPage()
  try {
    await page.setViewportSize({ width: 640, height: 900 })
    await page.goto(`${baseURL}/docs/getting-started`)

    const sidebar = page.locator('.site-docs-sidebar')
    const headerDrawerToggle = page.locator('ld-site-docs-drawer-toggle').first()
    const toggle = page.getByRole('button', { name: 'Open documentation menu' })
    expect(await toggle.isVisible()).toBe(true)
    expect(await toggle.getAttribute('aria-expanded')).toBe('false')
    expect(await sidebar.getAttribute('aria-hidden')).toBe('true')

    await toggle.click()
    await page.waitForFunction(() => document.querySelector('.site-docs-layout')?.classList.contains('site-docs-drawer-open'))
    expect(await headerDrawerToggle.evaluate((element) => element.shadowRoot?.querySelector('button')?.getAttribute('aria-expanded'))).toBe('true')
    expect(await sidebar.getAttribute('aria-hidden')).toBe('false')

    await page.getByRole('button', { name: 'Close documentation menu' }).last().click()
    await page.waitForFunction(() => !document.querySelector('.site-docs-layout')?.classList.contains('site-docs-drawer-open'))
    expect(await headerDrawerToggle.evaluate((element) => element.shadowRoot?.querySelector('button')?.getAttribute('aria-expanded'))).toBe('false')
  } finally {
    await page.close()
  }
})

test('chart showcase renders every supported visual type', async () => {
  const page = await browser.newPage()
  try {
    await page.goto(`${baseURL}/charts`)
    await page.waitForFunction(() => {
      const showcase = document.querySelector('ld-site-chart-showcase') as HTMLElement & { shadowRoot: ShadowRoot }
      return showcase?.shadowRoot?.querySelectorAll('.chart').length === 23
    })
    const visuals = await page.locator('ld-site-chart-showcase').evaluate((element) => {
      const root = element.shadowRoot
      return {
        cards: root?.querySelectorAll('.chart').length,
        charts: root?.querySelectorAll('ld-echart').length,
        kpis: root?.querySelectorAll('ld-kpi-card').length,
      }
    })
    expect(visuals).toEqual({ cards: 23, charts: 22, kpis: 1 })
    expect(await page.getByRole('heading', { name: 'Sunburst' }).isVisible()).toBe(true)
    await page.waitForFunction(() => {
      const showcase = document.querySelector('ld-site-chart-showcase') as HTMLElement & { shadowRoot: ShadowRoot }
      return showcase?.shadowRoot?.querySelectorAll('ld-report-table').length === 9
    })
    await page.waitForFunction(() => Array.from(document.querySelector('ld-site-chart-showcase')?.shadowRoot?.querySelectorAll('ld-report-table') ?? [])
      .every((table) => Boolean(table.shadowRoot?.querySelector('h2'))))
    const tables = await page.locator('ld-site-chart-showcase').evaluate((element) => ({
      cards: element.shadowRoot?.querySelectorAll('.table-card').length,
      tables: element.shadowRoot?.querySelectorAll('ld-report-table').length,
      titles: Array.from(element.shadowRoot?.querySelectorAll('ld-report-table') ?? []).map((table: any) => table.table?.title),
    }))
    expect(tables.cards).toBe(9)
    expect(tables.tables).toBe(9)
    expect(tables.titles).toContain('Orders conditional formatting')
  } finally {
    await page.close()
  }
}, 20_000)

async function waitForSite(): Promise<void> {
  const deadline = Date.now() + 10_000
  while (Date.now() < deadline) {
    try {
      const response = await fetch(baseURL)
      if (response.ok) return
    } catch {
      // The Go command is still compiling or binding its listener.
    }
    await Bun.sleep(100)
  }
  throw new Error('LibreDash site did not become ready')
}
