import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import * as echarts from 'echarts'
import type { ECharts } from 'echarts'
import { visualMenuIcon } from './visual-menu-icons'
import './chart/echarts-renderer'
import { chartRenderer } from './chart/registry'
import type { ChartDatum, ChartPayload, ChartType, VisualAction } from './chart/types'
import { chartColumns, chartRows, normalizeShape, normalizeType, stylesFor } from './chart/utils'

const chartStyles = css`
  :host {
    display: block;
    height: 100%;
    min-height: 0;
    color: var(--fgColor-default);
    font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  }

  .chart {
    display: grid;
    height: 100%;
    min-height: 0;
    grid-template-rows: auto minmax(0, 1fr);
    background: var(--report-chart-surface, var(--card-bgColor, var(--bgColor-default)));
  }

  header {
    display: flex;
    min-height: 34px;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 8px 5px 10px;
  }

  h2 {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.8rem;
    font-weight: 850;
    letter-spacing: 0;
    line-height: 1.1;
  }

  .unit {
    flex: 0 0 auto;
    color: var(--fgColor-muted);
    font-size: 0.6rem;
    font-weight: 900;
    text-transform: uppercase;
  }

  .header-main {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 8px;
  }

  .options {
    position: relative;
    flex: 0 0 auto;
  }

  .options summary {
    display: grid;
    width: 24px;
    height: 24px;
    place-items: center;
    border: 1px solid transparent;
    border-radius: 4px;
    color: var(--fgColor-muted);
    cursor: pointer;
    font-size: 1rem;
    font-weight: 900;
    line-height: 1;
    list-style: none;
  }

  .options summary::-webkit-details-marker {
    display: none;
  }

  .options summary:hover,
  .options summary:focus-visible,
  .options[open] summary {
    border-color: var(--borderColor-default);
    background: var(--bgColor-muted);
    color: var(--fgColor-default);
    outline: 0;
  }

  .menu {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 30;
    display: grid;
    width: 176px;
    border: 1px solid var(--borderColor-default);
    border-radius: 6px;
    background: var(--overlay-bgColor, var(--bgColor-default));
    box-shadow: var(--shadow-floating-small, 0 8px 24px rgb(0 0 0 / 18%));
    padding: 4px;
  }

  .menu button {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 27px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--fgColor-default);
    cursor: pointer;
    padding: 0 8px;
    font: inherit;
    font-size: 0.68rem;
    font-weight: 750;
    text-align: left;
  }

  .menu svg {
    flex: 0 0 auto;
    width: 14px;
    height: 14px;
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 2;
  }

  .menu button:hover,
  .menu button:focus-visible {
    background: var(--bgColor-muted);
    outline: 0;
  }

  .menu button:disabled {
    cursor: default;
    opacity: 0.48;
  }

  .menu button:disabled:hover {
    background: transparent;
  }

  .canvas {
    grid-row: 2;
    min-height: 0;
    width: 100%;
    height: 100%;
  }

  .canvas.idle {
    visibility: hidden;
  }

  .empty {
    grid-row: 2;
    display: grid;
    min-height: 0;
    place-items: center;
    margin: 12px;
    border: 1px dashed var(--borderColor-default);
    background: var(--report-panel-subtle, var(--bgColor-muted));
    color: var(--fgColor-muted);
    font-weight: 800;
  }
`

class EChartVisual extends LitElement {
  @property({ type: Object }) chart: ChartPayload | null = null
  @property({ type: Array }) data: ChartDatum[] = []
  @property({ type: String, attribute: 'chart-title' }) chartTitle = 'Chart'
  @property({ type: String }) unit = ''
  @property({ type: String, attribute: 'visual-id' }) visualId = ''
  @property({ type: String }) field = ''
  @property({ type: String }) type: ChartType | string = 'bar'
  @property({ type: Array }) selection: string[] = []

  static styles = chartStyles

