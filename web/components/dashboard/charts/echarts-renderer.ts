import type { EChartsOption } from 'echarts'
import * as echarts from 'echarts'
import { registerChartRenderer } from './registry'
import type { ChartDatum, ChartPayload, ChartRendererContext, ChartTokens } from './types'
import { deepMerge, payloadRowIndexFromData } from './utils'
import { buildEChartsOption } from './echarts-adapters'
import { brazilStatesGeoJSON } from './maps'

echarts.registerMap('brazil_states', brazilStatesGeoJSON as never)

registerChartRenderer('echarts', {
  mount(container: HTMLElement, context: ChartRendererContext) {
    const instance = echarts.init(container, null, { renderer: 'canvas' })
    let currentPayload: ChartPayload = {}

    instance.on('click', (event) => {
      const selected = datumForEvent(currentPayload, event)
      if (selected) context.selectDatum(selected.datum, selected.index)
    })

    return {
      update(payload: ChartPayload, tokens: ChartTokens): void {
        currentPayload = payload
        instance.setOption(buildOption(payload, tokens), true)
        instance.resize()
      },
      resize(): void {
        instance.resize()
      },
      clear(): void {
        instance.clear()
      },
      dispose(): void {
        instance.dispose()
      },
    }
  },
})

function buildOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const generated = buildEChartsOption(payload, tokens)
  const override = payload.rendererOptions?.echarts ?? {}
  return deepMerge(generated, override) as EChartsOption
}

function datumForEvent(payload: ChartPayload, event: echarts.ECElementEvent): { datum: ChartDatum; index: number } | undefined {
  const index = payloadRowIndexFromData(event.data)
  if (index === undefined) return undefined
  const datum = payload.data?.[index]
  if (!datum) return undefined
  return { datum, index }
}
