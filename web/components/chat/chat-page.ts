import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import type { ChatConversationSummary, ChatPageSignal, ChatSignal, DashboardTable, DashboardVisual } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import '../dashboard/visual-modal'
import './chat-thread'
import './chat-composer'
import './chat-list'

const emptyAgent: ChatSignal = {
  conversations: [],
  activeConversationId: '',
  transcript: [],
  status: { enabled: false, running: false },
  composer: { value: '', disabled: true, placeholder: 'Agent is not configured.' },
}

class LibreDashChatPage extends LitElement {
  @property({ converter: jsonAttribute<ChatPageSignal | null>(null) }) page: ChatPageSignal | null = null
  @property({ converter: jsonAttribute<ChatSignal>(emptyAgent) }) agent: ChatSignal = emptyAgent
  @property({ converter: jsonAttribute<Record<string, DashboardVisual>>({}) }) visuals: Record<string, DashboardVisual> = {}
  @property({ converter: jsonAttribute<Record<string, DashboardTable>>({}) }) tables: Record<string, DashboardTable> = {}
  @property({ type: Boolean, reflect: true }) pending = false
  @property({ attribute: 'composerdisabled', type: Boolean }) composerDisabled = false

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 100svh;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
      background: var(--ld-bg-app);
    }

    .route {
      display: block;
      min-height: 100svh;
      background: var(--ld-bg-app);
    }

    .main {
      display: grid;
      min-width: 0;
      height: 100svh;
      min-height: 0;
      grid-template-rows: auto minmax(0, 1fr);
      overflow: hidden;
      background: var(--ld-bg-app);
    }

    .main.list-main {
      height: auto;
      min-height: 100svh;
      grid-template-rows: minmax(0, 1fr);
      overflow: visible;
    }

    .conversation-titlebar {
      display: grid;
      min-width: 0;
      grid-template-columns: minmax(0, 1fr);
      padding: 14px var(--base-size-16) var(--base-size-8);
    }

    h1 {
      margin: 0;
    }

    h1 {
      overflow: hidden;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-title-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .body {
      display: grid;
      min-width: 0;
      min-height: 0;
      overflow: auto;
      background: var(--ld-bg-app);
    }

    .list-main .body {
      min-height: auto;
      overflow: visible;
    }

    .thread-stack {
      display: grid;
      min-width: 0;
      min-height: 0;
      grid-template-rows: minmax(0, 1fr) auto;
      overflow: hidden;
      background: var(--ld-bg-app);
    }

    ld-chat-thread {
      display: block;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
    }

    ld-chat-composer {
      display: block;
      background: var(--ld-bg-app);
    }

    @media (max-width: 640px) {
      .route {
        grid-template-columns: 1fr;
      }

      .main {
        height: auto;
        min-height: 100svh;
      }
    }
  `

  updated(): void {
    checkSignalContract('chat page', this.page, {
      title: 'required',
    })
    checkSignalContract('chat agent', this.agent, {
      transcript: 'required',
      status: 'required',
      composer: 'required',
    })
  }

  render() {
    const page = this.page
    const agent = this.agent ?? emptyAgent
    const status = agent.status ?? emptyAgent.status
    const composer = agent.composer ?? emptyAgent.composer
    const view = page?.view ?? 'conversation'
    const isList = view === 'list'
    const title = conversationTitle(agent)
    return html`
      <div class="route">
        <section class=${isList ? 'main list-main' : 'main'} aria-label="LibreDash chats">
          ${isList ? null : html`
            <div class="conversation-titlebar">
              <h1>${title}</h1>
            </div>
          `}
          <div class="body">
            ${isList ? html`
              <ld-chat-list
                .conversations=${agent.conversations ?? []}
                active-conversation-id=${agent.activeConversationId ?? ''}
              ></ld-chat-list>
            ` : html`
              <div class="thread-stack">
                <ld-chat-thread
                  .transcript=${agent.transcript ?? []}
                  .visuals=${this.visuals ?? {}}
                  .tables=${this.tables ?? {}}
                  .status=${status}
                  conversation-id=${agent.activeConversationId ?? ''}
                >${status.error ?? ''}</ld-chat-thread>
                <ld-chat-composer
                  .value=${composer.value ?? ''}
                  .disabled=${this.composerDisabled || status.running || composer.disabled}
                  .pending=${this.pending || status.running}
                  .placeholder=${composer.placeholder ?? emptyAgent.composer.placeholder}
                ></ld-chat-composer>
              </div>
            `}
          </div>
          <ld-visual-modal></ld-visual-modal>
        </section>
      </div>
    `
  }
}

function conversationTitle(agent: ChatSignal): string {
  const activeID = agent.activeConversationId?.trim()
  if (!activeID) return 'New chat'
  const active = (agent.conversations ?? []).find((conversation: ChatConversationSummary) => conversation.id === activeID)
  const title = active?.title?.trim()
  return title || 'New chat'
}

if (!customElements.get('ld-chat-page')) customElements.define('ld-chat-page', LibreDashChatPage)
