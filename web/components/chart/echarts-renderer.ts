import type { EChartsOption } from 'echarts'
import { registerChartRenderer } from './registry'
import type { ChartPayload, ChartTokens } from './types'
import { deepMerge } from './utils'
import { buildEChartsOption } from './echarts-adapters'

registerChartRenderer('echarts', {
  buildOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
    const generated = buildEChartsOption(payload, tokens)
    const override = payload.rendererOptions?.echarts ?? {}
    return deepMerge(generated, override) as EChartsOption
  },
})
