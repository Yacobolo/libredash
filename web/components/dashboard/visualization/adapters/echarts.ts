import type { VisualizationEnvelope, VisualizationFieldRef } from '../../../../generated/visualization'
import type { ECharts, EChartsOption } from 'echarts'
import type { RendererAdapter, RendererHandle } from '../host-controller'
import { interactionCommandForRow } from '../interaction-command'

export { interactionCommandForRow } from '../interaction-command'

export function echartsOption(envelope: VisualizationEnvelope): EChartsOption {
  const dataset = inlineDataset(envelope)
  const source = dataset ? selectedDatasetSource(envelope, dataset) : []
  const spec = envelope.spec
  const base: EChartsOption = {
    animation: false,
    aria: { enabled: true, description: spec.accessibility.description },
    dataset: { source: source as any },
    tooltip: { trigger: 'item' },
    title: envelope.status.kind === 'error' ? { text: envelope.status.message ?? 'Visualization error' } : undefined,
    visualMap: envelope.selection.length > 0 ? { show: false, dimension: '__ld_selected', pieces: [{ value: true, opacity: 1 }, { value: false, opacity: 0.35 }] } as any : undefined,
  }

  switch (spec.kind) {
    case 'cartesian': {
      return cartesianOption(base, envelope)
    }
    case 'proportional':
      return {
        ...base,
        legend: legend(spec.presentation.legend),
        series: [{
          type: spec.mark === 'funnel' ? 'funnel' : 'pie',
          radius: spec.mark === 'donut' ? ['45%', '72%'] : undefined,
          encode: { itemName: spec.category.field, value: spec.value.field },
          label: { show: spec.presentation.showLabels },
          roseType: spec.presentation.rose ? 'radius' : false,
          orient: spec.presentation.orientation,
        } as any],
      }
    case 'hierarchy':
      return hierarchyOption(base, envelope)
    case 'polar':
      if (spec.mark === 'gauge') {
        const value = firstScalar(envelope, spec.value)
        const minimum = spec.presentation.minimum ?? 0, maximum = spec.presentation.maximum ?? 100
        const colors = (spec.presentation.thresholds ?? []).map((threshold) => [Math.max(0, Math.min(1, (threshold.value - minimum) / (maximum - minimum))), toneColor(threshold.tone)])
        return { ...base, series: [{ type: 'gauge', min: minimum, max: maximum, data: [{ value }], pointer: { show: spec.presentation.showPointer }, progress: { show: true, width: spec.presentation.progressWidth }, axisLine: colors.length ? { lineStyle: { color: colors } } : undefined }] as any }
      }
      return radarOption(base, envelope)
    default:
      throw new Error(`ECharts cannot render visualization kind ${JSON.stringify(spec.kind)}`)
  }
}

function selectedDatasetSource(envelope: VisualizationEnvelope, dataset: Extract<VisualizationEnvelope['dataState'], { kind: 'inline' }>['datasets'][number]): unknown[][] {
  if (envelope.selection.length === 0) return [dataset.columns, ...dataset.rows]
  const schema = envelope.spec.datasets.find((candidate) => candidate.id === dataset.id)
  const identityFields = (schema?.fields ?? []).filter((field) => field.role === 'identity')
  if (identityFields.length === 0) return [dataset.columns, ...dataset.rows]
  const selected = envelope.selection.filter((entry) => entry.datum.dataset === dataset.id && entry.datum.dataRevision === envelope.dataRevision)
  const rows = dataset.rows.map((row) => {
    const matches = selected.some((entry) => identityFields.every((field) => Object.is(row[dataset.columns.indexOf(field.id)], entry.datum.identity[field.id])))
    return [...row, matches]
  })
  return [[...dataset.columns, '__ld_selected'], ...rows]
}

