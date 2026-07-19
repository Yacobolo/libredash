import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import { EllipsisVertical } from 'lucide'
import { lucideIcon } from '../../shared/lucide-icons'
import { visualMenuIcon } from '../visual-menu-icons'
import { visualActionStyles } from '../visual-action-styles'
import './renderers'
import { chartInteractionDetailForDatum } from './interactions'
import { chartRenderer } from './registry'
import type { ChartDatum, ChartPayload, ChartRendererHandle, ChartType, InteractionSelectionEntry, VisualAction } from './types'
import { chartColumns, chartRows, formatValue, normalizeShape, normalizeType, numberValue, stringValue, stylesFor } from './utils'

const chartStyles = css`
  :host {
    display: block;
    height: 100%;
    min-height: 0;
    color: var(--ld-fg-default);
    font-family: var(--fontStack-system);
  }

  .chart {
    display: grid;
    height: 100%;
    min-height: 0;
    grid-template-rows: auto minmax(0, 1fr);
    background: var(--ld-chart-surface);
  }

  header {
    display: flex;
    min-height: calc(var(--control-medium-size) + var(--base-size-2));
    align-items: center;
    justify-content: space-between;
    gap: var(--base-size-8);
    padding: var(--control-medium-paddingBlock) var(--control-medium-paddingInline-normal);
  }

  h2 {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-strong);
    letter-spacing: 0;
    line-height: var(--ld-line-height-compact);
  }

  .unit {
    flex: 0 0 auto;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
    text-transform: uppercase;
  }

  .header-main {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: var(--base-size-8);
  }

  .options {
    position: relative;
    flex: 0 0 auto;
  }

  .options summary {
    display: grid;
    width: var(--ld-button-height-xs, var(--control-xsmall-size));
    height: var(--ld-button-height-xs, var(--control-xsmall-size));
    place-items: center;
    border: var(--borderWidth-default, var(--ld-border-width)) solid var(--ld-button-invisible-border-rest, var(--control-transparent-borderColor-rest, var(--ld-line-muted)));
    border-radius: var(--ld-radius-tight);
    background: var(--ld-button-invisible-bg-rest, var(--control-transparent-bgColor-rest, var(--ld-bg-panel)));
    color: var(--ld-button-invisible-icon-rest, var(--ld-icon-muted));
    cursor: pointer;
    font-size: var(--ld-font-size-body-lg);
    font-weight: var(--ld-font-weight-strong);
    line-height: var(--ld-line-height-none);
    list-style: none;
  }

  .options summary::-webkit-details-marker {
    display: none;
  }

  .options summary svg {
    width: var(--base-size-16);
    height: var(--base-size-16);
  }

  .options summary:hover,
  .options summary:focus-visible,
  .options[open] summary {
    border-color: var(--ld-button-invisible-border-hover, var(--control-transparent-borderColor-hover, var(--ld-line-default)));
    background: var(--ld-button-invisible-bg-hover, var(--control-transparent-bgColor-hover, var(--ld-bg-panel-muted)));
    color: var(--ld-icon-default);
    outline: var(--focus-outline, var(--ld-border-default));
    outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
    outline-offset: var(--focus-outline-offset, var(--base-size-2));
  }

  .menu {
    position: absolute;
    top: calc(100% + var(--base-size-4));
    right: 0;
    z-index: var(--zIndex-dropdown);
    display: grid;
    width: calc(var(--overlay-width-xsmall) - var(--base-size-16));
    border: var(--ld-border-default);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-overlay);
    box-shadow: var(--shadow-floating-small);
    padding: var(--base-size-4);
  }

  .menu button {
    display: flex;
    align-items: center;
    gap: var(--base-size-8);
    min-height: var(--ld-button-height-sm, var(--control-small-size));
    border: var(--borderWidth-default, var(--ld-border-width)) solid var(--ld-button-invisible-border-rest, var(--control-transparent-borderColor-rest, var(--ld-line-muted)));
    border-radius: var(--ld-radius-tight);
    background: var(--ld-button-invisible-bg-rest, var(--control-transparent-bgColor-rest, var(--ld-bg-panel)));
    color: var(--ld-button-invisible-fg-rest, var(--ld-fg-default));
    cursor: pointer;
    padding: 0 var(--ld-button-padding-inline-xs, var(--control-xsmall-paddingInline-normal));
    font: inherit;
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    text-align: left;
  }

  .menu svg {
    flex: 0 0 auto;
    width: var(--base-size-16);
    height: var(--base-size-16);
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 2;
  }

  .menu button:hover,
  .menu button:focus-visible {
    border-color: var(--ld-button-invisible-border-hover, var(--control-transparent-borderColor-hover, var(--ld-line-default)));
    background: var(--ld-button-invisible-bg-hover, var(--control-transparent-bgColor-hover, var(--ld-bg-panel-muted)));
    outline: var(--focus-outline, var(--ld-border-default));
    outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
    outline-offset: var(--focus-outline-offset, var(--base-size-2));
  }

  .menu button:disabled {
    cursor: default;
    opacity: var(--opacity-disabled);
  }

  .menu button:disabled:hover {
    background: var(--ld-button-invisible-bg-rest, var(--control-transparent-bgColor-rest, var(--ld-bg-panel)));
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
    border: 1px dashed var(--ld-line-default);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
    font-weight: var(--ld-font-weight-medium);
  }
`

