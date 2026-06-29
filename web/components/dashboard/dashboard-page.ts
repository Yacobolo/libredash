import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { RefreshCw } from 'lucide'
import type {
  DashboardComponentSignal,
  DashboardFilters,
  DashboardPageNavSignal,
  DashboardPageSignal,
  DashboardStatus,
  DashboardTable,
  DashboardVisual,
  ReportFilterConfig,
} from '../../generated/signals'
import { lucideIcon } from '../shared/lucide-icons'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import './filters/filter-dock'
import './report-canvas'
import './report-footer'
import './visual-modal'
import { loadDashboardComponent } from './registry'

const emptyFilters: DashboardFilters = { controls: {}, selections: [] }
const emptyStatus: DashboardStatus = {
  loading: false,
  error: '',
  lastUpdated: '',
  dataDirectory: '',
  setupRequired: false,
}

class LibreDashDashboardPage extends LitElement {
  @property({ converter: jsonAttribute<DashboardPageSignal | null>(null) }) page: DashboardPageSignal | null = null
  @property({ attribute: 'filterconfig', converter: jsonAttribute<ReportFilterConfig[]>([]) }) filterConfig: ReportFilterConfig[] = []
  @property({ converter: jsonAttribute<DashboardFilters>(emptyFilters) }) filters: DashboardFilters = emptyFilters
  @property({ attribute: 'filteroptions', converter: jsonAttribute<Record<string, unknown>>({}) }) filterOptions: Record<string, unknown> = {}
  @property({ converter: jsonAttribute<Record<string, DashboardVisual>>({}) }) visuals: Record<string, DashboardVisual> = {}
  @property({ converter: jsonAttribute<Record<string, DashboardTable>>({}) }) tables: Record<string, DashboardTable> = {}
  @property({ converter: jsonAttribute<DashboardStatus>(emptyStatus) }) status: DashboardStatus = emptyStatus

  @state() private unsupportedKinds = new Set<string>()

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
      padding: var(--base-size-10) var(--base-size-16);
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
      display: grid;
      min-width: 0;
      min-height: 0;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: stretch;
      overflow: hidden;
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
  `

  connectedCallback(): void {
    super.connectedCallback()
    this.loadRenderedComponents()
  }

  updated(): void {
    checkSignalContract('dashboard page', this.page, {
      dashboardId: 'required',
      pageId: 'required',
      components: 'required',
    })
    this.loadRenderedComponents()
  }

  render() {
    if (!this.page) return html`<slot></slot>`
    return html`
      <div class="route">
        <ld-sub-sidebar .config=${this.pageSidebar()}></ld-sub-sidebar>
        <section class="main" aria-label="LibreDash report canvas">
          <header class="header">
            <div class="title-block">
              <h1>${this.page.title}</h1>
              <p class="detail">${this.page.headerDetail}</p>
            </div>
            <div class="actions">
              <button class="icon-button" type="button" title="Refresh model materializations" aria-label="Refresh model materializations" ?disabled=${this.status.loading} @click=${this.refreshMaterializations}>
                ${lucideIcon(RefreshCw)}
              </button>
            </div>
          </header>
          <div class="body">
            <div class="canvas-wrap">
              <ld-report-canvas width=${this.page.canvas.width} height=${this.page.canvas.height}>
                ${this.page.components.map((component) => this.renderCanvasComponent(component))}
              </ld-report-canvas>
            </div>
            ${this.renderFilterDock()}
          </div>
          <ld-report-footer .status=${this.status}></ld-report-footer>
        </section>
      </div>
      <ld-visual-modal></ld-visual-modal>
    `
  }

  private pageSidebar() {
    return {
      label: 'Pages',
      railLabel: 'Pages',
      ariaLabel: 'Report pages',
      storageKey: 'libredash-report-sidebar-collapsed',
      activeId: this.page?.pageId ?? '',
      items: this.page?.pages.map((item: DashboardPageNavSignal) => ({
        id: item.id,
        title: item.title,
        href: item.href,
        active: item.active,
      })) ?? [],
    }
  }

  private renderCanvasComponent(component: DashboardComponentSignal) {
    const filterVisual = component.kind === 'filter_card'
    return html`
      <ld-dashboard-visual-frame
        data-canvas-visual
        ?data-canvas-filter-visual=${filterVisual}
        data-x=${component.x}
        data-y=${component.y}
        data-w=${component.width}
        data-h=${component.height}
        .transparent=${component.kind === 'header'}
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
        config=${json(this.filterConfig)}
        filters=${json(this.filters)}
        options=${json(this.filterOptions)}
        loading=${String(this.status.loading)}
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
    const table = component.table ? this.tables[component.table] : undefined
    if (!table) return this.missingPayload('table')
    return html`<ld-data-table table-id=${component.table ?? ''} .table=${table}></ld-data-table>`
  }

  private renderFilterDock() {
    return html`
      <ld-filter-dock
        .config=${this.filterConfig}
        .filters=${this.filters}
        .options=${this.filterOptions}
        .loading=${this.status.loading}
      ></ld-filter-dock>
    `
  }

  private missingPayload(kind: string) {
    return html`<div class="unsupported">Missing ${kind} payload</div>`
  }

  private visualFor(component: DashboardComponentSignal): DashboardVisual | undefined {
    return component.visual ? this.visuals[component.visual] : undefined
  }

  private refreshMaterializations = (): void => {
    this.dispatchEvent(new CustomEvent('ld-refresh-materializations', { bubbles: true, composed: true }))
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

class DashboardVisualFrame extends LitElement {
  @property({ type: Boolean, reflect: true }) transparent = false

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
  `

  render() {
    return html`<article class="frame"><slot></slot></article>`
  }
}

function tagForComponent(component: DashboardComponentSignal): string {
  switch (component.kind) {
    case 'filter_card':
      return 'ld-filter-card'
    case 'kpi_card':
      return 'ld-kpi-card'
    case 'table':
      return 'ld-data-table'
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
