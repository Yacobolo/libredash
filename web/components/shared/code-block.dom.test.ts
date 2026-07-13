import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/code-block-test')

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

test('code block highlights json and toon, and falls back plainly for text and unknown languages', async () => {
  const page = await browser.newPage({ viewport: { width: 900, height: 700 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('ld-code-block'))

    const state = await page.evaluate(async () => {
      const waitFor = async (predicate: () => boolean, timeoutMs = 5000): Promise<void> => {
        const started = performance.now()
        while (!predicate()) {
          if (performance.now() - started > timeoutMs) throw new Error('timed out waiting for condition')
          await new Promise((resolve) => setTimeout(resolve, 20))
        }
      }
      const json = document.createElement('ld-code-block') as any
      json.language = 'json'
      json.code = '{\n  "ok": true,\n  "count": 3\n}'
      document.body.append(json)
      await waitFor(() => Boolean(json.querySelector('.shiki')))

      const toon = document.createElement('ld-code-block') as any
      toon.language = 'toon'
      toon.code = 'items[2]{id,title}:\n  1,Sales\n  2,Ops\ncount: 2'
      document.body.append(toon)
      await waitFor(() => Boolean(toon.querySelector('.shiki')))

      const text = document.createElement('ld-code-block') as any
      text.language = 'text'
      text.code = 'plain text'
      document.body.append(text)
      await text.updateComplete

      const unknown = document.createElement('ld-code-block') as any
      unknown.language = 'made-up'
      unknown.code = 'fallback text'
      document.body.append(unknown)
      await unknown.updateComplete

      const compact = document.createElement('ld-code-block') as any
      compact.compact = true
      compact.language = 'json'
      compact.code = '{"compact":true}'
      document.body.append(compact)
      await compact.updateComplete
      const compactPre = compact.querySelector('pre') as HTMLElement

      return {
        jsonHighlighted: Boolean(json.querySelector('.shiki')),
        toonHighlighted: Boolean(toon.querySelector('.shiki')),
        jsonText: json.textContent || '',
        toonText: toon.textContent || '',
        textFallback: Boolean(text.querySelector('.code-block-fallback')),
        textError: Boolean(text.querySelector('.code-block-error')),
        unknownFallback: Boolean(unknown.querySelector('.code-block-fallback')),
        unknownError: Boolean(unknown.querySelector('.code-block-error')),
        compactAttr: compact.hasAttribute('compact'),
        compactWhiteSpace: getComputedStyle(compactPre).whiteSpace,
        compactOverflowX: getComputedStyle(compactPre).overflowX,
      }
    })

    expect(state.jsonHighlighted).toBe(true)
    expect(state.toonHighlighted).toBe(true)
    expect(state.jsonText).toContain('"ok"')
    expect(state.toonText).toContain('items[2]{id,title}:')
    expect(state.textFallback).toBe(true)
    expect(state.textError).toBe(false)
    expect(state.unknownFallback).toBe(true)
    expect(state.unknownError).toBe(false)
    expect(state.compactAttr).toBe(true)
    expect(state.compactWhiteSpace).toBe('pre')
    expect(state.compactOverflowX).toBe('auto')
  } finally {
    await page.close()
  }
})

function testDocument(): string {
  return `
    <!doctype html>
    <html>
      <head>
        <style>
          html, body { margin: 0; min-height: 100%; }
          body { --fontStack-monospace: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; --ld-bg-panel-muted: #f6f8fa; --ld-fg-default: #24292f; --ld-fg-muted: #57606a; --ld-border-muted: 1px solid #d8dee4; --borderRadius-medium: 6px; --base-size-8: 8px; --base-size-12: 12px; --base-size-16: 16px; --ld-font-size-caption: 12px; --ld-font-size-body-sm: 14px; --ld-line-height-snug: 1.35; }
          ld-code-block { display: block; width: 760px; margin: 24px; }
        </style>
      </head>
      <body>
        <script type="module" src="/code-block-under-test.js"></script>
      </body>
    </html>
  `
}
