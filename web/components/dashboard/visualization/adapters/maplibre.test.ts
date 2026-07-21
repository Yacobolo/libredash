import { expect, test } from 'bun:test'

import type { VisualizationEnvelope, VisualizationGeographicLayer } from '../../../../generated/visualization'
import type { FeatureCollection } from 'geojson'
import { applyFeatureScales, basemapBoundaryLayer, basemapLayer, concreteCSSColor, coordinateGeometry, coordinateReferenceGrid, fitMapToGeographicData, installWebGLRecovery, interactionCommandForRenderedFeatures, joinGeometry, loadMapStyleAsset, mapAccessibleData, mapInteractionCommand, mapLayer, mapOutlineLayer, mapPointerOptions, mapThemeColors, mapTooltipEntries, normalizeFeatureWeights, pathGeometry, removeRendererFrame, sameOriginGeometryURL, spatialWindowRequest, updateSelectionSources, verifyGeometryDigest } from './maplibre'
import { adapterObservation } from '../telemetry'

test('MapLibre geometry assets are same-origin and content addressed', async () => {
  expect(sameOriginGeometryURL('/static/geometry/states.geojson', 'https://dash.example/workspaces/sales').href).toBe('https://dash.example/static/geometry/states.geojson')
  expect(() => sameOriginGeometryURL('https://attacker.example/states.geojson', 'https://dash.example/workspaces/sales')).toThrow(/same-origin/)
  await expect(verifyGeometryDigest(new TextEncoder().encode('geometry'), 'sha256:invalid')).rejects.toThrow(/canonical SHA-256/)
  await expect(verifyGeometryDigest(new TextEncoder().encode('geometry'), `sha256:${'0'.repeat(64)}`)).rejects.toThrow(/digest mismatch/)
})

test('MapLibre map styles rewrite only pinned same-origin PMTiles and assets', async () => {
  const style = new TextEncoder().encode(JSON.stringify({ version: 8, sources: { base: { type: 'vector', url: 'pmtiles://__LIBREDASH_ARCHIVE__' } }, layers: [] }))
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', style))
  const asset = {
    id: 'streets', styleUrl: '/map-assets/streets/style.json', styleDigest: `sha256:${[...digest].map((value) => value.toString(16).padStart(2, '0')).join('')}`,
    archiveUrl: '/map-assets/streets/map.pmtiles', archiveDigest: `sha256:${'a'.repeat(64)}`, glyphsUrl: '/map-assets/streets/glyphs/{fontstack}/{range}.pbf', spriteUrl: '/map-assets/streets/sprite',
    source: 'OSM', license: 'ODbL', attribution: 'OSM', minimumZoom: 0, maximumZoom: 6, bounds: [-180, -85, 180, 85], labelAnchor: 'labels',
  } as const
  const previous = globalThis.fetch
  globalThis.fetch = (async () => new Response(style)) as typeof fetch
  try {
    const loaded = await loadMapStyleAsset(asset, 'https://dash.example/workspaces/maps')
    expect((loaded.sources.base as { url?: string }).url).toBe('pmtiles://https://dash.example/map-assets/streets/map.pmtiles')
    expect(loaded.glyphs).toBe('https://dash.example/map-assets/streets/glyphs/{fontstack}/{range}.pbf')
    await expect(loadMapStyleAsset({ ...asset, styleUrl: 'https://attacker.example/style.json' }, 'https://dash.example/maps')).rejects.toThrow(/same-origin/)
  } finally { globalThis.fetch = previous }
})

test('MapLibre prevents permanent WebGL loss, repaints after restoration, and removes recovery listeners', () => {
  const canvas = new EventTarget()
  let resized = 0
  let repainted = 0
  const observations: string[] = []
  const dispose = installWebGLRecovery(canvas, {
    resize: () => { resized++ },
    triggerRepaint: () => { repainted++ },
  }, (stage) => observations.push(stage))

  const lost = new Event('webglcontextlost', { cancelable: true })
  canvas.dispatchEvent(lost)
  expect(lost.defaultPrevented).toBe(true)
  canvas.dispatchEvent(new Event('webglcontextrestored'))
  expect([resized, repainted]).toEqual([1, 1])
  expect(observations).toEqual(['webgl_context_loss', 'webgl_context_restored'])

  dispose()
  canvas.dispatchEvent(new Event('webglcontextrestored'))
  expect([resized, repainted]).toEqual([1, 1])
})

