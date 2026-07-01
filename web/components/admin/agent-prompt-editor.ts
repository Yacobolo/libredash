import { LitElement, css, html, nothing, type PropertyValues } from 'lit'
import { property, state } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import { Eye, SquarePen } from 'lucide'
import { lucideIcon } from '../shared/lucide-icons'
import '../shared/code-editor'

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: false,
})

type PromptMode = 'preview' | 'edit'

class AgentPromptEditor extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) disabled = false
  @state() private mode: PromptMode = 'preview'
  @state() private draft = ''
  @state() private dirty = false
  @state() private status = ''
  private draftInitialized = false

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      color: var(--ld-fg-default);
      --ld-agent-prompt-font-size: var(--ld-font-size-caption);
      --ld-agent-prompt-line-height: var(--ld-line-height-snug);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    .prompt-editor {
      display: grid;
      min-width: 0;
      overflow: hidden;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
    }

    .prompt-actions {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: var(--base-size-8);
      padding: var(--base-size-12);
    }

    .prompt-status {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
    }

    .prompt-control-row {
      display: flex;
      justify-content: flex-end;
      padding: var(--base-size-8) var(--base-size-8) 0;
    }

    .mode-toggle {
      display: inline-flex;
      overflow: hidden;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel-muted);
      padding: 2px;
    }

    .mode-toggle button,
    .save-button {
      border: 0;
      border-radius: calc(var(--ld-radius-default) - 2px);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
      cursor: pointer;
    }

    .mode-toggle button {
      display: inline-grid;
      width: 2rem;
      height: 2rem;
      place-items: center;
      background: transparent;
      padding: 0;
      color: var(--ld-fg-muted);
    }

    .mode-toggle button.is-active {
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      box-shadow: 0 0 0 1px color-mix(in srgb, var(--ld-fg-default) 8%, transparent);
    }

    .mode-toggle button:focus-visible,
    .save-button:focus-visible {
      outline: 2px solid var(--ld-fg-accent);
      outline-offset: 2px;
    }

    .prompt-body {
      display: grid;
      min-width: 0;
      padding: var(--base-size-8) var(--base-size-12) var(--base-size-12);
    }

    ld-code-editor,
    .markdown-view {
      box-sizing: border-box;
      width: 100%;
      min-height: 22rem;
    }

    .markdown-view {
      border: 0;
      border-radius: 0;
      background: var(--ld-bg-panel);
      padding: var(--base-size-16);
      color: var(--ld-fg-default);
      font-size: var(--ld-agent-prompt-font-size);
      line-height: var(--ld-agent-prompt-line-height);
    }

    ld-code-editor {
      --ld-code-editor-border: 0;
      --ld-code-editor-font-size: var(--ld-agent-prompt-font-size);
      --ld-code-editor-line-height: var(--ld-agent-prompt-line-height);
      --ld-code-editor-radius: 0;
    }

    .prompt-panel {
      min-width: 0;
      grid-area: 1 / 1;
    }

    .prompt-panel.is-hidden {
      visibility: hidden;
      pointer-events: none;
    }

    .markdown-view {
      max-height: 42rem;
      overflow: auto;
      overflow-wrap: anywhere;
    }

    .markdown-view :is(h1, h2, h3, h4, p, ul, ol, pre, blockquote) {
      margin-block: 0 var(--base-size-12);
    }

    .markdown-view :is(h1, h2, h3, h4, p, ul, ol, pre, blockquote):last-child {
      margin-bottom: 0;
    }

    .markdown-view h1,
    .markdown-view h2,
    .markdown-view h3,
    .markdown-view h4 {
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .markdown-view h1 {
      font-size: var(--ld-font-size-body-sm);
    }

    .markdown-view h2,
    .markdown-view h3,
    .markdown-view h4 {
      font-size: var(--ld-agent-prompt-font-size);
    }

    .markdown-view ul,
    .markdown-view ol {
      padding-left: 1.35rem;
    }

    .markdown-view li + li {
      margin-top: var(--base-size-4);
    }

    .markdown-view code {
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      padding: 0.1rem 0.25rem;
      font-family: var(--fontStack-monospace, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
      font-size: inherit;
    }

    .markdown-view pre {
      max-width: 100%;
      overflow: auto;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      padding: var(--base-size-12);
    }

    .markdown-view pre code {
      border-radius: 0;
      background: transparent;
      padding: 0;
      font-size: inherit;
    }

    .markdown-view blockquote {
      border-left: 3px solid var(--ld-line-muted);
      padding-left: var(--base-size-12);
      color: var(--ld-fg-muted);
    }

    .markdown-view a {
      color: var(--ld-fg-accent);
      text-underline-offset: 0.15em;
    }

    .empty-preview {
      margin: 0;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
    }

    .prompt-actions {
      border-top: var(--ld-border-muted);
      justify-content: flex-start;
    }

    .save-button {
      background: var(--ld-bg-accent);
      padding: var(--base-size-6) var(--base-size-12);
      color: var(--ld-fg-on-accent);
    }

    .save-button:disabled {
      cursor: not-allowed;
      opacity: 0.6;
    }

    @media (max-width: 640px) {
      .prompt-control-row {
        padding-inline: var(--base-size-8);
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    this.adoptValueAttribute()
  }

  attributeChangedCallback(name: string, oldValue: string | null, value: string | null): void {
    super.attributeChangedCallback(name, oldValue, value)
    if (name !== 'value' || oldValue === value || this.dirty) return
    this.value = value ?? ''
    this.draft = this.value
    this.draftInitialized = true
  }

  protected willUpdate(changed: PropertyValues<this>): void {
    if ((changed.has('value') || !this.draftInitialized) && !this.dirty) {
      this.draft = this.promptSource
      this.draftInitialized = true
    }
  }

  render() {
    const prompt = this.currentPrompt
    const canSave = !this.disabled && this.dirty && prompt.trim().length > 0
    return html`
      <div class="prompt-editor">
        <div class="prompt-control-row">
          <div class="mode-toggle" role="group" aria-label="System prompt view mode">
            ${this.renderModeButton('preview')}
            ${this.renderModeButton('edit')}
          </div>
        </div>
        <div class="prompt-body">
          <div
            class=${this.mode === 'preview' ? 'prompt-panel' : 'prompt-panel is-hidden'}
            aria-hidden=${String(this.mode !== 'preview')}
            ?inert=${this.mode !== 'preview'}
          >
            ${this.renderPreview(prompt)}
          </div>
          <div
            class=${this.mode === 'edit' ? 'prompt-panel' : 'prompt-panel is-hidden'}
            aria-hidden=${String(this.mode !== 'edit')}
            ?inert=${this.mode !== 'edit'}
          >
            ${this.renderEditor(prompt)}
          </div>
        </div>
        <div class="prompt-actions">
          <button class="save-button" type="button" ?disabled=${!canSave} @click=${this.savePrompt}>Save</button>
          <span class="prompt-status">${this.status || (this.disabled ? 'Read-only' : nothing)}</span>
        </div>
      </div>
    `
  }

  private renderModeButton(mode: PromptMode) {
    const label = mode === 'preview' ? 'Preview' : 'Edit'
    return html`
      <button
        class=${this.mode === mode ? 'is-active' : ''}
        type="button"
        aria-label=${label}
        aria-pressed=${String(this.mode === mode)}
        title=${label}
        @click=${() => {
          if (!this.dirty) this.draft = this.promptSource
          this.mode = mode
        }}
      >${lucideIcon(mode === 'preview' ? Eye : SquarePen, { size: 15, strokeWidth: 2 })}</button>
    `
  }

  private renderEditor(prompt: string) {
    return html`
      <ld-code-editor
        aria-label="System prompt"
        language="markdown"
        value=${prompt}
        .value=${prompt}
        ?disabled=${this.disabled}
        @ld-code-editor-change=${this.updateDraftFromCodeEditor}
      ></ld-code-editor>
    `
  }

  private renderPreview(prompt: string) {
    const trimmed = prompt.trim()
    if (!trimmed) {
      return html`<div class="markdown-view"><p class="empty-preview">No system prompt configured.</p></div>`
    }
    return html`<article class="markdown-view">${unsafeHTML(renderMarkdownHTML(trimmed))}</article>`
  }

  private updateDraftFromCodeEditor(event: CustomEvent<{ value: string }>): void {
    this.draft = event.detail.value
    this.dirty = true
    this.status = 'Unsaved changes'
  }

  private savePrompt(): void {
    const systemPrompt = this.currentPrompt.trim()
    if (this.disabled || !systemPrompt) return
    this.dispatchEvent(new CustomEvent('ld-agent-system-prompt-save', {
      bubbles: true,
      composed: true,
      detail: { systemPrompt },
    }))
    this.dirty = false
    this.status = 'Saved'
  }

  private get promptSource(): string {
    return this.value || this.getAttribute('value') || ''
  }

  private get currentPrompt(): string {
    if (this.dirty) return this.draft
    if (this.draft) return this.draft
    return this.promptSource
  }

  private adoptValueAttribute(): void {
    if (this.dirty || this.value !== '') return
    const value = this.getAttribute('value')
    if (value === null) return
    this.value = value
    this.draft = value
    this.draftInitialized = true
  }
}

function renderMarkdownHTML(value: string): string {
  return DOMPurify.sanitize(markdown.render(value), {
    USE_PROFILES: { html: true },
  })
}

if (!customElements.get('ld-agent-prompt-editor')) customElements.define('ld-agent-prompt-editor', AgentPromptEditor)
