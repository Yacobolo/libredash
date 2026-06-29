import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import type { ChatPageSignal, ChatSignal, DashboardTable, DashboardVisual } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import '../dashboard/visual-modal'
import './chat-thread'
import './chat-composer'

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
      display: grid;
      min-height: 100svh;
      grid-template-columns: auto minmax(0, 1fr);
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

    header {
      display: grid;
      min-width: 0;
      grid-template-columns: minmax(0, 1fr);
      gap: var(--base-size-4);
      border-bottom: var(--ld-border-muted);
      padding: var(--base-size-10) var(--base-size-16);
    }

    h1,
    p {
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

    p {
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .body {
      display: grid;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
      background: var(--ld-bg-app);
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
      border-top: var(--ld-border-default);
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
      sidebar: 'required',
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
    return html`
      <div class="route">
        <ld-sub-sidebar .config=${page?.sidebar ?? null}></ld-sub-sidebar>
        <section class="main" aria-label="LibreDash chats">
          <header>
            <h1>${page?.title ?? 'Chats'}</h1>
            <p>${page?.description ?? 'Ask read-only questions about dashboards, semantic models, measures, and fields.'}</p>
          </header>
          <div class="body">
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
          </div>
          <ld-visual-modal></ld-visual-modal>
        </section>
      </div>
    `
  }
}

if (!customElements.get('ld-chat-page')) customElements.define('ld-chat-page', LibreDashChatPage)