test('MapLibre observations enter the shared visualization telemetry contract', () => {
  expect(adapterObservation({ stage: 'basemap_load', durationMs: 12, visualID: 'map', rendererID: 'maplibre' })).toEqual({
    stage: 'adapter_observation', adapterStage: 'basemap_load', durationMs: 12, visualID: 'map', rendererID: 'maplibre',
  })
  expect(adapterObservation({ stage: 'unknown', durationMs: 12, visualID: 'map', rendererID: 'maplibre' })).toBeUndefined()
})

test('MapLibre point, heat, and density layers use typed in-memory coordinates without geometry fetches', () => {
  const layer = {
    id: 'stores', kind: 'point', latitude: { dataset: 'primary', field: 'lat' }, longitude: { dataset: 'primary', field: 'lon' }, value: { dataset: 'primary', field: 'value' },
  } as VisualizationGeographicLayer
  const envelope = {
    dataRevision: 9,
    dataState: { kind: 'inline', datasets: [{ id: 'primary', columns: ['lat', 'lon', 'value'], rows: [[55.67, 12.56, 3], ['invalid', 12, 9], [91, 12, 4], [20, 181, 5]] }] },
    selection: [{ datum: { dataset: 'primary', dataRevision: 9, identity: { lat: 55.67, lon: 12.56 } }, label: 'Copenhagen' }],
  } as VisualizationEnvelope
  const geometry = coordinateGeometry(envelope, layer)
  expect(geometry.features).toHaveLength(1)
  expect(geometry.features[0]?.geometry).toEqual({ type: 'Point', coordinates: [12.56, 55.67] })
  expect(geometry.features[0]?.properties?.__ld_value).toBe(3)
  expect(geometry.features[0]?.properties?.__ld_selected).toBe(true)
  expect(geometry.features[0]?.properties).toMatchObject({ __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'stores' })
})

test('MapLibre selectable features carry only a validated internal row locator', () => {
  const envelope = selectableEnvelope()
  const layer = envelope.spec.kind === 'geographic' ? envelope.spec.layers[0]! : undefined
  const geometry = {
    type: 'FeatureCollection',
    features: [
      { type: 'Feature', id: 'SP', geometry: { type: 'Polygon', coordinates: [] }, properties: { id: 'SP', publicGeometryName: 'São Paulo' } },
      { type: 'Feature', id: 'BA', geometry: { type: 'Polygon', coordinates: [] }, properties: { id: 'BA' } },
    ],
  } as FeatureCollection
  const joined = joinGeometry(envelope, layer!, geometry)
  expect(joined.features[0]?.properties).toMatchObject({
    id: 'SP', publicGeometryName: 'São Paulo', __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'states', __ld_value: 10,
  })
  expect(joined.features[0]?.properties).not.toHaveProperty('customer_secret')
  expect(joined.features[1]?.properties).not.toHaveProperty('__ld_row_index')
})

test('MapLibre hit testing selects the topmost valid point or region and rejects forged locators', () => {
  const envelope = selectableEnvelope()
  const features = [
    { layer: { id: 'ld-states' }, properties: { __ld_dataset: 'primary', __ld_row_index: 1, __ld_layer_id: 'states' } },
    { layer: { id: 'ld-states' }, properties: { __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'states' } },
  ]
  expect(interactionCommandForRenderedFeatures(envelope, features, ['ld-states'])?.mappings[0]?.value).toBe('RJ')
  expect(interactionCommandForRenderedFeatures(envelope, [{ layer: { id: 'ld-states' }, properties: { __ld_dataset: 'forged', __ld_row_index: 0, __ld_layer_id: 'states' } }], ['ld-states'])).toBeUndefined()
  expect(interactionCommandForRenderedFeatures(envelope, [{ layer: { id: 'ld-states' }, properties: { __ld_dataset: 'primary', __ld_row_index: 99, __ld_layer_id: 'states' } }], ['ld-states'])).toBeUndefined()
  expect(interactionCommandForRenderedFeatures(envelope, [{ layer: { id: 'ld-heat' }, properties: { __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'heat' } }], ['ld-states'])).toBeUndefined()
})

