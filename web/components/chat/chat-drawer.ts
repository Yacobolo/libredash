import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { Bot, ExternalLink, Plus, X } from 'lucide'
import type {
  AgentContextSignal,
  AgentReferenceSearchSignal,
  AgentReferenceSignal,
  ChatSignal,
  DashboardVisual,
} from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
import { lucideIcon } from '../shared/lucide-icons'
import './chat-composer'
import './chat-thread'

const emptyAgent: ChatSignal = {
  conversations: [],
  activeConversationId: '',
  transcript: [],
  status: { enabled: false, running: false },
  composer: { value: '', disabled: true, placeholder: 'Agent is not configured.' },
}

class ChatDrawer extends DatastarLit(LitElement) {
  @property({ type: Boolean, reflect: true }) open = false
  @property({ attribute: false }) suggestions: AgentReferenceSignal[] = []
  @state() private references: AgentReferenceSignal[] = []

  static styles = css`
    :host {
      display: block;
      width: 0;
      min-width: 0;
      height: 100svh;
      overflow: hidden;
      border-left: 0 solid var(--ld-line-muted);
      background: var(--ld-bg-app);
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
      --ld-chat-stack-width: 100%;
    }

    :host([open]) {
			width: 100%;
      border-left-width: 1px;
    }

    .drawer {
      display: grid;
			width: 100%;
      height: 100%;
      min-height: 0;
      grid-template-rows: auto minmax(0, 1fr) auto;
      background: var(--ld-bg-app);
    }

    .header {
      display: grid;
			gap: var(--ld-space-sm);
			padding: var(--ld-space-md) var(--ld-space-lg) var(--ld-space-sm);
    }

    .toolbar {
      display: flex;
      min-width: 0;
      align-items: center;
    }

    .title {
      display: flex;
      min-width: 0;
      flex: 1;
      align-items: center;
			gap: var(--ld-space-sm);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .toolbar-actions {
      display: flex;
      align-items: center;
      gap: var(--ld-space-2xs);
    }

    .title svg,
    button svg,
    a svg {
      width: 16px;
      height: 16px;
    }

    button,
    a {
      display: inline-grid;
			width: var(--control-medium-size);
			height: var(--control-medium-size);
      place-items: center;
      border: 0;
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      padding: 0;
      text-decoration: none;
    }

    button:hover,
    button:focus-visible,
    a:hover,
    a:focus-visible {
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .close-action {
      margin-left: var(--ld-space-xs);
    }

    .context {
      display: grid;
			gap: var(--ld-space-sm);
      border: 0;
			padding: 0;
      background: var(--ld-bg-app);
      font-size: var(--ld-font-size-caption);
    }

    .context-line {
      display: flex;
      min-width: 0;
      align-items: center;
			gap: var(--ld-space-xs);
    }

    .page-context {
      overflow: hidden;
      color: var(--ld-fg-default);
      font-weight: var(--ld-font-weight-strong);
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .context-separator {
      color: var(--ld-fg-muted);
    }

    .filter-context {
      color: var(--ld-fg-muted);
      white-space: nowrap;
    }

    ld-chat-thread {
      display: block;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
    }

    ld-chat-composer {
      display: block;
      border-top: 0;
      background: transparent;
    }

    @media (max-width: 720px) {
      :host([open]) {
        position: fixed;
        inset: 0;
        z-index: var(--zIndex-modal, 200);
        width: 100vw;
        border-left: 0;
      }

      .drawer {
        width: 100vw;
      }
    }

    @media (prefers-reduced-motion: reduce) {
      :host { transition: none; }
    }
  `

  get agent(): ChatSignal {
    return this.signal<ChatSignal>('agent', emptyAgent)
  }

  get context(): AgentContextSignal | null {
    return this.signal<AgentContextSignal | null>('agentContext', null)
  }

  get visuals(): Record<string, DashboardVisual> {
    return this.signal<Record<string, DashboardVisual>>('agentVisuals', {})
  }

	get dashboardFilters(): AgentContextSignal['filters'] {
		return this.signal<AgentContextSignal['filters']>('filters', this.context?.filters ?? { controls: {}, selections: [] })
	}

  get pending(): boolean {
    return this.signal<boolean>('agentTurnPending', false) || Boolean(this.agent.status.running)
  }

	get referenceSearch(): AgentReferenceSearchSignal {
		return this.signal<AgentReferenceSearchSignal>('agentReferenceSearch', {
			query: '', workspaceId: '', dashboardId: '', pageId: '', results: [],
		})
	}

  public openDrawer(): void {
    this.open = true
		void this.updateComplete.then(() => {
			const composer = this.shadowRoot?.querySelector('ld-chat-composer')
			composer?.shadowRoot?.querySelector('textarea')?.focus()
		})
  }

