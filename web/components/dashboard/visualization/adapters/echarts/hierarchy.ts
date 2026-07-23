import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import type { RendererContext } from '../../host-controller'
import { escapeHTML, formatField, inlineDataset, legend, type EChartsTranslation } from './common'

type HierarchyNode = { name: string; value?: unknown; __lv_dataset: string; __lv_row_index: number; children?: HierarchyNode[] }

export function hierarchyOption(envelope: VisualizationEnvelope, context: RendererContext): EChartsTranslation {
  const spec = envelope.spec
  if (spec.kind !== 'hierarchy') return {}
  const dataset = inlineDataset(envelope, spec.node.dataset)
  if (spec.mark === 'sankey' || spec.mark === 'graph') {
    const columns = dataset?.columns ?? []
    const sourceIndex = spec.source ? columns.indexOf(spec.source.field) : -1
    const targetIndex = spec.target ? columns.indexOf(spec.target.field) : -1
    const valueIndex = spec.value ? columns.indexOf(spec.value.field) : -1
    const links = (dataset?.rows ?? []).map((row, rowIndex) => ({
      source: String(row[sourceIndex]), target: String(row[targetIndex]), value: valueIndex >= 0 ? row[valueIndex] : undefined,
      __lv_dataset: dataset?.id ?? 'primary', __lv_row_index: rowIndex,
    }))
    const names = [...new Set(links.flatMap((link) => [link.source, link.target]))]
    const series: EChartsTranslation = {
      id: `series:hierarchy:${spec.mark}`, type: spec.mark, data: names.map((name) => ({ name })), links,
      lineStyle: { curveness: spec.presentation.curveness }, emphasis: { focus: spec.presentation.focus },
      label: { show: spec.presentation.showLabels, color: context.colors.foreground },
      tooltip: { formatter: (params: { data?: { source?: unknown; target?: unknown; value?: unknown } }) => {
        const link = params.data
        if (!link || link.source === undefined || link.target === undefined) return ''
        const value = formatField(envelope, spec.value, link.value, context)
        return `${escapeHTML(String(link.source))} → ${escapeHTML(String(link.target))}: ${escapeHTML(value)}`
      } },
    }
    if (spec.mark === 'graph') {
      series.roam = spec.presentation.roam
      series.layout = spec.presentation.layout === 'circular' ? 'circular' : 'force'
    } else {
      series.orient = spec.presentation.orientation
      series.nodeGap = spec.presentation.nodeGap
    }
    return { legend: legend(spec.presentation.legend, context), series: [series] }
  }
  const data = hierarchyData(envelope)
  const common: EChartsTranslation = {
    id: `series:hierarchy:${spec.mark}`, type: spec.mark, data, roam: spec.presentation.roam,
    label: { show: spec.presentation.showLabels, color: context.colors.foreground },
    tooltip: { formatter: (params: { data?: HierarchyNode }) => params.data ? `${escapeHTML(params.data.name)}: ${escapeHTML(hierarchyTooltipValue(envelope, params.data, context))}` : '' },
  }
  if (spec.mark === 'tree') {
    common.orient = spec.presentation.orientation === 'vertical' ? 'TB' : 'LR'
    common.layout = spec.presentation.layout === 'circular' ? 'radial' : 'orthogonal'
    common.initialTreeDepth = spec.presentation.initialDepth
  }
  if (spec.mark === 'treemap') {
    common.breadcrumb = { show: spec.presentation.breadcrumb }
    common.leafDepth = spec.presentation.initialDepth
  }
  if (spec.mark === 'sunburst') common.nodeClick = spec.presentation.roam ? 'rootToNode' : false
  return { legend: legend(spec.presentation.legend, context), series: [common] }
}

export function hierarchyData(envelope: VisualizationEnvelope): HierarchyNode[] {
  const spec = envelope.spec
  if (spec.kind !== 'hierarchy' || spec.mark === 'graph' || spec.mark === 'sankey') return []
  const dataset = inlineDataset(envelope, spec.node.dataset)
  if (!dataset) return []
  const nodeIndex = dataset.columns.indexOf(spec.node.field)
  const parentIndex = spec.parent ? dataset.columns.indexOf(spec.parent.field) : -1
  const valueIndex = spec.value ? dataset.columns.indexOf(spec.value.field) : -1
  const byID = new Map<string, HierarchyNode>()
  const parentByID = new Map<string, string | undefined>()
  for (let rowIndex = 0; rowIndex < dataset.rows.length; rowIndex++) {
    const row = dataset.rows[rowIndex]!
    const name = String(row[nodeIndex])
    const parent = parentIndex >= 0 && row[parentIndex] !== null && row[parentIndex] !== undefined && row[parentIndex] !== '' ? String(row[parentIndex]) : undefined
    const id = parent ? `${parent}\u001f${escapeSegment(name)}` : escapeSegment(name)
    if (byID.has(id)) throw new Error(`duplicate hierarchy node ${JSON.stringify(id)}`)
    byID.set(id, { name, value: valueIndex >= 0 ? row[valueIndex] : undefined, __lv_dataset: dataset.id, __lv_row_index: rowIndex })
    parentByID.set(id, parent)
  }
  const roots: HierarchyNode[] = []
  for (const [id, node] of byID) {
    const parentID = parentByID.get(id)
    if (!parentID) { roots.push(node); continue }
    const parent = byID.get(parentID)
    if (!parent) throw new Error(`hierarchy node ${JSON.stringify(id)} references missing parent ${JSON.stringify(parentID)}`)
    ;(parent.children ??= []).push(node)
  }
  return roots
}

export function hierarchyTooltipValue(envelope: VisualizationEnvelope, node: HierarchyNode, context: RendererContext): string {
  const spec = envelope.spec
  return spec.kind === 'hierarchy' ? formatField(envelope, spec.value, node.value, context) : String(node.value ?? '—')
}

function escapeSegment(value: string): string { return value.replaceAll('\u001f', '\u001f\u001f') }