test('MapLibre keeps semantic selection active when pan and zoom are disabled', () => {
  const envelope = selectableEnvelope()
  expect(mapPointerOptions(envelope)).toEqual({
    interactive: true,
    scrollZoom: false, boxZoom: false, dragRotate: false, dragPan: false, keyboard: false,
    doubleClickZoom: false, touchZoomRotate: false, touchPitch: false,
  })
})

test('MapLibre blank hits clear only the selection owned by that map', () => {
  const envelope = selectableEnvelope()
  expect(mapInteractionCommand(envelope, [], ['ld-states'])).toBeUndefined()
  const selected = {
    ...envelope,
    selection: [{ datum: { dataset: 'primary', dataRevision: 4, identity: { state: 'SP' } }, label: 'SP' }],
  } as VisualizationEnvelope
  expect(mapInteractionCommand(selected, [], ['ld-states'])).toEqual({
    sourceKind: 'visual', sourceId: 'state-map', interactionKind: 'point_selection', action: 'clear', toggle: false, mappings: [],
  })
})

test('MapLibre selection-only refreshes update existing sources without rebuilding map state', () => {
  const envelope = selectableEnvelope()
  const layer = envelope.spec.kind === 'geographic' ? envelope.spec.layers[0]! : undefined
  const geometry = { type: 'FeatureCollection', features: [{ type: 'Feature', id: 'SP', geometry: { type: 'Polygon', coordinates: [] }, properties: { id: 'SP' } }] } as FeatureCollection
  const updates: FeatureCollection[] = []
  const updated = updateSelectionSources(envelope, [{ spec: layer!, sourceID: 'ld-states', geometry }], (sourceID) => sourceID === 'ld-states' ? { setData: (data) => updates.push(data as FeatureCollection) } : undefined)
  expect(updated).toBe(1)
  expect(updates).toHaveLength(1)
  expect(updates[0]?.features[0]?.properties).toMatchObject({ __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'states' })
})

test('MapLibre spatial requests preserve revision and viewport identity', () => {
  const envelope = {
    ...selectableEnvelope(),
    dataRevision: 12,
    dataState: {
      kind: 'spatial_windowed', specRevision: 'sha256:test', dataRevision: 12, generation: 2,
      schema: selectableEnvelope().spec.datasets[0], cardinality: { kind: 'estimate', count: 1_000_000 },
      extent: { west: -180, south: -85, east: 180, north: 85 }, rowCap: 1_000_000, featureCap: 5000,
      resetVersion: 4,
    },
  } as VisualizationEnvelope
  expect(spatialWindowRequest(envelope, { west: 170, south: -20, east: -170, north: 25 }, 3.25, 960, 540, 8)).toEqual({
    visualID: 'state-map', specRevision: 'sha256:test', dataRevision: 12, requestSeq: 8, resetVersion: 4,
    bounds: { west: 170, south: -20, east: -170, north: 25 }, zoom: 3.25, width: 960, height: 540,
    windowID: '170.000000,-20.000000,-170.000000,25.000000@3.250:960x540',
  })
  expect(spatialWindowRequest(selectableEnvelope(), { west: 0, south: 0, east: 1, north: 1 }, 1, 100, 100, 1)).toBeUndefined()
})

test('a superseded MapLibre mount cannot remove the winning renderer frame', () => {
  const container = {} as ParentNode
  let staleRemoved = false
  const staleFrame = {
    parentNode: null,
    remove: () => { staleRemoved = true },
  }
  removeRendererFrame(container, staleFrame as unknown as HTMLElement)
  expect(staleRemoved).toBe(false)

  let ownedRemoved = false
  const ownedFrame = { parentNode: container, remove: () => { ownedRemoved = true } }
  removeRendererFrame(container, ownedFrame as unknown as HTMLElement)
  expect(ownedRemoved).toBe(true)
})

