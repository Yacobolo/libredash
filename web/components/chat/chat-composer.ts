import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { BarChart3, Boxes, Columns3, Database, File, Filter, LayoutDashboard, PanelsTopLeft, Plug, Search, Send, Sigma, Table2, X } from 'lucide'
import type { AgentReferenceSignal } from '../../generated/signals'
import { lucideIcon } from '../shared/lucide-icons'

export type ChatContextReference = AgentReferenceSignal

const maxPinnedMentionSuggestions = 8
const maxGlobalMentionSuggestions = 8
const defaultReferenceLimit = 12

class ChatComposer extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) disabled = false
  @property({ type: Boolean, reflect: true }) pending = false
  @property({ type: String }) placeholder = 'Ask about dashboards, metrics, or models...'
	@property({ attribute: false }) references: ChatContextReference[] = []
	@property({ attribute: false }) pinnedSuggestions: ChatContextReference[] = []
	@property({ attribute: false }) suggestions: ChatContextReference[] = []
  @property({ type: Number, attribute: 'reference-limit' }) referenceLimit = defaultReferenceLimit
  @state() private draft = ''
	@state() private mentionIndex = 0
	@state() private mentionSearchPending = false
  private lastSearchQuery: string | null = null
  private resizeObserver?: ResizeObserver
  private observedWidth = -1

  static styles = css`
    :host {
      position: relative;
      display: block;
      background: linear-gradient(to bottom, transparent, var(--ld-bg-app) var(--ld-space-lg));
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    form {
			position: relative;
      width: min(calc(100% - var(--ld-space-lg) - var(--ld-space-lg)), var(--ld-chat-stack-width));
      margin-inline: auto;
      padding: calc(var(--ld-space-lg) + var(--ld-space-sm)) var(--ld-space-lg) var(--ld-space-lg);
    }

    .composer-surface {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: end;
      gap: var(--ld-space-sm);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-large);
      background: var(--ld-bg-panel);
      padding: var(--ld-space-sm);
      box-shadow: none;
      transition:
        background var(--ld-transition-fast),
        border-color var(--ld-transition-fast),
        box-shadow var(--ld-transition-fast);
    }

    .composer-surface:hover:not(.is-disabled) {
      border-color: var(--ld-line-muted);
      box-shadow: none;
    }

    .composer-surface:focus-within {
      border-color: var(--ld-line-accent-muted);
      box-shadow: 0 0 0 var(--ld-border-width-focus) var(--ld-bg-accent-muted);
    }

    .composer-surface.is-disabled {
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      box-shadow: none;
    }

    textarea {
      box-sizing: border-box;
      min-height: var(--ld-control-medium);
      max-height: 160px;
      width: 100%;
      grid-column: 1;
      grid-row: 1;
      resize: none;
      overflow-y: auto;
      border: 0;
      border-radius: calc(var(--ld-radius-default) - var(--ld-space-2xs));
      background: transparent;
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-normal);
      padding: var(--ld-space-xs) var(--ld-space-sm);
      outline: 0;
    }

    textarea:focus {
      outline: 0;
    }

    textarea::placeholder {
      color: var(--ld-fg-muted);
    }

    .actions {
      display: flex;
      grid-column: 2;
      grid-row: 1;
      min-height: var(--ld-control-medium);
      align-items: center;
      justify-content: flex-end;
    }

		.mention-picker {
			display: grid;
			position: absolute;
			inset: auto var(--ld-space-lg) calc(100% - var(--ld-space-lg) - var(--ld-space-sm));
			z-index: var(--zIndex-dropdown);
			max-height: 180px;
			overflow: auto;
			border: var(--ld-border-muted);
			border-radius: var(--ld-radius-large);
			background: var(--ld-bg-panel);
			padding: var(--ld-space-xs);
			box-shadow: var(--ld-shadow-floating-sm);
		}

		.mention-option {
			display: grid;
			width: 100%;
			height: auto;
			min-height: var(--ld-control-small);
			grid-template-columns: 16px minmax(0, 1fr);
			align-items: center;
			gap: var(--ld-space-xs);
			border: 0;
			border-radius: var(--ld-radius-default);
			background: transparent;
			color: var(--ld-fg-default);
			padding: var(--ld-space-2xs) var(--ld-space-sm);
			box-shadow: none;
			text-align: left;
		}

		.mention-group {
			display: grid;
		}

		.mention-section-label {
			min-width: 0;
			padding: var(--ld-space-xs) var(--ld-space-sm) var(--ld-space-2xs);
			color: var(--ld-fg-muted);
			font-size: var(--ld-font-size-caption);
			font-weight: var(--ld-font-weight-strong);
		}

		.mention-icon {
			display: grid;
			width: 16px;
			height: 16px;
			place-items: center;
			color: var(--ld-fg-muted);
		}

		.mention-icon svg {
			width: 14px;
			height: 14px;
		}

		.mention-copy {
			display: flex;
			min-width: 0;
			align-items: baseline;
			gap: var(--ld-space-sm);
		}

		.mention-title,
		.mention-description {
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
		}

		.mention-description {
			min-width: 0;
			flex: 1 1 auto;
			color: var(--ld-fg-muted);
			font-size: var(--ld-font-size-caption);
		}

		.mention-title {
			flex: 0 1 auto;
		}

		.mention-status {
			display: flex;
			min-height: var(--ld-control-small);
			align-items: center;
			gap: var(--ld-space-sm);
			padding: var(--ld-space-2xs) var(--ld-space-sm);
			color: var(--ld-fg-muted);
			font-size: var(--ld-font-size-caption);
		}

		.mention-status svg {
			width: 14px;
			height: 14px;
		}

		.selected-references {
			display: flex;
			grid-column: 1 / -1;
			grid-row: 1;
			flex-wrap: wrap;
			gap: var(--ld-space-xs);
			padding: var(--ld-space-xs) var(--ld-space-sm) 0;
		}

		.reference-chip {
			display: inline-flex;
			width: auto;
			height: 24px;
			max-width: 100%;
			align-items: center;
			gap: var(--ld-space-xs);
			border: 0;
			border-radius: var(--ld-radius-full);
			background: var(--ld-bg-control);
			color: var(--ld-fg-default);
			padding: 0 var(--ld-space-sm);
			font: inherit;
			font-size: var(--ld-font-size-caption);
			cursor: pointer;
		}

		.reference-chip svg {
			width: 12px;
			height: 12px;
		}

		.composer-surface:has(.selected-references) textarea,
		.composer-surface:has(.selected-references) .actions {
			grid-row: 2;
		}

		.mention-option[data-active='true'],
		.mention-option:hover {
			background: var(--ld-bg-control-hover);
			transform: none;
		}

    .send-button {
      display: inline-flex;
      width: var(--ld-button-height, var(--ld-control-medium));
      height: var(--ld-button-height, var(--ld-control-medium));
      min-width: var(--ld-button-height, var(--ld-control-medium));
      align-items: center;
      justify-content: center;
      border: var(--borderWidth-default, var(--ld-border-width)) solid var(--ld-button-accent-border-rest, var(--ld-accent));
      border-radius: var(--ld-button-radius, var(--ld-radius-default));
      background: var(--ld-button-accent-bg-rest, var(--ld-accent));
      color: var(--ld-button-accent-fg-rest, var(--ld-accent-fg));
      cursor: pointer;
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      padding: 0;
      box-shadow: var(--ld-button-shadow-resting, var(--shadow-resting-small));
      transition:
        background var(--duration-fast) var(--ease-ld),
        border-color var(--duration-fast) var(--ease-ld),
        color var(--duration-fast) var(--ease-ld),
        transform var(--duration-fast) var(--ease-ld);
    }

    .send-button svg {
      width: var(--ld-button-icon-size, var(--base-size-16));
      height: var(--ld-button-icon-size, var(--base-size-16));
    }

    .send-button:hover:not(:disabled) {
      border-color: var(--ld-button-accent-border-hover, var(--ld-accent));
      background: var(--ld-button-accent-bg-hover, var(--ld-accent));
      transform: translateY(-1px);
    }

    .send-button:focus-visible {
      outline: var(--focus-outline, var(--ld-border-default));
      outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
      outline-offset: var(--focus-outline-offset, var(--ld-space-xs));
    }

    .spinner {
      width: 14px;
      height: 14px;
      border: var(--borderWidth-thick) solid transparent;
      border-top-color: currentColor;
      border-radius: 999px;
      animation: spin 0.8s linear infinite;
    }

    @keyframes spin {
      to {
        transform: rotate(360deg);
      }
    }

    .send-button:disabled {
      border-color: var(--ld-button-accent-border-disabled, var(--ld-line-default));
      background: var(--ld-button-accent-bg-disabled, var(--ld-bg-control));
      color: var(--ld-button-accent-fg-disabled, var(--ld-fg-muted));
      cursor: not-allowed;
      opacity: 1;
      box-shadow: none;
    }

    textarea:disabled {
      cursor: not-allowed;
      color: var(--ld-fg-muted);
      opacity: 1;
    }
    @media (max-width: 560px) {
      form {
        width: min(calc(100% - var(--ld-space-md) - var(--ld-space-md)), var(--ld-chat-stack-width));
        padding: calc(var(--ld-space-lg) + var(--ld-space-sm)) var(--ld-space-md) var(--ld-space-md);
      }
    }
  `

  updated(changed: Map<string, unknown>) {
    if (changed.has('value')) {
      this.draft = this.value || ''
      void this.updateComplete.then(() => this.resizeTextarea())
    }
		if (changed.has('suggestions') && this.mentionSearchPending) {
			this.mentionSearchPending = false
		}
  }

  connectedCallback() {
    super.connectedCallback()
    this.draft = this.value || ''
  }

  protected firstUpdated() {
    this.resizeTextarea()
    this.resizeObserver = new ResizeObserver(([entry]) => {
      const width = Math.round(entry?.contentRect.width ?? 0)
      if (width === this.observedWidth) return
      this.observedWidth = width
      this.resizeTextarea()
    })
    this.resizeObserver.observe(this)
  }

  disconnectedCallback() {
    this.resizeObserver?.disconnect()
    this.resizeObserver = undefined
    super.disconnectedCallback()
  }

  public remeasure(): void {
    this.resizeTextarea()
  }

  render() {
    const blocked = this.disabled || this.pending
		const activeMention = this.activeMention()
		const mentionGroups = this.mentionSuggestionGroups()
		const mentions = [...mentionGroups.pinned, ...mentionGroups.global]
		const referenceLimitReached = this.referenceLimitReached()
    return html`
      <form @submit=${this.submit}>
			${activeMention ? html`
				<div class="mention-picker" role="listbox" aria-label="Add LibreDash context" aria-busy=${String(this.mentionSearchPending)}>
					${mentionGroups.pinned.length > 0 ? html`
						<div class="mention-group" role="group" aria-label="On this page">
							<div class="mention-section-label">On this page</div>
							${mentionGroups.pinned.map((reference, index) => this.renderMentionOption(reference, index))}
						</div>
					` : null}
					${mentionGroups.global.length > 0 ? html`
						<div class="mention-group" role="group" aria-label=${mentionGroups.pinned.length > 0 ? 'All accessible' : 'Results'}>
							${mentionGroups.pinned.length > 0 ? html`<div class="mention-section-label">All accessible</div>` : null}
							${mentionGroups.global.map((reference, index) => this.renderMentionOption(reference, mentionGroups.pinned.length + index))}
						</div>
					` : null}
					${mentions.length === 0 ? html`
						<div class="mention-status" role="status">
							${lucideIcon(Search)}<span>${referenceLimitReached
								? `Up to ${this.normalizedReferenceLimit()} items can be attached`
								: this.mentionSearchPending ? 'Searching…' : 'No matching context'}</span>
						</div>
					` : null}
				</div>
			` : null}
        <div class=${['composer-surface', blocked ? 'is-disabled' : ''].filter(Boolean).join(' ')}>
			${this.references.length ? html`
				<div class="selected-references" aria-label="Attached context">
					${this.references.map((reference) => html`
						<button class="reference-chip" type="button" title="Remove ${reference.title}" @click=${() => this.removeReference(reference)}>
							${referenceIcon(reference.kind)}<span>${reference.title}</span>${lucideIcon(X)}
						</button>
					`)}
				</div>
			` : null}
          <textarea
            .value=${this.draft}
            ?disabled=${this.disabled}
            placeholder=${this.placeholder}
            rows="1"
            @input=${this.input}
            @keydown=${this.keydown}
          ></textarea>
          <div class="actions">
            <button
						class="send-button"
              type="submit"
              aria-label=${this.pending ? 'Sending' : 'Send'}
              title="Send"
              ?disabled=${this.disabled || this.pending || this.draft.trim() === ''}
            >
              ${this.pending ? html`<span class="spinner" aria-hidden="true"></span>` : lucideIcon(Send)}
            </button>
          </div>
        </div>
      </form>
    `
  }

  private input(event: Event) {
    const textarea = event.target as HTMLTextAreaElement
    this.draft = textarea.value
		this.mentionIndex = 0
		const mention = this.activeMention(textarea)
		this.requestMentionSearch(mention?.query ?? null)
    this.resizeTextarea(textarea)
  }

	private keydown(event: KeyboardEvent) {
		const mention = this.activeMention()
		const mentions = this.mentionSuggestions()
		if (mention) {
			if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
				if (mentions.length === 0) return
				event.preventDefault()
				const direction = event.key === 'ArrowDown' ? 1 : -1
				this.mentionIndex = (this.mentionIndex + direction + mentions.length) % mentions.length
				void this.updateComplete.then(() => this.scrollActiveMentionIntoView())
				return
			}
			if (event.key === 'Enter' && !event.shiftKey && mentions.length > 0) {
				event.preventDefault()
				this.selectMention(mentions[this.mentionIndex] ?? mentions[0])
				return
			}
			if (event.key === 'Escape') {
				event.preventDefault()
				this.removeActiveMention()
				return
			}
		}
    if (event.key !== 'Enter' || event.shiftKey) return
    event.preventDefault()
    this.dispatchSubmit()
  }

  private submit(event: Event) {
    event.preventDefault()
    this.dispatchSubmit()
  }

  private dispatchSubmit() {
    const input = this.draft.trim()
    if (this.disabled || this.pending || input === '') return
    this.dispatchEvent(new CustomEvent('ld-chat-submit', {
      bubbles: true,
      composed: true,
			detail: { input, references: this.references },
    }))
  }

	private mentionSuggestions(): ChatContextReference[] {
		const groups = this.mentionSuggestionGroups()
		return [...groups.pinned, ...groups.global]
	}

	private mentionSuggestionGroups(): { pinned: ChatContextReference[]; global: ChatContextReference[] } {
		const mention = this.activeMention()
		if (!mention || this.referenceLimitReached()) return { pinned: [], global: [] }
		const query = mention.query.toLocaleLowerCase()
		const selected = new Set(this.references.map(referenceIdentity))
		const pinnedCandidates = uniqueReferences(this.pinnedSuggestions)
			.filter((reference) => !selected.has(referenceIdentity(reference)))
			.filter((reference) => matchesReferenceQuery(reference, query))
		const pinned = pinnedCandidates.slice(0, maxPinnedMentionSuggestions)
		const excluded = new Set([...selected, ...pinnedCandidates.map(referenceIdentity)])
		const global = uniqueReferences(this.suggestions)
			.filter((reference) => !excluded.has(referenceIdentity(reference)))
			.filter((reference) => matchesReferenceQuery(reference, query))
			.slice(0, maxGlobalMentionSuggestions)
		return { pinned, global }
	}

	private renderMentionOption(reference: ChatContextReference, index: number) {
		return html`
			<button
				type="button"
				class="mention-option"
				role="option"
				aria-label=${`${reference.title}, ${reference.kind}${reference.description ? `, ${reference.description}` : ''}`}
				aria-selected=${String(index === this.mentionIndex)}
				data-active=${String(index === this.mentionIndex)}
				@mousedown=${(event: MouseEvent) => event.preventDefault()}
				@click=${() => this.selectMention(reference)}
			>
				<span class="mention-icon" aria-hidden="true">${referenceIcon(reference.kind)}</span>
				<span class="mention-copy">
					<span class="mention-title">${reference.title}</span>
					${reference.description ? html`<span class="mention-description">${reference.description}</span>` : null}
				</span>
			</button>
		`
	}

	private selectMention(reference: ChatContextReference | undefined) {
		if (!reference || this.referenceLimitReached()) return
		this.removeActiveMention()
		if (!this.references.some((current) => referenceIdentity(current) === referenceIdentity(reference))) {
			this.references = [...this.references, reference]
		}
		this.mentionIndex = 0
		this.lastSearchQuery = null
		this.dispatchEvent(new CustomEvent('ld-chat-references-change', {
			bubbles: true,
			composed: true,
			detail: { references: this.references },
		}))
		void this.updateComplete.then(() => this.shadowRoot?.querySelector('textarea')?.focus())
	}

	private removeReference(reference: ChatContextReference) {
		this.references = this.references.filter((current) => referenceIdentity(current) !== referenceIdentity(reference))
		this.notifyReferences()
		this.requestMentionSearch(this.activeMention()?.query ?? null)
	}

	private requestMentionSearch(query: string | null) {
		if (query === null || this.referenceLimitReached()) {
			this.lastSearchQuery = null
			this.mentionSearchPending = false
			return
		}
		if (query === this.lastSearchQuery) return
		this.lastSearchQuery = query
		this.mentionSearchPending = true
		this.dispatchEvent(new CustomEvent('ld-chat-reference-search', {
			bubbles: true,
			composed: true,
			detail: { query },
		}))
	}

	private notifyReferences() {
		this.dispatchEvent(new CustomEvent('ld-chat-references-change', {
			bubbles: true,
			composed: true,
			detail: { references: this.references },
		}))
	}

	private activeMention(textarea = this.shadowRoot?.querySelector('textarea') as HTMLTextAreaElement | null): { start: number; end: number; query: string } | null {
		const end = textarea?.selectionStart ?? this.draft.length
		const beforeCaret = this.draft.slice(0, end)
		const match = beforeCaret.match(/(?:^|\s)@([^@\n]*)$/)
		if (!match) return null
		const start = beforeCaret.lastIndexOf('@')
		return { start, end, query: (match[1] ?? '').trim() }
	}

	private removeActiveMention() {
		const textarea = this.shadowRoot?.querySelector('textarea') as HTMLTextAreaElement | null
		const mention = this.activeMention(textarea)
		if (!mention) return
		const before = this.draft.slice(0, mention.start).replace(/\s+$/, '')
		const after = this.draft.slice(mention.end)
		this.draft = before + after
		this.mentionSearchPending = false
		this.lastSearchQuery = null
		void this.updateComplete.then(() => {
			const next = this.shadowRoot?.querySelector('textarea') as HTMLTextAreaElement | null
			if (!next) return
			next.setSelectionRange(before.length, before.length)
			next.focus()
		})
	}

	private scrollActiveMentionIntoView() {
		const active = this.shadowRoot?.querySelector<HTMLElement>('.mention-option[data-active="true"]')
		active?.scrollIntoView({ block: 'nearest' })
	}

	private normalizedReferenceLimit(): number {
		return Number.isFinite(this.referenceLimit) && this.referenceLimit > 0
			? Math.floor(this.referenceLimit)
			: defaultReferenceLimit
	}

	private referenceLimitReached(): boolean {
		return this.references.length >= this.normalizedReferenceLimit()
	}

  private resizeTextarea(textarea = this.shadowRoot?.querySelector('textarea') as HTMLTextAreaElement | null) {
    if (!textarea) return
    if (textarea.getBoundingClientRect().width <= 0) {
      textarea.style.height = ''
      return
    }
    const maxHeight = 160
    textarea.style.height = 'auto'
    const height = Math.min(textarea.scrollHeight, maxHeight)
    textarea.style.height = `${height}px`
    textarea.style.overflowY = textarea.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }
}