function cartesianOption(base: EChartsOption, envelope: VisualizationEnvelope): EChartsOption {
  const spec = envelope.spec
  if (spec.kind !== 'cartesian') return base
  const horizontal = spec.presentation.orientation === 'horizontal' || spec.mark === 'bar'
  const axes: Pick<EChartsOption, 'xAxis' | 'yAxis'> = horizontal
    ? { xAxis: { type: 'value' }, yAxis: { type: 'category' } }
    : { xAxis: { type: 'category' }, yAxis: { type: 'value' } }
  const dataZoom = spec.presentation.dataZoom ? [{ type: 'inside' }, { type: 'slider' }] : undefined
  if (spec.mark === 'histogram') {
    const value = spec.y.find((field) => field.field === 'value') ?? spec.y.at(-1)
    return { ...base, ...axes, dataZoom, series: [{ type: 'bar', encode: { x: spec.x.field, y: value?.field }, label: { show: spec.presentation.showLabels } }] as any }
  }
  if (spec.mark === 'waterfall') {
    const start = spec.y.find((field) => field.field === 'start')
    const value = spec.y.find((field) => field.field === 'value') ?? spec.y[0]
    return {
      ...base, ...axes, dataZoom,
      series: [
        { type: 'bar', stack: 'waterfall', silent: true, itemStyle: { color: 'transparent' }, encode: { x: spec.x.field, y: start?.field } },
        { type: 'bar', stack: 'waterfall', encode: { x: spec.x.field, y: value?.field }, label: { show: spec.presentation.showLabels } },
      ] as any,
    }
  }
  const multiValue = spec.mark === 'candlestick' || spec.mark === 'boxplot'
  if (multiValue) {
    return {
      ...base, ...axes, dataZoom, legend: legend(spec.presentation.legend),
      series: [{ type: cartesianSeriesType(spec.mark), name: spec.title, encode: { x: spec.x.field, y: spec.y.map((field) => field.field) } }] as any,
    }
  }
  if (spec.mark === 'heatmap' && spec.y.length >= 2) {
    return {
      ...base, xAxis: { type: 'category' }, yAxis: { type: 'category' },
      visualMap: { min: 0, calculable: true, orient: 'horizontal', left: 'center', bottom: 0 },
      series: [{ type: 'heatmap', encode: { x: spec.x.field, y: spec.y[0]?.field, value: spec.y[1]?.field }, label: { show: spec.presentation.showLabels } }] as any,
    }
  }
  const splitSeries = splitCartesianSeries(envelope)
  if (splitSeries) {
    const secondary = splitSeries.series.some((series) => series.yAxisIndex === 1)
    return {
      ...base,
      dataset: splitSeries.datasets as any,
      legend: legend(spec.presentation.legend),
      xAxis: { type: 'category' },
      yAxis: secondary ? [{ type: 'value' }, { type: 'value' }] : { type: 'value' },
      dataZoom,
      series: splitSeries.series as any,
    }
  }
  const series = spec.y.map((value) => ({
    type: cartesianSeriesType(spec.mark),
    name: fieldLabel(spec, value),
    encode: horizontal ? { x: value.field, y: spec.x.field } : { x: spec.x.field, y: value.field },
    smooth: spec.presentation.smooth,
    symbol: spec.presentation.showSymbols ? undefined : 'none',
    symbolSize: spec.presentation.symbolSize,
    stack: spec.presentation.stacked ? 'total' : undefined,
    areaStyle: spec.presentation.area || spec.mark === 'area' ? {} : undefined,
    step: spec.presentation.step ? 'middle' : false,
    label: { show: spec.presentation.showLabels, position: spec.presentation.labelPosition },
  }))
  return { ...base, ...axes, legend: legend(spec.presentation.legend), dataZoom, series: series as any }
}