test('MapLibre normalizes finite measure values without losing raw tooltip values', () => {
  const data = {
    type: 'FeatureCollection',
    features: [
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-70, -20] }, properties: { __ld_value: 10 } },
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-60, -10] }, properties: { __ld_value: 20 } },
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-50, 0] }, properties: { __ld_value: 30 } },
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-40, 10] }, properties: { __ld_value: null } },
    ],
  } as FeatureCollection

  const normalized = normalizeFeatureWeights(data)
  expect(normalized.features.map((feature) => feature.properties?.__ld_value)).toEqual([10, 20, 30, null])
  expect(normalized.features.map((feature) => feature.properties?.__ld_weight)).toEqual([0, 0.5, 1, 0])

  const fixed = normalizeFeatureWeights(data, { domainMinimum: 0, domainMaximum: 40 })
  expect(fixed.features.map((feature) => feature.properties?.__ld_weight)).toEqual([0.25, 0.5, 0.75, 0])

  const diverging = normalizeFeatureWeights(data, { domainMinimum: 0, domainMidpoint: 10, domainMaximum: 30 })
  expect(diverging.features.map((feature) => feature.properties?.__ld_weight)).toEqual([0.5, 0.75, 1, 0])
})

test('MapLibre categorical scales assign deterministic colors by category', () => {
  const layer = {
    id: 'stores', kind: 'point', latitude: { dataset: 'primary', field: 'lat' }, longitude: { dataset: 'primary', field: 'lon' },
    category: { dataset: 'primary', field: 'category' }, color: { kind: 'categorical', palette: 'teal', reverse: false, nullColor: '#ccc' },
    tooltip: [], position: 'above_labels', visibility: { minimumZoom: 0, maximumZoom: 24 },
    size: { minimumRadius: 4, maximumRadius: 18 }, stroke: { color: '#fff', width: 1, opacity: 1 },
    cluster: { enabled: false, radius: 40, maximumZoom: 14, minimumPoints: 2, showCount: true }, opacity: .8,
  } as VisualizationGeographicLayer
  const envelope = {
    ...selectableEnvelope(),
    dataState: { kind: 'inline', datasets: [{ id: 'primary', columns: ['lat', 'lon', 'category'], rows: [[1, 1, 'B'], [2, 2, 'A'], [3, 3, 'B']] }] },
  } as VisualizationEnvelope
  const decorated = applyFeatureScales(coordinateGeometry(envelope, layer), layer)
  const colors = decorated.features.map((feature) => feature.properties?.__ld_color)
  expect(colors[0]).toBe(colors[2])
  expect(colors[0]).not.toBe(colors[1])
  expect(mapLayer('ld-stores', layer).paint['circle-color']).toEqual(['coalesce', ['get', '__ld_color'], '#ccc'])
})

test('MapLibre tooltips use compiled fields and contractual formatting without exposing other columns', () => {
  const envelope = selectableEnvelope()
  const entries = mapTooltipEntries(envelope, [{ layer: { id: 'ld-states' }, properties: { __ld_dataset: 'primary', __ld_row_index: 0, __ld_layer_id: 'states' } }])
  expect(entries).toEqual([{ label: 'State', value: 'SP' }, { label: 'Revenue', value: '10' }])
  expect(entries.some((entry) => entry.value.includes('governed'))).toBe(false)
})

test('MapLibre exposes a bounded formatted tabular equivalent without unrelated fields', () => {
  const data = mapAccessibleData(selectableEnvelope(), 1)
  expect(data.totalRows).toBe(2)
  expect(data.columns.map((column) => column.label)).toEqual(['State', 'Revenue'])
  expect(data.rows).toEqual([['SP', '10']])
  expect(data.columns.some((column) => column.id === 'customer_secret')).toBe(false)
})

