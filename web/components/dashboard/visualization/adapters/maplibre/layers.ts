import type { FeatureCollection } from 'geojson'
import type { VisualizationGeographicLayer } from '../../../../../generated/visualization'

export function mapLayer(id: string, layerOrKind: VisualizationGeographicLayer | VisualizationGeographicLayer['kind']): any {
  const layer = typeof layerOrKind === 'string' ? undefined : layerOrKind
  const kind = typeof layerOrKind === 'string' ? layerOrKind : layerOrKind.kind
  if (kind === 'choropleth') {
    const choropleth = layer?.kind === 'choropleth' ? layer : undefined
    return { id, source: id, type: 'fill', paint: { 'fill-color': ['case', ['==', ['get', '__ld_value'], null], choropleth?.color.nullColor ?? '#d8dee4', layerColorExpression(choropleth?.color)], 'fill-opacity': ['case', ['get', '__ld_selected'], 1, ['get', '__ld_has_selection'], 0.4, choropleth?.opacity ?? 0.82], 'fill-outline-color': choropleth?.stroke.color ?? '#ffffff' } }
  }
  if (kind === 'reference') {
    const reference = layer?.kind === 'reference' ? layer : undefined
    return { id, source: id, type: 'fill', filter: ['==', ['geometry-type'], 'Polygon'], paint: { 'fill-color': paletteColors(reference?.color)[2], 'fill-opacity': reference?.opacity ?? 0.18, 'fill-outline-color': reference?.stroke.color ?? '#57606a' } }
  }
  if (kind === 'path') {
    const path = layer?.kind === 'path' ? layer : undefined
    return { id, source: id, type: 'line', paint: { 'line-color': path?.category || path?.value ? layerColorExpression(path?.color) : path?.stroke.color ?? '#0969da', 'line-width': path?.line.width ?? 3, 'line-opacity': (path?.opacity ?? 0.82) * (path?.stroke.opacity ?? 1) } }
  }
  if (kind === 'point') {
    const point = layer?.kind === 'point' ? layer : undefined
    const minimumRadius = point?.size?.minimumRadius ?? 5, maximumRadius = point?.size?.maximumRadius ?? 10
    return { id, source: id, type: 'circle', filter: ['!', ['has', 'point_count']], minzoom: point?.visibility.minimumZoom, maxzoom: point?.visibility.maximumZoom, paint: { 'circle-radius': ['case', ['get', '__ld_selected'], maximumRadius + 3, ['interpolate', ['linear'], ['sqrt', ['get', '__ld_weight']], 0, minimumRadius, 1, maximumRadius]], 'circle-color': layerColorExpression(point?.color), 'circle-stroke-color': point?.stroke.color ?? '#ffffff', 'circle-stroke-opacity': point?.stroke.opacity ?? 1, 'circle-stroke-width': ['case', ['get', '__ld_selected'], (point?.stroke.width ?? 1.5) + 1, point?.stroke.width ?? 1.5], 'circle-opacity': ['case', ['get', '__ld_selected'], 1, ['get', '__ld_has_selection'], 0.3, point?.opacity ?? 0.78] } }
  }
  const heat = layer?.kind === 'heat' || layer?.kind === 'density' ? layer : undefined
  const colors = paletteColors(heat?.color)
  return { id, source: id, type: 'heatmap', paint: {
    'heatmap-weight': ['*', ['get', '__ld_weight'], ['case', ['get', '__ld_selected'], 1, 0.75]],
    'heatmap-intensity': heat?.heat.intensity ?? (kind === 'density' ? 1.35 : 1),
    'heatmap-radius': heat?.heat.radius ?? (kind === 'density' ? 24 : 32),
    'heatmap-opacity': heat?.opacity ?? 0.86,
    'heatmap-color': ['interpolate', ['linear'], ['heatmap-density'], 0, transparentColor(colors[0]), 0.15, colors[0], 0.35, colors[1], 0.6, colors[2], 0.85, colors[3], 1, colors[4]],
  } }
}

function colorInterpolation(scale?: { palette: string; reverse: boolean }): unknown[] {
  const colors = paletteColors(scale)
  return ['interpolate', ['linear'], ['get', '__ld_weight'], 0, colors[0], 0.25, colors[1], 0.5, colors[2], 0.75, colors[3], 1, colors[4]]
}

