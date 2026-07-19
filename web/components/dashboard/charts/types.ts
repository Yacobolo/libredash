import type {
  InteractionMapping,
  InteractionSelectionEntry,
} from '../interaction-selection'

export type { InteractionMapping, InteractionSelectionEntry, InteractionSelectionValue } from '../interaction-selection'

export type ChartType =
  | 'line'
  | 'area'
  | 'bar'
  | 'column'
  | 'pie'
  | 'donut'
  | 'scatter'
  | 'funnel'
  | 'treemap'
  | 'gauge'
  | 'heatmap'
  | 'sankey'
  | 'graph'
  | 'map'
  | 'candlestick'
  | 'boxplot'
  | 'combo'
  | 'waterfall'
  | 'histogram'
  | 'radar'
  | 'tree'
  | 'sunburst'
  | 'kpi'

export type ChartShape =
  | 'category_value'
  | 'category_series_value'
  | 'category_multi_measure'
  | 'category_delta'
  | 'single_value'
  | 'matrix'
  | 'graph'
  | 'geo'
  | 'ohlc'
  | 'distribution'
  | 'binned_measure'
  | 'hierarchy'

export type ChartDatum = Record<string, unknown>

export type InteractionConfig = {
  kind?: string
  toggle?: boolean
  mappings?: InteractionMapping[]
  targets?: string[]
}

export type ChartPayload = {
  version?: number
  id?: string
  shape?: ChartShape | string
  renderer?: string
  type?: ChartType | string
  title?: string
  unit?: string
  format?: string
  interaction?: InteractionConfig
  dimensions?: string[]
  measure?: string
  measures?: string[]
  series?: string[]
  selection?: InteractionSelectionEntry[]
  data?: ChartDatum[]
  options?: Record<string, unknown>
  rendererOptions?: Record<string, Record<string, unknown>>
}

export type VisualAction = 'focus' | 'show-data' | 'copy-data' | 'export-csv' | 'clear-selection'

export type ChartRendererContext = {
  selectDatum(datum: ChartDatum, index: number): void
}

export type ChartRendererHandle = {
  update(payload: ChartPayload, tokens: ChartTokens): void
  resize(): void
  clear(): void
  dispose(): void
}

export type ChartRenderer = {
  mount(container: HTMLElement, context: ChartRendererContext): ChartRendererHandle
}

export type ChartTokens = {
  text: string
  muted: string
  border: string
  grid: string
  surface: string
  fill: string
  dimmed: string
  palette: string[]
}
