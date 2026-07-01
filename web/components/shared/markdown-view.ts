import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: false,
})

class MarkdownView extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) compact = false
  @property({ type: String }) emptyText = ''

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-relaxed);
      overflow-wrap: anywhere;
      --ld-markdown-block-gap: var(--ld-chat-markdown-block-gap);
      --ld-markdown-list-indent: var(--ld-chat-markdown-list-indent);
      --ld-markdown-list-item-gap: var(--ld-chat-markdown-list-item-gap);
      --ld-markdown-code-radius: var(--ld-chat-code-radius);
      --ld-markdown-code-padding-block: var(--ld-chat-code-padding-block);
      --ld-markdown-code-padding-inline: var(--ld-chat-code-padding-inline);
      --ld-markdown-code-font-scale: var(--ld-chat-code-font-scale);
      --ld-markdown-pre-padding-block: var(--ld-chat-pre-padding-block);
      --ld-markdown-pre-padding-inline: var(--ld-chat-pre-padding-inline);
      --ld-markdown-quote-border-width: var(--ld-chat-quote-border-width);
      --ld-markdown-quote-padding-inline: var(--ld-chat-bubble-padding-block);
      --ld-markdown-link-underline-thickness: var(--ld-chat-link-underline-thickness);
      --ld-markdown-link-underline-offset: var(--ld-chat-link-underline-offset);
    }

    :host([compact]) {
      font-size: var(--ld-font-size-caption);
      line-height: var(--ld-line-height-snug);
      --ld-markdown-block-gap: var(--base-size-12);
      --ld-markdown-list-indent: var(--base-size-16);
      --ld-markdown-list-item-gap: var(--base-size-4);
      --ld-markdown-code-radius: var(--ld-radius-default);
      --ld-markdown-code-padding-block: 0.1rem;
      --ld-markdown-code-padding-inline: 0.25rem;
      --ld-markdown-code-font-scale: 1;
      --ld-markdown-pre-padding-block: var(--base-size-12);
      --ld-markdown-pre-padding-inline: var(--base-size-12);
      --ld-markdown-quote-border-width: 3px;
      --ld-markdown-quote-padding-inline: var(--base-size-12);
      --ld-markdown-link-underline-thickness: var(--ld-border-width);
      --ld-markdown-link-underline-offset: 0.15em;
    }

    .markdown,
    .empty {
      min-width: 0;
    }

    .empty {
      margin: 0;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
    }

    .markdown > * {
      margin-block: 0 var(--ld-markdown-block-gap);
    }

    .markdown > :last-child {
      margin-bottom: 0;
    }

    .markdown :is(h1, h2, h3, h4, h5, h6) {
      margin-block: var(--base-size-16) var(--base-size-8);
      color: var(--ld-fg-default);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .markdown > :is(h1, h2, h3, h4, h5, h6):first-child {
      margin-top: 0;
    }

    .markdown h1 {
      font-size: var(--ld-font-size-title-md);
    }

    .markdown h2 {
      font-size: var(--ld-font-size-title-sm);
    }

    .markdown h3 {
      font-size: var(--ld-font-size-body-md);
    }

    .markdown :is(h4, h5, h6) {
      font-size: var(--ld-font-size-body-sm);
    }

    .markdown :is(p, li, blockquote, td, th) {
      line-height: var(--ld-line-height-normal);
    }

    .markdown ul,
    .markdown ol {
      padding-left: var(--ld-markdown-list-indent);
    }

    .markdown :is(ul, ol) :is(ul, ol) {
      margin-block: var(--base-size-4) 0;
    }

    .markdown li + li {
      margin-top: var(--ld-markdown-list-item-gap);
    }

    .markdown li > p {
      margin-block: 0 var(--base-size-8);
    }

    .markdown :is(strong, b) {
      font-weight: var(--ld-font-weight-strong);
    }

    .markdown :is(em, i) {
      font-style: italic;
    }

    .markdown s {
      text-decoration-line: line-through;
    }

    .markdown code {
      border-radius: var(--ld-markdown-code-radius);
      background: var(--ld-bg-control);
      padding: var(--ld-markdown-code-padding-block) var(--ld-markdown-code-padding-inline);
      font-family: var(--fontStack-monospace);
      font-size: var(--ld-markdown-code-font-scale);
    }

    .markdown pre {
      max-width: 100%;
      overflow: auto;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      padding: var(--ld-markdown-pre-padding-block) var(--ld-markdown-pre-padding-inline);
      line-height: var(--ld-line-height-normal);
    }

    .markdown pre code {
      border-radius: 0;
      background: transparent;
      padding: 0;
      font-size: var(--ld-font-size-caption);
    }

    .markdown blockquote {
      border-left: var(--ld-markdown-quote-border-width) solid var(--ld-line-muted);
      padding-left: var(--ld-markdown-quote-padding-inline);
      color: var(--ld-fg-muted);
    }

    .markdown blockquote > :last-child {
      margin-bottom: 0;
    }

    .markdown a {
      color: var(--ld-fg-accent);
      text-decoration-thickness: var(--ld-markdown-link-underline-thickness);
      text-underline-offset: var(--ld-markdown-link-underline-offset);
    }

    .markdown hr {
      height: var(--ld-border-width);
      border: 0;
      background: var(--ld-line-muted);
    }

    .markdown table {
      display: block;
      max-width: 100%;
      overflow: auto;
      border-spacing: 0;
      border-collapse: collapse;
      font-size: inherit;
    }

    .markdown th,
    .markdown td {
      border: var(--ld-border-muted);
      padding: var(--base-size-8) var(--base-size-12);
      text-align: left;
      vertical-align: top;
    }

    .markdown th {
      background: var(--ld-bg-panel-muted);
      font-weight: var(--ld-font-weight-strong);
    }

    .markdown img {
      display: block;
      max-width: 100%;
      height: auto;
      border-radius: var(--ld-radius-default);
    }
  `

  render() {
    const value = this.value.trim()
    if (!value) {
      return this.emptyText ? html`<p class="empty">${this.emptyText}</p>` : nothing
    }
    return html`<div class="markdown">${unsafeHTML(renderMarkdownHTML(value))}</div>`
  }
}

export function renderMarkdownHTML(value: string): string {
  return DOMPurify.sanitize(markdown.render(value), {
    USE_PROFILES: { html: true },
  })
}

if (!customElements.get('ld-markdown-view')) customElements.define('ld-markdown-view', MarkdownView)