  public openWithReference(reference: AgentReferenceSignal): void {
    if (!this.references.some((current) => referenceKey(current) === referenceKey(reference))) {
      this.references = [...this.references, reference]
      this.notifyReferences()
    }
    this.openDrawer()
  }

  protected updated(changed: Map<string, unknown>): void {
    if (!changed.has('open') || !this.open) return
    const composer = this.shadowRoot?.querySelector('ld-chat-composer') as (HTMLElement & { remeasure(): void }) | null
    composer?.remeasure()
  }

  render() {
    const agent = this.agent
    const context = this.context
		const currentFilters = this.dashboardFilters
		const controls = Object.keys(currentFilters.controls ?? {}).length
		const selections = currentFilters.selections?.length ?? 0
		const searchResults = this.referenceSearch.results ?? []
		const pinnedSuggestions = mergeReferences(
			this.suggestions,
			searchResults.filter((reference) => isOnPageReference(reference, context)),
		)
		const workspaceSuggestions = searchResults.filter((reference) => !isOnPageReference(reference, context))
    const conversationHref = agent.activeConversationId
      ? `/chats/${encodeURIComponent(agent.activeConversationId)}`
      : '/chats/new'
    return html`
		<aside class="drawer" aria-label="Dashboard agent" aria-hidden=${String(!this.open)} ?inert=${!this.open}>
        <header class="header">
          <div class="toolbar">
            <div class="title">${lucideIcon(Bot)}<span>Agent</span></div>
            <div class="toolbar-actions">
              <button type="button" title="New chat" aria-label="New chat" @click=${this.newChat}>${lucideIcon(Plus)}</button>
              <a href=${conversationHref} title="Open full chat" aria-label="Open full chat">${lucideIcon(ExternalLink)}</a>
					  <button class="close-action" type="button" title="Close" aria-label="Close agent" @click=${this.closeDrawer}>${lucideIcon(X)}</button>
            </div>
          </div>
          <section class="context" aria-label="Included dashboard context">
            <div class="context-line">
              <span class="page-context">${context?.pageTitle || 'Current page'}</span>
              <span class="context-separator" aria-hidden="true">·</span>
              <span class="filter-context">${controls} ${controls === 1 ? 'filter' : 'filters'} · ${selections} ${selections === 1 ? 'selection' : 'selections'}</span>
            </div>
          </section>
        </header>
        <ld-chat-thread
          surface="drawer"
          .transcript=${agent.transcript ?? []}
          .visuals=${this.visuals}
          .status=${agent.status}
          conversation-id=${agent.activeConversationId ?? ''}
        ></ld-chat-thread>
        <ld-chat-composer
          .value=${agent.composer.value ?? ''}
          .disabled=${this.pending || agent.composer.disabled}
          .pending=${this.pending}
          .placeholder=${agent.composer.placeholder || 'Ask about this dashboard…'}
          .references=${this.references}
          .referenceLimit=${context?.referenceLimit ?? 12}
          .pinnedSuggestions=${pinnedSuggestions}
          .suggestions=${workspaceSuggestions}
          @ld-chat-references-change=${this.referencesChanged}
        ></ld-chat-composer>
      </aside>
    `
  }

  private newChat() {
    this.references = []
    this.notifyReferences()
		this.dispatchEvent(new CustomEvent('ld-chat-new', { bubbles: true, composed: true }))
  }

	private closeDrawer() {
		this.open = false
		this.dispatchEvent(new CustomEvent('ld-chat-drawer-close', { bubbles: true, composed: true }))
	}

  private referencesChanged(event: CustomEvent<{ references: AgentReferenceSignal[] }>) {
    this.references = event.detail.references ?? []
    this.notifyReferences()
  }

  private notifyReferences() {
    this.dispatchEvent(new CustomEvent('ld-agent-references-change', {
      bubbles: true,
      composed: true,
      detail: { references: this.references },
    }))
  }
}

function referenceKey(reference: AgentReferenceSignal): string {
	return `${reference.workspaceId}:${reference.kind}:${reference.id || reference.componentId || reference.visualId || reference.title}`
}

function isOnPageReference(reference: AgentReferenceSignal, context: AgentContextSignal | null): boolean {
	return Boolean(
		context?.dashboardId
		&& context.pageId
		&& reference.dashboardId === context.dashboardId
		&& reference.pageId === context.pageId,
	)
}

function mergeReferences(...groups: AgentReferenceSignal[][]): AgentReferenceSignal[] {
	const seen = new Set<string>()
	return groups.flat().filter((reference) => {
		const key = referenceKey(reference)
		if (seen.has(key)) return false
		seen.add(key)
		return true
	})
}

if (!customElements.get('ld-chat-drawer')) customElements.define('ld-chat-drawer', ChatDrawer)