test('MapLibre paths group and deterministically order valid coordinates', () => {
  const envelope = selectableEnvelope()
  const path = {
    id: 'route', kind: 'path', latitude: { dataset: 'primary', field: 'lat' }, longitude: { dataset: 'primary', field: 'lon' }, path: { dataset: 'primary', field: 'state' }, order: { dataset: 'primary', field: 'value' }, tooltip: [], position: 'below_labels', visibility: { minimumZoom: 0, maximumZoom: 24 }, color: { kind: 'sequential', palette: 'blue', reverse: false, nullColor: '#ccc' }, stroke: { color: '#0969da', width: 3, opacity: 1 }, line: { width: 3, curvature: 0 }, opacity: .8,
  } as VisualizationGeographicLayer
  const withCoordinates = { ...envelope, dataState: { ...envelope.dataState, datasets: [{ ...(envelope.dataState as any).datasets[0], columns: ['state', 'value', 'lat', 'lon'], rows: [['SP', 2, -20, -40], ['SP', 1, -21, -41], ['RJ', 1, null, -42]] }] } } as VisualizationEnvelope
  const result = pathGeometry(withCoordinates, path as Extract<VisualizationGeographicLayer, { kind: 'path' }>)
  expect(result.features).toHaveLength(1)
  expect(result.features[0]?.geometry).toEqual({ type: 'LineString', coordinates: [[-41, -21], [-40, -20]] })
})

test('MapLibre fits the combined valid feature extent with bounded padding and zoom', () => {
  const calls: unknown[][] = []
  const map = { fitBounds: (...args: unknown[]) => calls.push(args) }
  const data = {
    type: 'FeatureCollection',
    features: [
      { type: 'Feature', geometry: { type: 'Polygon', coordinates: [[[-74, -34], [-34, -34], [-34, 5], [-74, 5], [-74, -34]]] }, properties: {} },
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-46.63, -23.55] }, properties: {} },
    ],
  } as FeatureCollection

  expect(fitMapToGeographicData(map, [data])).toBe(true)
  expect(calls).toEqual([[[[-74, -34], [-34, 5]], { padding: 24, duration: 0, maxZoom: 10 }]])
  expect(fitMapToGeographicData(map, [{ type: 'FeatureCollection', features: [] }])).toBe(false)
})

test('MapLibre coordinate maps get a bounded geographic reference grid', () => {
  const data = {
    type: 'FeatureCollection',
    features: [
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-73.9, -33.7] }, properties: {} },
      { type: 'Feature', geometry: { type: 'Point', coordinates: [-35.1, 4.2] }, properties: {} },
    ],
  } as FeatureCollection

  const grid = coordinateReferenceGrid([data])
  expect(grid.features.length).toBeGreaterThanOrEqual(8)
  expect(grid.features.every((feature) => feature.geometry.type === 'LineString')).toBe(true)
  expect(grid.features.flatMap((feature) => feature.geometry.type === 'LineString' ? feature.geometry.coordinates : [])
    .every(([longitude, latitude]) => longitude! >= -180 && longitude! <= 180 && latitude! >= -90 && latitude! <= 90)).toBe(true)
  expect(coordinateReferenceGrid([{ type: 'FeatureCollection', features: [] }]).features).toEqual([])
})

test('MapLibre heat palettes increase monotonically from transparent to dark', () => {
  for (const kind of ['heat', 'density'] as const) {
    const layer = mapLayer('observations', kind)
    expect(layer.type).toBe('heatmap')
    expect(layer.paint['heatmap-color'].slice(0, 3)).toEqual(['interpolate', ['linear'], ['heatmap-density']])
    expect(layer.paint['heatmap-color'][3]).toBe(0)
    expect(layer.paint['heatmap-color'].at(-1)).toBe('#0550ae')
  }
})

test('MapLibre translates typed palettes and heat styling without renderer options', () => {
  const choropleth = {
    id: 'states', kind: 'choropleth', color: { kind: 'sequential', palette: 'teal', reverse: false, nullColor: '#ccc' },
    stroke: { color: '#fff', width: 2, opacity: .9 }, opacity: .7,
  } as VisualizationGeographicLayer
  const fill = mapLayer('states', choropleth)
  expect(JSON.stringify(fill.paint['fill-color'])).toContain('#006d77')
  expect(fill.paint['fill-opacity']).toContain(.7)

  const heat = {
    id: 'heat', kind: 'heat', color: { kind: 'sequential', palette: 'orange', reverse: false, nullColor: '#ccc' },
    heat: { radius: 18, intensity: 1.7 }, opacity: .65,
  } as VisualizationGeographicLayer
  const translated = mapLayer('heat', heat)
  expect(translated.paint['heatmap-radius']).toBe(18)
  expect(translated.paint['heatmap-intensity']).toBe(1.7)
  expect(translated.paint['heatmap-opacity']).toBe(.65)
  expect(translated.paint['heatmap-color']).toContain('#bc4c00')
})

