import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import type {
  DashboardComponentSignal,
  DashboardComponentStatus,
  DashboardFilters,
  DashboardInteractionSelection,
  DashboardInteractionSelectionEntry,
  DashboardPageNavSignal,
  DashboardPageSignal,
  DashboardStatus,
  DashboardVisual,
  ReportFilterConfig,
} from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
import { checkSignalContract } from '../shared/signal-contract'
import '../shared/loading-spinner'
import '../navigation/sub-sidebar'
import './filters/filter-dock'
import './report-canvas'
import './report-footer'
import './visual-modal'
import { loadDashboardComponent } from './registry'
import { normalizeTable } from './table/block-source'
import type { TableSignal } from './table/types'
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
  status: DashboardStatus
  componentStatus: Record<string, DashboardComponentStatus>
}

type DashboardRefreshProgress = {
  active: boolean
  complete: boolean
  generation: number
  percent: number
}

type VisualLoadingPresentation = 'none' | 'center' | 'header'

class LeapViewDashboardPage extends DatastarLit(LitElement) {
  @state() private unsupportedKinds = new Set<string>()
  @state() private optimisticSelections: CanonicalInteractionSelection[] | null = null
  @state() private optimisticTargetKeys = new Set<string>()
  private optimisticExpectedGeneration = 0
  private optimisticRollbackTimer?: ReturnType<typeof setTimeout>
  private renderSnapshot?: DashboardRenderSnapshot
  private visualProjectionCache = new Map<string, { signature: string; value: DashboardVisual }>()
  private tableProjectionCache = new Map<string, { signature: string; value: TableSignal }>()

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 100svh;
      color: var(--lv-fg-default);
      font-family: var(--lv-font-family-ui, var(--fontStack-system));
      background: var(--lv-bg-app);
    }

    .route {
      display: grid;
      min-height: 100svh;
      grid-template-columns: auto minmax(0, 1fr);
      background: var(--lv-bg-app);
    }

    .main {
      display: grid;
      min-width: 0;
      height: 100svh;
      min-height: 0;
      grid-template-rows: auto minmax(0, 1fr) auto;
      overflow: hidden;
      background: var(--lv-bg-app);
    }

    .header {
      display: grid;
      min-width: 0;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: center;
      gap: var(--base-size-8);
      border-bottom: var(--lv-border-muted);
      padding: var(--lv-space-control) var(--base-size-16);
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
      color: var(--lv-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--lv-font-size-title-sm);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-compact);
    }

    .detail {
      margin-top: var(--base-size-4);
      overflow: hidden;
      color: var(--lv-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--lv-font-size-body-sm);
      line-height: var(--lv-line-height-compact);
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
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: transparent;
      color: var(--lv-fg-default);
      cursor: pointer;
      padding: 0;
    }

    .icon-button:hover,
    .icon-button:focus-visible {
      background: var(--lv-bg-control-hover);
      outline: 0;
    }

    .icon-button[disabled] {
      cursor: not-allowed;
      color: var(--lv-fg-muted);
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
      background: var(--lv-line-muted);
      opacity: 0;
      pointer-events: none;
      transition: opacity var(--motion-transition-stateChange);
      transition-delay: 0s;
    }

    .dashboard-refresh-progress[data-active='true'] {
      opacity: 1;
      transition-delay: var(--lv-loading-delay-short);
    }

    .dashboard-refresh-progress[data-active='false'][data-complete='true'] {
      transition-delay: 180ms;
    }

    .dashboard-refresh-progress-value {
      width: 0;
      height: 100%;
      background: var(--lv-line-accent);
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
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
      line-height: var(--lv-line-height-tight);
      text-transform: uppercase;
    }

    .heading-visual h2 {
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-title-lg);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-tight);
    }

    .badges {
      display: flex;
      flex-wrap: wrap;
      justify-content: flex-end;
      gap: var(--base-size-8);
    }

    .badge {
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-full);
      background: var(--lv-bg-panel-muted);
      color: var(--lv-fg-muted);
      padding: var(--base-size-2) var(--base-size-8);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
      text-transform: uppercase;
    }

    .unsupported {
      display: grid;
      height: 100%;
      place-items: center;
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-panel);
      color: var(--lv-fg-muted);
      padding: var(--base-size-16);
      text-align: center;
      font-size: var(--lv-font-size-body-sm);
      font-weight: var(--lv-font-weight-medium);
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
    this.addEventListener('lv-interaction-select', this.handleOptimisticInteraction as EventListener, { capture: true })
    this.loadRenderedComponents()
  }

  disconnectedCallback(): void {
    this.removeEventListener('lv-interaction-select', this.handleOptimisticInteraction as EventListener, { capture: true })
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
      status: this.status,
      componentStatus: this.componentStatus,
    }
    this.renderSnapshot = snapshot
    const refreshProgress = this.refreshProgress(snapshot)
    return html`
      <div class="route">
        <lv-sub-sidebar .config=${this.pageSidebar(page)}></lv-sub-sidebar>
        <section class="main" aria-label="LeapView report canvas">
          <header class="header">
            <div class="title-block">
              <h1>${page.title}</h1>
              <p class="detail">${page.headerDetail}</p>
            </div>
          </header>
          <div class="body">
            ${this.renderRefreshProgress(refreshProgress)}
            <div class="canvas-wrap">
              <lv-report-canvas width=${page.canvas.width} height=${page.canvas.height}>
                ${page.components.map((component) => this.renderCanvasComponent(component))}
              </lv-report-canvas>
            </div>
            ${this.renderFilterDock()}
          </div>
          <lv-report-footer .status=${snapshot.status}></lv-report-footer>
        </section>
      </div>
      <lv-visual-modal></lv-visual-modal>
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
      storageKey: 'leapview-report-sidebar-collapsed',
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
    const filterVisual = component.kind === 'filter'
    const visualType = component.visual ? this.visuals[component.visual]?.type ?? '' : ''
    const statusKey = this.componentStatusKey(component)
    const componentRefreshStatus = statusKey ? this.refreshStatusFor(statusKey) : undefined
    return html`
              <lv-dashboard-visual-frame
                data-canvas-visual
                data-component-kind=${component.kind}
                data-visual-type=${visualType}
        ?data-canvas-filter-visual=${filterVisual}
        data-x=${component.x}
        data-y=${component.y}
        data-w=${component.width}
        data-h=${component.height}
        data-component-status-key=${statusKey || nothing}
        .transparent=${component.kind === 'header'}
        .refreshStatus=${componentRefreshStatus}
        .loadingPresentation=${this.loadingPresentationFor(component, visualType)}
      >
        ${this.renderComponentContent(component)}
      </lv-dashboard-visual-frame>
    `
  }

  private renderComponentContent(component: DashboardComponentSignal) {
    switch (component.kind) {
      case 'header':
        return this.renderHeadingComponent(component)
      case 'filter':
        return this.renderFilterCard(component)
      case 'visual': {
        const visual = this.visualFor(component)
        if (!visual) return this.missingPayload('visual')
        if (visual.type === 'kpi') return this.renderKPI(component, visual)
        if (isTabularVisualType(visual.type)) return this.renderTable(component, visual)
        return this.renderChart(component, visual)
      }
      default:
        return html`<div class="unsupported">Unsupported dashboard component: ${component.kind}</div>`
    }
  }

  private renderHeadingComponent(component: DashboardComponentSignal) {
    return html`
      <div class="heading-visual">
        <div>
          <p class="eyebrow">${component.eyebrow || 'LeapView report'}</p>
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
      <lv-filter-card
        filter-id=${component.filter}
        config=${json(this.renderSnapshot?.filterConfig ?? this.filterConfig)}
        filters=${json(this.effectiveFilters)}
        options=${json(this.renderSnapshot?.filterOptions ?? this.filterOptions)}
        loading=${String((this.renderSnapshot?.status ?? this.status).loading)}
      ></lv-filter-card>
    `
  }

  private renderKPI(component: DashboardComponentSignal, visual: DashboardVisual) {
    return html`<lv-kpi-card visual-id=${component.visual ?? ''} .visual=${visual}></lv-kpi-card>`
  }

  private renderChart(component: DashboardComponentSignal, visual: DashboardVisual) {
    return html`<lv-echart visual-id=${component.visual ?? ''} .chart=${visual}></lv-echart>`
  }

  private renderTable(component: DashboardComponentSignal, visual: DashboardVisual) {
    const table = this.tableFor(component, visual)
    return html`<lv-report-table table-id=${component.visual ?? ''} .table=${table}></lv-report-table>`
  }

  private renderFilterDock() {
    return html`
      <lv-filter-dock
        .config=${this.renderSnapshot?.filterConfig ?? this.filterConfig}
        .filters=${this.effectiveFilters}
        .options=${this.renderSnapshot?.filterOptions ?? this.filterOptions}
        .loading=${(this.renderSnapshot?.status ?? this.status).loading}
      ></lv-filter-dock>
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

  private tableFor(component: DashboardComponentSignal, visual: DashboardVisual): TableSignal {
    const visualID = component.visual ?? ''
    const selection = generatedSelectionEntries(canonicalSelectionEntriesForSource(this.effectiveFilters.selections, 'visual', visualID))
    const signature = stableSignature([visual, selection])
    const cached = this.tableProjectionCache.get(visualID)
    if (cached?.signature === signature) return cached.value
    const value = normalizeTable({ ...(visual as unknown as Partial<TableSignal>), selection })
    this.tableProjectionCache.set(visualID, { signature, value })
    return value
  }

  private componentStatusKey(component: DashboardComponentSignal): string {
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

  private loadingPresentationFor(component: DashboardComponentSignal, visualType: string): VisualLoadingPresentation {
    if (component.kind !== 'visual' || !component.visual || isTabularVisualType(visualType)) return 'none'
    const visuals = this.renderSnapshot?.visuals ?? this.visuals
    const visual = visuals[component.visual]
    if (!visual) return 'center'
    const hasData = (visual.data?.length ?? 0) > 0
    if (visualType === 'kpi') return hasData ? 'none' : 'center'
    return hasData ? 'header' : 'center'
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

  private interactionConfigFor(sourceKind: 'visual', sourceId: string): InteractionConfigLike | undefined {
    return this.visuals[sourceId]?.interaction
  }

  private targetStatusKeys(targets: readonly string[]): Set<string> {
    const wanted = new Set(targets)
    const keys = new Set<string>()
    for (const component of this.page?.components ?? []) {
      if (component.visual && (wanted.has(component.visual) || wanted.has(component.id))) {
        keys.add(`visual:${component.visual}`)
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
    const kinds = new Set<string>(['lv-filter-panel'])
    for (const component of this.page?.components ?? []) {
      const tag = tagForComponent(component, this.visuals)
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
  if (command.sourceKind !== 'visual') return undefined
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
  @property({ type: String, attribute: false }) loadingPresentation: VisualLoadingPresentation = 'none'

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
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-panel);
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
      background: color-mix(in srgb, var(--lv-bg-panel) 78%, transparent);
      color: var(--lv-fg-muted);
      padding: var(--base-size-12);
      box-sizing: border-box;
      pointer-events: none;
    }

    .refresh-overlay.error {
      align-content: center;
      gap: var(--base-size-4);
      border: var(--lv-border-danger);
      background: color-mix(in srgb, var(--lv-bg-danger-muted) 92%, transparent);
      color: var(--lv-fg-danger);
      text-align: center;
    }

    .refresh-overlay strong {
      font-size: var(--lv-font-size-body-sm);
      font-weight: var(--lv-font-weight-strong);
    }

    .refresh-overlay span {
      max-width: 100%;
      overflow: hidden;
      text-overflow: ellipsis;
      font-size: var(--lv-font-size-caption);
    }

    .loading-status {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }

    .loading-indicator {
      position: absolute;
      z-index: 2;
      display: grid;
      color: var(--lv-fg-muted);
      opacity: 0;
      pointer-events: none;
      visibility: hidden;
      animation-name: reveal-loading-indicator;
      animation-duration: 0s;
      animation-fill-mode: forwards;
    }

    .loading-indicator.header {
      top: var(--base-size-12);
      right: calc(var(--base-size-24) + var(--base-size-24) + var(--base-size-16));
      width: var(--base-size-12);
      height: var(--base-size-12);
      place-items: center;
      animation-delay: var(--lv-loading-delay-long);
    }

    .loading-indicator.header lv-loading-spinner {
      --lv-spinner-size: var(--base-size-12);
    }

    .loading-indicator.center {
      inset: 0;
      place-items: center;
      background: var(--lv-bg-panel);
      animation-delay: var(--lv-loading-delay-short);
    }

    .loading-indicator.center lv-loading-spinner {
      --lv-spinner-size: var(--base-size-24);
    }

    @keyframes reveal-loading-indicator {
      to {
        opacity: 1;
        visibility: visible;
      }
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
          <span class="loading-status" role="status" aria-label="Refreshing component">Refreshing component</span>
          ${this.loadingPresentation === 'none' ? nothing : html`
            <div class=${`loading-indicator ${this.loadingPresentation}`} aria-hidden="true">
              <lv-loading-spinner></lv-loading-spinner>
            </div>
          `}
        ` : nothing}
      </article>
    `
  }
}

function tagForComponent(component: DashboardComponentSignal, visuals: Record<string, DashboardVisual>): string {
  switch (component.kind) {
    case 'filter':
      return 'lv-filter-card'
    case 'visual': {
      const visualType = component.visual ? visuals[component.visual]?.type : undefined
      if (visualType === 'kpi') return 'lv-kpi-card'
      if (isTabularVisualType(visualType)) return 'lv-report-table'
      return visualType ? 'lv-echart' : ''
    }
    default:
      return ''
  }
}

function isTabularVisualType(type: string | undefined): boolean {
  return type === 'table' || type === 'matrix' || type === 'pivot'
}

function json(value: unknown): string {
  return JSON.stringify(value ?? {})
}

if (!customElements.get('lv-dashboard-page')) customElements.define('lv-dashboard-page', LeapViewDashboardPage)
if (!customElements.get('lv-dashboard-visual-frame')) customElements.define('lv-dashboard-visual-frame', DashboardVisualFrame)
