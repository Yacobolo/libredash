import type { EChartsOption } from 'echarts'

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

export type ChartPayload = {
  version?: number
  id?: string
  kind?: string
  shape?: ChartShape | string
  renderer?: string
  type?: ChartType | string
  title?: string
  unit?: string
  field?: string
  dimensions?: string[]
  measure?: string
  measures?: string[]
  series?: string[]
  selection?: string[]
  data?: ChartDatum[]
  options?: Record<string, unknown>
  rendererOptions?: Record<string, Record<string, unknown>>
}

export type VisualAction = 'focus' | 'show-data' | 'copy-data' | 'export-csv' | 'clear-selection'

export type ChartRenderer = {
  buildOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption
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
