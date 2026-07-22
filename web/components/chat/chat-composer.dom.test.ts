import { afterAll, beforeAll, expect, test } from 'bun:test'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { join, normalize } from 'node:path'
import { chromium, type Browser } from '@playwright/test'

let server: Server
let baseURL = ''
let browser: Browser

const root = join(process.cwd(), '.tmp/chat-composer-test')

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

test('composer renders an elevated centered prompt surface', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('lv-chat-composer'))
    await page.locator('lv-chat-composer').evaluate((element: any) => element.updateComplete)

    const state = await page.locator('lv-chat-composer').evaluate((element: any) => {
      const root = element.shadowRoot
      const form = root.querySelector('form') as HTMLElement
      const surface = root.querySelector('.composer-surface') as HTMLElement
      const textarea = root.querySelector('textarea') as HTMLTextAreaElement
      const actions = root.querySelector('.actions') as HTMLElement
      const button = root.querySelector('button') as HTMLButtonElement
      const formRect = form.getBoundingClientRect()
      const surfaceRect = surface.getBoundingClientRect()
      const surfaceStyle = getComputedStyle(surface)
      const textareaStyle = getComputedStyle(textarea)
      return {
        formWidth: Math.round(formRect.width),
        formLeft: Math.round(formRect.left),
        surfaceWidth: Math.round(surfaceRect.width),
        surfaceLeft: Math.round(surfaceRect.left),
        surfaceDisplay: surfaceStyle.display,
        surfaceRadius: surfaceStyle.borderRadius,
        surfaceShadow: surfaceStyle.boxShadow,
        textareaPlaceholder: textarea.getAttribute('placeholder'),
        textareaResize: textareaStyle.resize,
        textareaMinHeight: Math.round(parseFloat(textareaStyle.minHeight)),
        textareaMaxHeight: Math.round(parseFloat(textareaStyle.maxHeight)),
        actionsJustify: getComputedStyle(actions).justifyContent,
        buttonWidth: Math.round(button.getBoundingClientRect().width),
        buttonHeight: Math.round(button.getBoundingClientRect().height),
        buttonDisabled: button.disabled,
      }
    })

    expect(state).toMatchObject({
      formWidth: 784,
      formLeft: 248,
      surfaceWidth: 760,
      surfaceLeft: 260,
      surfaceDisplay: 'grid',
      surfaceRadius: '12px',
      surfaceShadow: 'rgba(0, 0, 0, 0.12) 0px 8px 24px 0px',
      textareaPlaceholder: 'Ask about dashboards, metrics, or models...',
      textareaResize: 'none',
      textareaMinHeight: 36,
      textareaMaxHeight: 160,
      actionsJustify: 'flex-end',
      buttonWidth: 32,
      buttonHeight: 32,
      buttonDisabled: true,
    })
  } finally {
    await page.close()
  }
})