test('MapLibre point and region styles strongly distinguish a current selection', () => {
  const point = mapLayer('customers', 'point')
  expect(point.paint['circle-radius'][2]).toBe(13)
  expect(point.paint['circle-opacity']).toContain(0.3)
  const region = mapLayer('states', 'choropleth')
  expect(region.paint['fill-opacity']).toContain(0.4)
  expect(mapOutlineLayer('states-selected', 'states')).toMatchObject({
    source: 'states', type: 'line', filter: ['==', ['get', '__ld_selected'], true],
    paint: { 'line-color': '#bf3989', 'line-width': 3 },
  })
})

test('MapLibre renders the typed basemap below data with theme-derived land and boundaries', () => {
  expect(concreteCSSColor('', '#afb8c1')).toBe('#afb8c1')
  expect(concreteCSSColor('rgb(1, 2, 3)', '#afb8c1')).toBe('rgb(1, 2, 3)')
  expect(basemapLayer('world', { boundary: 'rgb(175, 184, 193)', land: 'rgb(234, 238, 242)' })).toEqual({
    id: 'world',
    source: 'world',
    type: 'fill',
    paint: {
      'fill-color': 'rgb(234, 238, 242)',
      'fill-opacity': 1,
    },
  })
  expect(basemapBoundaryLayer('world-boundaries', 'world', 'rgb(175, 184, 193)')).toEqual({
    id: 'world-boundaries',
    source: 'world',
    type: 'line',
    paint: {
      'line-color': 'rgb(175, 184, 193)',
      'line-opacity': 0.92,
      'line-width': 1.5,
    },
  })
})

test('MapLibre auto basemaps follow the resolved application color scheme', () => {
  expect(mapThemeColors('auto', 'dark')).toEqual(mapThemeColors('dark', 'light'))
  expect(mapThemeColors('auto', 'light')).toEqual(mapThemeColors('light', 'dark'))
  expect(mapThemeColors('auto', 'dark')).not.toEqual(mapThemeColors('auto', 'light'))
})

function selectableEnvelope(): VisualizationEnvelope {
  return {
    schemaVersion: 2, visualID: 'state-map', rendererID: 'maplibre', specRevision: 'sha256:test', dataRevision: 4,
    spec: {
      kind: 'geographic', title: 'States', datasets: [{ id: 'primary', fields: [
        { id: 'state', role: 'identity', dataType: 'string', nullable: false, label: 'State' },
        { id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Revenue' },
        { id: 'customer_secret', role: 'dimension', dataType: 'string', nullable: true, label: 'Secret' },
      ] }],
      dataBudget: { maxRows: 100, requiredCompleteness: 'complete' }, accessibility: { title: 'States', description: 'States' },
      interactions: [{ id: 'point_selection', kind: 'select', mode: 'single', requiresStableIdentity: true, targets: ['detail'], mappings: [
        { source: { dataset: 'primary', field: 'state' }, targetFieldID: 'customers.state', targetFactID: 'customers' },
      ] }],
      layers: [{ id: 'states', kind: 'choropleth', geometry: {} as any, join: { dataset: 'primary', field: 'state' }, value: { dataset: 'primary', field: 'value' }, tooltip: [{ dataset: 'primary', field: 'state' }, { dataset: 'primary', field: 'value' }], position: 'below_labels', visibility: { minimumZoom: 0, maximumZoom: 24 }, color: { kind: 'sequential', palette: 'blue', reverse: false, nullColor: '#d0d7de' }, stroke: { color: '#fff', width: 1.5, opacity: 1 }, opacity: .82 }],
      presentation: { legend: 'hidden', showLabels: false, roam: false, theme: 'auto', labelDensity: 'normal', camera: { mode: 'fit_data', padding: 24, minimumZoom: 0, maximumZoom: 10 }, controls: { zoom: false, reset: false, compass: false } },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 4, generation: 1, datasets: [{
      id: 'primary', specRevision: 'sha256:test', dataRevision: 4, generation: 1, columns: ['state', 'value', 'customer_secret'], rows: [['SP', 10, 'governed-a'], ['RJ', 20, 'governed-b']], completeness: 'complete',
    }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
}
