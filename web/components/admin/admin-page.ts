import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { CheckCircle2, Clock3, Copy, X, XCircle } from 'lucide'
import type { AdminPageSignal, AdminContentSectionSignal, AdminQueryEventSignal, AdminStorageSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { lucideIcon } from '../shared/lucide-icons'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import '../shared/code-block'
import '../shared/record-table'
import './agent-tools'
import './agent-prompt-editor'
import './storage-explorer'

const emptyStorage: AdminStorageSignal = {
  summary: { duckdbDir: '', databaseCount: 0, totalSizeLabel: '', tableCount: 0 },
  status: '',
  warnings: [],
  tables: [],
  selectedKey: '',
  selectedTable: null,
}

class LibreDashAdminPage extends LitElement {
  @property({ converter: jsonAttribute<AdminPageSignal | null>(null) }) page: AdminPageSignal | null = null
  @property({ converter: jsonAttribute<AdminStorageSignal>(emptyStorage) }) storage: AdminStorageSignal = emptyStorage
  @property({ attribute: 'agent-prompt' }) agentPrompt = ''
  @state() private queryFilters: QueryAuditFilters = {}
  @state() private selectedQueryEventID = ''
  @state() private copiedQueryDetailValue = ''

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

    .query-audit {
      display: grid;
      min-width: 0;
      gap: var(--base-size-12);
    }

    .query-filters {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
      gap: var(--base-size-8);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--base-size-12);
    }

    .query-filter {
      display: grid;
      gap: var(--base-size-4);
      min-width: 0;
    }

    .query-filter label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .query-filter input,
    .query-filter select {
      min-width: 0;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-small, 6px);
      background: var(--ld-bg-input, var(--ld-bg-app));
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
      padding: var(--base-size-8) var(--base-size-10);
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
      box-shadow: var(--ld-shadow-floating, -12px 0 36px rgba(31, 35, 40, 0.18));
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
      color: var(--ld-fg-success, #1a7f37);
    }

    .query-detail-status-danger svg {
      color: var(--ld-fg-danger, #d1242f);
    }

    .query-detail-status-attention svg {
      color: var(--ld-fg-warning, #9a6700);
    }

    .query-detail-status-muted svg {
      color: var(--ld-fg-muted);
    }

    .query-detail-close,
    .query-detail-copy {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border: var(--ld-border-transparent, 1px solid transparent);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      font: inherit;
    }

    .query-detail-close {
      width: var(--control-medium-size, 32px);
      height: var(--control-medium-size, 32px);
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
        box-shadow: var(--ld-shadow-floating, 0 -12px 36px rgba(31, 35, 40, 0.18));
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
    super.disconnectedCallback()
  }

  updated(): void {
    checkSignalContract('admin page', this.page, { kind: 'required', title: 'required', sidebar: 'required' })
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
          ${page.active === 'storage' ? this.renderStorage(page) : page.active === 'agent' ? this.renderAgent(page) : page.active === 'queries' ? this.renderQueries(page) : page.sections?.map(renderSection)}
        </section>
      </div>
    `
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
    const events = page.queryEvents ?? []
    const filtered = filterQueryEvents(events, this.queryFilters)
    const selected = filtered.find((event) => event.id === this.selectedQueryEventID) ?? events.find((event) => event.id === this.selectedQueryEventID) ?? null
    return html`
      <section class="query-audit" aria-label="Query audit">
        <div class="query-filters" aria-label="Query event filters">
          ${this.renderTextFilter('workspace', 'Workspace')}
          ${this.renderTextFilter('principal', 'User')}
          ${this.renderSelectFilter('surface', 'Source', uniqueValues(events.map((event) => event.surface)))}
          ${this.renderSelectFilter('kind', 'Kind', uniqueValues(events.map((event) => event.queryKind)))}
          ${this.renderSelectFilter('status', 'Status', uniqueValues(events.map((event) => event.status)))}
          ${this.renderTextFilter('target', 'Target')}
          ${this.renderTextFilter('search', 'Statement / ID')}
        </div>
        <div class="panel" @ld-record-table-action=${this.handleQueryTableAction}>
          <ld-record-table variant="compact" .table=${queryEventsTable(filtered)}></ld-record-table>
        </div>
        ${selected ? this.renderQueryDetail(selected) : nothing}
      </section>
    `
  }

  private renderTextFilter(key: keyof QueryAuditFilters, label: string) {
    return html`
      <div class="query-filter">
        <label for=${`query-filter-${key}`}>${label}</label>
        <input
          id=${`query-filter-${key}`}
          type="search"
          .value=${this.queryFilters[key] ?? ''}
          @input=${(event: Event) => this.setQueryFilter(key, (event.currentTarget as HTMLInputElement).value)}
        >
      </div>
    `
  }

  private renderSelectFilter(key: keyof QueryAuditFilters, label: string, values: string[]) {
    return html`
      <div class="query-filter">
        <label for=${`query-filter-${key}`}>${label}</label>
        <select
          id=${`query-filter-${key}`}
          .value=${this.queryFilters[key] ?? ''}
          @change=${(event: Event) => this.setQueryFilter(key, (event.currentTarget as HTMLSelectElement).value)}
        >
          <option value="">All</option>
          ${values.map((value) => html`<option value=${value}>${value}</option>`)}
        </select>
      </div>
    `
  }

  private setQueryFilter(key: keyof QueryAuditFilters, value: string) {
    this.queryFilters = { ...this.queryFilters, [key]: value }
  }

  private handleQueryTableAction = (event: CustomEvent) => {
    if (event.detail?.action !== 'detail') return
    this.selectedQueryEventID = String(event.detail.row?.id ?? '')
    this.copiedQueryDetailValue = ''
  }

  private closeQueryDetail = () => {
    this.selectedQueryEventID = ''
    this.copiedQueryDetailValue = ''
  }

  private handleWindowKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'Escape' || !this.selectedQueryEventID) return
    this.closeQueryDetail()
  }

  private renderQueryDetail(event: AdminQueryEventSignal) {
    const statusTone = queryEventStatusTone(event.status)
    return html`
      <aside class="query-detail-drawer" role="dialog" aria-modal="true" aria-label="Query event detail">
        <header class="query-detail-header">
          <div class="query-detail-header-row">
            <div class=${`query-detail-status query-detail-status-${statusTone}`}>
              ${lucideIcon(queryEventStatusIconComponent(event.status), { size: 16, strokeWidth: 2 })}
              <span>${queryEventStatusLabel(event.status)}</span>
            </div>
            <button class="query-detail-close" type="button" aria-label="Close query details" @click=${this.closeQueryDetail}>
              ${lucideIcon(X, { size: 18, strokeWidth: 2 })}
            </button>
          </div>
        </header>
        <div class="query-detail-body">
          <section class="query-detail-section" aria-label="Query identity">
            <h2>Query identity</h2>
            <div class="query-detail-facts">
              ${this.renderCopyableFact('ID', event.id)}
              ${this.renderCopyableFact('Request ID', event.requestId)}
              ${this.renderCopyableFact('Correlation ID', event.correlationId)}
            </div>
          </section>
          <section class="query-detail-section" aria-label="Query text">
            <h2>Query text</h2>
            <ld-code-block language="sql" format copy .code=${queryEventExpandedContent(event)}></ld-code-block>
          </section>
          <section class="query-detail-section" aria-label="Timing">
            <h2>Timing</h2>
            <div class="query-detail-facts">
              ${queryDetailFact('Duration', `${event.durationMs ?? 0} ms`)}
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
              ${queryDetailFact('Object', queryEventObjectLabel(event))}
            </div>
          </section>
          <section class="query-detail-section" aria-label="Result">
            <h2>Result</h2>
            <div class="query-detail-facts">
              ${queryDetailFact('Rows returned', String(event.rowsReturned ?? 0))}
              ${queryDetailFact('Status', event.status)}
            </div>
            ${event.error ? html`<pre class="query-detail-code query-detail-error"><code>${event.error}</code></pre>` : nothing}
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

type QueryAuditFilters = {
  workspace?: string
  principal?: string
  surface?: string
  kind?: string
  status?: string
  target?: string
  search?: string
}

function filterQueryEvents(events: AdminQueryEventSignal[], filters: QueryAuditFilters): AdminQueryEventSignal[] {
  return events.filter((event) => {
    if (!matchesText(event.workspaceId, filters.workspace)) return false
    if (!matchesText(event.principalId, filters.principal)) return false
    if (!matchesExact(event.surface, filters.surface)) return false
    if (!matchesExact(event.queryKind, filters.kind)) return false
    if (!matchesExact(event.status, filters.status)) return false
    if (!matchesText(event.target, filters.target)) return false
    if (!matchesText(querySearchText(event), filters.search)) return false
    return true
  })
}

function queryEventsTable(events: AdminQueryEventSignal[]) {
  return {
    columns: [
      { id: 'query', header: 'Query', kind: 'query', width: '560px', toggleable: false },
      { id: 'started_at', header: 'Started', width: '150px' },
      { id: 'duration_ms', header: 'Duration', kind: 'number', align: 'right', width: '105px' },
      { id: 'source', header: 'Source type', width: '120px' },
      { id: 'runtime', header: 'Runtime', kind: 'code', width: '130px' },
      { id: 'principal_id', header: 'User', kind: 'code', width: '150px' },
      { id: 'rows_returned', header: 'Rows', kind: 'number', align: 'right', width: '90px' },
      { id: 'operation', header: 'Operation', kind: 'code', width: '145px' },
      { id: 'kind', header: 'Kind', kind: 'code', width: '170px' },
      { id: 'model', header: 'Model', kind: 'code', width: '130px' },
      { id: 'target', header: 'Target', kind: 'code', width: '150px' },
      { id: 'object', header: 'Object', kind: 'code', width: '220px' },
      { id: 'request_id', header: 'Request ID', kind: 'code', width: '170px' },
      { id: 'correlation_id', header: 'Correlation ID', kind: 'code', width: '170px' },
      { id: 'error', header: 'Error', kind: 'code', width: '220px' },
    ],
    rows: events.map((event) => ({
      id: event.id,
      query: {
        label: queryEventStatement(event),
        statusLabel: event.status,
        tone: queryEventStatusTone(event.status),
        icon: queryEventStatusIcon(event.status),
        expandedContent: queryEventExpandedContent(event),
      },
      started_at: event.createdAt,
      duration_ms: { label: `${event.durationMs ?? 0} ms`, value: event.durationMs ?? 0 },
      source: event.surface,
      runtime: queryEventRuntimeLabel(event),
      principal_id: event.principalId,
      rows_returned: event.rowsReturned,
      operation: event.operation,
      kind: event.queryKind,
      model: event.modelId,
      target: event.target,
      object: queryEventObjectLabel(event),
      request_id: event.requestId,
      correlation_id: event.correlationId,
      error: event.error,
    })),
    empty: 'No query events match these filters.',
    minWidth: '1305px',
    density: 'tight',
    rowAction: 'detail',
    columnSelector: {
      enabled: true,
      label: 'Columns',
      defaultColumns: ['started_at', 'duration_ms', 'source', 'runtime', 'principal_id', 'rows_returned'],
    },
  }
}

function queryEventStatement(event: AdminQueryEventSignal): string {
  const sql = collapseWhitespace(event.sql)
  if (sql) return sql
  const parts = [event.operation, event.queryKind, [event.modelId, event.target].filter(Boolean).join('.')]
    .map((part) => collapseWhitespace(part))
    .filter(Boolean)
  return parts.join(' · ') || event.id
}

function queryEventExpandedContent(event: AdminQueryEventSignal): string {
  return event.sql || queryEventStatement(event)
}

function queryEventObjectLabel(event: AdminQueryEventSignal): string {
  const object = [event.objectType, event.objectId].filter(Boolean).join(':')
  if (object) return object
  return [event.modelId, event.target].filter(Boolean).join(':') || '-'
}

function queryEventRuntimeLabel(event: AdminQueryEventSignal): string {
  return event.workspaceId || '-'
}

function collapseWhitespace(value: string | undefined | null): string {
  return String(value ?? '').replace(/\s+/g, ' ').trim()
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

function uniqueValues(values: Array<string | undefined | null>): string[] {
  return Array.from(new Set(values.map((value) => String(value ?? '').trim()).filter(Boolean))).sort()
}

function matchesExact(value: string, filter = ''): boolean {
  return !filter || value === filter
}

function matchesText(value: string, filter = ''): boolean {
  return !filter || String(value ?? '').toLowerCase().includes(filter.toLowerCase())
}

function querySearchText(event: AdminQueryEventSignal): string {
  return [
    event.workspaceId,
    event.principalId,
    event.surface,
    event.operation,
    event.queryKind,
    event.modelId,
    event.target,
    event.objectType,
    event.objectId,
    event.requestId,
    event.correlationId,
    event.status,
    event.error,
    event.sql,
    event.planText,
    event.queryJson,
  ].join(' ')
}

function storageHasPayload(storage: AdminStorageSignal | null | undefined): storage is AdminStorageSignal {
  if (!storage) return false
  return Boolean(storage.tables?.length || storage.status || storage.selectedKey || storage.selectedTable || storage.warnings?.length)
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
