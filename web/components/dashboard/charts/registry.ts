import type { ChartRenderer } from './types'

const chartRenderers: Record<string, ChartRenderer> = {}

export function registerChartRenderer(name: string, renderer: ChartRenderer) {
  chartRenderers[name] = renderer
}

export function chartRenderer(name: string | undefined): ChartRenderer | undefined {
  return chartRenderers[name || 'echarts']
}
