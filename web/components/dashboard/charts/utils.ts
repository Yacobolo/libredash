import type { ChartDatum, ChartPayload, ChartShape, ChartTokens, ChartType } from './types'
import {
	interactionMappingIdentityEqual,
	interactionMappingKey,
	interactionSelectionValue,
} from '../interaction-selection'

export const leapViewPayloadRowIndexKey = '__leapviewPayloadRowIndex'

export function stylesFor(element: HTMLElement): ChartTokens {
  const styles = getComputedStyle(element)
  const token = (...names: string[]) => {
    for (const name of names) {
      const resolved = styles.getPropertyValue(name).trim()
      if (resolved) return resolved
    }
    return ''
  }
  const value = (...names: string[]) => token(...names) || 'currentColor'
  const chart1 = value('--lv-data-1', '--lv-chart-1', '--fgColor-accent')
  return {
    text: value('--lv-fg-default', '--fgColor-default'),
    muted: value('--lv-fg-muted', '--fgColor-muted'),
    border: value('--lv-line-default', '--borderColor-default'),
    grid: value('--lv-chart-grid', '--lv-line-muted', '--borderColor-muted'),
    surface: value('--lv-chart-surface', '--report-chart-surface', '--card-bgColor', '--bgColor-default'),
    fill: token('--lv-data-1-muted') || token('--lv-chart-1-muted') || colorWithAlpha(chart1, 0.35),
    dimmed: value('--lv-line-muted', '--borderColor-muted'),
    palette: [
      chart1,
      value('--lv-data-2', '--lv-chart-2', '--fgColor-success'),
      value('--lv-data-3', '--lv-chart-3', '--fgColor-done'),
      value('--lv-data-4', '--lv-chart-4', '--fgColor-danger'),
      value('--lv-data-5', '--lv-chart-5', '--fgColor-success'),
      value('--lv-data-6', '--lv-chart-6', '--fgColor-sponsors'),
      value('--lv-data-7', '--fgColor-accent'),
      value('--lv-data-8', '--fgColor-attention'),
    ],
  }
}

export function normalizeType(type: string | undefined): ChartType {
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
    case 'heatmap_chart':
      return 'heatmap'
    case 'sankey_chart':
      return 'sankey'
    case 'graph_chart':
      return 'graph'
    case 'map_chart':
      return 'map'
    case 'candlestick_chart':
      return 'candlestick'
    case 'boxplot_chart':
      return 'boxplot'
    case 'combo_chart':
      return 'combo'
    case 'waterfall_chart':
      return 'waterfall'
    case 'histogram_chart':
      return 'histogram'
    case 'radar_chart':
      return 'radar'
    case 'tree_chart':
      return 'tree'
    case 'sunburst_chart':
      return 'sunburst'
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
    case 'heatmap':
    case 'sankey':
    case 'graph':
    case 'map':
    case 'candlestick':
    case 'boxplot':
    case 'combo':
    case 'waterfall':
    case 'histogram':
    case 'radar':
    case 'tree':
    case 'sunburst':
    case 'kpi':
      return type
    default:
      return 'bar'
  }
}

export function normalizeShape(shape: string | undefined, type?: string, hasSeries?: boolean): ChartShape {
  switch (shape) {
    case 'category_series_value':
    case 'category_multi_measure':
    case 'category_delta':
    case 'single_value':
    case 'category_value':
    case 'matrix':
    case 'graph':
    case 'geo':
    case 'ohlc':
    case 'distribution':
    case 'binned_measure':
    case 'hierarchy':
      return shape
  }
  switch (normalizeType(type)) {
    case 'combo':
      return 'category_multi_measure'
    case 'waterfall':
      return 'category_delta'
    case 'histogram':
      return 'binned_measure'
    case 'tree':
    case 'sunburst':
      return 'hierarchy'
    case 'gauge':
      return 'single_value'
    case 'heatmap':
      return 'matrix'
    case 'sankey':
    case 'graph':
      return 'graph'
    case 'map':
      return 'geo'
    case 'candlestick':
      return 'ohlc'
    case 'boxplot':
      return 'distribution'
    default:
      return hasSeries ? 'category_series_value' : 'category_value'
  }
}

export function unique(values: string[]): string[] {
  return [...new Set(values)]
}

export function stringValue(row: ChartDatum | undefined, key: string): string {
  const value = row?.[key]
  if (value === undefined || value === null) return ''
  return String(value)
}

