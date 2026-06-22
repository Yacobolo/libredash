import { LitElement, css, html, nothing, svg as svgTemplate } from 'lit'
import { property } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'

type ChatStatus = {
  enabled?: boolean
  running?: boolean
  error?: string
}

type ChatTranscriptItem = {
  id: string
  kind: 'user' | 'assistant' | 'tool' | 'error' | 'summary' | string
  text?: string
  markdown?: string
  toolCallId?: string
  name?: string
  title?: string
  status?: 'running' | 'complete' | 'error' | 'streaming' | string
  summary?: string
  error?: string
  conversationId?: string
  runId?: string
  createdAt?: string
}

type ChatRenderUnit =
  | { kind: 'user'; item: ChatTranscriptItem }
  | { kind: 'agent'; items: ChatTranscriptItem[] }

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: false,
})

const jsonConverter = <T,>(fallback: T) => ({
  fromAttribute(value: string | null): T {
    if (!value) return fallback
    try {
      return JSON.parse(value) as T
    } catch {
      return fallback
    }
  },
  toAttribute(value: T): string {
    return JSON.stringify(value ?? fallback)
  },
})

class ChatThread extends LitElement {
  @property({ attribute: false }) transcript: ChatTranscriptItem[] = []
  @property({ attribute: 'transcript', converter: jsonConverter<ChatTranscriptItem[]>([]) }) transcriptAttribute: ChatTranscriptItem[] = []
  @property({ attribute: 'status', converter: jsonConverter<ChatStatus>({}) }) status: ChatStatus = {}
  @property({ attribute: 'conversation-id' }) conversationId = ''

  static styles = css`
    :host {
      box-sizing: border-box;
      display: block;
      height: 100%;
      min-height: 0;
      overflow: hidden;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    *,
    *::before,
    *::after {
      box-sizing: inherit;
    }

    .thread {
      display: grid;
      height: 100%;
      min-height: 0;
      grid-template-rows: minmax(0, 1fr);
      overflow: hidden;
      background: var(--ld-bg-page);
    }

    .scroll {
      height: 100%;
      min-height: 0;
      overflow: auto;
      overscroll-behavior: contain;
      padding: var(--ld-chat-thread-padding);
    }

    .stack {
      display: grid;
      width: min(100%, var(--ld-chat-stack-width));
      margin-inline: auto;
      gap: var(--ld-chat-stack-gap);
    }

    .empty,
    .alert {
      display: grid;
      gap: var(--ld-space-sm);
      align-content: center;
      min-height: var(--ld-chat-empty-min-height);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--ld-chat-thread-padding);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      text-align: center;
    }

    .alert {
      min-height: auto;
      border-color: var(--ld-line-danger-muted);
      background: var(--ld-bg-danger-muted);
      color: var(--ld-fg-default);
      text-align: left;
    }

    .message {
      display: grid;
      max-width: min(var(--ld-chat-message-width), 100%);
    }

    .message.user {
      justify-self: end;
    }

    .agent-turn,
    .message.error {
      justify-self: start;
    }

    .label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }

    .bubble {
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--ld-chat-bubble-padding-block) var(--ld-chat-bubble-padding-inline);
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-relaxed);
      overflow-wrap: anywhere;
    }

    .bubble.plain {
      white-space: pre-wrap;
    }

    .bubble.markdown {
      display: block;
    }

    .agent-turn {
      display: grid;
      max-width: min(var(--ld-chat-message-width), 100%);
    }

    .agent-stack {
      display: grid;
      gap: var(--ld-chat-agent-item-gap);
    }

    .agent-markdown {
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-relaxed);
      overflow-wrap: anywhere;
    }

    .markdown :is(p, ul, ol, pre, blockquote) {
      margin-block: 0 var(--ld-chat-markdown-block-gap);
    }

    .markdown :is(p, ul, ol, pre, blockquote):last-child {
      margin-bottom: 0;
    }

    .markdown ul,
    .markdown ol {
      padding-left: var(--ld-chat-markdown-list-indent);
    }

    .markdown li + li {
      margin-top: var(--ld-chat-markdown-list-item-gap);
    }

    .markdown code {
      border-radius: var(--ld-chat-code-radius);
      background: var(--ld-bg-control);
      padding: var(--ld-chat-code-padding-block) var(--ld-chat-code-padding-inline);
      font-family: var(--fontStack-monospace);
      font-size: var(--ld-chat-code-font-scale);
    }

    .markdown pre {
      max-width: 100%;
      overflow: auto;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      padding: var(--ld-chat-pre-padding-block) var(--ld-chat-pre-padding-inline);
    }

    .markdown pre code {
      border-radius: 0;
      background: transparent;
      padding: 0;
      font-size: var(--ld-font-size-caption);
    }

    .markdown blockquote {
      border-left: var(--ld-chat-quote-border-width) solid var(--ld-line-muted);
      padding-left: var(--ld-chat-bubble-padding-block);
      color: var(--ld-fg-muted);
    }

    .markdown a {
      color: var(--ld-fg-accent);
      text-decoration-thickness: var(--ld-chat-link-underline-thickness);
      text-underline-offset: var(--ld-chat-link-underline-offset);
    }

    .user .bubble {
      border-color: var(--ld-line-accent-muted);
      background: var(--ld-bg-accent-muted);
    }

    .message.error .bubble {
      border-color: var(--ld-line-danger-muted);
      background: var(--ld-bg-danger-muted);
    }

    .activity {
      display: flex;
      width: fit-content;
      max-width: 100%;
      align-items: center;
      gap: var(--ld-chat-activity-gap);
      margin-block: var(--ld-chat-agent-tool-gap);
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-panel-subtle);
      padding: var(--ld-chat-activity-padding-block) var(--ld-chat-activity-padding-inline);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-snug);
    }

    .tool-icon {
      display: inline-flex;
      width: var(--ld-chat-activity-icon-size);
      height: var(--ld-chat-activity-icon-size);
      flex: 0 0 var(--ld-chat-activity-icon-size);
      color: var(--ld-fg-muted);
    }

    .tool-icon svg {
      display: block;
      width: 100%;
      height: 100%;
      fill: none;
      stroke: currentColor;
      stroke-width: 1.8;
      stroke-linecap: round;
      stroke-linejoin: round;
    }

    .activity.running .tool-icon {
      color: var(--ld-fg-warning);
      animation: pulse 1.1s ease-in-out infinite;
    }

    .activity.error .tool-icon {
      color: var(--ld-fg-danger);
    }

    .activity-text {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .activity-detail {
      color: var(--ld-fg-muted);
      font-weight: var(--ld-font-weight-regular);
    }

    @keyframes pulse {
      0%,
      100% {
        opacity: 0.45;
      }
      50% {
        opacity: 1;
      }
    }

    @media (max-width: 720px) {
      .scroll {
        padding: var(--ld-chat-thread-padding-compact);
      }
    }
  `