class ChartVisual extends LitElement {
  @property({ type: Object }) chart: ChartPayload | null = null
  @property({ type: Array }) data: ChartDatum[] = []
  @property({ type: String, attribute: 'chart-title' }) chartTitle = 'Chart'
  @property({ type: String }) unit = ''
  @property({ type: String, attribute: 'visual-id' }) visualId = ''
  @property({ type: String }) type: ChartType | string = 'bar'
  @property({ type: Array }) selection: InteractionSelectionEntry[] = []

  static styles = [visualActionStyles, chartStyles]

  private rendererHandle?: ChartRendererHandle
  private rendererName = ''
  private rendererInputSignature = ''
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
    this.observer = new ResizeObserver(() => this.rendererHandle?.resize())
    document.addEventListener('pointerdown', this.handleOutsidePointerDown)
    document.addEventListener('keydown', this.handleDocumentKeyDown)
    if (this.hasUpdated) {
      queueMicrotask(() => {
        this.observer?.observe(this)
        this.renderChart()
      })
    }
  }

  firstUpdated(): void {
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
    this.rendererHandle?.dispose()
    this.rendererHandle = undefined
    this.rendererName = ''
    this.rendererInputSignature = ''
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
          <div class="visual-actions">
            <button class="icon-action" type="button" aria-label="Expand visual" title="Expand visual" @click=${() => this.runAction('focus')}>${visualMenuIcon('focus')}</button>
            <details class="options">
              <summary aria-label="Visual options" title="Visual options">${lucideIcon(EllipsisVertical)}</summary>
              <div class="menu" role="menu">
                <button type="button" role="menuitem" @click=${() => this.runAction('show-data')}>${visualMenuIcon('show-data')}<span>Show data</span></button>
                <button type="button" role="menuitem" @click=${() => this.runAction('copy-data')}>${visualMenuIcon('copy-data')}<span>Copy data</span></button>
                <button type="button" role="menuitem" @click=${() => this.runAction('export-csv')}>${visualMenuIcon('export-csv')}<span>Export CSV</span></button>
                <button type="button" role="menuitem" ?disabled=${!this.hasSelection(payload)} @click=${() => this.runAction('clear-selection')}>${visualMenuIcon('clear-selection')}<span>Clear selection</span></button>
              </div>
            </details>
          </div>
        </header>
        <div class=${data.length === 0 ? 'canvas idle' : 'canvas'}></div>
        ${data.length === 0 ? html`<div class="empty">Waiting for signal data</div>` : null}
      </section>
    `
  }

  private renderChart(): void {
    const payload = this.payload
    const tokens = stylesFor(this)
    const signature = JSON.stringify([payload, tokens])
    if (signature === this.rendererInputSignature) return
    this.rendererInputSignature = signature
    const data = payload.data ?? []
    if (data.length === 0) {
      this.rendererHandle?.clear()
      return
    }
    const renderer = this.ensureRenderer(payload.renderer)
    if (!renderer) {
      this.rendererHandle?.clear()
      return
    }
    renderer.update(payload, tokens)
  }

  private ensureRenderer(name: string | undefined): ChartRendererHandle | undefined {
    const nextName = name || 'echarts'
    if (this.rendererHandle && this.rendererName === nextName) return this.rendererHandle
    this.rendererHandle?.dispose()
    this.rendererHandle = undefined
    this.rendererName = ''

    const canvas = this.renderRoot.querySelector('.canvas') as HTMLDivElement | null
    const renderer = chartRenderer(nextName)
    if (!canvas || !renderer) return undefined

    this.rendererHandle = renderer.mount(canvas, { selectDatum: (datum, index) => this.selectDatum(datum, index) })
    this.rendererName = nextName
    return this.rendererHandle
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
      format: chart.format ?? '',
      interaction: chart.interaction ?? {},
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

  private selectDatum(datum: ChartDatum, _index: number): void {
    const payload = this.payload
    const detail = chartInteractionDetailForDatum(payload, datum)
    if (!detail) return
    this.dispatchEvent(
      new CustomEvent('ld-interaction-select', {
        bubbles: true,
        composed: true,
        detail,
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
      this.dispatchEvent(
        new CustomEvent('ld-interaction-select', {
          bubbles: true,
          composed: true,
          detail: {
            sourceKind: 'visual',
            sourceId: payload.id || this.visualId,
            interactionKind: payload.interaction?.kind || 'point_selection',
            action: 'clear',
            toggle: payload.interaction?.toggle !== false,
            mappings: [],
          },
        }),
      )
    }
  }

  private hasSelection(payload: ChartPayload): boolean {
    return Boolean(payload.selection?.length || payload.data?.some((row) => row.selected))
  }
}

class KPICard extends LitElement {
  @property({ type: Object }) visual: ChartPayload | null = null
  @property({ type: String, attribute: 'visual-id' }) visualId = ''

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      container-type: inline-size;
    }

    .kpi {
      display: grid;
      height: 100%;
      min-height: 0;
      position: relative;
      min-height: 104px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-chart-surface);
      box-shadow: var(--shadow-resting-small);
      padding: 12px 14px 12px 16px;
      overflow: hidden;
      align-content: center;
    }

    .kpi::before {
      content: '';
      position: absolute;
      inset-block: 0;
      left: 0;
      width: 5px;
      background: var(--ld-line-muted);
    }

    .label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      text-transform: uppercase;
    }

    .value {
      margin: 8px 0 4px;
      font-size: clamp(1.75rem, 7cqi, var(--ld-font-size-display));
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
      letter-spacing: 0;
      white-space: nowrap;
    }

    .note {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-compact);
      overflow-wrap: anywhere;
    }

    .green::before {
      background: var(--ld-data-2);
    }
    .amber::before {
      background: var(--ld-accent);
    }
    .coral::before {
      background: var(--ld-data-4);
    }
    .ink::before {
      background: var(--ld-data-1);
    }
    .neutral::before {
      background: var(--ld-line-muted);
    }

    @container (max-width: 220px) {
      .kpi {
        padding-inline: 12px 10px;
      }

      .value {
        margin-block: 6px 3px;
        font-size: clamp(1.35rem, 12cqi, 1.85rem);
      }

      .note {
        font-size: var(--ld-font-size-body-sm);
      }
    }

  `

  render() {
    const payload = this.payload
    const point = payload.data?.[0] ?? {}
    const tone = String(payload.options?.tone || 'neutral')
    const note = String(payload.options?.note || '')
    return html`
      <article class="kpi ${tone}">
        <div class="label">${payload.title || stringValue(point, 'label') || 'Metric'}</div>
        <div class="value">${formatKPIValue(numberValue(point, 'value'), payload.format, payload.unit)}</div>
        <div class="note">${note}</div>
      </article>
    `
  }

  private get payload(): ChartPayload {
    return {
      version: this.visual?.version ?? 3,
      id: this.visual?.id || this.visualId,
      kind: this.visual?.kind || 'kpi',
      shape: this.visual?.shape || 'single_value',
      renderer: this.visual?.renderer || 'html',
      type: this.visual?.type || 'kpi',
      title: this.visual?.title || '',
      unit: this.visual?.unit || '',
      format: this.visual?.format || '',
      options: this.visual?.options ?? {},
      data: this.visual?.data ?? [],
    }
  }
}

function formatKPIValue(value: number, format?: string, unit?: string): string {
  if (!Number.isFinite(value)) return '-'
  if (format === 'currency') return formatValue(value, unit || 'R$')
  if (format === 'integer') return formatValue(value, unit)
  if (format === 'decimal') return value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return formatValue(value, unit)
}

class LegacyLineChart extends ChartVisual {}
class LegacyBarChart extends ChartVisual {}

if (!customElements.get('ld-echart')) customElements.define('ld-echart', ChartVisual)
if (!customElements.get('ld-line-chart')) customElements.define('ld-line-chart', LegacyLineChart)
if (!customElements.get('ld-bar-chart')) customElements.define('ld-bar-chart', LegacyBarChart)
if (!customElements.get('ld-kpi-card')) customElements.define('ld-kpi-card', KPICard)