export function numberValue(row: ChartDatum | undefined, key: string): number {
  const value = row?.[key]
  if (typeof value === 'number') return Number.isFinite(value) ? value : 0
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

export function booleanValue(row: ChartDatum | undefined, key: string): boolean {
  return row?.[key] === true
}

export function withPayloadRowIndex<T extends Record<string, unknown>>(item: T, index: number): T {
  return { ...item, [leapViewPayloadRowIndexKey]: index } as T
}

export function payloadRowIndexFromData(data: unknown): number | undefined {
  if (!data || typeof data !== 'object') return undefined
  const index = (data as Record<string, unknown>)[leapViewPayloadRowIndexKey]
  return typeof index === 'number' && Number.isInteger(index) && index >= 0 ? index : undefined
}

export function colorWithAlpha(color: string, alpha: number): string {
  if (color.startsWith('#') && color.length === 7) {
    const r = Number.parseInt(color.slice(1, 3), 16)
    const g = Number.parseInt(color.slice(3, 5), 16)
    const b = Number.parseInt(color.slice(5, 7), 16)
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
  }
  return color
}

export function formatValue(value: number, unit?: string): string {
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

export function deepMerge(base: unknown, override: unknown): unknown {
  if (!isPlainObject(base) || !isPlainObject(override)) {
    return override === undefined ? base : override
  }
  const result: Record<string, unknown> = { ...base }
  for (const [key, value] of Object.entries(override)) {
    if (Array.isArray(value)) {
      result[key] = value
      continue
    }
    result[key] = deepMerge(result[key], value)
  }
  return result
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

export function chartColumns(payload: ChartPayload) {
  switch (normalizeShape(payload.shape, payload.type, Boolean(payload.series?.length))) {
    case 'matrix':
      return [
        { key: 'row', label: 'Row' },
        { key: 'column', label: 'Column' },
        { key: 'value', label: 'Value', align: 'right' },
      ]
    case 'graph':
      return [
        { key: 'source', label: 'Source' },
        { key: 'target', label: 'Target' },
        { key: 'value', label: 'Value', align: 'right' },
      ]
    case 'geo':
      return [
        { key: 'name', label: 'Name' },
        { key: 'value', label: 'Value', align: 'right' },
      ]
    case 'ohlc':
      return ['label', 'open', 'close', 'low', 'high'].map((key) => ({ key, label: titleCase(key), align: key === 'label' ? undefined : 'right' }))
    case 'distribution':
      return ['label', 'min', 'q1', 'median', 'q3', 'max'].map((key) => ({ key, label: titleCase(key), align: key === 'label' ? undefined : 'right' }))
    case 'category_delta':
      return [
        { key: 'label', label: 'Label' },
        { key: 'value', label: 'Delta', align: 'right' },
        { key: 'start', label: 'Start', align: 'right' },
        { key: 'end', label: 'End', align: 'right' },
      ]
    case 'binned_measure':
      return [
        { key: 'label', label: 'Bin' },
        { key: 'binStart', label: 'From', align: 'right' },
        { key: 'binEnd', label: 'To', align: 'right' },
        { key: 'value', label: 'Rows', align: 'right' },
      ]
    case 'hierarchy':
      return [
        { key: 'path', label: 'Path' },
        { key: 'value', label: 'Value', align: 'right' },
      ]
    default:
      return [
        { key: 'label', label: 'Label' },
        { key: 'series', label: 'Series' },
        { key: 'value', label: 'Value', align: 'right' },
      ]
  }
}

export function chartRows(payload: ChartPayload) {
  const shape = normalizeShape(payload.shape, payload.type, Boolean(payload.series?.length))
  return (payload.data ?? []).map((row) => {
    if (shape === 'hierarchy' && Array.isArray(row.path)) {
      return { ...row, path: row.path.join(' / ') }
    }
    return { ...row }
  })
}

export function selectedRows(payload: ChartPayload, fallbackKey = 'label') {
	const rows = payload.data ?? []
	const mappings = payload.interaction?.mappings ?? []
	const entries = payload.selection ?? []
	const tupleKeys = new Set(entries.map((entry) => selectedEntryKey(entry, mappings)).filter(Boolean))
	const fallbackValues = new Set(rows.filter((row) => booleanValue(row, 'selected')).map((row) => stringValue(row, fallbackKey)))
	const hasSelection = tupleKeys.size > 0 || fallbackValues.size > 0
	return {
		hasSelection,
		isSelected(row: ChartDatum, key = fallbackKey): boolean {
			if (tupleKeys.size > 0) return tupleKeys.has(datumSelectionKey(row, mappings))
			return booleanValue(row, 'selected') || fallbackValues.has(stringValue(row, key))
		},
	}
}

function selectedEntryKey(entry: NonNullable<ChartPayload['selection']>[number], mappings: NonNullable<ChartPayload['interaction']>['mappings']): string {
	if (!mappings?.length || !entry.mappings?.length) return ''
	const parts: string[] = []
	for (const mapping of mappings) {
		const selected = entry.mappings.find((candidate) => interactionMappingIdentityEqual(candidate, mapping))
		if (!selected || selected.value === undefined) return ''
		parts.push(interactionMappingKey(mapping, selected.value))
	}
	return parts.join('\u0001')
}

function datumSelectionKey(row: ChartDatum, mappings: NonNullable<ChartPayload['interaction']>['mappings']): string {
	if (!mappings?.length) return ''
	const parts: string[] = []
	for (const mapping of mappings) {
		const value = interactionSelectionValue(row[mapping.value])
		if (value === undefined) return ''
		parts.push(interactionMappingKey(mapping, value))
	}
	return parts.join('\u0001')
}

export function titleCase(value: string): string {
  return value.slice(0, 1).toUpperCase() + value.slice(1).replaceAll('_', ' ')
}
