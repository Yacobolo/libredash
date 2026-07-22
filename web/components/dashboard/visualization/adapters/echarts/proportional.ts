import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import type { RendererContext } from '../../host-controller'
import { labelFormatter, legend, type EChartsTranslation } from './common'

export function proportionalOption(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation {
  const spec = envelope.spec
  if (spec.kind !== 'proportional') return {}
  const presentation = spec.presentation
  const radius = spec.mark === 'donut'
    ? [percent(presentation.innerRadius, 0.45), percent(presentation.outerRadius, 0.72)]
    : presentation.outerRadius ? percent(presentation.outerRadius, 0.72) : undefined
  const series: EChartsTranslation = {
    id: `series:primary:${spec.mark}`, type: spec.mark === 'funnel' ? 'funnel' : 'pie',
    encode: { itemName: spec.category.field, value: spec.value.field },
    label: { show: presentation.showLabels, position: presentation.labelPosition === 'inside' ? 'inside' : 'outside', formatter: labelFormatter(envelope, spec.value, context) },
    roseType: presentation.rose ? 'radius' : false,
  }
  if (radius !== undefined) series.radius = radius
  if (spec.mark === 'funnel') {
    series.orient = presentation.orientation
    if (presentation.align !== undefined) series.funnelAlign = presentation.align
    series.sort = presentation.sort === 'ascending' ? 'ascending' : presentation.sort === 'descending' ? 'descending' : 'none'
  }
  const center = presentation.centerLabel && spec.mark === 'donut' ? {
    graphic: [{ type: 'text', left: 'center', top: 'middle', silent: true, style: { text: presentation.centerLabel, fill: context.colors.foreground, fontFamily: context.fontFamily, textAlign: 'center' } }],
  } : {}
  return { legend: legend(presentation.legend, context), series: [series], ...center }
}

function percent(value: number | undefined, fallback: number): string {
  return `${Math.round((value ?? fallback) * 10000) / 100}%`
}
