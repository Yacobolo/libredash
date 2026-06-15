import type { EChartsOption } from 'echarts'
import type { ChartDatum, ChartPayload, ChartTokens, ChartType } from './types'
import { booleanValue, colorWithAlpha, formatValue, normalizeShape, normalizeType, numberValue, selectedValues, stringValue, unique } from './utils'

export function buildEChartsOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  switch (normalizeShape(payload.shape, payload.type, Boolean(payload.series?.length))) {
    case 'single_value':
      return singleValueAdapter(payload, tokens)
    case 'matrix':
      return matrixAdapter(payload, tokens)
    case 'graph':
      return graphAdapter(payload, tokens)
    case 'geo':
      return geoAdapter(payload, tokens)
    case 'ohlc':
      return ohlcAdapter(payload, tokens)
    case 'distribution':
      return distributionAdapter(payload, tokens)
    case 'category_series_value':
    case 'category_value':
    default:
      if (isPartToWholeType(normalizeType(payload.type))) return partToWholeAdapter(payload, tokens)
      return categoryAdapter(payload, tokens)
  }
}

function baseOption(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const type = normalizeType(payload.type)
  return {
    backgroundColor: 'transparent',
    color: tokens.palette,
    aria: { show: true },
    animationDuration: 220,
    animationDurationUpdate: 260,
    tooltip: {
      trigger: ['line', 'area', 'bar', 'column', 'scatter', 'heatmap', 'candlestick', 'boxplot'].includes(type) ? 'axis' : 'item',
      valueFormatter: (value) => formatValue(Number(value), payload.unit),
      borderColor: tokens.border,
      backgroundColor: tokens.surface,
      textStyle: { color: tokens.text },
    },
    grid: {
      top: 16,
      right: 20,
      bottom: 32,
      left: 44,
      containLabel: true,
    },
  }
}

function itemDataFor(payload: ChartPayload, tokens: ChartTokens) {
  const { selected, hasSelection } = selectedValues(payload)
  return (payload.data ?? []).map((row, index) => {
    const label = stringValue(row, 'label')
    return {
      name: label,
      value: numberValue(row, 'value'),
      selected: selected.has(label),
      itemStyle: {
        color: tokens.palette[index % tokens.palette.length],
        opacity: hasSelection && !selected.has(label) ? 0.35 : 1,
      },
    }
  })
}

function partToWholeAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const type = normalizeType(payload.type)
  const itemData = itemDataFor(payload, tokens)
  const base = baseOption(payload, tokens)

  if (type === 'pie' || type === 'donut') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'pie',
          radius: type === 'donut' ? ['48%', '72%'] : ['0%', '72%'],
          center: ['50%', '52%'],
          data: itemData,
          selectedMode: 'multiple',
          label: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
          universalTransition: true,
        },
      ],
    }
  }

  if (type === 'funnel') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'funnel',
          left: '8%',
          top: 18,
          width: '84%',
          bottom: 18,
          sort: 'descending',
          data: itemData,
          label: { color: tokens.text, fontSize: 10, fontWeight: 700 },
        },
      ],
    }
  }

  if (type === 'treemap') {
    return {
      ...base,
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'treemap',
          roam: false,
          nodeClick: false,
          breadcrumb: { show: false },
          data: itemData,
          label: { color: tokens.text, fontSize: 10, fontWeight: 800 },
          upperLabel: { show: false },
        },
      ],
    }
  }
  return categoryAdapter(payload, tokens)
}

function singleValueAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const point = payload.data?.[0]
  const value = numberValue(point, 'value')
  return {
    ...baseOption(payload, tokens),
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'gauge',
        min: 0,
        max: Math.max(100, Math.ceil(value * 1.2)),
        progress: { show: true, width: 12 },
        axisLine: { lineStyle: { width: 12, color: [[1, tokens.grid]] } },
        axisTick: { show: false },
        splitLine: { length: 8, lineStyle: { color: tokens.border } },
        axisLabel: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
        pointer: { width: 4 },
        anchor: { show: true, size: 6, itemStyle: { color: tokens.palette[0] } },
        detail: {
          valueAnimation: true,
          color: tokens.text,
          fontSize: 24,
          fontWeight: 850,
          formatter: (next: number) => formatValue(next, payload.unit),
        },
        data: [{ name: stringValue(point, 'label') || payload.title, value, itemStyle: { color: tokens.palette[0] } }],
      },
    ],
  }
}

function categoryAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const type = normalizeType(payload.type)
  const data = payload.data ?? []
  const { selected, hasSelection } = selectedValues(payload)
  const stacked = Boolean(payload.options?.stacked)
  const horizontal = type === 'bar'
  const seriesType = type === 'area' ? 'line' : type === 'column' ? 'bar' : type
  const labels = unique(data.map((row) => stringValue(row, 'label')))
  const seriesNames = unique(data.map((row) => stringValue(row, 'series') || payload.title || 'Value'))
  const multiSeries = seriesNames.length > 1 || data.some((row) => stringValue(row, 'series'))
  return {
    ...baseOption(payload, tokens),
    yAxis: horizontal
      ? {
          ...axis('category', tokens),
          data: labels,
          inverse: true,
          axisLabel: { color: tokens.text, fontWeight: 750, fontSize: 10 },
        }
      : axis('value', tokens),
    xAxis: horizontal
      ? axis('value', tokens)
      : {
          ...axis('category', tokens),
          data: labels,
          axisLabel: {
            color: tokens.muted,
            fontWeight: 700,
            fontSize: 10,
            interval: Math.ceil(labels.length / 6),
          },
        },
    series: seriesNames.map((seriesName, seriesIndex) => ({
      id: `${payload.id || 'chart'}:${seriesName}`,
      name: multiSeries ? seriesName : payload.title,
      type: seriesType,
      stack: stacked ? payload.id || 'chart' : undefined,
      smooth: type === 'line' || type === 'area',
      areaStyle: type === 'area' ? { color: colorWithAlpha(tokens.palette[seriesIndex % tokens.palette.length], 0.24) } : undefined,
      symbolSize: type === 'scatter' ? 9 : 7,
      barMaxWidth: 18,
      data: labels.map((label, labelIndex) => {
        const point = data.find((candidate) => stringValue(candidate, 'label') === label && (stringValue(candidate, 'series') || payload.title || 'Value') === seriesName)
        const isSelected = selected.has(label)
        return {
          name: label,
          value: numberValue(point, 'value'),
          itemStyle: {
            color:
              hasSelection && !isSelected
                ? tokens.dimmed
                : tokens.palette[(multiSeries ? seriesIndex : labelIndex) % tokens.palette.length],
            opacity: hasSelection && !isSelected ? 0.35 : 1,
          },
        }
      }),
      lineStyle: { color: tokens.palette[seriesIndex % tokens.palette.length], width: 2.5 },
      universalTransition: true,
    })),
  }
}

function matrixAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const data = payload.data ?? []
  const rows = unique(data.map((row) => stringValue(row, 'row')))
  const columns = unique(data.map((row) => stringValue(row, 'column')))
  const values = data.map((row) => numberValue(row, 'value'))
  const maxValue = Math.max(1, ...values)
  const { selected, hasSelection } = selectedValues(payload, 'row')
  return {
    ...baseOption(payload, tokens),
    tooltip: { trigger: 'item', borderColor: tokens.border, backgroundColor: tokens.surface, textStyle: { color: tokens.text } },
    grid: { top: 18, right: 18, bottom: 48, left: 56, containLabel: true },
    xAxis: { ...axis('category', tokens), data: columns, axisLabel: { color: tokens.muted, fontSize: 10, fontWeight: 700 } },
    yAxis: { ...axis('category', tokens), data: rows, axisLabel: { color: tokens.text, fontSize: 10, fontWeight: 750 } },
    visualMap: {
      min: 0,
      max: maxValue,
      calculable: false,
      orient: 'horizontal',
      left: 'center',
      bottom: 6,
      inRange: { color: [colorWithAlpha(tokens.palette[0], 0.16), tokens.palette[0]] },
      textStyle: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
    },
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'heatmap',
        data: data.map((row) => {
          const rowName = stringValue(row, 'row')
          const isSelected = selected.has(rowName)
          return {
            name: rowName,
            value: [columns.indexOf(stringValue(row, 'column')), rows.indexOf(rowName), numberValue(row, 'value')],
            itemStyle: { opacity: hasSelection && !isSelected ? 0.35 : 1 },
          }
        }),
        label: { show: false },
        emphasis: { itemStyle: { borderColor: tokens.text, borderWidth: 1 } },
      },
    ],
  }
}

function graphAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const type = normalizeType(payload.type)
  const data = payload.data ?? []
  const nodeNames = unique(data.flatMap((row) => [stringValue(row, 'source'), stringValue(row, 'target')]).filter(Boolean))
  if (type === 'graph') {
    return {
      ...baseOption(payload, tokens),
      tooltip: { trigger: 'item', borderColor: tokens.border, backgroundColor: tokens.surface, textStyle: { color: tokens.text } },
      series: [
        {
          id: payload.id || 'chart',
          name: payload.title,
          type: 'graph',
          layout: 'force',
          roam: true,
          label: { show: true, color: tokens.text, fontSize: 10, fontWeight: 700 },
          force: { repulsion: 80, edgeLength: 80 },
          data: nodeNames.map((name, index) => ({ name, itemStyle: { color: tokens.palette[index % tokens.palette.length] } })),
          links: data.map((row) => ({ source: stringValue(row, 'source'), target: stringValue(row, 'target'), value: numberValue(row, 'value') })),
          lineStyle: { color: tokens.border, curveness: 0.18 },
        },
      ],
    }
  }
  return {
    ...baseOption(payload, tokens),
    tooltip: { trigger: 'item', borderColor: tokens.border, backgroundColor: tokens.surface, textStyle: { color: tokens.text } },
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'sankey',
        left: 12,
        right: 18,
        top: 12,
        bottom: 12,
        nodeGap: 8,
        data: nodeNames.map((name) => ({ name })),
        links: data.map((row) => ({ source: stringValue(row, 'source'), target: stringValue(row, 'target'), value: numberValue(row, 'value') })),
        label: { color: tokens.text, fontSize: 10, fontWeight: 700 },
        lineStyle: { color: 'gradient', curveness: 0.5 },
        emphasis: { focus: 'adjacency' },
      },
    ],
  }
}

function geoAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  return {
    ...baseOption(payload, tokens),
    tooltip: { trigger: 'item', borderColor: tokens.border, backgroundColor: tokens.surface, textStyle: { color: tokens.text } },
    visualMap: {
      min: 0,
      max: Math.max(1, ...(payload.data ?? []).map((row) => numberValue(row, 'value'))),
      left: 8,
      bottom: 8,
      textStyle: { color: tokens.muted, fontSize: 10, fontWeight: 700 },
      inRange: { color: [colorWithAlpha(tokens.palette[0], 0.18), tokens.palette[0]] },
    },
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'map',
        map: String(payload.options?.map || 'world'),
        roam: true,
        data: (payload.data ?? []).map((row) => ({ name: stringValue(row, 'name'), value: numberValue(row, 'value'), selected: booleanValue(row, 'selected') })),
        label: { color: tokens.text, fontSize: 9, fontWeight: 700 },
        itemStyle: { borderColor: tokens.border },
      },
    ],
  }
}

function ohlcAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const data = payload.data ?? []
  const labels = data.map((row) => stringValue(row, 'label'))
  return {
    ...baseOption(payload, tokens),
    xAxis: { ...axis('category', tokens), data: labels, axisLabel: { color: tokens.muted, fontSize: 10, fontWeight: 700 } },
    yAxis: axis('value', tokens),
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'candlestick',
        data: data.map((row) => [numberValue(row, 'open'), numberValue(row, 'close'), numberValue(row, 'low'), numberValue(row, 'high')]),
        itemStyle: { color: tokens.palette[1], color0: tokens.palette[3], borderColor: tokens.palette[1], borderColor0: tokens.palette[3] },
      },
    ],
  }
}

function distributionAdapter(payload: ChartPayload, tokens: ChartTokens): EChartsOption {
  const data = payload.data ?? []
  const labels = data.map((row) => stringValue(row, 'label'))
  return {
    ...baseOption(payload, tokens),
    xAxis: { ...axis('category', tokens), data: labels, axisLabel: { color: tokens.muted, fontSize: 10, fontWeight: 700 } },
    yAxis: axis('value', tokens),
    series: [
      {
        id: payload.id || 'chart',
        name: payload.title,
        type: 'boxplot',
        data: data.map((row) => [numberValue(row, 'min'), numberValue(row, 'q1'), numberValue(row, 'median'), numberValue(row, 'q3'), numberValue(row, 'max')]),
        itemStyle: { color: colorWithAlpha(tokens.palette[0], 0.28), borderColor: tokens.palette[0] },
      },
    ],
  }
}

function axis(type: 'category' | 'value', tokens: ChartTokens) {
  return {
    type,
    axisLine: { lineStyle: { color: tokens.border } },
    axisTick: { show: false },
    axisLabel: { color: tokens.muted, fontWeight: 700, fontSize: 10 },
    splitLine: { lineStyle: { color: tokens.grid } },
  }
}

function isPartToWholeType(type: ChartType): boolean {
  return type === 'pie' || type === 'donut' || type === 'funnel' || type === 'treemap'
}
