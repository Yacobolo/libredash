import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import type { RendererContext } from '../../host-controller'
import { axis, field, fieldLabel, labelFormatter, legend, selectedDatasetSource, type EChartsTranslation } from './common'

type CartesianSpec = Extract<VisualizationEnvelope['spec'], { kind: 'cartesian' }>

export function cartesianOption(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation {
  const spec = envelope.spec as CartesianSpec
  const horizontal = spec.presentation.orientation === 'horizontal' || spec.mark === 'bar'
  const xType = axisType(envelope, spec.x, horizontal ? 'value' : 'category')
  const xAxis = axis(envelope, horizontal ? spec.y[0]! : spec.x, xType, context)
  const yAxis = axis(envelope, horizontal ? spec.x : spec.y[0]!, horizontal ? 'category' : 'value', context)
  const axes = { grid: { left: 12, right: 16, top: 16, bottom: spec.presentation.dataZoom ? 54 : 16, containLabel: true }, xAxis, yAxis }
  const dataZoom = spec.presentation.dataZoom ? [{ type: 'inside' }, { type: 'slider' }] : undefined
  if (spec.mark === 'histogram') {
    const value = spec.y.find((item) => item.field === 'value') ?? spec.y.at(-1)
    return { ...axes, dataZoom, series: [{ id: seriesID(value?.dataset, value?.field), type: 'bar', encode: { x: spec.x.field, y: value?.field }, label: chartLabel(envelope, value, spec, context) }] }
  }
  if (spec.mark === 'waterfall') {
    const start = spec.y.find((item) => item.field === 'start')
    const value = spec.y.find((item) => item.field === 'value') ?? spec.y[0]
    return {
      ...axes, dataZoom,
      series: [
        { id: 'series:waterfall:offset', type: 'bar', stack: 'waterfall', silent: true, itemStyle: { color: 'transparent' }, encode: { x: spec.x.field, y: start?.field } },
        { id: seriesID(value?.dataset, value?.field), type: 'bar', stack: 'waterfall', encode: { x: spec.x.field, y: value?.field }, label: chartLabel(envelope, value, spec, context) },
      ],
    }
  }
  if (spec.mark === 'candlestick' || spec.mark === 'boxplot') {
    return {
      ...axes, dataZoom, legend: legend(spec.presentation.legend, context),
      series: [{ id: `series:primary:${spec.mark}`, type: spec.mark, name: spec.title, encode: { x: spec.x.field, y: spec.y.map((item) => item.field) } }],
    }
  }
  if (spec.mark === 'heatmap' && spec.y.length >= 2) {
    return {
      xAxis: axis(envelope, spec.x, 'category', context), yAxis: axis(envelope, spec.y[0]!, 'category', context),
      visualMap: { min: 0, calculable: true, orient: 'horizontal', left: 'center', bottom: 0, textStyle: { color: context.colors.muted } },
      series: [{ id: 'series:primary:heatmap', type: 'heatmap', encode: { x: spec.x.field, y: spec.y[0]?.field, value: spec.y[1]?.field }, label: chartLabel(envelope, spec.y[1], spec, context) }],
    }
  }
  const split = splitCartesianSeries(envelope, context)
  if (split) {
    const secondary = split.series.some((item) => item.yAxisIndex === 1)
    return {
      dataset: split.datasets, legend: legend(spec.presentation.legend, context), xAxis: axis(envelope, spec.x, axisType(envelope, spec.x, 'category'), context),
      yAxis: secondary ? [axis(envelope, spec.y[0]!, 'value', context), axis(envelope, spec.y[0]!, 'value', context)] : axis(envelope, spec.y[0]!, 'value', context),
      dataZoom, series: [...split.series, ...interactionHitSeries(envelope, spec, split.series)],
    }
  }
  const series = spec.y.map((value) => ({
    id: seriesID(value.dataset, value.field), type: cartesianSeriesType(spec.mark), name: fieldLabel(envelope, value),
    encode: horizontal ? { x: value.field, y: spec.x.field } : { x: spec.x.field, y: value.field },
    smooth: spec.presentation.smooth, symbol: spec.presentation.showSymbols ? undefined : 'none', symbolSize: spec.presentation.symbolSize,
    stack: spec.presentation.stacked ? 'total' : undefined, areaStyle: spec.presentation.area || spec.mark === 'area' ? {} : undefined,
    step: spec.presentation.step ? 'middle' : false, label: chartLabel(envelope, value, spec, context),
  }))
  return { ...axes, legend: legend(spec.presentation.legend, context), dataZoom, series: [...series, ...interactionHitSeries(envelope, spec, series)] }
}

function interactionHitSeries(envelope: VisualizationEnvelope, spec: CartesianSpec, series: EChartsTranslation[]): EChartsTranslation[] {
  if (!spec.interactions.some((interaction) => interaction.kind === 'select')) return []
  return series.flatMap((candidate, index) => {
    if (candidate.type !== 'line') return []
    const yField = typeof candidate.encode?.y === 'string' ? candidate.encode.y : spec.y[index]?.field ?? `value-${index}`
    const identity = candidate.datasetId
      ? `${spec.x.dataset}:${spec.x.field}:${encodeURIComponent(String(candidate.datasetId))}`
      : `${spec.x.dataset}:${spec.x.field}:${yField}`
    return [{
      id: `series:interaction-hit:${identity}`,
      type: 'scatter',
      ...(candidate.datasetId ? { datasetId: candidate.datasetId } : {}),
      encode: candidate.encode,
      ...(candidate.xAxisIndex !== undefined ? { xAxisIndex: candidate.xAxisIndex } : {}),
      ...(candidate.yAxisIndex !== undefined ? { yAxisIndex: candidate.yAxisIndex } : {}),
      symbolSize: Math.max(18, spec.presentation.symbolSize ?? 0),
      itemStyle: { color: 'rgba(0,0,0,0.001)' },
      emphasis: { disabled: true },
      tooltip: { show: false },
      silent: false,
      z: 10,
    }]
  })
}

function chartLabel(envelope: VisualizationEnvelope, value: CartesianSpec['y'][number] | undefined, spec: CartesianSpec, context: RendererContext) {
  const authored = spec.presentation.labelPosition
  const horizontal = spec.presentation.orientation === 'horizontal' || spec.mark === 'bar'
  const position = authored === 'automatic' ? undefined : authored === 'outside' ? horizontal ? 'right' : 'top' : authored
  return { show: spec.presentation.showLabels, position, formatter: labelFormatter(envelope, value, context) }
}

function splitCartesianSeries(envelope: VisualizationEnvelope, context: RendererContext): { datasets: EChartsTranslation[]; series: EChartsTranslation[] } | undefined {
  const spec = envelope.spec
  if (spec.kind !== 'cartesian' || !spec.series || spec.y.length !== 1 || envelope.dataState.kind !== 'inline') return undefined
  const dataset = envelope.dataState.datasets.find((candidate) => candidate.id === spec.series?.dataset)
  const seriesIndex = dataset?.columns.indexOf(spec.series.field) ?? -1
  if (!dataset || seriesIndex < 0) return undefined
  const available = [...new Set(dataset.rows.map((row) => row[seriesIndex]).filter((value): value is string | number | boolean => typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'))]
  const configured = new Map((spec.presentation.comboSeries ?? []).map((item) => [String(item.seriesValue), item]))
  const configuredOrder = (spec.presentation.comboSeries ?? []).map((item) => String(item.seriesValue))
  const values = [
    ...configuredOrder.filter((value) => available.some((candidate) => String(candidate) === value)),
    ...available.filter((value) => !configured.has(String(value))).sort((left, right) => String(left).localeCompare(String(right), 'en')),
  ]
  const datasets: EChartsTranslation[] = [{ id: `dataset:${dataset.id}`, source: selectedDatasetSource(envelope, dataset) }]
  const series = values.map((value) => {
    const token = encodeURIComponent(String(value))
    const datasetID = `dataset:series:${spec.series?.field}:${token}`
    datasets.push({ id: datasetID, fromDatasetId: `dataset:${dataset.id}`, transform: { type: 'filter', config: { dimension: spec.series?.field, '=': value } } })
    const combo = configured.get(String(value))
    const mark = combo?.mark ?? (spec.mark === 'combo' ? 'line' : spec.mark)
    return {
      id: `series:${spec.series?.dataset}:${spec.series?.field}:${token}`, datasetId: datasetID, name: String(value), type: cartesianSeriesType(mark), yAxisIndex: combo?.axis === 'secondary' ? 1 : 0,
      encode: { x: spec.x.field, y: spec.y[0]?.field }, smooth: spec.presentation.smooth, symbol: spec.presentation.showSymbols ? undefined : 'none',
      stack: spec.presentation.stacked ? 'total' : undefined, areaStyle: spec.presentation.area || mark === 'area' ? {} : undefined,
      step: spec.presentation.step ? 'middle' : false, label: chartLabel(envelope, spec.y[0], spec, context),
    }
  })
  return { datasets, series }
}

function axisType(envelope: VisualizationEnvelope, ref: CartesianSpec['x'], fallback: 'category' | 'value'): 'category' | 'value' | 'time' {
  const dataType = field(envelope, ref)?.dataType
  return dataType === 'temporal' || dataType === 'date' ? 'time' : fallback
}

function seriesID(dataset = 'primary', value = 'value'): string { return `series:${dataset}:${value}` }

function cartesianSeriesType(mark: CartesianSpec['mark']): string {
  switch (mark) {
    case 'bar': case 'column': case 'waterfall': case 'histogram': return 'bar'
    case 'scatter': return 'scatter'
    case 'candlestick': return 'candlestick'
    case 'boxplot': return 'boxplot'
    default: return 'line'
  }
}
