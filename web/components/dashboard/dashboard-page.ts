import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { RefreshCw } from 'lucide'
import type {
  DashboardComponentSignal,
  DashboardComponentStatus,
  DashboardFilters,
  DashboardInteractionSelection,
  DashboardInteractionSelectionEntry,
  DashboardPageNavSignal,
  DashboardPageSignal,
  DashboardStatus,
  DashboardTable,
  DashboardVisual,
  ReportFilterConfig,
} from '../../generated/signals'
import { lucideIcon } from '../shared/lucide-icons'
import { DatastarLit } from '../shared/datastar-lit'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import './filters/filter-dock'
import './report-canvas'
import './report-footer'
import './visual-modal'
import { loadDashboardComponent } from './registry'
import {
  applyOptimisticInteraction,
  canonicalSelectionEntriesForSource,
  validateInteractionCommand,
  type CanonicalInteractionSelection,
  type InteractionConfigLike,
  type OptimisticInteractionCommand,
} from './interaction-selection'

const emptyFilters: DashboardFilters = { controls: {}, selections: [] }
const emptyStatus: DashboardStatus = {
  loading: false,
  error: '',
  generation: 0,
  lastUpdated: '',
  refreshId: '',
  setupRequired: false,
  progressPercent: 100,
}

type DashboardRenderSnapshot = {
  page: DashboardPageSignal
  filterConfig: ReportFilterConfig[]
  filters: DashboardFilters
  filterOptions: Record<string, unknown>
  visuals: Record<string, DashboardVisual>
  tables: Record<string, DashboardTable>
  status: DashboardStatus
  componentStatus: Record<string, DashboardComponentStatus>
}

type DashboardRefreshProgress = {
  active: boolean
  complete: boolean
  generation: number
  percent: number
}

