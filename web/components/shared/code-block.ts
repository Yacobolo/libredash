import { LitElement, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import sql from '@shikijs/langs/sql'
import githubDark from '@shikijs/themes/github-dark'
import githubLight from '@shikijs/themes/github-light'

type CodeTheme = 'github-light' | 'github-dark'

const highlighterPromise: Promise<HighlighterCore> = createHighlighterCore({
  themes: [githubLight, githubDark],
  langs: [sql],
  engine: createJavaScriptRegexEngine(),
})

class CodeBlock extends LitElement {
  @property({ type: String }) code = ''
  @property({ type: String }) language = 'sql'
  @state() private highlighted = ''
  @state() private error = ''
  private renderToken = 0

  createRenderRoot(): HTMLElement {
    return this
  }

  connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('libredash-theme-applied', this.handleThemeApplied)
  }

  disconnectedCallback(): void {
    document.removeEventListener('libredash-theme-applied', this.handleThemeApplied)
    super.disconnectedCallback()
  }

  firstUpdated(): void {
    void this.highlight()
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('code') || changed.has('language')) {
      void this.highlight()
    }
  }

  render() {
    const code = this.code.trim()
    return html`
      <style>
        ${codeBlockStyles}
      </style>
      <div class="code-block-shell">
        ${this.error
          ? html`<pre class="code-block-fallback"><code>${code}</code></pre>`
          : this.highlighted
            ? unsafeHTML(this.highlighted)
            : html`<pre class="code-block-fallback"><code>${code || 'Loading...'}</code></pre>`}
        ${this.error ? html`<p class="code-block-error">${this.error}</p>` : nothing}
      </div>
    `
  }

  private handleThemeApplied = (): void => {
    void this.highlight()
  }

  private async highlight(): Promise<void> {
    const token = ++this.renderToken
    const code = this.code.trim()
    if (!code) {
      this.highlighted = ''
      this.error = ''
      return
    }
    try {
      const highlighter = await highlighterPromise
      if (token !== this.renderToken) return
      this.highlighted = highlighter.codeToHtml(code, {
        lang: this.language || 'sql',
        theme: this.theme,
      })
      this.error = ''
    } catch {
      if (token !== this.renderToken) return
      this.highlighted = ''
      this.error = 'Syntax highlighting is unavailable.'
    }
  }

  private get theme(): CodeTheme {
    const colorScheme = document.documentElement.style.colorScheme
    if (colorScheme === 'dark') return 'github-dark'
    return 'github-light'
  }
}

const codeBlockStyles = `
  ld-code-block {
    display: block;
    min-width: 0;
    max-width: 100%;
  }

  ld-code-block .code-block-shell {
    min-width: 0;
    max-width: 100%;
    overflow: hidden;
    border: var(--ld-border-muted);
    border-radius: var(--borderRadius-medium, 6px);
    background: var(--ld-bg-panel-muted);
  }

  ld-code-block .shiki,
  ld-code-block .code-block-fallback {
    box-sizing: border-box;
    max-width: 100%;
    max-height: min(44rem, 68vh);
    margin: 0;
    overflow: auto;
    padding: var(--base-size-16);
    font-family: var(--fontStack-monospace, ui-monospace, SFMono-Regular, SFMono-Regular, Consolas, Liberation Mono, monospace);
    font-size: var(--ld-font-size-body-sm, 0.875rem);
    line-height: 1.65;
    tab-size: 2;
  }

  ld-code-block .shiki code,
  ld-code-block .code-block-fallback code {
    font-family: inherit;
  }

  ld-code-block .code-block-fallback {
    color: var(--ld-fg-default);
    background: var(--ld-bg-panel-muted);
    white-space: pre;
  }

  ld-code-block .code-block-error {
    margin: 0;
    border-top: var(--ld-border-muted);
    padding: var(--base-size-8) var(--base-size-16);
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
  }
`

if (!customElements.get('ld-code-block')) {
  customElements.define('ld-code-block', CodeBlock)
}
