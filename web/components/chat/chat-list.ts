import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { MoreHorizontal, Search } from 'lucide'
import type { ChatConversationSummary } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { lucideIcon } from '../shared/lucide-icons'

class LibreDashChatList extends LitElement {
  @property({ converter: jsonAttribute<ChatConversationSummary[]>([]) }) conversations: ChatConversationSummary[] = []
  @property({ attribute: 'active-conversation-id' }) activeConversationId = ''
  @state() private search = ''

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    .shell {
      display: grid;
      align-content: start;
      gap: var(--base-size-16);
      width: 100%;
      margin: 0 auto;
      padding: var(--base-size-16);
      box-sizing: border-box;
    }

    .header {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-12);
    }

    h2 {
      margin: 0;
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-title-md);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
      letter-spacing: 0;
    }

    .toolbar {
      position: relative;
      display: block;
      min-width: 0;
    }

    .search-icon {
      position: absolute;
      top: 50%;
      left: var(--base-size-12, 12px);
      display: grid;
      width: var(--base-size-16);
      height: var(--base-size-16);
      place-items: center;
      color: var(--ld-fg-muted);
      transform: translateY(-50%);
      pointer-events: none;
    }

    .search {
      width: 100%;
      min-width: 0;
      height: var(--control-large-size);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      padding: 0 var(--base-size-12) 0 var(--base-size-36, 36px);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
    }

    .search::placeholder {
      color: var(--ld-fg-muted);
      opacity: 1;
    }

    .search:focus-visible {
      border-color: var(--ld-accent);
      outline: 0;
      box-shadow: 0 0 0 var(--ld-border-width-focus, 2px) color-mix(in srgb, var(--ld-accent), transparent 78%);
    }

    .new-chat-link {
      display: inline-flex;
      min-height: var(--control-medium-size);
      flex: 0 0 auto;
      align-items: center;
      justify-content: center;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      padding: 0 var(--control-medium-paddingInline-spacious, var(--base-size-16));
      text-decoration: none;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .new-chat-link:hover,
    .new-chat-link:focus-visible {
      background: var(--ld-bg-hover);
      outline: 0;
    }

    .table-wrap {
      min-width: 0;
      overflow-x: auto;
      padding: var(--base-size-4);
      margin: calc(-1 * var(--base-size-4));
    }

    table {
      width: 100%;
      min-width: 280px;
      border-collapse: separate;
      border-spacing: 0;
      table-layout: auto;
    }

    thead th {
      width: 0;
      height: 0;
      overflow: hidden;
      padding: 0;
      border: 0;
      line-height: 0;
    }

    tbody tr {
      position: relative;
      color: var(--ld-fg-default);
      cursor: pointer;
    }

    td {
      height: var(--control-medium-size);
      border-bottom: var(--ld-border-muted);
      background-clip: padding-box;
      padding: var(--base-size-10) var(--base-size-12, 12px);
      vertical-align: middle;
    }

    tbody tr:first-child td {
      border-top: 0;
    }

    tbody tr:last-child td {
      border-bottom: 0;
    }

    tbody tr:hover td,
    tbody tr:focus-within td,
    tbody tr[data-active='true'] td {
      border-bottom-color: transparent;
      background: var(--ld-bg-hover);
    }

    tbody tr:hover td:first-child,
    tbody tr:focus-within td:first-child,
    tbody tr[data-active='true'] td:first-child {
      border-top-left-radius: var(--ld-radius-default);
      border-bottom-left-radius: var(--ld-radius-default);
    }

    tbody tr:hover td:last-child,
    tbody tr:focus-within td:last-child,
    tbody tr[data-active='true'] td:last-child {
      border-top-right-radius: var(--ld-radius-default);
      border-bottom-right-radius: var(--ld-radius-default);
    }

    .primary-cell {
      position: relative;
      overflow: hidden;
    }

    .primary-link {
      position: absolute;
      z-index: 1;
      inset: 0;
      border-radius: var(--ld-radius-default);
      outline: 0;
    }

    .primary-link:focus-visible {
      box-shadow: inset 0 0 0 var(--ld-border-width-focus, 2px) var(--ld-accent);
    }

    .row-content {
      display: flex;
      min-height: var(--control-medium-size);
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-16);
    }

    .title {
      overflow: hidden;
      min-width: 0;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium, 500);
    }

    .date {
      flex: 0 0 auto;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      white-space: nowrap;
      transition: opacity var(--duration-fast, 160ms) var(--ease-ld, ease);
    }

    .row-actions {
      position: absolute;
      z-index: 2;
      inset-block: 0;
      right: 0;
      display: flex;
      align-items: center;
      padding-right: var(--base-size-8);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--duration-fast, 160ms) var(--ease-ld, ease);
    }

    tbody tr:hover .date,
    tbody tr:focus-within .date {
      opacity: 0;
    }

    tbody tr:hover .row-actions,
    tbody tr:focus-within .row-actions {
      opacity: 1;
      pointer-events: auto;
    }

    .options-button {
      display: grid;
      width: var(--control-medium-size, 32px);
      height: var(--control-medium-size, 32px);
      place-items: center;
      border: 0;
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      padding: 0;
    }

    .options-button:hover,
    .options-button:focus-visible {
      background: var(--ld-bg-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    svg {
      width: var(--base-size-16);
      height: var(--base-size-16);
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .empty {
      padding: var(--base-size-16) 0;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
    }

    @media (max-width: 640px) {
      .shell {
        gap: var(--base-size-12);
        padding: var(--base-size-16, 16px);
      }

      .header {
        display: grid;
      }

      h2 {
        font-size: var(--ld-font-size-title-md);
      }

      .new-chat-link {
        width: 100%;
      }

      tbody tr:hover .date,
      tbody tr:focus-within .date {
        opacity: 1;
      }
    }
  `

  render() {
    const conversations = Array.isArray(this.conversations) ? this.conversations : []
    const query = this.search.trim().toLocaleLowerCase()
    const visible = query
      ? conversations.filter((conversation) => conversationTitle(conversation).toLocaleLowerCase().includes(query))
      : conversations
    const empty = query ? 'No matching chats.' : 'No chats yet.'

    return html`
      <section class="shell" aria-label="Chat history">
        <div class="header">
          <h2>Chats</h2>
          <a class="new-chat-link" href="/chat/new">New chat</a>
        </div>
        <label class="toolbar">
          <span class="search-icon" aria-hidden="true">${lucideIcon(Search)}</span>
          <input
            class="search"
            type="search"
            aria-label="Search chats"
            placeholder="Search chats..."
            autocomplete="off"
            spellcheck="false"
            .value=${this.search}
            @input=${this.onSearchInput}
          >
        </label>
        ${visible.length === 0 ? html`<span class="empty">${empty}</span>` : html`
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">Conversation</th>
                </tr>
              </thead>
              <tbody>
                ${visible.map((conversation) => this.renderRow(conversation))}
              </tbody>
            </table>
          </div>
        `}
      </section>
    `
  }

  private renderRow(conversation: ChatConversationSummary) {
    const title = conversationTitle(conversation)
    const href = `/chat/${conversation.id}`
    return html`
      <tr data-active=${String(conversation.id === this.activeConversationId)}>
        <td class="primary-cell">
          <a class="primary-link" href=${href} data-primary="true" aria-label=${title}></a>
          <div class="row-content">
            <span class="title">${title}</span>
            <time class="date" datetime=${conversation.updatedAt}>${conversation.updatedAt ? shortDate(conversation.updatedAt) : ''}</time>
          </div>
          <div class="row-actions">
            <button class="options-button" type="button" aria-label=${`More options for ${title}`} @click=${(event: Event) => this.emitOptions(event, conversation)}>
              ${lucideIcon(MoreHorizontal)}
            </button>
          </div>
        </td>
      </tr>
    `
  }

  private onSearchInput = (event: Event): void => {
    this.search = (event.target as HTMLInputElement).value
  }

  private emitOptions(event: Event, conversation: ChatConversationSummary): void {
    event.preventDefault()
    event.stopPropagation()
    this.dispatchEvent(new CustomEvent('ld-chat-list-options', {
      detail: { conversationId: conversation.id },
      bubbles: true,
      composed: true,
    }))
  }
}

function conversationTitle(conversation: ChatConversationSummary): string {
  return conversation.title || 'Conversation'
}

function shortDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date)
}

if (!customElements.get('ld-chat-list')) customElements.define('ld-chat-list', LibreDashChatList)
