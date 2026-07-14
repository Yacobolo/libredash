import { LitElement, css, html, nothing } from 'lit'
import { state } from 'lit/decorators.js'
import { CheckCircle2, Clock3, Copy, X, XCircle } from 'lucide'
import type { AdminPageSignal, AdminContentSectionSignal, AdminQueryDetailSignal, AdminQueryHistoryFilters, AdminQueryHistorySignal, AdminStorageSignal, FilterMenuCommand, FilterMenuSignal, RecordTableSignal } from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
import { lucideIcon } from '../shared/lucide-icons'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import '../shared/code-block'
import '../shared/filter-menu'
import '../shared/record-table'
import './agent-tools'
import './agent-prompt-editor'
import './storage-explorer'

const emptyStorage: AdminStorageSignal = {
  summary: {
    catalogPath: '',
    dataPath: '',
    catalogSizeLabel: '',
    dataSizeLabel: '',
    totalSizeLabel: '',
    totalDataSizeLabel: '',
    databaseCount: 0,
    tableCount: 0,
    snapshotCount: 0,
    dataFileCount: 0,
  },
  status: '',
  warnings: [],
  tables: [],
  snapshots: [],
  servingStates: [],
  selectedKey: '',
  selectedTable: null,
}

class LibreDashAdminPage extends DatastarLit(LitElement) {
  @state() private queryFilters: AdminQueryHistoryFilters = {}
  @state() private copiedQueryDetailValue = ''
  private queryFilterTimer: ReturnType<typeof setTimeout> | null = null
  private lastQueryHistoryKey = ''

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
      align-items: start;
      background: var(--ld-bg-app);
    }

    ld-sub-sidebar {
      position: sticky;
      top: 0;
      align-self: start;
      height: 100svh;
    }

    .main {
      display: grid;
      width: min(100%, var(--ld-page-content-max-width));
      min-width: 0;
      min-height: 100svh;
      align-content: start;
      gap: var(--base-size-12);
      box-sizing: border-box;
      justify-self: center;
      padding: var(--base-size-16);
    }

    .main-storage {
      width: 100%;
      grid-template-rows: minmax(0, 1fr);
      align-content: stretch;
      gap: 0;
      justify-self: stretch;
      padding: 0;
    }

    header {
      display: grid;
      min-width: 0;
      gap: var(--base-size-4);
    }

    h1,
    h2,
    p {
      margin: 0;
    }

    .eyebrow {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
      text-transform: uppercase;
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

    .detail {
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .metrics {
      display: grid;
      max-width: var(--ld-workspace-detail-max-width);
      grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
      gap: var(--base-size-12);
    }

    .metric,
    .panel {
      min-width: 0;
      overflow: hidden;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
    }

    .metric {
      display: grid;
      align-content: start;
      gap: var(--base-size-4);
      padding: var(--base-size-16);
    }

    .metric .label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .metric .value {
      overflow: hidden;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-title-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .metric .meta,
    .empty {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .empty {
      padding: var(--base-size-12);
    }

    .warnings {
      display: grid;
      max-width: var(--ld-workspace-detail-max-width);
      gap: var(--base-size-8);
    }

    .warning {
      border: var(--ld-border-attention);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-attention-muted);
      padding: var(--ld-space-control) var(--base-size-12);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
    }

    ld-storage-explorer {
      width: 100%;
      max-width: 100%;
      min-height: 0;
    }

    .section {
      display: grid;
      min-width: 0;
      align-content: start;
      gap: var(--base-size-12);
    }

    h2 {
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
      gap: var(--base-size-12);
    }

    .local-user-panel {
      display: grid;
      max-width: var(--ld-workspace-detail-max-width);
      gap: var(--base-size-12);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--base-size-12);
    }

    .local-user-form {
      display: grid;
      grid-template-columns: minmax(12rem, 1fr) minmax(12rem, 1fr) auto;
      gap: var(--base-size-8);
      align-items: end;
    }

    .local-user-form label {
      display: grid;
      gap: var(--base-size-4);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .local-user-form input {
      min-width: 0;
      min-height: var(--control-medium-size);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-small);
      background: var(--ld-bg-input);
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      padding: 0 var(--base-size-8);
    }

    .local-user-action {
      min-height: var(--control-medium-size);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      cursor: pointer;
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
      padding: 0 var(--base-size-12);
    }

    .local-user-action:hover,
    .local-user-action:focus-visible {
      background: var(--ld-bg-control-hover);
      outline: 0;
    }

    .local-user-action:disabled {
      cursor: not-allowed;
      opacity: 0.64;
    }

    .local-user-result {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .local-user-result code {
      color: var(--ld-fg-default);
    }

    .query-audit {
      display: grid;
      min-width: 0;
      gap: var(--base-size-12);
    }

    .query-filters {
      display: flex;
      flex-wrap: wrap;
      gap: var(--base-size-8);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--base-size-12);
    }

    .query-filter {
      display: grid;
      flex: 1 1 16rem;
      gap: var(--base-size-4);
      min-width: 0;
    }

    .query-filter label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .query-filter input {
      min-width: 0;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-small);
      background: var(--ld-bg-input);
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
      padding: var(--base-size-8) var(--ld-space-control);
    }

    .query-history-footer {
      display: flex;
      min-height: 2.75rem;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-12);
      border-top: var(--ld-border-muted);
      padding: var(--base-size-8) var(--base-size-12);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
    }

    .query-history-error {
      color: var(--ld-fg-danger);
    }

    .query-history-load-more {
      min-height: var(--ld-control-medium);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      cursor: pointer;
      font: inherit;
      padding: 0 var(--base-size-12);
    }

    .query-history-load-more:hover,
    .query-history-load-more:focus-visible {
      background: var(--ld-bg-control-hover, var(--ld-bg-panel-muted));
      outline: 0;
    }

    .query-history-load-more:disabled {
      cursor: not-allowed;
      opacity: 0.64;
    }

    .query-detail-drawer {
      position: fixed;
      z-index: 31;
      inset: 0 0 0 auto;
      display: grid;
      width: min(34rem, 100vw);
      grid-template-rows: auto minmax(0, 1fr);
      border-left: var(--ld-border-muted);
      background: var(--ld-bg-panel);
      box-shadow: var(--ld-shadow-floating-lg);
      color: var(--ld-fg-default);
      animation: query-detail-slide-in 180ms cubic-bezier(0.2, 0, 0, 1) both;
    }

    .query-detail-header {
      border-bottom: var(--ld-border-muted);
      padding: var(--base-size-16);
    }

    .query-detail-header-row,
    .query-detail-copy-row {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: var(--base-size-8);
    }

    .query-detail-header-row {
      justify-content: space-between;
    }

    .query-detail-status {
      display: inline-flex;
      min-width: 0;
      align-items: center;
      gap: var(--base-size-6);
      color: var(--ld-fg-default);
      font-weight: var(--ld-font-weight-strong);
    }

    .query-detail-status svg {
      display: block;
      width: var(--base-size-16);
      height: var(--base-size-16);
    }

    .query-detail-status-success svg {
      color: var(--ld-fg-success);
    }

    .query-detail-status-danger svg {
      color: var(--ld-fg-danger);
    }

    .query-detail-status-attention svg {
      color: var(--ld-fg-warning);
    }

    .query-detail-status-muted svg {
      color: var(--ld-fg-muted);
    }

    .query-detail-close,
    .query-detail-copy {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      font: inherit;
    }

    .query-detail-close {
      width: var(--ld-control-medium);
      height: var(--ld-control-medium);
    }

    .query-detail-copy {
      width: var(--base-size-20);
      height: var(--base-size-20);
      flex: none;
      padding: 0;
    }

    .query-detail-close:hover,
    .query-detail-close:focus-visible,
    .query-detail-copy:hover,
    .query-detail-copy:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover, var(--ld-bg-panel-muted));
      color: var(--ld-fg-default);
      outline: 0;
    }

    .query-detail-body {
      display: grid;
      align-content: start;
      gap: var(--base-size-16);
      min-width: 0;
      overflow: auto;
      padding: var(--base-size-16);
    }

    .query-detail-section {
      display: grid;
      gap: var(--base-size-8);
      min-width: 0;
    }

    .query-detail-section h2,
    .query-detail-section summary {
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .query-detail-facts {
      display: grid;
      gap: var(--base-size-6);
    }

    .query-detail-fact {
      display: grid;
      grid-template-columns: minmax(7rem, 0.44fr) minmax(0, 1fr);
      gap: var(--base-size-12);
      min-width: 0;
      align-items: start;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .query-detail-fact span {
      color: var(--ld-fg-muted);
    }

    .query-detail-fact code,
    .query-detail-fact strong {
      min-width: 0;
      overflow-wrap: anywhere;
    }

    .query-detail-fact code,
    .query-detail-code {
      font-family: var(--fontStack-monospace);
    }

    .query-detail-code {
      max-height: 15rem;
      min-width: 0;
      overflow: auto;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-default);
      margin: 0;
      padding: var(--base-size-12);
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-normal);
      white-space: pre;
    }

    .query-detail-error {
      border-color: var(--ld-line-danger-muted, var(--ld-line-muted));
      background: var(--ld-bg-danger-muted, var(--ld-bg-panel-muted));
    }

    .query-detail-raw {
      border-top: var(--ld-border-muted);
      padding-top: var(--base-size-12);
    }

    .query-detail-raw summary {
      cursor: pointer;
    }

    @keyframes query-detail-slide-in {
      from {
        transform: translateX(100%);
      }
      to {
        transform: translateX(0);
      }
    }

    @keyframes query-detail-mobile-slide-in {
      from {
        transform: translateY(100%);
      }
      to {
        transform: translateY(0);
      }
    }

    @media (prefers-reduced-motion: reduce) {
      .query-detail-drawer {
        animation-duration: 1ms;
      }
    }

    @media (max-width: 640px) {
      .route {
        grid-template-columns: 1fr;
      }

      ld-sub-sidebar {
        position: relative;
        width: 100%;
        height: auto;
      }

      .main {
        padding: var(--base-size-12);
      }

      .query-detail-drawer {
        inset: auto 0 0 0;
        width: 100vw;
        height: min(88svh, 44rem);
        border-top: var(--ld-border-muted);
        border-left: 0;
        box-shadow: var(--ld-shadow-floating-lg);
        animation-name: query-detail-mobile-slide-in;
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    window.addEventListener('keydown', this.handleWindowKeydown)
  }

  disconnectedCallback(): void {
    window.removeEventListener('keydown', this.handleWindowKeydown)
    if (this.queryFilterTimer) clearTimeout(this.queryFilterTimer)
    super.disconnectedCallback()
  }

  updated(): void {
    checkSignalContract('admin page', this.page, { kind: 'required', title: 'required', sidebar: 'required' })
    const historyKey = JSON.stringify(this.currentQueryHistory().filters)
    if (historyKey !== this.lastQueryHistoryKey) {
      this.lastQueryHistoryKey = historyKey
      const history = this.currentQueryHistory()
      this.queryFilters = { ...history.filters }
      if (this.queryDetail?.eventId && !tableRows(history.table).some((row) => String(row.id ?? '') === this.queryDetail?.eventId)) {
        this.closeQueryDetail()
      }
    }
  }

  get page(): AdminPageSignal | null {
    return this.signal<AdminPageSignal | null>('page', null)
  }

  get storage(): AdminStorageSignal {
    return this.signal<AdminStorageSignal>('adminStorage', emptyStorage)
  }

  get queryHistory(): AdminQueryHistorySignal | null {
    return this.signal<AdminQueryHistorySignal | null>('adminQueryHistory', null)
  }

  get queryDetail(): AdminQueryDetailSignal | null {
    return this.signal<AdminQueryDetailSignal | null>('adminQueryDetail', null)
  }

  get agentPrompt(): string {
    return this.signal<string>('adminAgentCommand.systemPrompt', '')
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    return html`
      <div class="route">
        <ld-sub-sidebar .config=${page.sidebar}></ld-sub-sidebar>
        <section class=${page.active === 'storage' ? 'main main-storage' : 'main'} aria-label="Admin">
          ${page.active === 'storage' ? nothing : html`
            <header>
              <p class="eyebrow">Admin</p>
              <h1>${page.headerTitle || page.title}</h1>
              ${page.headerDetail ? html`<p class="detail">${page.headerDetail}</p>` : nothing}
            </header>
          `}
          ${page.empty && page.active !== 'storage' ? html`<div class="panel"><div class="empty">${page.empty}</div></div>` : nothing}
          ${page.metrics?.length && page.active !== 'storage' && page.active !== 'queries' ? html`
            <div class="metrics">
              ${page.metrics.map((metric) => html`
                <div class="metric">
                  <span class="label">${metric.label}</span>
                  <span class="value">${metric.value || '-'}</span>
                  ${metric.detail ? html`<span class="meta">${metric.detail}</span>` : nothing}
                </div>
              `)}
            </div>
          ` : nothing}
          ${this.renderLocalUserAdmin(page)}
          ${page.active === 'storage' ? this.renderStorage(page) : page.active === 'agent' ? this.renderAgent(page) : page.active === 'queries' ? this.renderQueries(page) : page.sections?.map(renderSection)}
        </section>
      </div>
    `
  }

  private renderLocalUserAdmin(page: AdminPageSignal) {
    if (page.active === 'principals') {
      return html`
        <section class="local-user-panel" aria-label="Create local user">
          <form class="local-user-form" method="post" action="/api/v1/principals">
            <input type="hidden" name="gorilla.csrf.Token" value=${csrfToken()}>
            <label>
              Email
              <input name="email" type="email" autocomplete="off" required>
            </label>
            <label>
              Display name
              <input name="displayName" type="text" autocomplete="off">
            </label>
            <button class="local-user-action" type="submit">Create user</button>
          </form>
        </section>
      `
    }
    if (page.active === 'principal-detail') {
      const principalId = principalIDFromPage(page)
      if (!principalId) return nothing
      return html`
        <section class="local-user-panel" aria-label="Reset local password">
          <form method="post" action=${`/api/v1/principals/${encodeURIComponent(principalId)}/password-reset`}>
            <input type="hidden" name="gorilla.csrf.Token" value=${csrfToken()}>
            <button class="local-user-action" type="submit">Reset local password</button>
          </form>
        </section>
      `
    }
    return nothing
  }

  private renderAgent(page: AdminPageSignal) {
    const agent = page.agent
    const systemPrompt = this.agentPrompt || agent?.systemPrompt || ''
    return html`
      ${agent ? html`
        <section class="section" aria-label="System prompt">
          <h2>System prompt</h2>
          <slot name="agent-prompt">
            <ld-agent-prompt-editor value=${systemPrompt} .value=${systemPrompt} ?disabled=${!agent.canWrite}></ld-agent-prompt-editor>
          </slot>
        </section>
        <section class="section" aria-label="Tools">
          <h2>Tools</h2>
          <ld-agent-tools .tools=${agent.tools}></ld-agent-tools>
        </section>
      ` : nothing}
    `
  }

  private renderStorage(page: AdminPageSignal) {
    const storage = storageHasPayload(this.storage) ? this.storage : page.storage ?? emptyStorage
    return html`
      <ld-storage-explorer .storage=${storage}></ld-storage-explorer>
    `
  }

  private renderQueries(page: AdminPageSignal) {
    const history = this.currentQueryHistory(page)
    const rows = tableRows(history.table)
    const detail = this.queryDetail ?? emptyQueryDetail
    return html`
      <section class="query-audit" aria-label="Query audit">
        <div class="query-filters" aria-label="Query event filters" @ld-filter-menu-command=${this.handleFilterMenuCommand}>
          ${history.filterMenus?.map((menu) => this.renderFilterMenu(menu))}
          ${this.renderTextFilter('search', 'Statement / ID')}
        </div>
        <div class="panel" @ld-record-table-action=${this.handleQueryTableAction}>
          <ld-record-table variant="compact" .table=${history.table}></ld-record-table>
          <div class="query-history-footer" aria-live="polite">
            <span class=${history.error ? 'query-history-error' : ''}>${history.error || history.loadedCountLabel || `${rows.length} queries loaded`}</span>
            ${history.hasMore ? html`
              <button
                type="button"
                class="query-history-load-more"
                ?disabled=${history.loading}
                @click=${this.loadMoreQueryHistory}
              >
                ${history.loading ? 'Loading...' : 'Load more'}
              </button>
            ` : nothing}
          </div>
        </div>
        ${detail.eventId || detail.loading || detail.error ? this.renderQueryDetail(detail) : nothing}
      </section>
    `
  }

  private renderTextFilter(key: keyof AdminQueryHistoryFilters, label: string) {
    return html`
      <div class="query-filter">
        <label for=${`query-filter-${key}`}>${label}</label>
        <input
          id=${`query-filter-${key}`}
          type="search"
          .value=${this.queryFilters[key] ?? this.currentQueryHistory().filters[key] ?? ''}
          @input=${(event: Event) => this.setQueryFilter(key, (event.currentTarget as HTMLInputElement).value)}
        >
      </div>
    `
  }

  private renderFilterMenu(menu: FilterMenuSignal) {
    return html`<ld-filter-menu .menu=${menu}></ld-filter-menu>`
  }

  private setQueryFilter(key: keyof AdminQueryHistoryFilters, value: string) {
    const filters = { ...this.queryFilters, [key]: value }
    this.queryFilters = filters
    if (this.queryFilterTimer) clearTimeout(this.queryFilterTimer)
    this.queryFilterTimer = setTimeout(() => {
      this.emitQueryHistoryCommand('reset', filters, '')
    }, 200)
  }

  private handleFilterMenuCommand = (event: CustomEvent<FilterMenuCommand>): void => {
    const command = event.detail
    if (!command?.menuId) return
    const action = command.action === 'search' ? 'filter_search' : command.action === 'clear' ? 'filter_clear' : 'filter_toggle'
    this.emitQueryHistoryCommand(action, this.currentQueryHistory().filters, '', '', command)
  }

  private loadMoreQueryHistory = () => {
    const history = this.currentQueryHistory()
    if (!history.hasMore || history.loading || !history.nextCursor) return
    this.emitQueryHistoryCommand('load_more', history.filters, history.nextCursor)
  }

  private emitQueryHistoryCommand(action: 'reset' | 'load_more' | 'select_detail' | 'close_detail' | 'filter_search' | 'filter_toggle' | 'filter_clear', filters: AdminQueryHistoryFilters, pageToken: string, eventId = '', filterMenu?: FilterMenuCommand) {
    const history = this.currentQueryHistory()
    this.dispatchEvent(new CustomEvent('ld-query-history-command', {
      bubbles: true,
      composed: true,
      detail: {
        action,
        filters,
        pageToken,
        limit: history.limit || 50,
        eventId,
        filterMenu,
      },
    }))
  }

  private currentQueryHistory(page = this.page): AdminQueryHistorySignal {
    const pageHistory = page ? (page as AdminPageSignal & { queryHistory?: AdminQueryHistorySignal }).queryHistory : null
    const history = this.queryHistory ?? pageHistory ?? null
    if (history) return history
    return {
      table: emptyQueryHistoryTable,
      filters: {},
      nextCursor: '',
      loadedCountLabel: '0 queries loaded',
      hasMore: false,
      loading: false,
      error: '',
      limit: 50,
    }
  }

  private handleQueryTableAction = (event: CustomEvent) => {
    if (event.detail?.action !== 'detail') return
    const eventId = String(event.detail.row?.id ?? '')
    if (!eventId) return
    this.copiedQueryDetailValue = ''
    this.emitQueryHistoryCommand('select_detail', this.currentQueryHistory().filters, '', eventId)
  }

  private closeQueryDetail = () => {
    this.copiedQueryDetailValue = ''
    this.emitQueryHistoryCommand('close_detail', this.currentQueryHistory().filters, '')
  }

  private handleWindowKeydown = (event: KeyboardEvent) => {
    const detail = this.queryDetail ?? emptyQueryDetail
    if (event.key !== 'Escape' || (!detail.eventId && !detail.loading && !detail.error)) return
    this.closeQueryDetail()
  }

  private renderQueryDetail(event: AdminQueryDetailSignal) {
    const statusTone = queryEventStatusTone(event.status ?? '')
    return html`
      <aside class="query-detail-drawer" role="dialog" aria-modal="true" aria-label="Query event detail">
        <header class="query-detail-header">
          <div class="query-detail-header-row">
            <div class=${`query-detail-status query-detail-status-${statusTone}`}>
              ${lucideIcon(queryEventStatusIconComponent(event.status ?? ''), { size: 16, strokeWidth: 2 })}
              <span>${event.loading ? 'Loading' : event.statusLabel || queryEventStatusLabel(event.status ?? '')}</span>
            </div>
            <button class="query-detail-close" type="button" aria-label="Close query details" @click=${this.closeQueryDetail}>
              ${lucideIcon(X, { size: 18, strokeWidth: 2 })}
            </button>
          </div>
        </header>
        <div class="query-detail-body">
          ${event.loading ? html`<section class="query-detail-section"><p class="detail">Loading query details...</p></section>` : nothing}
          ${event.error && !event.status ? html`<section class="query-detail-section"><pre class="query-detail-code query-detail-error"><code>${event.error}</code></pre></section>` : nothing}
          <section class="query-detail-section" aria-label="Query identity">
            <h2>Query identity</h2>
            <div class="query-detail-facts">
              ${this.renderCopyableFact('ID', event.eventId)}
              ${this.renderCopyableFact('Request ID', event.requestId)}
              ${this.renderCopyableFact('Correlation ID', event.correlationId)}
            </div>
          </section>
          <section class="query-detail-section" aria-label="Query text">
            <h2>Query text</h2>
            <ld-code-block language="sql" format copy .code=${event.sql || event.eventId || ''}></ld-code-block>
          </section>
          <section class="query-detail-section" aria-label="Timing">
            <h2>Timing</h2>
            <div class="query-detail-facts">
              ${queryDetailFact('Duration', `${event.durationMs ?? 0} ms`)}
              ${queryDetailFact('Planning', `${event.planningMs ?? 0} ms`)}
              ${queryDetailFact('Connection wait', `${event.connectionWaitMs ?? 0} ms`)}
              ${queryDetailFact('Database', `${event.databaseMs ?? 0} ms`)}
              ${queryDetailFact('Started at', event.createdAt)}
              ${queryDetailFact('Operation', event.operation)}
              ${queryDetailFact('Kind', event.queryKind)}
            </div>
          </section>
          <section class="query-detail-section" aria-label="Query target">
            <h2>Query target</h2>
            <div class="query-detail-facts">
              ${queryDetailFact('Workspace', event.workspaceId)}
              ${queryDetailFact('Principal', event.principalId)}
              ${queryDetailFact('Source type', event.surface)}
              ${queryDetailFact('Model', event.modelId)}
              ${queryDetailFact('Target', event.target)}
              ${queryDetailFact('Object', queryDetailObjectLabel(event))}
            </div>
          </section>
          <section class="query-detail-section" aria-label="Result">
            <h2>Result</h2>
            <div class="query-detail-facts">
              ${queryDetailFact('Rows returned', String(event.rowsReturned ?? 0))}
              ${queryDetailFact('Status', event.status)}
            </div>
            ${event.queryError ? html`<pre class="query-detail-code query-detail-error"><code>${event.queryError}</code></pre>` : nothing}
          </section>
          ${event.planText || event.queryJson ? html`
            <details class="query-detail-raw">
              <summary>Raw metadata</summary>
              ${event.planText ? html`<pre class="query-detail-code"><code>${event.planText}</code></pre>` : nothing}
              ${event.queryJson ? html`<pre class="query-detail-code"><code>${formatQueryJSON(event.queryJson)}</code></pre>` : nothing}
            </details>
          ` : nothing}
        </div>
      </aside>
    `
  }

  private renderCopyableFact(label: string, value: string | undefined | null) {
    const normalized = value == null || value === '' ? '-' : String(value)
    return html`
      <div class="query-detail-fact">
        <span>${label}</span>
        <div class="query-detail-copy-row">
          <code>${normalized}</code>
          ${normalized !== '-' ? html`
            <button
              type="button"
              class="query-detail-copy"
              aria-label=${`Copy ${label}`}
              title=${this.copiedQueryDetailValue === normalized ? 'Copied' : `Copy ${label}`}
              @click=${() => this.copyQueryDetailValue(normalized)}
            >
              ${lucideIcon(Copy, { size: 13, strokeWidth: 2 })}
            </button>
          ` : nothing}
        </div>
      </div>
    `
  }

  private async copyQueryDetailValue(value: string): Promise<void> {
    try {
      await navigator.clipboard?.writeText(value)
      this.copiedQueryDetailValue = value
    } catch {
      this.copiedQueryDetailValue = ''
    }
  }

}

