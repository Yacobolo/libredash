import { expect, test } from 'bun:test'

import type { VisualizationEnvelope } from '../../../../generated/visualization'
import { Change, defaultRendererContext } from '../host-controller'
import { createEChartsRendererFrame, echartsOption, echartsUpdatePlan, interactionCommandForRow, normalizeRendererLocale, removeEChartsRendererFrame, waitForEChartsFrame } from './echarts'

test('superseded ECharts mounts own isolated renderer frames', () => {
  const mounted: HTMLElement[] = []
  const container = {
    replaceChildren: (frame: HTMLElement) => { mounted.push(frame) },
  } as unknown as HTMLElement
  const frames: HTMLElement[] = []
  const createFrame = () => {
    const frame = { style: { cssText: '' } } as unknown as HTMLElement
    frames.push(frame)
    return frame
  }

  const stale = createEChartsRendererFrame(container, createFrame)
  const current = createEChartsRendererFrame(container, createFrame)

  expect(stale).not.toBe(current)
  expect(mounted).toEqual([stale, current])
  expect(current.style.cssText).toContain('width:100%')
  expect(current.style.cssText).toContain('height:100%')

  let staleRemoved = false
  removeEChartsRendererFrame(container, { parentNode: null, remove: () => { staleRemoved = true } } as unknown as HTMLElement)
  expect(staleRemoved).toBe(false)
})