function referenceIdentity(reference: ChatContextReference): string {
	return `${reference.workspaceId ?? ''}:${reference.kind}:${reference.id || reference.componentId || reference.visualId || reference.title}`
}

function uniqueReferences(references: ChatContextReference[]): ChatContextReference[] {
	const seen = new Set<string>()
	return references.filter((reference) => {
		const key = referenceIdentity(reference)
		if (seen.has(key)) return false
		seen.add(key)
		return true
	})
}

const mentionStopWords = new Set(['a', 'an', 'and', 'by', 'for', 'in', 'of', 'on', 'the', 'to'])

function matchesReferenceQuery(reference: ChatContextReference, query: string): boolean {
	const tokens = query
		.toLocaleLowerCase()
		.split(/[^\p{L}\p{N}_]+/u)
		.filter((token) => token !== '' && !mentionStopWords.has(token))
	if (tokens.length === 0) return true
	const haystack = `${reference.title} ${reference.description ?? ''} ${reference.kind}`.toLocaleLowerCase()
	return tokens.every((token) => haystack.includes(token))
}

function referenceIcon(kind: string) {
	switch (kind) {
		case 'dashboard': return lucideIcon(LayoutDashboard)
		case 'page': return lucideIcon(PanelsTopLeft)
		case 'visual': return lucideIcon(BarChart3)
		case 'filter': return lucideIcon(Filter)
		case 'semantic_model': return lucideIcon(Boxes)
		case 'dataset':
		case 'semantic_table': return lucideIcon(Database)
		case 'measure': return lucideIcon(Sigma)
		case 'field': return lucideIcon(Columns3)
		case 'source': return lucideIcon(Plug)
		case 'table': return lucideIcon(Table2)
		case 'asset': return lucideIcon(File)
		default: return lucideIcon(Search)
	}
}

if (!customElements.get('ld-chat-composer')) customElements.define('ld-chat-composer', ChatComposer)
