import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/chat-page-test')

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
  test(`chat page composes route UI on ${viewport.name}`, async () => {
    const page = await browser.newPage({ viewport })
    try {
      await page.goto(baseURL)
      await page.waitForFunction(() => (
        customElements.get('ld-chat-page')
          && customElements.get('ld-sub-sidebar')
          && customElements.get('ld-chat-thread')
          && customElements.get('ld-chat-composer')
      ))
      await page.locator('ld-chat-page').evaluate((element: any) => element.updateComplete)

      const state = await page.locator('ld-chat-page').evaluate((element: any) => {
        const root = element.shadowRoot
        const composer = root.querySelector('ld-chat-composer') as any
        const thread = root.querySelector('ld-chat-thread') as any
        return {
          title: root.querySelector('h1')?.textContent?.trim(),
          hasSubSidebar: Boolean(root.querySelector('ld-sub-sidebar')),
          hasThread: Boolean(thread),
          hasComposer: Boolean(composer),
          conversationId: thread?.conversationId,
          composerDisabled: composer?.disabled,
          composerPending: composer?.pending,
        }
      })

      assert.deepEqual(state, {
        title: 'Chats',
        hasSubSidebar: true,
        hasThread: true,
        hasComposer: true,
        conversationId: 'c1',
        composerDisabled: false,
        composerPending: false,
      })
    } finally {
      await page.close()
    }
  })
}

function testDocument(): string {
  const page = {
    kind: 'chat',
    title: 'Chats',
    description: 'Ask read-only questions about dashboards, semantic models, measures, and fields.',
    sidebar: {
      label: 'Chats',
      railLabel: 'Chats',
      ariaLabel: 'Chat conversations',
      storageKey: 'libredash-chat-conversations-collapsed',
      activeId: 'c1',
      collapsible: false,
      numbered: false,
      items: [
        { id: 'new', title: 'New chat', href: '/chat/new', active: false },
        { id: 'c1', title: 'Revenue check', href: '/chat/c1', active: true },
      ],
    },
  }
  const agent = {
    conversations: [],
    activeConversationId: 'c1',
    transcript: [{ role: 'assistant', content: 'Ready.' }],
    status: { enabled: true, running: false },
    composer: { value: '', disabled: false, placeholder: 'Ask about dashboards, metrics, or models...' },
  }
  const attr = (value: unknown) => escapeHTML(JSON.stringify(value))
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-system: system-ui; --ld-bg-app: #f6f8fa; --ld-bg-panel: #fff; --ld-bg-control: #f6f8fa; --ld-bg-control-hover: #f3f4f6; --ld-bg-accent-muted: #ddf4ff; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-fg-link: #0969da; --ld-accent: #0969da; --ld-accent-fg: #fff; --ld-line-default: #d0d7de; --ld-line-muted: #d8dee4; --ld-line-accent: #0969da; --ld-line-accent-muted: #54aeff; --ld-border-default: 1px solid #d0d7de; --ld-border-muted: 1px solid #d8dee4; --ld-border-width-focus: 2px; --ld-radius-default: 6px; --ld-radius-tight: 4px; --base-size-4: 4px; --base-size-8: 8px; --base-size-10: 10px; --base-size-16: 16px; --ld-space-2xs: 2px; --ld-space-xs: 4px; --ld-space-sm: 8px; --ld-space-md: 12px; --ld-space-lg: 16px; --ld-chat-stack-width: 760px; --ld-chat-thread-padding: 16px; --ld-chat-thread-padding-compact: 12px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-font-size-title-sm: 16px; --ld-font-weight-strong: 600; --ld-line-height-compact: 1.3; --ld-line-height-normal: 1.5; --shadow-resting-small: 0 1px 2px rgb(0 0 0 / .08); --duration-fast: 160ms; --ease-ld: ease; }
          ld-chat-page { min-height: 720px; }
        </style>
      </head>
      <body>
        <ld-chat-page page="${attr(page)}" agent="${attr(agent)}"></ld-chat-page>
        <script type="module" src="/chat-page-under-test.js"></script>
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
