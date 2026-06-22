import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'
import { unsafeHTML } from 'lit/directives/unsafe-html.js'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'

type ChatStatus = {
  enabled?: boolean
  running?: boolean
  error?: string
}

type ChatEvent = {
  id: string
  conversationId?: string
  runId?: string
  seq?: number
  type: string
  severity?: string
  createdAt?: string
  payload?: Record<string, unknown>
}

type ChatMessage = {
  id: string
  seq?: number
  role: string
  content: string
  toolName?: string
  isError?: boolean
}

type ToolActivity = {
  id: string
  label: string
  done: boolean
  error: boolean
}

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
  @property({ attribute: false }) events: ChatEvent[] = []
  @property({ attribute: 'events', converter: jsonConverter<ChatEvent[]>([]) }) eventsAttribute: ChatEvent[] = []
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
      gap: var(--ld-chat-message-gap);
      max-width: min(var(--ld-chat-message-width), 100%);
    }

    .message.user {
      justify-self: end;
    }

    .message.assistant,
    .message.tool,
    .message.summary {
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

    .bubble.markdown :is(p, ul, ol, pre, blockquote) {
      margin-block: 0 var(--ld-chat-markdown-block-gap);
    }

    .bubble.markdown :is(p, ul, ol, pre, blockquote):last-child {
      margin-bottom: 0;
    }

    .bubble.markdown ul,
    .bubble.markdown ol {
      padding-left: var(--ld-chat-markdown-list-indent);
    }

    .bubble.markdown li + li {
      margin-top: var(--ld-chat-markdown-list-item-gap);
    }

    .bubble.markdown code {
      border-radius: var(--ld-chat-code-radius);
      background: var(--ld-bg-control);
      padding: var(--ld-chat-code-padding-block) var(--ld-chat-code-padding-inline);
      font-family: var(--fontStack-monospace);
      font-size: var(--ld-chat-code-font-scale);
    }

    .bubble.markdown pre {
      max-width: 100%;
      overflow: auto;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      padding: var(--ld-chat-pre-padding-block) var(--ld-chat-pre-padding-inline);
    }

    .bubble.markdown pre code {
      border-radius: 0;
      background: transparent;
      padding: 0;
      font-size: var(--ld-font-size-caption);
    }

    .bubble.markdown blockquote {
      border-left: var(--ld-chat-quote-border-width) solid var(--ld-line-muted);
      padding-left: var(--ld-chat-bubble-padding-block);
      color: var(--ld-fg-muted);
    }

    .bubble.markdown a {
      color: var(--ld-fg-accent);
      text-decoration-thickness: var(--ld-chat-link-underline-thickness);
      text-underline-offset: var(--ld-chat-link-underline-offset);
    }

    .user .bubble {
      border-color: var(--ld-line-accent-muted);
      background: var(--ld-bg-accent-muted);
    }

    .tool .bubble {
      max-height: var(--ld-chat-tool-max-height);
      overflow: auto;
      font-family: var(--fontStack-monospace);
      font-size: var(--ld-font-size-caption);
      color: var(--ld-fg-muted);
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
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-panel-muted);
      padding: var(--ld-chat-activity-padding-block) var(--ld-chat-activity-padding-inline);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }

    .dot {
      width: var(--ld-chat-activity-dot-size);
      height: var(--ld-chat-activity-dot-size);
      flex: 0 0 auto;
      border-radius: var(--ld-radius-full);
      background: var(--ld-fg-warning);
    }

    .activity.done .dot {
      background: var(--ld-fg-success);
    }

    .activity.error .dot {
      background: var(--ld-fg-danger);
    }

    @media (max-width: 720px) {
      .scroll {
        padding: var(--ld-chat-thread-padding-compact);
      }
    }
  `

  render() {
    const events = this.resolvedEvents
    const messages = messagesFromEvents(events)
    const activities = activitiesFromEvents(events)
    const streaming = streamingText(events, messages)

    return html`
      <div class="thread">
        <div class="scroll">
          <div class="stack">
            ${this.status.error ? html`<div class="alert">${this.status.error}</div>` : nothing}
            ${messages.length === 0 && !streaming && activities.length === 0
              ? html`<div class="empty">Start a conversation from the composer.</div>`
              : nothing}
            ${messages.map((message) => this.renderMessage(message))}
            ${activities.map((activity) => this.renderActivity(activity))}
            ${streaming ? this.renderMessage({ id: 'streaming', role: 'assistant', content: streaming }) : nothing}
          </div>
        </div>
      </div>
    `
  }

  private get resolvedEvents(): ChatEvent[] {
    return Array.isArray(this.events) && this.events.length > 0 ? this.events : this.eventsAttribute
  }

  private renderMessage(message: ChatMessage) {
    const role = message.role || 'assistant'
    const label = message.toolName || roleLabel(role)
    const renderMarkdown = role === 'assistant' || role === 'summary'
    return html`
      <article class=${['message', role, message.isError ? 'error' : ''].filter(Boolean).join(' ')}>
        <div class="label">${label}</div>
        <div class=${['bubble', renderMarkdown ? 'markdown' : 'plain'].join(' ')}>
          ${renderMarkdown ? unsafeHTML(renderMarkdownHTML(message.content || '-')) : message.content || '-'}
        </div>
      </article>
    `
  }

  private renderActivity(activity: ToolActivity) {
    return html`
      <div class=${['activity', activity.done ? 'done' : '', activity.error ? 'error' : ''].filter(Boolean).join(' ')}>
        <span class="dot" aria-hidden="true"></span>
        <span>${activity.label}</span>
      </div>
    `
  }
}

function messagesFromEvents(events: ChatEvent[]): ChatMessage[] {
  const out: ChatMessage[] = []
  for (const event of events) {
    if (event.type !== 'message_appended') continue
    const message = asRecord(event.payload?.message)
    out.push({
      id: String(message.id ?? event.id),
      seq: Number(event.seq ?? 0),
      role: String(message.role ?? 'assistant'),
      content: String(message.content ?? ''),
      toolName: String(message.tool_name || ''),
      isError: Boolean(message.is_error),
    })
  }
  return out.sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))
}

function activitiesFromEvents(events: ChatEvent[]): ToolActivity[] {
  const activities = new Map<string, ToolActivity>()
  for (const event of events) {
    if (event.type !== 'tool_start' && event.type !== 'tool_end') continue
    const id = String(event.payload?.tool_call_id || event.id)
    const name = String(event.payload?.tool_name || 'tool')
    const existing = activities.get(id) ?? { id, label: `Running ${name}`, done: false, error: false }
    if (event.type === 'tool_end') {
      existing.label = `Finished ${name}`
      existing.done = true
      existing.error = event.severity === 'error'
    }
    activities.set(id, existing)
  }
  return [...activities.values()]
}

function streamingText(events: ChatEvent[], messages: ChatMessage[]): string {
  if (messages.some((message) => message.role === 'assistant' && message.content.trim() !== '')) return ''
  return events
    .filter((event) => event.type === 'message_delta')
    .map((event) => String(event.payload?.delta ?? ''))
    .join('')
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? value as Record<string, unknown> : {}
}

function renderMarkdownHTML(value: string): string {
  return DOMPurify.sanitize(markdown.render(value), {
    USE_PROFILES: { html: true },
  })
}

function roleLabel(role: string): string {
  switch (role) {
    case 'user':
      return 'You'
    case 'tool':
      return 'Tool'
    case 'summary':
      return 'Summary'
    default:
      return 'LibreDash'
  }
}

if (!customElements.get('ld-chat-thread')) customElements.define('ld-chat-thread', ChatThread)