  render() {
    const transcript = this.resolvedTranscript

    return html`
      <div class="thread">
        <div class="scroll">
          <div class="stack">
            ${this.status.error ? html`<div class="alert">${this.status.error}</div>` : nothing}
            ${transcript.length === 0
              ? html`<div class="empty">Start a conversation from the composer.</div>`
              : nothing}
            ${groupTranscript(transcript).map((unit) => this.renderUnit(unit))}
          </div>
        </div>
      </div>
    `
  }

  private get resolvedTranscript(): ChatTranscriptItem[] {
    return Array.isArray(this.transcript) && this.transcript.length > 0 ? this.transcript : this.transcriptAttribute
  }

  private renderUnit(unit: ChatRenderUnit) {
    if (unit.kind === 'user') return this.renderMessage('user', unit.item.text || '-')
    return this.renderAgentTurn(unit.items)
  }

  private renderAgentTurn(items: ChatTranscriptItem[]) {
    return html`
      <article class="agent-turn">
        <div class="agent-stack">
          ${items.map((item) => this.renderAgentItem(item))}
        </div>
      </article>
    `
  }

  private renderAgentItem(item: ChatTranscriptItem) {
    switch (item.kind) {
      case 'tool':
        return this.renderTool(item)
      case 'error':
        return this.renderMessage('error', item.text || item.error || '-', false, true)
      case 'summary':
      case 'assistant':
      default:
        return this.renderAssistantContent(item.markdown || item.text || '-')
    }
  }

  private renderMessage(role: string, content: string, renderMarkdown = false, error = false) {
    return html`
      <article class=${['message', role, error ? 'error' : ''].filter(Boolean).join(' ')}>
        ${this.renderBubble(content, renderMarkdown)}
      </article>
    `
  }

  private renderBubble(content: string, renderMarkdown: boolean) {
    return html`<div class=${['bubble', renderMarkdown ? 'markdown' : 'plain'].join(' ')}>${renderMarkdown ? unsafeHTML(renderMarkdownHTML(content)) : content}</div>`
  }