class LibreDashDashboardPage extends DatastarLit(LitElement) {
  @state() private unsupportedKinds = new Set<string>()
  @state() private optimisticSelections: CanonicalInteractionSelection[] | null = null
  @state() private optimisticTargetKeys = new Set<string>()
  private optimisticExpectedGeneration = 0
  private optimisticRollbackTimer?: ReturnType<typeof setTimeout>
  private renderSnapshot?: DashboardRenderSnapshot
  private visualProjectionCache = new Map<string, { signature: string; value: DashboardVisual }>()
  private tableProjectionCache = new Map<string, { signature: string; value: DashboardTable }>()

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
      grid-template-rows: auto minmax(0, 1fr) auto;
      overflow: hidden;
      background: var(--ld-bg-app);
    }

    .header {
      display: grid;
      min-width: 0;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: center;
      gap: var(--base-size-8);
      border-bottom: var(--ld-border-muted);
      padding: var(--ld-space-control) var(--base-size-16);
    }

    .title-block {
      min-width: 0;
    }

    h1,
    h2,
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

    .detail {
      margin-top: var(--base-size-4);
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .actions {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: flex-end;
      gap: var(--base-size-8);
    }

    button {
      font: inherit;
    }

    .icon-button {
      display: inline-grid;
      width: var(--control-medium-size);
      height: var(--control-medium-size);
      min-height: var(--control-medium-size);
      place-items: center;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0;
    }

    .icon-button:hover,
    .icon-button:focus-visible {
      background: var(--ld-bg-control-hover);
      outline: 0;
    }

    .icon-button[disabled] {
      cursor: not-allowed;
      color: var(--ld-fg-muted);
      opacity: 0.64;
    }

    .body {
      position: relative;
      display: grid;
      min-width: 0;
      min-height: 0;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: stretch;
      overflow: hidden;
    }

    .dashboard-refresh-progress {
      position: absolute;
      inset: 0 0 auto;
      z-index: var(--zIndex-sticky, 50);
      height: 2px;
      overflow: hidden;
      background: var(--ld-line-muted);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--motion-transition-stateChange);
      transition-delay: 0s;
    }

    .dashboard-refresh-progress[data-active='true'] {
      opacity: 1;
      transition-delay: 0s;
    }

    .dashboard-refresh-progress[data-active='false'][data-complete='true'] {
      transition-delay: 180ms;
    }

    .dashboard-refresh-progress-value {
      width: 0;
      height: 100%;
      background: var(--ld-line-accent);
      transition: width var(--motion-transition-stateChange);
    }

    .canvas-wrap {
      display: grid;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
      background: transparent;
      padding: var(--base-size-20) var(--base-size-24);
    }

    .heading-visual {
      display: grid;
      height: 100%;
      min-height: 0;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: center;
      gap: var(--base-size-12);
      padding: var(--base-size-8);
    }

    .eyebrow {
      margin-bottom: var(--base-size-4);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
      text-transform: uppercase;
    }

    .heading-visual h2 {
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-title-lg);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-tight);
    }

    .badges {
      display: flex;
      flex-wrap: wrap;
      justify-content: flex-end;
      gap: var(--base-size-8);
    }

    .badge {
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-muted);
      padding: var(--base-size-2) var(--base-size-8);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .unsupported {
      display: grid;
      height: 100%;
      place-items: center;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-muted);
      padding: var(--base-size-16);
      text-align: center;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
    }

    @media (max-width: 640px) {
      .route,
      .body {
        grid-template-columns: 1fr;
      }

      .main {
        height: auto;
        min-height: 0;
        overflow: visible;
      }

      .canvas-wrap {
        padding: var(--base-size-12);
        overflow: auto;
      }

    }

    @media (prefers-reduced-motion: reduce) {
      .dashboard-refresh-progress,
      .dashboard-refresh-progress-value {
        transition: none;
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    this.addEventListener('ld-interaction-select', this.handleOptimisticInteraction as EventListener, { capture: true })
    this.loadRenderedComponents()
  }

  disconnectedCallback(): void {
    this.removeEventListener('ld-interaction-select', this.handleOptimisticInteraction as EventListener, { capture: true })
    this.clearOptimisticRollbackTimer()
    super.disconnectedCallback()
  }

  updated(): void {
    const page = this.page
    if (!page) return
    checkSignalContract('dashboard page', page, {
      dashboardId: 'required',
      pageId: 'required',
      components: 'required',
    })
    this.loadRenderedComponents()
    if (this.optimisticSelections && this.status.generation >= this.optimisticExpectedGeneration) {
      this.clearOptimisticState()
    }
  }

  get page(): DashboardPageSignal | null {
    return this.signal<DashboardPageSignal | null>('page', null)
  }

  private get filterConfig(): ReportFilterConfig[] {
    return this.signal<ReportFilterConfig[]>('filterConfig', [])
  }

  private get filters(): DashboardFilters {
    return this.signal<DashboardFilters>('filters', emptyFilters)
  }

  private get effectiveFilters(): DashboardFilters {
    const filters = this.renderSnapshot?.filters ?? this.filters
    if (!this.optimisticSelections) return filters
    return {
      ...filters,
      selections: this.optimisticSelections as DashboardInteractionSelection[],
    }
  }

  private get filterOptions(): Record<string, unknown> {
    return this.signal<Record<string, unknown>>('filterOptions', {})
  }

  private get visuals(): Record<string, DashboardVisual> {
    return this.signal<Record<string, DashboardVisual>>('visuals', {})
  }

  private get tables(): Record<string, DashboardTable> {
    return this.signal<Record<string, DashboardTable>>('tables', {})
  }

  private get status(): DashboardStatus {
    return this.signal<DashboardStatus>('status', emptyStatus)
  }

  private get componentStatus(): Record<string, DashboardComponentStatus> {
    return this.signal<Record<string, DashboardComponentStatus>>('componentStatus', {})
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    const snapshot: DashboardRenderSnapshot = {
      page,
      filterConfig: this.filterConfig,
      filters: this.filters,
      filterOptions: this.filterOptions,
      visuals: this.visuals,
      tables: this.tables,
      status: this.status,
      componentStatus: this.componentStatus,
    }
    this.renderSnapshot = snapshot
    const refreshProgress = this.refreshProgress(snapshot)
    return html`
      <div class="route">
        <ld-sub-sidebar .config=${this.pageSidebar(page)}></ld-sub-sidebar>
        <section class="main" aria-label="LibreDash report canvas">
          <header class="header">
            <div class="title-block">
              <h1>${page.title}</h1>
              <p class="detail">${page.headerDetail}</p>
            </div>
            <div class="actions">
              <button class="icon-button" type="button" title="Refresh model materializations" aria-label="Refresh model materializations" ?disabled=${snapshot.status.loading} @click=${this.refreshMaterializations}>
                ${lucideIcon(RefreshCw)}
              </button>
            </div>
          </header>
          <div class="body">
            ${this.renderRefreshProgress(refreshProgress)}
            <div class="canvas-wrap">
              <ld-report-canvas width=${page.canvas.width} height=${page.canvas.height}>
                ${page.components.map((component) => this.renderCanvasComponent(component))}
              </ld-report-canvas>
            </div>
            ${this.renderFilterDock()}
          </div>
          <ld-report-footer .status=${snapshot.status}></ld-report-footer>
        </section>
      </div>
      <ld-visual-modal></ld-visual-modal>
    `
  }

  private renderRefreshProgress(progress: DashboardRefreshProgress) {
    const valueText = `${Math.round(progress.percent)}% of dashboard refresh complete`
    return html`
      <div
        class="dashboard-refresh-progress"
        data-dashboard-refresh-progress
        data-active=${String(progress.active)}
        data-complete=${String(progress.complete)}
        data-generation=${progress.generation}
        role="progressbar"
        aria-label="Refreshing dashboard"
        aria-hidden=${String(!progress.active)}
        aria-valuemin="0"
        aria-valuenow=${progress.percent}
        aria-valuemax="100"
        aria-valuetext=${valueText}
      >
        <div
          class="dashboard-refresh-progress-value"
          style=${`width:${progress.percent}%`}
        ></div>
      </div>
    `
  }

  private refreshProgress(snapshot: DashboardRenderSnapshot): DashboardRefreshProgress {
    const percent = snapshot.status.progressPercent ?? (snapshot.status.loading ? 0 : 100)
    return {
      active: snapshot.status.loading,
      complete: !snapshot.status.loading && percent === 100,
      generation: snapshot.status.generation,
      percent,
    }
  }

  private pageSidebar(page: DashboardPageSignal) {
    return {
      label: 'Pages',
      railLabel: 'Pages',
      ariaLabel: 'Report pages',
      storageKey: 'libredash-report-sidebar-collapsed',
      activeId: page.pageId,
      items: page.pages.map((item: DashboardPageNavSignal) => ({
        id: item.id,
        title: item.title,
        href: item.href,
        active: item.active,
      })) ?? [],
    }
  }

  private renderCanvasComponent(component: DashboardComponentSignal) {
    const filterVisual = component.kind === 'filter_card'
    const statusKey = this.componentStatusKey(component)
    const componentRefreshStatus = statusKey ? this.refreshStatusFor(statusKey) : undefined
    return html`
      <ld-dashboard-visual-frame
        data-canvas-visual
        ?data-canvas-filter-visual=${filterVisual}
        data-x=${component.x}
        data-y=${component.y}
        data-w=${component.width}
        data-h=${component.height}
        data-component-status-key=${statusKey || nothing}
        .transparent=${component.kind === 'header'}
        .refreshStatus=${componentRefreshStatus}
      >
        ${this.renderComponentContent(component)}
      </ld-dashboard-visual-frame>
    `
  }

  private renderComponentContent(component: DashboardComponentSignal) {
    switch (component.kind) {
      case 'header':
        return this.renderHeadingComponent(component)
      case 'filter_card':
        return this.renderFilterCard(component)
      case 'kpi_card':
        return this.renderKPI(component)
      case 'table':
        return this.renderTable(component)
      default:
        if (isChartKind(component.kind)) return this.renderChart(component)
        return html`<div class="unsupported">Unsupported dashboard component: ${component.kind}</div>`
    }
  }

  private renderHeadingComponent(component: DashboardComponentSignal) {
    return html`
      <div class="heading-visual">
        <div>
          <p class="eyebrow">${component.eyebrow || 'LibreDash report'}</p>
          <h2>${component.title || 'Dashboard'}</h2>
        </div>
        <div class="badges">
          ${(component.badges ?? []).map((badge) => html`<span class="badge">${badge}</span>`)}
        </div>
      </div>
    `
  }

  private renderFilterCard(component: DashboardComponentSignal) {
    if (!component.filter) return this.missingPayload('filter')
    return html`
      <ld-filter-card
        filter-id=${component.filter}
        config=${json(this.renderSnapshot?.filterConfig ?? this.filterConfig)}
        filters=${json(this.effectiveFilters)}
        options=${json(this.renderSnapshot?.filterOptions ?? this.filterOptions)}
        loading=${String((this.renderSnapshot?.status ?? this.status).loading)}
      ></ld-filter-card>
    `
  }

  private renderKPI(component: DashboardComponentSignal) {
    const visual = this.visualFor(component)
    if (!visual) return this.missingPayload('visual')
    return html`<ld-kpi-card visual-id=${component.visual ?? ''} .visual=${visual}></ld-kpi-card>`
  }

  private renderChart(component: DashboardComponentSignal) {
    const visual = this.visualFor(component)
    if (!visual) return this.missingPayload('visual')
    return html`<ld-echart visual-id=${component.visual ?? ''} .chart=${visual}></ld-echart>`
  }

  private renderTable(component: DashboardComponentSignal) {
    const table = this.tableFor(component)
    if (!table) return this.missingPayload('table')
    return html`<ld-report-table table-id=${component.table ?? ''} .table=${table}></ld-report-table>`
  }

  private renderFilterDock() {
    return html`
      <ld-filter-dock
        .config=${this.renderSnapshot?.filterConfig ?? this.filterConfig}
        .filters=${this.effectiveFilters}
        .options=${this.renderSnapshot?.filterOptions ?? this.filterOptions}
        .loading=${(this.renderSnapshot?.status ?? this.status).loading}
      ></ld-filter-dock>
    `
  }

  private missingPayload(kind: string) {
    return html`<div class="unsupported">Missing ${kind} payload</div>`
  }

  private visualFor(component: DashboardComponentSignal): DashboardVisual | undefined {
    const visuals = this.renderSnapshot?.visuals ?? this.visuals
    const visual = component.visual ? visuals[component.visual] : undefined
    if (!visual) return undefined
    const selection = generatedSelectionEntries(canonicalSelectionEntriesForSource(this.effectiveFilters.selections, 'visual', component.visual ?? ''))
    const signature = stableSignature([visual, selection])
    const cached = this.visualProjectionCache.get(component.visual ?? '')
    if (cached?.signature === signature) return cached.value
    const value = {
      ...visual,
      selection,
    }
    this.visualProjectionCache.set(component.visual ?? '', { signature, value })
    return value
  }

  private tableFor(component: DashboardComponentSignal): DashboardTable | undefined {
    const tables = this.renderSnapshot?.tables ?? this.tables
    const table = component.table ? tables[component.table] : undefined
    if (!table) return undefined
    const selection = generatedSelectionEntries(canonicalSelectionEntriesForSource(this.effectiveFilters.selections, 'table', component.table ?? ''))
    const signature = stableSignature([table, selection])
    const cached = this.tableProjectionCache.get(component.table ?? '')
    if (cached?.signature === signature) return cached.value
    const value = { ...table, selection }
    this.tableProjectionCache.set(component.table ?? '', { signature, value })
    return value
  }

  private componentStatusKey(component: DashboardComponentSignal): string {
    if (component.table) return `table:${component.table}`
    if (component.visual) return `visual:${component.visual}`
    return ''
  }

  private refreshStatusFor(key: string): DashboardComponentStatus | undefined {
    if (this.optimisticTargetKeys.has(key)) {
      return { generation: this.optimisticExpectedGeneration, loading: true, error: '' }
    }
    const snapshot = this.renderSnapshot
    const refreshStatus = (snapshot?.componentStatus ?? this.componentStatus)[key]
    if (!refreshStatus) return undefined
    return {
      ...refreshStatus,
      loading: refreshStatus.loading && refreshStatus.generation === (snapshot?.status ?? this.status).generation,
    }
  }

  private refreshMaterializations = (): void => {
    this.dispatchEvent(new CustomEvent('ld-refresh-materializations', { bubbles: true, composed: true }))
  }

  private handleOptimisticInteraction = (event: CustomEvent<unknown>): void => {
    const command = optimisticCommand(event.detail)
    if (!command) return
    const configured = this.interactionConfigFor(command.sourceKind, command.sourceId)
    if (!validateInteractionCommand(command, configured)) return

    const current = this.optimisticSelections ?? this.filters.selections
    this.optimisticSelections = applyOptimisticInteraction(current, {
      ...command,
      toggle: configured?.toggle !== false,
    })
    this.optimisticTargetKeys = this.targetStatusKeys(configured?.targets ?? [])
    this.optimisticExpectedGeneration = Math.max(
      this.status.generation + 1,
      this.optimisticExpectedGeneration + 1,
    )
    this.scheduleOptimisticRollback()
  }

  private interactionConfigFor(sourceKind: 'visual' | 'table', sourceId: string): InteractionConfigLike | undefined {
    if (sourceKind === 'visual') return this.visuals[sourceId]?.interaction
    return this.tables[sourceId]?.interaction
  }

  private targetStatusKeys(targets: readonly string[]): Set<string> {
    const wanted = new Set(targets)
    const keys = new Set<string>()
    for (const component of this.page?.components ?? []) {
      if (component.visual && (wanted.has(component.visual) || wanted.has(component.id))) {
        keys.add(`visual:${component.visual}`)
      }
      if (component.table && (wanted.has(component.table) || wanted.has(component.id))) {
        keys.add(`table:${component.table}`)
      }
    }
    return keys
  }

  private scheduleOptimisticRollback(): void {
    this.clearOptimisticRollbackTimer()
    this.optimisticRollbackTimer = setTimeout(() => this.clearOptimisticState(), 10_000)
  }

  private clearOptimisticRollbackTimer(): void {
    if (this.optimisticRollbackTimer !== undefined) clearTimeout(this.optimisticRollbackTimer)
    this.optimisticRollbackTimer = undefined
  }

  private clearOptimisticState(): void {
    this.clearOptimisticRollbackTimer()
    this.optimisticSelections = null
    this.optimisticTargetKeys = new Set<string>()
    this.optimisticExpectedGeneration = this.status.generation
  }

  private loadRenderedComponents(): void {
    const kinds = new Set<string>(['ld-filter-panel'])
    for (const component of this.page?.components ?? []) {
      const tag = tagForComponent(component)
      if (tag) kinds.add(tag)
    }
    for (const kind of kinds) {
      loadDashboardComponent(kind).catch(() => {
        if (!this.unsupportedKinds.has(kind)) {
          this.unsupportedKinds = new Set([...this.unsupportedKinds, kind])
        }
      })
    }
  }
}