const emptyQueryHistoryTable: RecordTableSignal = {
  columns: [],
  rows: [],
  empty: 'No query events match these filters.',
}

const emptyQueryDetail: AdminQueryDetailSignal = {
  eventId: '',
  loading: false,
  error: '',
}

function tableRows(table: RecordTableSignal | undefined | null): Array<Record<string, unknown>> {
  return Array.isArray(table?.rows) ? table.rows as Array<Record<string, unknown>> : []
}

function queryDetailObjectLabel(event: AdminQueryDetailSignal): string {
  const object = [event.objectType, event.objectId].filter(Boolean).join(':')
  if (object) return object
  return [event.modelId, event.target].filter(Boolean).join(':') || '-'
}

function queryEventStatusTone(status: string): string {
  switch (status) {
    case 'success':
      return 'success'
    case 'canceled':
      return 'muted'
    case 'timeout':
      return 'attention'
    default:
      return 'danger'
  }
}

function queryEventStatusIcon(status: string): string {
  switch (status) {
    case 'success':
      return 'check'
    case 'canceled':
    case 'timeout':
      return 'clock'
    default:
      return 'x'
  }
}

function queryEventStatusIconComponent(status: string): any {
  switch (queryEventStatusIcon(status)) {
    case 'check':
      return CheckCircle2
    case 'clock':
      return Clock3
    default:
      return XCircle
  }
}