test('composer preserves submit, multiline, disabled, and pending behavior', async () => {
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } })
  try {
    await page.goto(baseURL)
    await page.waitForFunction(() => customElements.get('lv-chat-composer'))
    await page.locator('lv-chat-composer').evaluate((element: any) => element.updateComplete)

    const events = await page.locator('lv-chat-composer').evaluate(async (element: any) => {
      const root = element.shadowRoot
      const textarea = root.querySelector('textarea') as HTMLTextAreaElement
      const button = root.querySelector('button') as HTMLButtonElement
      const received: string[] = []
      element.addEventListener('lv-chat-submit', (event: CustomEvent) => received.push(event.detail.input))

      textarea.value = '  Revenue trend  '
      textarea.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true, inputType: 'insertText', data: 'Revenue trend' }))
      await element.updateComplete
      const enabledAfterInput = !button.disabled
      const singleLineHeight = Math.round(textarea.getBoundingClientRect().height)

      textarea.value = ['one', 'two', 'three', 'four', 'five', 'six'].join('\n')
      textarea.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true, inputType: 'insertLineBreak', data: '\n' }))
      await element.updateComplete
      const multilineHeight = Math.round(textarea.getBoundingClientRect().height)
      const multilineOverflowY = getComputedStyle(textarea).overflowY

      textarea.value = '  Revenue trend  '
      textarea.dispatchEvent(new InputEvent('input', { bubbles: true, composed: true, inputType: 'insertText', data: 'Revenue trend' }))
      await element.updateComplete

      textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true, bubbles: true, composed: true }))
      const afterShiftEnter = received.length

      textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, composed: true }))
      const afterEnter = received.length

      element.pending = true
      await element.updateComplete
      const pendingDisabled = button.disabled
      const spinner = root.querySelector('lv-loading-spinner') as any
      await spinner?.updateComplete
      const hasSpinner = Boolean(spinner)
      const spinnerAnimationDuration = spinner?.shadowRoot?.querySelector('svg')
        ? getComputedStyle(spinner.shadowRoot.querySelector('svg')).animationDuration
        : ''
      const spinnerInheritsButtonColor = spinner ? getComputedStyle(spinner).color === getComputedStyle(button).color : false

      element.pending = false
      element.disabled = true
      await element.updateComplete
      const textareaDisabled = textarea.disabled
      const disabledButton = button.disabled

      return { received, enabledAfterInput, singleLineHeight, multilineHeight, multilineOverflowY, afterShiftEnter, afterEnter, pendingDisabled, hasSpinner, spinnerAnimationDuration, spinnerInheritsButtonColor, textareaDisabled, disabledButton }
    })

    expect(events.received).toEqual(['Revenue trend'])
    expect(events.enabledAfterInput).toBe(true)
    expect(events.singleLineHeight).toBe(36)
    expect(events.multilineHeight).toBeGreaterThan(events.singleLineHeight)
    expect(events.multilineHeight).toBeLessThanOrEqual(160)
    expect(events.multilineOverflowY).toBe('hidden')
    expect(events.afterShiftEnter).toBe(0)
    expect(events.afterEnter).toBe(1)
    expect(events.pendingDisabled).toBe(true)
    expect(events.hasSpinner).toBe(true)
    expect(events.spinnerAnimationDuration).toBe('1.8s')
    expect(events.spinnerInheritsButtonColor).toBe(true)
    expect(events.textareaDisabled).toBe(true)
    expect(events.disabledButton).toBe(true)
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
          body {
            --fontStack-system: system-ui;
            --lv-bg-app: #f6f8fa;
            --lv-bg-panel: #fff;
            --lv-bg-control: #f6f8fa;
            --lv-bg-hover: #eff2f5;
            --lv-bg-accent-muted: #ddf4ff;
            --lv-fg-default: #24292f;
            --lv-fg-muted: #57606a;
            --lv-accent: #0969da;
            --lv-accent-fg: #fff;
            --lv-line-default: #d0d7de;
            --lv-line-muted: #d8dee4;
            --lv-line-accent: #0969da;
            --lv-line-accent-muted: #54aeff;
            --lv-border-default: 1px solid #d0d7de;
            --lv-border-muted: 1px solid #d8dee4;
            --lv-border-width-focus: 2px;
            --lv-radius-default: 6px;
            --lv-radius-large: 12px;
            --lv-space-2xs: 2px;
            --lv-space-xs: 4px;
            --lv-space-sm: 6px;
            --lv-space-md: 8px;
            --lv-space-lg: 12px;
            --lv-space-xl: 16px;
            --lv-control-medium: 32px;
            --lv-chat-stack-width: 760px;
            --lv-font-size-body-sm: 14px;
            --lv-font-weight-strong: 600;
            --lv-line-height-normal: 1.5;
            --lv-transition-fast: 160ms ease;
            --lv-shadow-floating-sm: 0 8px 24px rgb(0 0 0 / .12);
            --lv-spinner-size-md: 16px;
            --lv-spinner-duration: 1800ms;
            --duration-fast: 160ms;
            --ease-lv: ease;
          }
        </style>
      </head>
      <body>
        <lv-chat-composer placeholder="Ask about dashboards, metrics, or models..."></lv-chat-composer>
        <script type="module" src="/chat-composer-under-test.js"></script>
      </body>
    </html>
  `
}
