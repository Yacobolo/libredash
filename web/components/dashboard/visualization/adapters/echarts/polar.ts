import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import type { RendererContext } from '../../host-controller'
import { formatField, inlineDataset, legend, toneColor, type EChartsTranslation } from './common'

export function polarOption(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation {
  const spec = envelope.spec
  if (spec.kind !== 'polar') return {}
  if (spec.mark === 'gauge') {
    const dataset = inlineDataset(envelope, spec.value.dataset)
    const valueIndex = dataset?.columns.indexOf(spec.value.field) ?? -1
    const value = valueIndex >= 0 ? dataset?.rows[0]?.[valueIndex] : undefined
    const minimum = spec.presentation.minimum ?? 0, maximum = spec.presentation.maximum ?? 100
    const span = maximum - minimum
    const authoredColors = (spec.presentation.thresholds ?? []).map((threshold) => [Math.max(0, Math.min(1, span > 0 ? (threshold.value - minimum) / span : 1)), toneColor(threshold.tone, context)])
    const colors = authoredColors.length ? authoredColors : [[1, context.colors.accent]]
    return {
      series: [{
        id: 'series:polar:gauge', type: 'gauge', min: minimum, max: maximum,
        data: [{ value, __lv_dataset: dataset?.id ?? 'primary', __lv_row_index: 0 }], pointer: { show: spec.presentation.showPointer },
        progress: { show: true, width: spec.presentation.progressWidth }, axisLine: { lineStyle: { color: colors } },
        detail: { formatter: (raw: unknown) => formatField(envelope, spec.value, raw, context), color: context.colors.foreground },
      }],
    }
  }
  const dataset = inlineDataset(envelope, spec.value.dataset)
  if (!dataset) return {}
  const categoryIndex = spec.category ? dataset.columns.indexOf(spec.category.field) : -1
  const valueIndex = dataset.columns.indexOf(spec.value.field)
  const seriesIndex = spec.series ? dataset.columns.indexOf(spec.series.field) : -1
  const categories = [...new Set(dataset.rows.map((row, index) => String(categoryIndex >= 0 ? row[categoryIndex] : index + 1)))]
  const seriesValues = [...new Set(dataset.rows.map((row) => String(seriesIndex >= 0 ? row[seriesIndex] : spec.title)))]
  const values = seriesValues.map((series) => ({
    name: series,
    value: categories.map((category) => dataset.rows.find((row, index) => String(seriesIndex >= 0 ? row[seriesIndex] : spec.title) === series && String(categoryIndex >= 0 ? row[categoryIndex] : index + 1) === category)?.[valueIndex] ?? null),
  }))
  const configuredMaximum = spec.presentation.maximum
  const maxima = categories.map((_, index) => configuredMaximum ?? Math.max(1, ...values.map((series) => typeof series.value[index] === 'number' ? series.value[index] as number : 0)))
  return {
    dataset: undefined, legend: legend(spec.presentation.legend, context),
    radar: { indicator: categories.map((name, index) => ({ name, max: maxima[index], color: context.colors.muted })) },
    series: [{ id: 'series:polar:radar', type: 'radar', data: values, areaStyle: spec.presentation.area ? {} : undefined, label: { show: spec.presentation.showLabels } }],
  }
}