function layerColorExpression(scale?: { kind: string; palette: string; reverse: boolean; nullColor: string }): unknown[] {
  if (scale?.kind === 'categorical') return ['coalesce', ['get', '__ld_color'], scale.nullColor]
  return colorInterpolation(scale)
}

export function paletteColors(scale?: { palette: string; reverse: boolean }): string[] {
  const palettes: Record<string, string[]> = {
    blue: ['#ddf4ff', '#80ccff', '#54aeff', '#0969da', '#0550ae'],
    teal: ['#e1f7f5', '#90e0d9', '#39c5bb', '#008c95', '#006d77'],
    purple: ['#fbefff', '#d8b9ff', '#bf87ff', '#8250df', '#6639ba'],
    orange: ['#fff1e5', '#ffc680', '#fb8f44', '#d15704', '#bc4c00'],
    red: ['#ffebe9', '#ffb3b6', '#ff8182', '#cf222e', '#a40e26'],
  }
  const selected = [...(palettes[scale?.palette ?? 'blue'] ?? palettes.blue!)]
  return scale?.reverse ? selected.reverse() : selected
}

function transparentColor(color: string): string {
  if (/^#[0-9a-f]{6}$/i.test(color)) return `${color}00`
  return 'rgba(9,105,218,0)'
}

function layerWeightDomain(layer: VisualizationGeographicLayer): { domainMinimum?: number; domainMidpoint?: number; domainMaximum?: number } | undefined {
  if (layer.kind === 'point' && layer.size && (layer.size.domainMinimum !== undefined || layer.size.domainMaximum !== undefined)) return layer.size
  if ('color' in layer) return layer.color
  return undefined
}

export function mapOutlineLayer(id: string, source: string): any {
  return {
    id, source, type: 'line',
    filter: ['==', ['get', '__ld_selected'], true],
    paint: { 'line-color': '#bf3989', 'line-opacity': 1, 'line-width': 3 },
  }
}

export function normalizeFeatureWeights(data: FeatureCollection, domain?: { domainMinimum?: number; domainMidpoint?: number; domainMaximum?: number }): FeatureCollection {
  const values = data.features.map((feature) => feature.properties?.__ld_value).filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
  const minimum = domain?.domainMinimum ?? (values.length > 0 ? Math.min(...values) : 0)
  const maximum = domain?.domainMaximum ?? (values.length > 0 ? Math.max(...values) : 0)
  const span = maximum - minimum
  return {
    ...data,
    features: data.features.map((feature) => {
      const value = feature.properties?.__ld_value
      let weight = typeof value !== 'number' || !Number.isFinite(value) ? 0 : span === 0 ? (value === 0 ? 0 : 1) : Math.max(0, Math.min(1, (value - minimum) / span))
      const midpoint = domain?.domainMidpoint
      if (typeof value === 'number' && Number.isFinite(value) && midpoint !== undefined && midpoint > minimum && midpoint < maximum) {
        weight = value <= midpoint
          ? 0.5 * Math.max(0, Math.min(1, (value - minimum) / (midpoint - minimum)))
          : 0.5 + 0.5 * Math.max(0, Math.min(1, (value - midpoint) / (maximum - midpoint)))
      }
      return { ...feature, properties: { ...feature.properties, __ld_weight: weight } }
    }),
  }
}

export function applyFeatureScales(data: FeatureCollection, layer: VisualizationGeographicLayer): FeatureCollection {
  const normalized = normalizeFeatureWeights(data, layerWeightDomain(layer))
  if (!('color' in layer) || layer.color.kind !== 'categorical') return normalized
  const categories = [...new Set(normalized.features.map((feature) => feature.properties?.__ld_category).filter((value) => value !== null && value !== undefined).map(String))].sort((a, b) => a.localeCompare(b))
  const colors = paletteColors(layer.color)
  const colorByCategory = new Map(categories.map((category, index) => [category, colors[index % colors.length]!]))
  return {
    ...normalized,
    features: normalized.features.map((feature) => {
      const category = feature.properties?.__ld_category
      const color = category === null || category === undefined ? layer.color.nullColor : colorByCategory.get(String(category)) ?? layer.color.nullColor
      return { ...feature, properties: { ...feature.properties, __ld_color: color } }
    }),
  }
}