function generatedSelectionEntries(entries: ReturnType<typeof canonicalSelectionEntriesForSource>): DashboardInteractionSelectionEntry[] {
  return entries.map((entry) => ({
    ...entry,
    mappings: entry.mappings ?? [],
  }))
}

function optimisticCommand(value: unknown): OptimisticInteractionCommand | undefined {
  if (!value || typeof value !== 'object') return undefined
  const command = value as Partial<OptimisticInteractionCommand>
  if (command.sourceKind !== 'visual' && command.sourceKind !== 'table') return undefined
  if (typeof command.sourceId !== 'string' || typeof command.interactionKind !== 'string') return undefined
  if (command.action !== 'set' && command.action !== 'replace' && command.action !== 'clear') return undefined
  if (typeof command.toggle !== 'boolean' || !Array.isArray(command.mappings)) return undefined
  return command as OptimisticInteractionCommand
}

function stableSignature(value: unknown): string {
  return JSON.stringify(value)
}

class DashboardVisualFrame extends LitElement {
  @property({ type: Boolean, reflect: true }) transparent = false
  @property({ type: Object, attribute: false }) refreshStatus?: DashboardComponentStatus

  static styles = css`
    :host {
      display: block;
      height: 100%;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
      box-sizing: border-box;
    }

    .frame {
      position: relative;
      height: 100%;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      box-sizing: border-box;
    }

    :host([transparent]) .frame {
      border-color: transparent;
      background: transparent;
    }

    :host([data-canvas-filter-visual]) {
      overflow: visible;
      z-index: 5;
    }

    :host([data-canvas-filter-visual]) .frame {
      overflow: visible;
    }

    ::slotted(*) {
      display: block;
      width: 100%;
      height: 100%;
    }

    .refresh-overlay {
      position: absolute;
      inset: 0;
      z-index: 2;
      display: grid;
      place-items: center;
      background: color-mix(in srgb, var(--ld-bg-panel) 78%, transparent);
      color: var(--ld-fg-muted);
      padding: var(--base-size-12);
      box-sizing: border-box;
      pointer-events: none;
    }

    .refresh-overlay.error {
      align-content: center;
      gap: var(--base-size-4);
      border: var(--ld-border-danger);
      background: color-mix(in srgb, var(--ld-bg-danger-muted) 92%, transparent);
      color: var(--ld-fg-danger);
      text-align: center;
    }

    .refresh-overlay strong {
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .refresh-overlay span {
      max-width: 100%;
      overflow: hidden;
      text-overflow: ellipsis;
      font-size: var(--ld-font-size-caption);
    }

    .spinner {
      width: var(--base-size-16);
      height: var(--base-size-16);
      border: 2px solid var(--ld-line-muted);
      border-top-color: var(--ld-fg-link);
      border-radius: 50%;
      animation: spin 700ms linear infinite;
    }

    @keyframes spin {
      to { transform: rotate(360deg); }
    }

    @media (prefers-reduced-motion: reduce) {
      .spinner { animation: none; }
    }
  `