  private instance?: ECharts
  private observer?: ResizeObserver
  private handleOutsidePointerDown = (event: PointerEvent) => {
    const details = this.renderRoot.querySelector<HTMLDetailsElement>('.options')
    if (!details?.open) return
    if (!event.composedPath().includes(details)) details.removeAttribute('open')
  }
  private handleDocumentKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    this.renderRoot.querySelector<HTMLDetailsElement>('.options')?.removeAttribute('open')
  }

  connectedCallback(): void {
    super.connectedCallback()
    this.observer = new ResizeObserver(() => this.instance?.resize())
    document.addEventListener('pointerdown', this.handleOutsidePointerDown)
    document.addEventListener('keydown', this.handleDocumentKeyDown)
  }

  firstUpdated(): void {
    const canvas = this.renderRoot.querySelector('.canvas') as HTMLDivElement | null
    if (!canvas) return
    this.instance = echarts.init(canvas, null, { renderer: 'canvas' })
    this.instance.on('click', (event) => {
      const label = selectionValueForEvent(this.payload, event)
      if (label) this.selectLabel(label)
    })
    this.observer?.observe(this)
    this.renderChart()
  }

  updated(): void {
    this.renderChart()
  }

  disconnectedCallback(): void {
    document.removeEventListener('pointerdown', this.handleOutsidePointerDown)
    document.removeEventListener('keydown', this.handleDocumentKeyDown)
    this.observer?.disconnect()
    this.instance?.dispose()
    super.disconnectedCallback()
  }

  render() {
    const payload = this.payload
    const data = payload.data ?? []
    return html`
      <section class="chart">
        <header>
          <div class="header-main">
            <h2>${payload.title ?? 'Chart'}</h2>
            <span class="unit">${payload.unit ?? ''}</span>
          </div>
          <details class="options">
            <summary aria-label="Visual options" title="Visual options">⋮</summary>
            <div class="menu" role="menu">
              <button type="button" role="menuitem" @click=${() => this.runAction('focus')}>${visualMenuIcon('focus')}<span>Focus mode</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('show-data')}>${visualMenuIcon('show-data')}<span>Show data</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('copy-data')}>${visualMenuIcon('copy-data')}<span>Copy data</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('export-csv')}>${visualMenuIcon('export-csv')}<span>Export CSV</span></button>
              <button type="button" role="menuitem" ?disabled=${!this.hasSelection(payload)} @click=${() => this.runAction('clear-selection')}>${visualMenuIcon('clear-selection')}<span>Clear selection</span></button>
            </div>
          </details>
        </header>
        <div class=${data.length === 0 ? 'canvas idle' : 'canvas'}></div>
        ${data.length === 0 ? html`<div class="empty">Waiting for signal data</div>` : null}
      </section>
    `
  }

  private renderChart(): void {
    if (!this.instance) return
    const payload = this.payload
    const data = payload.data ?? []
    if (data.length === 0) {
      this.instance.clear()
      return
    }
    const renderer = chartRenderer(payload.renderer)
    if (!renderer) {
      this.instance.clear()
      return
    }
    this.instance.setOption(renderer.buildOption(payload, stylesFor(this)), true)
    this.instance.resize()
  }

  private get payload(): ChartPayload {
    const chart = this.chart ?? {}
    const type = normalizeType(chart.type || this.type)
    return {
      version: chart.version ?? 3,
      id: chart.id || this.visualId,
      kind: chart.kind || 'chart',
      shape: normalizeShape(chart.shape, type, Boolean(chart.series?.length)),
      renderer: chart.renderer || 'echarts',
      type,
      title: chart.title || this.chartTitle,
      unit: chart.unit ?? this.unit,
      field: chart.field || this.field,
      dimensions: chart.dimensions ?? [],
      measure: chart.measure ?? '',
      measures: chart.measures ?? (chart.measure ? [chart.measure] : []),
      series: chart.series ?? [],
      selection: chart.selection ?? this.selection ?? [],
      data: chart.data ?? this.data ?? [],
      options: chart.options ?? {},
      rendererOptions: chart.rendererOptions ?? {},
    }
  }

  private selectLabel(label: string): void {
    const payload = this.payload
    if (!payload.id || !payload.field || !label) return
    this.dispatchEvent(
      new CustomEvent('ld-chart-select', {
        bubbles: true,
        composed: true,
        detail: {
          visualId: payload.id,
          field: payload.field,
          value: label,
          label,
          mode: 'toggle',
        },
      }),
    )
  }

  private runAction(action: VisualAction): void {
    const payload = this.payload
    this.renderRoot.querySelector<HTMLDetailsElement>('.options')?.removeAttribute('open')
    this.dispatchEvent(
      new CustomEvent('ld-visual-action', {
        bubbles: true,
        composed: true,
        detail: {
          action,
          visualType: 'chart',
          visualId: payload.id || this.visualId,
          title: payload.title || 'Chart',
          columns: chartColumns(payload),
          rows: chartRows(payload),
          selection: payload.selection ?? [],
          chart: payload,
        },
      }),
    )
    if (action === 'clear-selection') {
      this.dispatchEvent(new CustomEvent('ld-chart-clear-selection', { bubbles: true, composed: true }))
    }
  }

  private hasSelection(payload: ChartPayload): boolean {
    return Boolean(payload.selection?.length || payload.data?.some((row) => row.selected))
  }
}