function splitCartesianSeries(envelope: VisualizationEnvelope): { datasets: object[]; series: Array<object & { yAxisIndex: number }> } | undefined {
  const spec = envelope.spec
  if (spec.kind !== 'cartesian' || !spec.series || spec.y.length !== 1) return undefined
  const dataset = inlineDataset(envelope)
  if (!dataset) return undefined
  const seriesIndex = dataset.columns.indexOf(spec.series.field)
  if (seriesIndex < 0) return undefined
  const values = [...new Set(dataset.rows.map((row) => row[seriesIndex]).filter((value): value is string | number | boolean => typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'))]
  const configured = new Map((spec.presentation.comboSeries ?? []).map((item) => [String(item.seriesValue), item]))
  const source = [dataset.columns, ...dataset.rows]
  const datasets: object[] = [{ id: 'source', source }]
  const series = values.map((value, index) => {
    const datasetID = `series-${index}`
    datasets.push({ id: datasetID, fromDatasetId: 'source', transform: { type: 'filter', config: { dimension: spec.series?.field, '=': value } } })
    const combo = configured.get(String(value))
    const mark = combo?.mark ?? (spec.mark === 'combo' ? 'line' : spec.mark)
    return {
      datasetId: datasetID,
      name: String(value),
      type: cartesianSeriesType(mark),
      yAxisIndex: combo?.axis === 'secondary' ? 1 : 0,
      encode: { x: spec.x.field, y: spec.y[0]?.field },
      smooth: spec.presentation.smooth,
      symbol: spec.presentation.showSymbols ? undefined : 'none',
      stack: spec.presentation.stacked ? 'total' : undefined,
      areaStyle: spec.presentation.area || mark === 'area' ? {} : undefined,
      step: spec.presentation.step ? 'middle' : false,
      label: { show: spec.presentation.showLabels, position: spec.presentation.labelPosition },
    }
  })
  return { datasets, series }
}

export const adapter: RendererAdapter = {
  async mount(container, envelope) {
    const echarts = await import('echarts')
    const chart = echarts.init(container, undefined, { renderer: 'canvas' })
    return new EChartsHandle(container, chart, envelope)
  },
}

class EChartsHandle implements RendererHandle {
  private envelope: VisualizationEnvelope

  constructor(private readonly container: HTMLElement, private readonly chart: ECharts, envelope: VisualizationEnvelope) {
    this.envelope = envelope
    this.chart.setOption(echartsOption(envelope), { notMerge: true })
    this.chart.on('click', this.handleClick)
  }

  update(envelope: VisualizationEnvelope): void {
    this.envelope = envelope
    this.chart.setOption(echartsOption(envelope), { notMerge: true, lazyUpdate: true })
  }
  resize(width: number, height: number): void { this.chart.resize({ width, height, silent: true }) }
  async snapshot(): Promise<Blob> {
    const response = await fetch(this.chart.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: 'transparent' }))
    return response.blob()
  }
  dispose(): void {
    this.chart.off('click', this.handleClick)
    this.chart.dispose()
  }

  private readonly handleClick = (params: unknown) => {
    const row = (params as { value?: unknown })?.value
    if (!Array.isArray(row)) return
    const interaction = this.envelope.spec.interactions.find((candidate) => candidate.kind === 'select')
    const datasetID = interaction?.mappings[0]?.source.dataset
    if (!datasetID) return
    const command = interactionCommandForRow(this.envelope, datasetID, row)
    if (!command) return
    this.container.dispatchEvent(new CustomEvent('ld-interaction-select', { bubbles: true, composed: true, detail: command }))
  }
}

function inlineDataset(envelope: VisualizationEnvelope) {
  if (envelope.dataState.kind !== 'inline') return undefined
  return envelope.dataState.datasets[0]
}

function firstScalar(envelope: VisualizationEnvelope, ref: VisualizationFieldRef): unknown {
  if (envelope.dataState.kind !== 'inline') return undefined
  const dataset = envelope.dataState.datasets.find((candidate) => candidate.id === ref.dataset)
  const index = dataset?.columns.indexOf(ref.field) ?? -1
  return index >= 0 ? dataset?.rows[0]?.[index] : undefined
}

function radarOption(base: EChartsOption, envelope: VisualizationEnvelope): EChartsOption {
  const spec = envelope.spec
  if (spec.kind !== 'polar' || spec.mark !== 'radar') return base
  const dataset = inlineDataset(envelope)
  if (!dataset) return base
  const categoryIndex = spec.category ? dataset.columns.indexOf(spec.category.field) : -1
  const valueIndex = dataset.columns.indexOf(spec.value.field)
  const seriesIndex = spec.series ? dataset.columns.indexOf(spec.series.field) : -1
  const categories = [...new Set(dataset.rows.map((row, index) => String(categoryIndex >= 0 ? row[categoryIndex] : index + 1)))]
  const seriesValues = [...new Set(dataset.rows.map((row) => String(seriesIndex >= 0 ? row[seriesIndex] : spec.title)))]
  const values = seriesValues.map((series) => ({
    name: series,
    value: categories.map((category) => dataset.rows.find((row, index) => String(seriesIndex >= 0 ? row[seriesIndex] : spec.title) === series && String(categoryIndex >= 0 ? row[categoryIndex] : index + 1) === category)?.[valueIndex] ?? null),
  }))
  const maxima = categories.map((_, index) => Math.max(1, ...values.map((series) => typeof series.value[index] === 'number' ? series.value[index] as number : 0)))
  return { ...base, dataset: undefined, legend: legend(spec.presentation.legend), radar: { indicator: categories.map((name, index) => ({ name, max: maxima[index] })) }, series: [{ type: 'radar', data: values, areaStyle: spec.presentation.area ? {} : undefined }] as any }
}

function toneColor(tone: string): string {
  switch (tone) {
    case 'success': return '#1a7f37'
    case 'warning': return '#9a6700'
    case 'danger': return '#cf222e'
    case 'ink': return '#24292f'
    default: return '#0969da'
  }
}

function fieldLabel(spec: VisualizationEnvelope['spec'], ref: VisualizationFieldRef): string {
  return spec.datasets.find((dataset) => dataset.id === ref.dataset)?.fields.find((field) => field.id === ref.field)?.label ?? ref.field
}

function cartesianSeriesType(mark: Extract<VisualizationEnvelope['spec'], { kind: 'cartesian' }>['mark']): string {
  switch (mark) {
    case 'bar': case 'column': case 'waterfall': case 'histogram': return 'bar'
    case 'scatter': return 'scatter'
    case 'candlestick': return 'candlestick'
    case 'boxplot': return 'boxplot'
    default: return 'line'
  }
}

function legend(position: string): object | undefined {
  if (position === 'hidden') return undefined
  return { show: true, orient: position === 'left' || position === 'right' ? 'vertical' : 'horizontal', [position]: 0 }
}

function hierarchyOption(base: EChartsOption, envelope: VisualizationEnvelope): EChartsOption {
  const spec = envelope.spec
  if (spec.kind !== 'hierarchy') return base
  const dataset = inlineDataset(envelope)
  const columns = dataset?.columns ?? []
  const index = (ref?: VisualizationFieldRef) => ref ? columns.indexOf(ref.field) : -1
  const nodeIndex = index(spec.node), valueIndex = index(spec.value), sourceIndex = index(spec.source), targetIndex = index(spec.target)
  if (spec.mark === 'sankey' || spec.mark === 'graph') {
    const links = (dataset?.rows ?? []).map((row) => ({ source: String(row[sourceIndex]), target: String(row[targetIndex]), value: valueIndex >= 0 ? row[valueIndex] : undefined }))
    const names = [...new Set(links.flatMap((link) => [link.source, link.target]))]
    return { ...base, dataset: undefined, series: [{ type: spec.mark, data: names.map((name) => ({ name })), links, roam: spec.presentation.roam }] as any }
  }
  const data = (dataset?.rows ?? []).map((row) => ({ name: String(row[nodeIndex]), value: valueIndex >= 0 ? row[valueIndex] : undefined }))
  return { ...base, dataset: undefined, series: [{ type: spec.mark, data, roam: spec.presentation.roam, initialTreeDepth: spec.presentation.initialDepth }] as any }
}