  render() {
    const refreshStatus = this.refreshStatus
    return html`
      <article class="frame" aria-busy=${refreshStatus?.loading ? 'true' : 'false'}>
        <slot></slot>
        ${refreshStatus?.error ? html`
          <div class="refresh-overlay error" role="alert">
            <strong>Could not refresh this component</strong>
            <span>${refreshStatus.error}</span>
          </div>
        ` : refreshStatus?.loading ? html`
          <div class="refresh-overlay loading" role="status" aria-label="Refreshing component">
            <span class="spinner" aria-hidden="true"></span>
          </div>
        ` : nothing}
      </article>
    `
  }
}

function tagForComponent(component: DashboardComponentSignal): string {
  switch (component.kind) {
    case 'filter_card':
      return 'ld-filter-card'
    case 'kpi_card':
      return 'ld-kpi-card'
    case 'table':
      return 'ld-report-table'
    default:
      return isChartKind(component.kind) ? 'ld-echart' : ''
  }
}

function isChartKind(kind: string): boolean {
  return [
    'line_chart',
    'area_chart',
    'bar_chart',
    'column_chart',
    'pie_chart',
    'donut_chart',
    'scatter_chart',
    'funnel_chart',
    'treemap_chart',
    'gauge_chart',
    'heatmap_chart',
    'sankey_chart',
    'graph_chart',
    'map_chart',
    'candlestick_chart',
    'boxplot_chart',
    'combo_chart',
    'waterfall_chart',
    'histogram_chart',
    'radar_chart',
    'tree_chart',
    'sunburst_chart',
  ].includes(kind)
}

function json(value: unknown): string {
  return JSON.stringify(value ?? {})
}

if (!customElements.get('ld-dashboard-page')) customElements.define('ld-dashboard-page', LibreDashDashboardPage)
if (!customElements.get('ld-dashboard-visual-frame')) customElements.define('ld-dashboard-visual-frame', DashboardVisualFrame)