function queryEventStatusLabel(status: string): string {
  switch (status) {
    case 'success':
      return 'Finished'
    case 'canceled':
      return 'Canceled'
    case 'timeout':
      return 'Timeout'
    default:
      return status || 'Error'
  }
}

function queryDetailFact(label: string, value: string | number | undefined | null) {
  return html`
    <div class="query-detail-fact">
      <span>${label}</span>
      <code>${value == null || value === '' ? '-' : String(value)}</code>
    </div>
  `
}

function formatQueryJSON(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function storageHasPayload(storage: AdminStorageSignal | null | undefined): storage is AdminStorageSignal {
  if (!storage) return false
  return Boolean(storage.tables?.length || storage.status || storage.selectedKey || storage.selectedTable || storage.warnings?.length)
}

function csrfToken(): string {
  const token = document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content.trim() ?? ''
  return token
}

function principalIDFromPage(page: AdminPageSignal): string {
  const metric = page.metrics?.find((item) => item.label === 'Principal ID')
  return metric?.value ?? ''
}

function renderSection(section: AdminContentSectionSignal) {
  return html`
    <section class="section" aria-label=${section.title}>
      <h2>${section.title}</h2>
      ${section.table?.columns?.length
        ? html`<div class="panel"><ld-record-table variant="compact" .table=${section.table}></ld-record-table></div>`
        : html`<div class="facts">${section.facts?.map((fact) => html`
          <div class="metric">
            <span class="label">${fact.label}</span>
            <span class="value">${fact.value || '-'}</span>
          </div>
        `)}</div>`}
    </section>
  `
}

if (!customElements.get('ld-admin-page')) customElements.define('ld-admin-page', LibreDashAdminPage)
