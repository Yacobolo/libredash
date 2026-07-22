import type { VisualizationEnvelope, VisualizationField, VisualizationFieldRef } from '../../../../../generated/visualization'
import type { RendererContext } from '../../host-controller'
import { formatValue } from '../../format'

export type EChartsTranslation = Record<string, any>

export function inlineDataset(envelope: VisualizationEnvelope, datasetID = 'primary') {
  if (envelope.dataState.kind !== 'inline') return undefined
  return envelope.dataState.datasets.find((candidate) => candidate.id === datasetID)
}

export function field(envelope: VisualizationEnvelope, ref?: VisualizationFieldRef): VisualizationField | undefined {
  if (!ref) return undefined
  return envelope.spec.datasets.find((dataset) => dataset.id === ref.dataset)?.fields.find((candidate) => candidate.id === ref.field)
}

export function fieldLabel(envelope: VisualizationEnvelope, ref?: VisualizationFieldRef): string {
  return field(envelope, ref)?.label ?? ref?.field ?? ''
}

export function formatField(envelope: VisualizationEnvelope, ref: VisualizationFieldRef | undefined, value: unknown, context: RendererContext): string {
  const definition = field(envelope, ref)
  if (definition?.format) return formatValue(context.locale, definition.format, value)
  if (value === null || value === undefined) return '—'
  return String(value)
}

export function rowValue(envelope: VisualizationEnvelope, ref: VisualizationFieldRef | undefined, row: unknown[]): unknown {
  if (!ref) return undefined
  const dataset = inlineDataset(envelope, ref.dataset)
  const index = dataset?.columns.indexOf(ref.field) ?? -1
  return index >= 0 ? row[index] : undefined
}

export function selectedDatasetSource(envelope: VisualizationEnvelope, dataset: NonNullable<ReturnType<typeof inlineDataset>>): unknown[][] {
  if (envelope.selection.length === 0) return [dataset.columns, ...dataset.rows]
  const schema = envelope.spec.datasets.find((candidate) => candidate.id === dataset.id)
  const identityFields = (schema?.fields ?? []).filter((candidate) => candidate.role === 'identity')
  if (identityFields.length === 0) return [dataset.columns, ...dataset.rows]
  const selected = envelope.selection.filter((entry) => entry.datum.dataset === dataset.id && entry.datum.dataRevision === envelope.dataRevision)
  if (selected.length === 0) return [dataset.columns, ...dataset.rows]
  const rows = dataset.rows.map((row) => {
    const matches = selected.some((entry) => identityFields.every((identity) => Object.is(row[dataset.columns.indexOf(identity.id)], entry.datum.identity[identity.id])))
    return [...row, matches]
  })
  return [[...dataset.columns, '__lv_selected'], ...rows]
}

export function baseOption(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation {
  const dataset = inlineDataset(envelope)
  return {
    animation: false,
    aria: { enabled: true, description: envelope.spec.accessibility.description },
    backgroundColor: 'transparent',
    color: [...context.colors.data],
    textStyle: { color: context.colors.foreground, fontFamily: context.fontFamily },
    dataset: dataset ? { id: `dataset:${dataset.id}`, source: selectedDatasetSource(envelope, dataset) } : undefined,
    tooltip: {
      trigger: tooltipTrigger(envelope),
      backgroundColor: context.colors.surface,
      borderColor: context.colors.grid,
      textStyle: { color: context.colors.foreground, fontFamily: context.fontFamily },
      formatter: tooltipFormatter(envelope, context),
    },
    title: envelope.status.kind === 'error' ? { text: envelope.status.message ?? 'Visualization error', textStyle: { color: context.colors.danger } } : undefined,
    graphic: statusGraphic(envelope, context),
    visualMap: envelope.selection.length > 0 ? { show: false, dimension: '__lv_selected', pieces: [{ value: true, opacity: 1 }, { value: false, opacity: 0.35 }] } : undefined,
  }
}

function tooltipTrigger(envelope: VisualizationEnvelope): 'axis' | 'item' {
  if (envelope.spec.kind !== 'cartesian') return 'item'
  return envelope.spec.mark === 'scatter' || envelope.spec.mark === 'heatmap' ? 'item' : 'axis'
}

function statusGraphic(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation[] | undefined {
  if (envelope.status.kind === 'partial') {
    return [{ type: 'text', right: 8, top: 8, silent: true, style: { text: envelope.status.message ?? 'Partial data', fill: context.colors.attention, fontFamily: context.fontFamily, textAlign: 'right' } }]
  }
  if (envelope.status.kind !== 'idle' && envelope.status.kind !== 'loading' && envelope.status.kind !== 'no_data') return undefined
  const text = envelope.status.message ?? (envelope.status.kind === 'no_data' ? 'No data' : 'Loading…')
  return [{ type: 'text', left: 'center', top: 'middle', silent: true, style: { text, fill: context.colors.muted, fontFamily: context.fontFamily, textAlign: 'center' } }]
}

export function axis(envelope: VisualizationEnvelope, ref: VisualizationFieldRef, type: 'category' | 'value' | 'time', context: RendererContext): EChartsTranslation {
  return {
    type,
    axisLine: { lineStyle: { color: context.colors.grid } },
    axisTick: { lineStyle: { color: context.colors.grid } },
    splitLine: { lineStyle: { color: context.colors.grid } },
    axisLabel: { color: context.colors.muted, formatter: (value: unknown) => formatField(envelope, ref, value, context) },
    nameTextStyle: { color: context.colors.muted },
  }
}

export function legend(position: string, context: RendererContext): EChartsTranslation | undefined {
  if (position === 'hidden') return undefined
  return {
    show: true,
    orient: position === 'left' || position === 'right' ? 'vertical' : 'horizontal',
    [position]: 0,
    textStyle: { color: context.colors.muted, fontFamily: context.fontFamily },
  }
}

export function labelFormatter(envelope: VisualizationEnvelope, ref: VisualizationFieldRef | undefined, context: RendererContext) {
  return (params: { value?: unknown }) => {
    const value = Array.isArray(params?.value) ? rowValue(envelope, ref, params.value) : params?.value
    return formatField(envelope, ref, value, context)
  }
}

export function toneColor(tone: string, context: RendererContext): string {
  switch (tone) {
    case 'success': return context.colors.success
    case 'warning': return context.colors.attention
    case 'danger': return context.colors.danger
    case 'ink': return context.colors.foreground
    default: return context.colors.accent
  }
}

export function escapeHTML(value: string): string {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;')
}

function tooltipFormatter(envelope: VisualizationEnvelope, context: RendererContext) {
  return (raw: unknown): string => {
    const params = Array.isArray(raw) ? raw : [raw]
    const entries: string[] = []
    for (const item of params) {
      const value = (item as { value?: unknown })?.value
      if (!Array.isArray(value)) continue
      const dataset = inlineDataset(envelope)
      if (!dataset) continue
      const schema = envelope.spec.datasets.find((candidate) => candidate.id === dataset.id)
      for (const definition of schema?.fields ?? []) {
        const index = dataset.columns.indexOf(definition.id)
        if (index < 0) continue
        const formatted = definition.format ? formatValue(context.locale, definition.format, value[index]) : value[index] === null || value[index] === undefined ? '—' : String(value[index])
        entries.push(`${escapeHTML(definition.label)}: ${escapeHTML(formatted)}`)
      }
      if (entries.length) break
    }
    return entries.join('<br>')
  }
}
