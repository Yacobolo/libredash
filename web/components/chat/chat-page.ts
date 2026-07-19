import { LitElement, css, html } from 'lit'
import type { ChatConversationSummary, ChatPageSignal, ChatSignal, DashboardTable, DashboardVisual } from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
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

class LibreDashChatPage extends DatastarLit(LitElement) {
  private redirectedConversationID = ''

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

    .main.new-main {
      grid-template-rows: minmax(0, 1fr);
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

    .new-chat-stage {
      display: flex;
      min-width: 0;
      min-height: 100%;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: var(--ld-space-lg);
      background: var(--ld-bg-app);
    }

    .new-chat-stage > * {
      animation: new-chat-enter var(--ld-transition-medium) both;
    }

    .new-chat-stage ld-chat-composer {
      width: 100%;
      animation-delay: 70ms;
    }

    .new-chat-title {
      box-sizing: border-box;
      width: min(100%, var(--ld-chat-stack-width));
      padding-inline: var(--ld-space-lg);
      text-align: center;
      font-size: var(--ld-font-size-title-md);
    }

    @keyframes new-chat-enter {
      from {
        opacity: 0;
        transform: translateY(var(--ld-space-sm));
      }

      to {
        opacity: 1;
        transform: translateY(0);
      }
    }

    @media (prefers-reduced-motion: reduce) {
      .new-chat-stage > * {
        animation: none;
      }
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

      .main.new-main {
        height: 100svh;
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
    this.navigateFromDraft()
  }

  private navigateFromDraft(): void {
    const conversationID = this.agent.activeConversationId?.trim()
    if (this.page?.view !== 'new' || !conversationID || conversationID === this.redirectedConversationID) return
    this.redirectedConversationID = conversationID
    window.location.assign(`/chats/${encodeURIComponent(conversationID)}`)
  }

  get page(): ChatPageSignal | null {
    return this.signal<ChatPageSignal | null>('page', null)
  }

  get agent(): ChatSignal {
    return this.signal<ChatSignal>('agent', emptyAgent)
  }

  get visuals(): Record<string, DashboardVisual> {
    return this.signal<Record<string, DashboardVisual>>('visuals', {})
  }

  get tables(): Record<string, DashboardTable> {
    return this.signal<Record<string, DashboardTable>>('tables', {})
  }

  get pending(): boolean {
    return this.signal<boolean>('agentTurnPending', false) || Boolean(this.agent.status?.running)
  }

  get composerDisabled(): boolean {
    const agent = this.agent
    return this.pending || Boolean(agent.status?.running) || Boolean(agent.composer?.disabled)
  }

  render() {
    const page = this.page
    const agent = this.agent ?? emptyAgent
    const status = agent.status ?? emptyAgent.status
    const composer = agent.composer ?? emptyAgent.composer
    const view = page?.view ?? 'conversation'
    const isList = view === 'list'
    const isNew = view === 'new'
    const title = conversationTitle(agent)
    return html`
      <div class="route">
        <section class=${['main', isList ? 'list-main' : '', isNew ? 'new-main' : ''].filter(Boolean).join(' ')} aria-label="LibreDash chats">
          ${isList || isNew ? null : this.renderConversationTitlebar(title)}
          <div class="body">
            ${isList ? this.renderListView(agent) : isNew ? this.renderNewView(composer, status) : this.renderConversationView(agent, status, composer)}
          </div>
          <ld-visual-modal></ld-visual-modal>
        </section>
      </div>
    `
  }

  private renderConversationTitlebar(title: string) {
    return html`
      <div class="conversation-titlebar">
        <h1>${title}</h1>
      </div>
    `
  }

  private renderListView(agent: ChatSignal) {
    return html`
      <ld-chat-list
        .conversations=${agent.conversations ?? []}
        active-conversation-id=${agent.activeConversationId ?? ''}
      ></ld-chat-list>
    `
  }

  private renderNewView(composer: ChatSignal['composer'], status: ChatSignal['status']) {
    return html`
      <div class="new-chat-stage">
        <h1 class="new-chat-title">Ask about your data</h1>
        ${this.renderComposer(composer, status)}
      </div>
    `
  }

  private renderConversationView(agent: ChatSignal, status: ChatSignal['status'], composer: ChatSignal['composer']) {
    return html`
      <div class="thread-stack">
        <ld-chat-thread
          .transcript=${agent.transcript ?? []}
          .visuals=${this.visuals ?? {}}
          .tables=${this.tables ?? {}}
          .status=${status}
          conversation-id=${agent.activeConversationId ?? ''}
        >${status.error ?? ''}</ld-chat-thread>
        ${this.renderComposer(composer, status)}
      </div>
    `
  }

  private renderComposer(composer: ChatSignal['composer'], status: ChatSignal['status']) {
    return html`
      <ld-chat-composer
        .value=${composer.value ?? ''}
        .disabled=${this.composerDisabled || status.running || composer.disabled}
        .pending=${this.pending || status.running}
        .placeholder=${composer.placeholder ?? emptyAgent.composer.placeholder}
      ></ld-chat-composer>
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
