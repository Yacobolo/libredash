import { LitElement, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import type { HighlighterCore } from 'shiki/core'
import { format as formatSQL } from 'sql-formatter'
import { Check, Copy } from 'lucide'
import { lucideIcon } from './lucide-icons'

type CodeTheme = 'github-light' | 'github-dark'
type SupportedLanguage = 'json' | 'sql' | 'toon'

let highlighterPromise: Promise<HighlighterCore> | null = null

function loadHighlighter(): Promise<HighlighterCore> {
  highlighterPromise ??= (async () => {
    const [
      { createHighlighterCore },
      { createJavaScriptRegexEngine },
      { default: sql },
      { default: json },
      { default: githubDark },
      { default: githubLight },
      { toonLanguage },
    ] = await Promise.all([
      import('shiki/core'),
      import('shiki/engine/javascript'),
      import('@shikijs/langs/sql'),
      import('@shikijs/langs/json'),
      import('@shikijs/themes/github-dark'),
      import('@shikijs/themes/github-light'),
      import('./toon-language'),
    ])
    return createHighlighterCore({
      themes: [githubLight, githubDark],
      langs: [json, sql, toonLanguage],
      engine: createJavaScriptRegexEngine(),
    })
  })()
  return highlighterPromise
}

class CodeBlock extends LitElement {
  @property({ type: String }) code = ''
  @property({ type: String }) language = 'sql'
  @property({ type: Boolean, reflect: true }) compact = false
  @property({ type: Boolean, reflect: true }) format = false
  @property({ type: Boolean, reflect: true }) copy = false
  @property({ type: Boolean, reflect: true }) dense = false
  @state() private highlighted = ''
  @state() private error = ''
  @state() private copied = false
  private renderToken = 0
  private copiedTimeout = 0

  createRenderRoot(): HTMLElement {
    return this
  }

  connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('libredash-theme-applied', this.handleThemeApplied)
  }

  disconnectedCallback(): void {
    document.removeEventListener('libredash-theme-applied', this.handleThemeApplied)
    window.clearTimeout(this.copiedTimeout)
    super.disconnectedCallback()
  }

  firstUpdated(): void {
    void this.highlight()
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('code') || changed.has('language') || changed.has('format')) {
      void this.highlight()
    }
  }

  render() {
    const code = this.displayCode
    return html`
      <style>
        ${codeBlockStyles}
      </style>
      <div class="code-block-shell">
        ${this.copy && code ? html`
          <button type="button" class="code-block-copy" @click=${this.copyCode}>
            ${lucideIcon(this.copied ? Check : Copy, { size: 14, strokeWidth: 2 })}
            <span>${this.copied ? 'Copied' : 'Copy'}</span>
          </button>
        ` : nothing}
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
    const code = this.displayCode
    const language = supportedLanguage(this.language)
    if (!code.trim() || !language) {
      this.highlighted = ''
      this.error = ''
      return
    }
    try {
      const highlighter = await loadHighlighter()
      if (token !== this.renderToken) return
      this.highlighted = highlighter.codeToHtml(code, {
        lang: language,
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

  private get displayCode(): string {
    const code = this.code.trim()
    if (!this.format || supportedLanguage(this.language) !== 'sql' || !code) return code
    try {
      return formatSQL(code, {
        language: 'duckdb',
        keywordCase: 'upper',
      }).trim()
    } catch {
      return code
    }
  }

  private copyCode = async (event: Event): Promise<void> => {
    event.stopPropagation()
    try {
      await navigator.clipboard?.writeText(this.displayCode)
      this.copied = true
      window.clearTimeout(this.copiedTimeout)
      this.copiedTimeout = window.setTimeout(() => {
        this.copied = false
      }, 1600)
    } catch {
      this.copied = false
    }
  }
}

function supportedLanguage(language: string): SupportedLanguage | '' {
  const normalized = language.trim().toLowerCase()
  if (normalized === 'json' || normalized === 'sql' || normalized === 'toon') return normalized
  return ''
}

const codeBlockStyles = `
  ld-code-block {
    display: block;
    min-width: 0;
    max-width: 100%;
  }

  ld-code-block .code-block-shell {
    position: relative;
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

  ld-code-block[compact] .shiki,
  ld-code-block[compact] .code-block-fallback {
    max-height: var(--ld-chat-tool-max-height, 18rem);
    padding: var(--ld-chat-pre-padding-block, var(--base-size-8)) var(--ld-chat-pre-padding-inline, var(--base-size-12));
    font-size: var(--ld-font-size-caption, 0.75rem);
    line-height: var(--ld-line-height-snug, 1.35);
    white-space: pre;
  }

  ld-code-block[dense] .shiki,
  ld-code-block[dense] .code-block-fallback {
    max-height: min(22rem, 52vh);
    padding: var(--base-size-12);
    font-size: var(--ld-font-size-caption, 0.75rem);
    line-height: var(--ld-line-height-normal, 1.5);
  }

  ld-code-block[copy] .shiki,
  ld-code-block[copy] .code-block-fallback {
    padding-top: calc(var(--base-size-16) + var(--control-medium-size, 32px));
  }

  ld-code-block[copy][compact] .shiki,
  ld-code-block[copy][compact] .code-block-fallback,
  ld-code-block[copy][dense] .shiki,
  ld-code-block[copy][dense] .code-block-fallback {
    padding-top: calc(var(--base-size-12) + var(--control-medium-size, 32px));
  }

  ld-code-block .code-block-copy {
    position: absolute;
    top: var(--base-size-8);
    right: var(--base-size-8);
    z-index: 1;
    display: inline-flex;
    min-height: var(--control-medium-size, 32px);
    align-items: center;
    gap: var(--base-size-6, 6px);
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel);
    color: var(--ld-fg-muted);
    cursor: pointer;
    font: inherit;
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium, 500);
    padding: 0 var(--base-size-8);
  }

  ld-code-block .code-block-copy:hover,
  ld-code-block .code-block-copy:focus-visible {
    background: var(--ld-bg-control-hover, var(--ld-bg-panel-muted));
    color: var(--ld-fg-default);
    outline: 0;
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