  private renderAssistantContent(content: string) {
    return html`<div class="agent-markdown markdown">${unsafeHTML(renderMarkdownHTML(content))}</div>`
  }

  private renderTool(item: ChatTranscriptItem) {
    const status = item.status || 'running'
    const label = toolCallLabel(item)
    return html`
      <div
        class=${['activity', status === 'running' ? 'running' : '', status === 'complete' ? 'done' : '', status === 'error' ? 'error' : ''].filter(Boolean).join(' ')}
        title=${`${label}: ${statusLabel(status)}`}
      >
        <span class="tool-icon" aria-hidden="true">${toolIcon(item.name)}</span>
        <span class="activity-text">${label}</span>
      </div>
    `
  }

}

function groupTranscript(transcript: ChatTranscriptItem[]): ChatRenderUnit[] {
  const units: ChatRenderUnit[] = []
  let agentItems: ChatTranscriptItem[] = []
  const flushAgent = () => {
    if (agentItems.length === 0) return
    units.push({ kind: 'agent', items: agentItems })
    agentItems = []
  }

  for (const item of transcript) {
    if (item.kind === 'user') {
      flushAgent()
      units.push({ kind: 'user', item })
      continue
    }
    agentItems.push(item)
  }
  flushAgent()
  return units
}

function renderMarkdownHTML(value: string): string {
  return DOMPurify.sanitize(markdown.render(value), {
    USE_PROFILES: { html: true },
  })
}

function toolCallLabel(item: ChatTranscriptItem): string {
  const title = item.title || titleFromToolName(item.name || '')
  return title || 'Tool'
}

function titleFromToolName(name: string): string {
  return name
    .replace(/_/g, ' ')
    .trim()
    .replace(/\b\w/g, (match) => match.toUpperCase())
}

function toolIcon(name = '') {
  switch (name) {
    case 'list_dashboards':
      return lucideIcon(svgTemplate`<rect x="3" y="3" width="7" height="9" rx="1"></rect><rect x="14" y="3" width="7" height="5" rx="1"></rect><rect x="14" y="12" width="7" height="9" rx="1"></rect><rect x="3" y="16" width="7" height="5" rx="1"></rect>`)
    case 'describe_dashboard':
      return lucideIcon(svgTemplate`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"></path><path d="M14 2v6h6"></path><path d="M8 13h8"></path><path d="M8 17h6"></path>`)
    case 'list_metric_views':
      return lucideIcon(svgTemplate`<path d="M3 3v18h18"></path><path d="M7 16v-5"></path><path d="M12 16V7"></path><path d="M17 16v-3"></path>`)
    case 'describe_metric_view':
      return lucideIcon(svgTemplate`<path d="M3 3v18h18"></path><path d="M7 16v-5"></path><path d="M12 16V7"></path><path d="M17 16v-3"></path><path d="m17 7 4-4"></path><path d="M17 3h4v4"></path>`)
    case 'describe_model':
      return lucideIcon(svgTemplate`<path d="m21 16-9 5-9-5V8l9-5 9 5v8Z"></path><path d="m3.3 7 8.7 5 8.7-5"></path><path d="M12 22V12"></path>`)
    case 'query_dashboard_page':
      return lucideIcon(svgTemplate`<rect x="3" y="3" width="18" height="18" rx="2"></rect><path d="M3 9h18"></path><path d="M9 21V9"></path>`)
    case 'query_table':
      return lucideIcon(svgTemplate`<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M3 9h18"></path><path d="M9 21V9"></path><path d="M15 3h6v6"></path><path d="m15 9 6-6"></path>`)
    default:
      return lucideIcon(svgTemplate`<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.8-3.8a6 6 0 0 1-7.9 7.9l-6.6 6.6a2 2 0 0 1-2.8-2.8l6.6-6.6a6 6 0 0 1 7.9-7.9l-4 4Z"></path>`)
  }
}

function lucideIcon(content: unknown) {
  return svgTemplate`<svg viewBox="0 0 24 24" aria-hidden="true">${content}</svg>`
}

function statusLabel(status: string): string {
  switch (status) {
    case 'complete':
      return 'Complete'
    case 'error':
      return 'Failed'
    case 'streaming':
      return 'Streaming'
    default:
      return 'Running'
  }
}

if (!customElements.get('ld-chat-thread')) customElements.define('ld-chat-thread', ChatThread)
