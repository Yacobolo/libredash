import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import * as echarts from 'echarts'
import type { ECharts, EChartsOption } from 'echarts'

type ChartType = 'line' | 'area' | 'bar' | 'column' | 'pie' | 'donut' | 'scatter' | 'funnel' | 'treemap' | 'gauge'

type ChartPoint = {
  label: string
  value: number
  selected?: boolean
}

type ChartPayload = {
  version?: number
  id?: string
  type?: ChartType | string
  title?: string
  unit?: string
  field?: string
  selection?: string[]
  data?: ChartPoint[]
  options?: Record<string, unknown>
}

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
    background: var(--bgColor-default);
  }

  header {
    display: flex;
    min-height: 42px;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 12px 8px;
  }

  h2 {
    margin: 0;
    font-size: 0.98rem;
    font-weight: 850;
    letter-spacing: 0;
  }

  .unit {
    color: var(--fgColor-muted);
    font-size: 0.72rem;
    font-weight: 900;
    text-transform: uppercase;
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
    background: var(--bgColor-muted);
    color: var(--fgColor-muted);
    font-weight: 800;
  }
`

class EChartVisual extends LitElement {
  @property({ type: Object }) chart: ChartPayload | null = null
  @property({ type: Array }) data: ChartPoint[] = []
  @property({ type: String, attribute: 'chart-title' }) chartTitle = 'Chart'
  @property({ type: String }) unit = ''
  @property({ type: String, attribute: 'visual-id' }) visualId = ''
  @property({ type: String }) field = ''
  @property({ type: String }) type: ChartType | string = 'bar'
  @property({ type: Array }) selection: string[] = []

  static styles = chartStyles

  private instance?: ECharts
  private observer?: ResizeObserver

  connectedCallback(): void {
    super.connectedCallback()
    this.observer = new ResizeObserver(() => this.instance?.resize())
  }

  firstUpdated(): void {
    const canvas = this.renderRoot.querySelector('.canvas') as HTMLDivElement | null
    if (!canvas) return
    this.instance = echarts.init(canvas, null, { renderer: 'canvas' })
    this.instance.on('click', (event) => {
      const label = String(event.name || event.data?.name || '')
      if (label) this.selectLabel(label)
    })
    this.observer?.observe(this)
    this.renderChart()
  }

  updated(): void {
    this.renderChart()
  }

  disconnectedCallback(): void {
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
          <h2>${payload.title ?? 'Chart'}</h2>
          <span class="unit">${payload.unit ?? ''}</span>
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
    this.instance.setOption(buildOption(payload, stylesFor(this)), true)
    this.instance.resize()
  }

  private get payload(): ChartPayload {
    const chart = this.chart ?? {}
    return {
      version: chart.version ?? 1,
      id: chart.id || this.visualId,
      type: normalizeType(chart.type || this.type),
      title: chart.title || this.chartTitle,
      unit: chart.unit ?? this.unit,
      field: chart.field || this.field,
      selection: chart.selection ?? this.selection ?? [],
      data: chart.data ?? this.data ?? [],
      options: chart.options ?? {},
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
      background: var(--bgColor-default);
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

function buildOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const type = normalizeType(payload.type)
  const data = payload.data ?? []
  const selected = new Set([...(payload.selection ?? []), ...data.filter((point) => point.selected).map((point) => point.label)])
  const hasSelection = selected.size > 0
  const itemData = data.map((point, index) => ({
    name: point.label,
    value: point.value,
    selected: selected.has(point.label),
    itemStyle: {
      color: tokens.palette[index % tokens.palette.length],
      opacity: hasSelection && !selected.has(point.label) ? 0.35 : 1,
    },
  }))
  const dataset = [['label', 'value'], ...data.map((point) => [point.label, point.value])]
  const base: EChartsOption = {
    backgroundColor: 'transparent',
    color: tokens.palette,
    aria: { show: true },
    animationDuration: 220,
    animationDurationUpdate: 260,
    tooltip: {
      trigger: type === 'line' || type === 'area' || type === 'bar' || type === 'scatter' ? 'axis' : 'item',
      valueFormatter: (value) => formatValue(Number(value), payload.unit),
      borderColor: tokens.border,
      backgroundColor: tokens.surface,
      textStyle: { color: tokens.text },
    },
    grid: {
      top: 16,
      right: 20,
      bottom: 32,
      left: 44,
      containLabel: true,
    },
  }

  if (type === 'pie' || type === 'donut') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'pie',
          radius: type === 'donut' ? ['48%', '72%'] : ['0%', '72%'],
          center: ['50%', '52%'],
          data: itemData,
          selectedMode: 'multiple',
          label: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
          universalTransition: true,
        },
      ],
    }
  }

  if (type === 'funnel') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'funnel',
          left: '8%',
          top: 18,
          width: '84%',
          bottom: 18,
          sort: 'descending',
          data: itemData,
          label: { color: tokens.text, fontSize: 10, fontWeight: 700 },
        },
      ],
    }
  }

  if (type === 'treemap') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'treemap',
          roam: false,
          nodeClick: false,
          breadcrumb: { show: false },
          data: itemData,
          label: { color: tokens.text, fontSize: 10, fontWeight: 800 },
          upperLabel: { show: false },
        },
      ],
    }
  }

  if (type === 'gauge') {
    const point = data[0]
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'gauge',
          min: 0,
          max: Math.max(100, Math.ceil((point?.value ?? 0) * 1.2)),
          progress: { show: true, width: 12 },
          axisLine: { lineStyle: { width: 12, color: [[1, tokens.grid]] } },
          axisTick: { show: false },
          splitLine: { length: 8, lineStyle: { color: tokens.border } },
          axisLabel: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
          pointer: { width: 4 },
          anchor: { show: true, size: 6, itemStyle: { color: tokens.palette[0] } },
          detail: {
            valueAnimation: true,
            color: tokens.text,
            fontSize: 24,
            fontWeight: 850,
            formatter: (value: number) => formatValue(value, payload.unit),
          },
          data: [
            {
              name: point?.label ?? payload.title,
              value: point?.value ?? 0,
              itemStyle: { color: tokens.palette[0] },
            },
          ],
        },
      ],
    }
  }

  const horizontal = type === 'bar'
  const seriesType = type === 'area' ? 'line' : type === 'column' ? 'bar' : type
  return {
    ...base,
    dataset: { source: dataset },
    xAxis: horizontal
      ? axis('value', tokens)
      : {
          ...axis('category', tokens),
          axisLabel: {
            color: tokens.muted,
            fontWeight: 700,
            fontSize: 10,
            interval: Math.ceil(data.length / 6),
          },
        },
    yAxis: horizontal
      ? {
          ...axis('category', tokens),
          inverse: true,
          axisLabel: { color: tokens.text, fontWeight: 750, fontSize: 10 },
        }
      : axis('value', tokens),
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: seriesType,
        encode: horizontal ? { y: 'label', x: 'value' } : { x: 'label', y: 'value' },
        datasetIndex: 0,
        smooth: type === 'line' || type === 'area',
        areaStyle: type === 'area' ? { color: tokens.fill, opacity: 0.7 } : undefined,
        symbolSize: type === 'scatter' ? 9 : 7,
        barMaxWidth: 18,
        itemStyle: {
          color: (params: { dataIndex: number; name: string }) => {
            const point = data[params.dataIndex]
            const isSelected = selected.has(params.name || point?.label)
            return hasSelection && !isSelected ? tokens.dimmed : tokens.palette[params.dataIndex % tokens.palette.length]
          },
        },
        lineStyle: { color: tokens.palette[0], width: 2.5 },
        universalTransition: true,
      },
    ],
  }
}

function axis(type: 'category' | 'value', tokens: ChartTokens) {
  return {
    type,
    axisLine: { lineStyle: { color: tokens.border } },
    axisTick: { show: false },
    axisLabel: { color: tokens.muted, fontWeight: 700, fontSize: 10 },
    splitLine: { lineStyle: { color: tokens.grid } },
  }
}

type ChartTokens = {
  text: string
  muted: string
  border: string
  grid: string
  surface: string
  fill: string
  dimmed: string
  palette: string[]
}

function stylesFor(element: HTMLElement): ChartTokens {
  const styles = getComputedStyle(element)
  const value = (name: string, fallback: string) => styles.getPropertyValue(name).trim() || fallback
  return {
    text: value('--fgColor-default', '#1f2328'),
    muted: value('--fgColor-muted', '#59636e'),
    border: value('--borderColor-default', '#d0d7de'),
    grid: value('--ld-chart-grid', value('--borderColor-muted', '#d8dee4')),
    surface: value('--bgColor-default', '#ffffff'),
    fill: value('--ld-chart-1-muted', 'rgba(84, 174, 255, .35)'),
    dimmed: value('--borderColor-muted', '#d8dee4'),
    palette: [
      value('--ld-chart-1', '#0969da'),
      value('--ld-chart-2', '#1a7f37'),
      value('--ld-chart-3', '#8250df'),
      value('--ld-chart-4', '#cf222e'),
      value('--ld-chart-5', '#116329'),
      value('--ld-chart-6', '#bf3989'),
    ],
  }
}

function normalizeType(type: string | undefined): ChartType {
  switch (type) {
    case 'line_chart':
      return 'line'
    case 'area_chart':
      return 'area'
    case 'bar_chart':
      return 'bar'
    case 'column_chart':
      return 'column'
    case 'pie_chart':
      return 'pie'
    case 'donut_chart':
      return 'donut'
    case 'scatter_chart':
      return 'scatter'
    case 'funnel_chart':
      return 'funnel'
    case 'treemap_chart':
      return 'treemap'
    case 'gauge_chart':
      return 'gauge'
    case 'line':
    case 'area':
    case 'bar':
    case 'column':
    case 'pie':
    case 'donut':
    case 'scatter':
    case 'funnel':
    case 'treemap':
    case 'gauge':
      return type
    default:
      return 'bar'
  }
}

function formatValue(value: number, unit?: string): string {
  if (!Number.isFinite(value)) return '-'
  const formatted = formatCompact(value)
  if (unit === 'R$') return `R$ ${formatted}`
  return formatted
}

function formatCompact(value: number): string {
  if (Math.abs(value) >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`
  if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return value.toLocaleString(undefined, { maximumFractionDigits: 0 })
}

class LegacyLineChart extends EChartVisual {}
class LegacyBarChart extends EChartVisual {}

if (!customElements.get('ld-echart')) customElements.define('ld-echart', EChartVisual)
if (!customElements.get('ld-line-chart')) customElements.define('ld-line-chart', LegacyLineChart)
if (!customElements.get('ld-bar-chart')) customElements.define('ld-bar-chart', LegacyBarChart)
if (!customElements.get('ld-kpi-strip')) customElements.define('ld-kpi-strip', KPIStrip)
