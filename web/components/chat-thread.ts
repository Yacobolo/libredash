import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'

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
      display: block;
      min-height: 0;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    .thread {
      display: grid;
      min-height: 100%;
      grid-template-rows: minmax(0, 1fr);
      background: var(--ld-bg-page);
    }

    .scroll {
      min-height: 0;
      overflow: auto;
      padding: 20px;
    }

    .stack {
      display: grid;
      width: min(100%, 880px);
      margin-inline: auto;
      gap: 12px;
    }

    .empty,
    .alert {
      display: grid;
      gap: 6px;
      align-content: center;
      min-height: 260px;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: 20px;
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
      gap: 5px;
      max-width: min(760px, 100%);
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
      padding: 10px 12px;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-relaxed);
      white-space: pre-wrap;
      overflow-wrap: anywhere;
    }

    .user .bubble {
      border-color: var(--ld-line-accent-muted);
      background: var(--ld-bg-accent-muted);
    }

    .tool .bubble {
      max-height: 180px;
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
      gap: 8px;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-panel-muted);
      padding: 4px 9px;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }

    .dot {
      width: 7px;
      height: 7px;
      flex: 0 0 auto;
      border-radius: 999px;
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
        padding: 12px;
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
    return html`
      <article class=${['message', role, message.isError ? 'error' : ''].filter(Boolean).join(' ')}>
        <div class="label">${label}</div>
        <div class="bubble">${message.content || '-'}</div>
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
      role: String(message.role ?? 'assistant'),
      content: String(message.content ?? ''),
      toolName: String(message.tool_name || ''),
      isError: Boolean(message.is_error),
    })
  }
  return out
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