function selectionValueForEvent(payload: ChartPayload, event: echarts.ECElementEvent): string {
  const shape = normalizeShape(payload.shape, payload.type, Boolean(payload.series?.length))
  const data = (event.data ?? {}) as Record<string, unknown>
  if (shape === 'matrix') return String(data.name || event.name || '')
  if (shape === 'geo') return String(data.name || event.name || '')
  if (shape === 'graph') return String(event.name || data.source || '')
  return String(event.name || data.name || '')
}

class KPIStrip extends LitElement {
  @property({ type: Array }) items: Array<{ label: string; value: string; note: string; tone: string }> = []

  static styles = css`
    :host {
      display: block;
    }

    .strip {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
    }

    .kpi {
      position: relative;
      min-height: 104px;
      border: 1px solid var(--borderColor-default);
      border-radius: 6px;
      background: var(--report-chart-surface, var(--card-bgColor, var(--bgColor-default)));
      box-shadow: var(--shadow-resting-small);
      padding: 12px 14px 12px 16px;
      overflow: hidden;
    }

    .kpi::before {
      content: '';
      position: absolute;
      inset-block: 0;
      left: 0;
      width: 5px;
      background: var(--borderColor-muted);
    }

    .label {
      color: var(--fgColor-muted);
      font-size: 0.72rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .value {
      margin: 8px 0 4px;
      font-size: clamp(1.72rem, 3.5vw, 2.65rem);
      font-weight: 850;
      line-height: 1;
      letter-spacing: 0;
    }

    .note {
      color: var(--fgColor-muted);
      font-size: 0.85rem;
      font-weight: 700;
    }

    .green::before {
      background: var(--ld-chart-2, var(--data-green-color-emphasis));
    }
    .amber::before {
      background: var(--ld-accent, var(--data-yellow-color-emphasis));
    }
    .coral::before {
      background: var(--ld-chart-4, var(--data-coral-color-emphasis));
    }
    .ink::before {
      background: var(--ld-chart-1, var(--data-blue-color-emphasis));
    }
    .neutral::before {
      background: var(--borderColor-muted);
    }

    @media (max-width: 760px) {
      .strip {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }

    @media (max-width: 440px) {
      .strip {
        grid-template-columns: 1fr;
      }
    }
  `

  render() {
    const kpis = this.items ?? []
    return html`
      <section class="strip" aria-label="Key metrics">
        ${(kpis.length ? kpis : [{ label: 'Orders', value: '-', note: 'Waiting for stream', tone: 'neutral' }]).map(
          (item) => html`
            <article class="kpi ${item.tone ?? 'neutral'}">
              <div class="label">${item.label}</div>
              <div class="value">${item.value}</div>
              <div class="note">${item.note}</div>
            </article>
          `,
        )}
      </section>
    `
  }
}

class LegacyLineChart extends EChartVisual {}
class LegacyBarChart extends EChartVisual {}

if (!customElements.get('ld-echart')) customElements.define('ld-echart', EChartVisual)
if (!customElements.get('ld-line-chart')) customElements.define('ld-line-chart', LegacyLineChart)
if (!customElements.get('ld-bar-chart')) customElements.define('ld-bar-chart', LegacyBarChart)
if (!customElements.get('ld-kpi-strip')) customElements.define('ld-kpi-strip', KPIStrip)