test('ECharts translation uses dataset and encode without native option passthrough', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'revenue', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'cartesian', title: 'Revenue', mark: 'line',
      datasets: [{ id: 'primary', fields: [
        { id: 'month', role: 'dimension', dataType: 'string', nullable: false, label: 'Month' },
        { id: 'revenue', role: 'measure', dataType: 'decimal', nullable: false, label: 'Revenue' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue by month' }, interactions: [],
      x: { dataset: 'primary', field: 'month' }, y: [{ dataset: 'primary', field: 'revenue' }],
      presentation: { legend: 'bottom', showLabels: false, smooth: true, stacked: false, showSymbols: true, dataZoom: false, area: false, step: false },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [
      { id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['month', 'revenue'], rows: [['Jan', 10]], completeness: 'complete' },
    ] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  const option = echartsOption(envelope) as any
  expect(option.dataset.source).toEqual([['month', 'revenue'], ['Jan', 10]])
  expect(option.series[0].encode).toEqual({ x: 'month', y: 'revenue' })
  expect(JSON.stringify(option)).not.toContain('rendererOptions')
})

test('ECharts interactions translate stable IR field mappings without renderer row keys', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'orders', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 7,
    spec: {
      kind: 'cartesian', title: 'Orders', mark: 'bar',
      datasets: [{ id: 'primary', fields: [
        { id: 'status', role: 'identity', dataType: 'string', nullable: false, label: 'Status' },
        { id: 'count', role: 'measure', dataType: 'integer', nullable: false, label: 'Orders' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Orders', description: 'Orders by status' },
      interactions: [{ id: 'point_selection', kind: 'select', mode: 'multiple', requiresStableIdentity: true, targets: ['details'], mappings: [
        { source: { dataset: 'primary', field: 'status' }, targetFieldID: 'orders.status', targetFactID: 'orders', label: { dataset: 'primary', field: 'status' } },
      ] }],
      x: { dataset: 'primary', field: 'status' }, y: [{ dataset: 'primary', field: 'count' }],
      presentation: { legend: 'bottom', showLabels: false, smooth: false, stacked: false, showSymbols: true, dataZoom: false, area: false, step: false },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 7, generation: 2, datasets: [
      { id: 'primary', specRevision: 'sha256:test', dataRevision: 7, generation: 2, columns: ['status', 'count'], rows: [['delivered', 42]], completeness: 'complete' },
    ] },
    selection: [{ datum: { dataset: 'primary', dataRevision: 7, identity: { status: 'delivered' } }, label: 'Delivered' }], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  expect(interactionCommandForRow(envelope, 'primary', ['delivered', 42])).toEqual({
    sourceKind: 'visual', sourceId: 'orders', interactionKind: 'point_selection', action: 'set', toggle: true,
    mappings: [{ field: 'orders.status', fact: 'orders', value: 'delivered', label: 'delivered' }],
  })
  expect(interactionCommandForRow(envelope, 'primary', [{ forged: true }, 42])).toBeUndefined()
  const option = echartsOption(envelope) as any
  expect(option.dataset.source).toEqual([['status', 'count', '__lv_selected'], ['delivered', 42, true]])
  expect(option.visualMap.dimension).toBe('__lv_selected')
})

test('ECharts gives selectable line and area rows reliable hit targets at either symbol setting', () => {
  for (const mark of ['line', 'area'] as const) {
    const envelope = cartesianFixture(mark) as any
    envelope.spec.datasets[0].fields[0].role = 'identity'
    envelope.spec.interactions = [{
      id: 'point_selection', kind: 'select', mode: 'multiple', requiresStableIdentity: true, targets: ['details'], mappings: [
        { source: { dataset: 'primary', field: 'label' }, targetFieldID: 'orders.purchase_month', targetFactID: 'orders' },
      ],
    }]

    const option = echartsOption(envelope, defaultRendererContext) as any
    expect(option.series).toHaveLength(2)
    expect(option.series[0]).toMatchObject({ type: 'line', symbol: 'none' })
    expect(option.series[1]).toMatchObject({
      id: 'series:interaction-hit:primary:label:value',
      type: 'scatter',
      encode: { x: 'label', y: 'value' },
      symbolSize: 18,
      itemStyle: { color: 'rgba(0,0,0,0.001)' },
      tooltip: { show: false },
    })
    expect(option.series[1].silent).toBe(false)
  }

  const authoredSymbols = cartesianFixture('line') as any
  authoredSymbols.spec.presentation.showSymbols = true
  authoredSymbols.spec.interactions = [{
    id: 'point_selection', kind: 'select', mode: 'multiple', requiresStableIdentity: true, targets: ['details'], mappings: [
      { source: { dataset: 'primary', field: 'label' }, targetFieldID: 'orders.purchase_month', targetFactID: 'orders' },
    ],
  }]
  expect((echartsOption(authoredSymbols, defaultRendererContext) as any).series).toHaveLength(2)
  expect((echartsOption(cartesianFixture('line'), defaultRendererContext) as any).series).toHaveLength(1)
})

test('ECharts translation preserves combo series marks and axes', () => {
  const base = {
    schemaVersion: 3, visualID: 'combo', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'cartesian', title: 'Combo', mark: 'combo',
      datasets: [{ id: 'primary', fields: [
        { id: 'month', role: 'dimension', dataType: 'string', nullable: false, label: 'Month' },
        { id: 'series', role: 'dimension', dataType: 'string', nullable: false, label: 'Series' },
        { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Combo', description: 'Combo' }, interactions: [],
      x: { dataset: 'primary', field: 'month' }, y: [{ dataset: 'primary', field: 'value' }], series: { dataset: 'primary', field: 'series' },
      presentation: { legend: 'bottom', showLabels: false, smooth: false, stacked: false, showSymbols: true, dataZoom: false, area: false, step: false, comboSeries: [
        { seriesValue: 'Revenue', mark: 'line', axis: 'primary' },
        { seriesValue: 'Orders', mark: 'column', axis: 'secondary' },
      ] },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [
      { id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['month', 'series', 'value'], rows: [['Jan', 'Revenue', 10], ['Jan', 'Orders', 2]], completeness: 'complete' },
    ] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  const option = echartsOption(base) as any
  expect(option.dataset).toHaveLength(3)
  expect(option.series.map((series: any) => [series.name, series.type, series.yAxisIndex])).toEqual([
    ['Revenue', 'line', 0], ['Orders', 'bar', 1],
  ])
  expect(option.yAxis).toHaveLength(2)
  const reordered = structuredClone(base) as any
  reordered.dataState.datasets[0].rows.reverse()
  const reorderedOption = echartsOption(reordered) as any
  expect(new Set(reorderedOption.series.map((series: any) => series.id))).toEqual(new Set(option.series.map((series: any) => series.id)))
  expect(new Set(reorderedOption.dataset.map((dataset: any) => dataset.id))).toEqual(new Set(option.dataset.map((dataset: any) => dataset.id)))
})

test('ECharts translation emits one multi-value financial series', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'ohlc', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'cartesian', title: 'OHLC', mark: 'candlestick',
      datasets: [{ id: 'primary', fields: ['label', 'open', 'close', 'low', 'high'].map((id, index) => ({ id, role: index ? 'measure' : 'dimension', dataType: index ? 'decimal' : 'string', nullable: false, label: id })) }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'OHLC', description: 'OHLC' }, interactions: [],
      x: { dataset: 'primary', field: 'label' }, y: ['open', 'close', 'low', 'high'].map((field) => ({ dataset: 'primary', field })),
      presentation: { legend: 'hidden', showLabels: false, smooth: false, stacked: false, showSymbols: false, dataZoom: true, area: false, step: false },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [
      { id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['label', 'open', 'close', 'low', 'high'], rows: [['Jan', 1, 2, 0, 3]], completeness: 'complete' },
    ] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  const option = echartsOption(envelope) as any
  expect(option.series).toHaveLength(1)
  expect(option.series[0].encode).toEqual({ x: 'label', y: ['open', 'close', 'low', 'high'] })
})

test('ECharts translation builds radar indicators and aligned series from typed fields', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'quality', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'polar', title: 'Quality', mark: 'radar',
      datasets: [{ id: 'primary', fields: [
        { id: 'metric', role: 'dimension', dataType: 'string', nullable: false, label: 'Metric' },
        { id: 'team', role: 'dimension', dataType: 'string', nullable: false, label: 'Team' },
        { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Quality', description: 'Quality by team' }, interactions: [],
      category: { dataset: 'primary', field: 'metric' }, series: { dataset: 'primary', field: 'team' }, value: { dataset: 'primary', field: 'value' },
      presentation: { legend: 'bottom', showLabels: false, showPointer: false, area: true },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{
      id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['metric', 'team', 'value'],
      rows: [['Speed', 'A', 8], ['Quality', 'A', 9], ['Speed', 'B', 6], ['Quality', 'B', 7]], completeness: 'complete',
    }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
  const option = echartsOption(envelope) as any
  expect(option.radar.indicator.map((item: any) => item.name)).toEqual(['Speed', 'Quality'])
  expect(option.series[0].data).toEqual([{ name: 'A', value: [8, 9] }, { name: 'B', value: [6, 7] }])
})

test('ECharts normalizes supported document locales and fails closed on unknown locales', () => {
  expect(normalizeRendererLocale('en')).toBe('en-US')
  expect(normalizeRendererLocale('pt-BR')).toBe('pt-BR')
  expect(() => normalizeRendererLocale('da-DK')).toThrow(/unsupported visualization locale/)
})

test('ECharts uses stable IDs, contractual formatting, and resolved theme colors', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'revenue', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'cartesian', title: 'Revenue', mark: 'column',
      datasets: [{ id: 'primary', fields: [
        { id: 'month', role: 'dimension', dataType: 'string', nullable: false, label: 'Month' },
        { id: 'revenue', role: 'measure', dataType: 'decimal', nullable: false, label: 'Revenue', format: { kind: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 } },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue' }, interactions: [],
      x: { dataset: 'primary', field: 'month' }, y: [{ dataset: 'primary', field: 'revenue' }],
      presentation: { legend: 'bottom', showLabels: true, smooth: false, stacked: false, showSymbols: true, dataZoom: false, area: false, step: false },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['month', 'revenue'], rows: [['Jan', 1234.5]], completeness: 'complete' }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
  const context = { ...defaultRendererContext, locale: 'pt-BR' as const, colors: { ...defaultRendererContext.colors, foreground: '#eee', grid: '#333', data: ['#123456'] } }
  const option = echartsOption(envelope, context) as any
  expect(option.series[0].id).toBe('series:primary:revenue')
  expect(option.color).toEqual(['#123456'])
  expect(option.textStyle.color).toBe('#eee')
  expect(option.yAxis.axisLabel.formatter(1234.5)).toBe('R$\u00a01.234,50')
  expect(option.series[0].label.formatter({ value: ['Jan', 1234.5] })).toBe('R$\u00a01.234,50')
})

test('ECharts constructs deterministic nested hierarchy data and honors layout presentation', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'tree', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'hierarchy', title: 'Tree', mark: 'tree',
      datasets: [{ id: 'primary', fields: [
        { id: 'node', role: 'identity', dataType: 'string', nullable: false, label: 'Node' },
        { id: 'parent', role: 'dimension', dataType: 'string', nullable: true, label: 'Parent' },
        { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'Tree', description: 'Tree' }, interactions: [],
      node: { dataset: 'primary', field: 'node' }, parent: { dataset: 'primary', field: 'parent' }, value: { dataset: 'primary', field: 'value' },
      presentation: { legend: 'hidden', showLabels: true, orientation: 'horizontal', initialDepth: 2, roam: true, layout: 'standard', breadcrumb: true, nodeGap: 18, curveness: 0.4, focus: 'adjacency' },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['node', 'parent', 'value'], rows: [['root', null, 10], ['child', 'root', 4]], completeness: 'complete' }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.series[0].id).toBe('series:hierarchy:tree')
  expect(option.series[0].orient).toBe('LR')
  expect(option.series[0].data).toEqual([{ name: 'root', value: 10, __lv_dataset: 'primary', __lv_row_index: 0, children: [{ name: 'child', value: 4, __lv_dataset: 'primary', __lv_row_index: 1 }] }])
})

test('ECharts hierarchy source nodes select only when their compiled identity tuple is complete', () => {
  const envelope = hierarchyFixture('treemap') as any
  envelope.spec.datasets[0].fields.push(
    { id: 'category', role: 'identity', dataType: 'string', nullable: false, label: 'Category' },
    { id: 'status', role: 'identity', dataType: 'string', nullable: true, label: 'Status' },
  )
  envelope.spec.interactions = [{
    id: 'point_selection', kind: 'select', mode: 'single', requiresStableIdentity: true, targets: ['details'], mappings: [
      { source: { dataset: 'primary', field: 'category' }, targetFieldID: 'orders.category' },
      { source: { dataset: 'primary', field: 'status' }, targetFieldID: 'orders.status' },
    ],
  }]
  envelope.dataState.datasets[0].columns = ['node', 'parent', 'value', 'category', 'status']
  envelope.dataState.datasets[0].rows = [
    ['A', null, 10, 'A', null],
    ['delivered', 'A', 4, 'A', 'delivered'],
  ]

  expect(interactionCommandForRow(envelope, 'primary', envelope.dataState.datasets[0].rows[0])).toBeUndefined()
  expect(interactionCommandForRow(envelope, 'primary', envelope.dataState.datasets[0].rows[1])).toMatchObject({
    sourceId: 'treemap', action: 'set', mappings: [
      { field: 'orders.category', value: 'A' },
      { field: 'orders.status', value: 'delivered' },
    ],
  })
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.series[0].data[0].children[0]).toMatchObject({ __lv_dataset: 'primary', __lv_row_index: 1 })
})

test('ECharts network links retain source-row selection while aggregate nodes stay silent', () => {
  const envelope = networkFixture('sankey') as any
  envelope.spec.datasets[0].fields[0].role = 'identity'
  envelope.spec.datasets[0].fields[1].role = 'identity'
  envelope.spec.interactions = [{
    id: 'point_selection', kind: 'select', mode: 'single', requiresStableIdentity: true, targets: ['details'], mappings: [
      { source: { dataset: 'primary', field: 'source' }, targetFieldID: 'orders.category' },
      { source: { dataset: 'primary', field: 'target' }, targetFieldID: 'orders.status' },
    ],
  }]

  expect(interactionCommandForRow(envelope, 'primary', ['A', 'B', 4])).toMatchObject({
    mappings: [{ field: 'orders.category', value: 'A' }, { field: 'orders.status', value: 'B' }],
  })
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.series[0].links[0]).toMatchObject({ __lv_dataset: 'primary', __lv_row_index: 0 })
  expect(option.series[0].data[0].__lv_dataset).toBeUndefined()
})

test('ECharts incremental plans commit data synchronously, preserve interaction state, and do not resend data for context changes', () => {
  const option = {
    dataset: { id: 'dataset:primary', source: [['month', 'value'], ['Jan', 10]] },
    series: [{ id: 'series:primary:value', type: 'line', encode: { x: 'month', y: 'value' }, data: [10], label: { color: '#fff' } }],
    dataZoom: [{ type: 'inside' }], legend: { textStyle: { color: '#fff' } }, textStyle: { color: '#fff' },
  } as any

  const data = echartsUpdatePlan(Change.Data, option)
  expect(data.settings).toEqual({ notMerge: false, lazyUpdate: false, replaceMerge: ['dataset', 'series', 'visualMap'] })
  expect(data.option).toEqual({ dataset: option.dataset, series: option.series, visualMap: [] })

  const selection = echartsUpdatePlan(Change.Selection, option)
  expect(selection.settings.replaceMerge).toEqual(['dataset', 'visualMap'])
  expect(selection.option.series).toBeUndefined()

  const context = echartsUpdatePlan(Change.Context, option)
  expect(context.settings.replaceMerge).toBeUndefined()
  expect(context.option.dataset).toBeUndefined()
  expect(context.option.series[0].data).toBeUndefined()
  expect(context.option.series[0].encode).toBeUndefined()
  expect(context.option.dataZoom).toBeUndefined()
})

test('ECharts first-frame readiness resolves on finished and removes its listener', async () => {
  let listener: (() => void) | undefined
  let removed = false
  const chart = {
    on(event: string, callback: () => void) { expect(event).toBe('finished'); listener = callback },
    off(event: string, callback: () => void) { expect(event).toBe('finished'); removed = callback === listener },
  }
  const ready = waitForEChartsFrame(chart as any, 100)
  listener?.()
  await ready
  expect(removed).toBe(true)
})

test('ECharts first-frame readiness fails closed on timeout and removes its listener', async () => {
  let listener: (() => void) | undefined
  let removed = false
  const chart = {
    on(_event: string, callback: () => void) { listener = callback },
    off(_event: string, callback: () => void) { removed = callback === listener },
  }
  await expect(waitForEChartsFrame(chart as any, 1)).rejects.toThrow(/did not complete/)
  expect(removed).toBe(true)
})

test('ECharts translates every cartesian mark with stable renderer-owned identities', () => {
  const expectations: Array<[string, string]> = [
    ['line', 'line'], ['area', 'line'], ['bar', 'bar'], ['column', 'bar'], ['scatter', 'scatter'], ['histogram', 'bar'],
  ]
  for (const [mark, type] of expectations) {
    const option = echartsOption(cartesianFixture(mark), defaultRendererContext) as any
    expect(option.series[0].id).toMatch(/^series:/)
    expect(option.series[0].type).toBe(type)
  }
  expect((echartsOption(cartesianFixture('area')) as any).series[0].areaStyle).toEqual({})
  expect((echartsOption(cartesianFixture('bar')) as any).series[0].encode).toEqual({ x: 'value', y: 'label' })
  expect((echartsOption(cartesianFixture('scatter')) as any).series[0].symbolSize).toBe(12)

  const waterfall = echartsOption(cartesianFixture('waterfall', ['label', 'value', 'start']), defaultRendererContext) as any
  expect(waterfall.series.map((series: any) => [series.id, series.type, series.silent])).toEqual([
    ['series:waterfall:offset', 'bar', true], ['series:primary:value', 'bar', undefined],
  ])
  const heatmap = echartsOption(cartesianFixture('heatmap', ['label', 'row', 'value']), defaultRendererContext) as any
  expect(heatmap.series[0]).toMatchObject({ id: 'series:primary:heatmap', type: 'heatmap', encode: { x: 'label', y: 'row', value: 'value' } })
  const boxplot = echartsOption(cartesianFixture('boxplot', ['label', 'min', 'q1', 'median', 'q3', 'max']), defaultRendererContext) as any
  expect(boxplot.series[0]).toMatchObject({ id: 'series:primary:boxplot', type: 'boxplot', encode: { x: 'label', y: ['min', 'q1', 'median', 'q3', 'max'] } })
})

test('ECharts honors proportional presentation and hierarchy/network layout', () => {
  const donut = echartsOption(proportionalFixture('donut'), defaultRendererContext) as any
  expect(donut.series[0]).toMatchObject({ id: 'series:primary:donut', type: 'pie', radius: ['54%', '76%'], roseType: 'radius' })
  expect(donut.graphic[0].style.text).toBe('Orders')
  const funnel = echartsOption(proportionalFixture('funnel'), defaultRendererContext) as any
  expect(funnel.series[0]).toMatchObject({ id: 'series:primary:funnel', type: 'funnel', funnelAlign: 'left', sort: 'ascending', orient: 'vertical' })

  const graph = echartsOption(networkFixture('graph'), defaultRendererContext) as any
  expect(graph.series[0]).toMatchObject({ id: 'series:hierarchy:graph', type: 'graph', layout: 'circular', roam: true })
  expect(graph.series[0].links[0]).toMatchObject({ source: 'A', target: 'B', __lv_dataset: 'primary', __lv_row_index: 0 })
  const sankey = echartsOption(networkFixture('sankey'), defaultRendererContext) as any
  expect(sankey.series[0]).toMatchObject({ id: 'series:hierarchy:sankey', type: 'sankey', orient: 'vertical', nodeGap: 18 })

  for (const mark of ['treemap', 'sunburst'] as const) {
    const envelope = hierarchyFixture(mark)
    const option = echartsOption(envelope, defaultRendererContext) as any
    expect(option.series[0].id).toBe(`series:hierarchy:${mark}`)
    expect(option.series[0].data[0].children[0].name).toBe('child')
  }
})

test('ECharts leaves absent proportional geometry fields to renderer defaults', () => {
  const pie = echartsOption(proportionalFixture('pie'), defaultRendererContext) as any
  expect(Object.hasOwn(pie.series[0], 'radius')).toBe(false)

  const defaultFunnelEnvelope = proportionalFixture('funnel') as any
  defaultFunnelEnvelope.spec.presentation.align = undefined
  const defaultFunnel = echartsOption(defaultFunnelEnvelope, defaultRendererContext) as any
  expect(Object.hasOwn(defaultFunnel.series[0], 'funnelAlign')).toBe(false)
})

test('ECharts formats gauges, applies semantic thresholds, and renders status states', () => {
  const envelope = gaugeFixture()
  const option = echartsOption(envelope, { ...defaultRendererContext, locale: 'pt-BR' },) as any
  expect(option.series[0].id).toBe('series:polar:gauge')
  expect(option.series[0].axisLine.lineStyle.color).toEqual([[0.5, defaultRendererContext.colors.attention], [0.8, defaultRendererContext.colors.danger]])
  expect(option.series[0].detail.formatter(0.75)).toBe('75,0%')

  const noData = { ...cartesianFixture('line'), status: { kind: 'no_data', message: 'No matching rows' } } as VisualizationEnvelope
  const statusOption = echartsOption(noData, defaultRendererContext) as any
  expect(statusOption.graphic[0]).toMatchObject({ type: 'text', style: { text: 'No matching rows' } })
})

test('ECharts owns the complete gauge color scale when thresholds are omitted', () => {
  const envelope = gaugeFixture() as any
  envelope.spec.presentation.thresholds = undefined
  const option = echartsOption(envelope, defaultRendererContext) as any
  expect(option.series[0].axisLine.lineStyle.color).toEqual([[1, defaultRendererContext.colors.accent]])
})

function cartesianFixture(mark: string, columns = ['label', 'value']): VisualizationEnvelope {
  const fields = columns.map((id, index) => ({ id, role: index === 0 ? 'dimension' : 'measure', dataType: index === 0 || id === 'row' ? 'string' : 'decimal', nullable: false, label: id }))
  const y = columns.slice(1).map((field) => ({ dataset: 'primary', field }))
  const row = columns.map((id, index) => index === 0 ? 'A' : id === 'row' ? 'R1' : index)
  return {
    schemaVersion: 3, visualID: mark, rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: { kind: 'cartesian', title: mark, mark, datasets: [{ id: 'primary', fields }], dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: mark, description: mark }, interactions: [], x: { dataset: 'primary', field: 'label' }, y, presentation: { legend: 'bottom', showLabels: true, smooth: true, stacked: true, showSymbols: false, dataZoom: true, area: mark === 'area', step: true, symbolSize: 12, labelPosition: 'top', orientation: mark === 'bar' ? 'horizontal' : 'vertical', histogramBins: mark === 'histogram' ? 10 : undefined } },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns, rows: [row], completeness: 'complete' }] }, selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
}

function proportionalFixture(mark: 'pie' | 'donut' | 'funnel'): VisualizationEnvelope {
  return {
    schemaVersion: 3, visualID: mark, rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: { kind: 'proportional', title: mark, mark, datasets: [{ id: 'primary', fields: [{ id: 'label', role: 'dimension', dataType: 'string', nullable: false, label: 'Label' }, { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' }] }], dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: mark, description: mark }, interactions: [], category: { dataset: 'primary', field: 'label' }, value: { dataset: 'primary', field: 'value' }, presentation: { legend: 'right', showLabels: true, orientation: 'vertical', rose: true, centerLabel: mark === 'donut' ? 'Orders' : undefined, labelPosition: 'outside', innerRadius: mark === 'donut' ? 0.54 : undefined, outerRadius: mark === 'donut' ? 0.76 : undefined, align: mark === 'funnel' ? 'left' : undefined, sort: mark === 'funnel' ? 'ascending' : undefined } },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['label', 'value'], rows: [['A', 10]], completeness: 'complete' }] }, selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
}

function hierarchyFixture(mark: 'tree' | 'treemap' | 'sunburst'): VisualizationEnvelope {
  const envelope = cartesianFixture('line') as any
  envelope.visualID = mark
  envelope.spec = { kind: 'hierarchy', title: mark, mark, datasets: [{ id: 'primary', fields: [{ id: 'node', role: 'identity', dataType: 'string', nullable: false, label: 'Node' }, { id: 'parent', role: 'dimension', dataType: 'string', nullable: true, label: 'Parent' }, { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' }] }], dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: mark, description: mark }, interactions: [], node: { dataset: 'primary', field: 'node' }, parent: { dataset: 'primary', field: 'parent' }, value: { dataset: 'primary', field: 'value' }, presentation: { legend: 'hidden', showLabels: true, orientation: 'vertical', initialDepth: 2, roam: true, layout: 'standard', breadcrumb: true } }
  envelope.dataState.datasets[0] = { ...envelope.dataState.datasets[0], columns: ['node', 'parent', 'value'], rows: [['root', null, 10], ['child', 'root', 4]] }
  return envelope
}

function networkFixture(mark: 'graph' | 'sankey'): VisualizationEnvelope {
  const envelope = hierarchyFixture('tree') as any
  envelope.visualID = mark
  envelope.spec.mark = mark
  envelope.spec.node = { dataset: 'primary', field: 'source' }
  envelope.spec.parent = undefined
  envelope.spec.source = { dataset: 'primary', field: 'source' }
  envelope.spec.target = { dataset: 'primary', field: 'target' }
  envelope.spec.presentation = { ...envelope.spec.presentation, orientation: 'vertical', layout: 'circular', nodeGap: 18, curveness: 0.3, focus: 'adjacency' }
  envelope.spec.datasets[0].fields = [{ id: 'source', role: 'dimension', dataType: 'string', nullable: false, label: 'Source' }, { id: 'target', role: 'dimension', dataType: 'string', nullable: false, label: 'Target' }, { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Value' }]
  envelope.dataState.datasets[0] = { ...envelope.dataState.datasets[0], columns: ['source', 'target', 'value'], rows: [['A', 'B', 4]] }
  return envelope
}

function gaugeFixture(): VisualizationEnvelope {
  return {
    schemaVersion: 3, visualID: 'gauge', rendererID: 'echarts', specRevision: 'sha256:test', dataRevision: 1,
    spec: { kind: 'polar', title: 'Gauge', mark: 'gauge', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Rate', format: { kind: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 } }] }], dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Gauge', description: 'Gauge' }, interactions: [], value: { dataset: 'primary', field: 'value' }, presentation: { legend: 'hidden', showLabels: true, minimum: 0, maximum: 1, showPointer: true, progressWidth: 12, thresholds: [{ value: 0.5, tone: 'warning' }, { value: 0.8, tone: 'danger' }] } },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['value'], rows: [[0.75]], completeness: 'complete' }] }, selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
}
