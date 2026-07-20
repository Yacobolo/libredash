import { expect, test } from 'bun:test'

import type { VisualizationEnvelope, VisualizationGeographicLayer } from '../../../../generated/visualization'
import type { FeatureCollection } from 'geojson'
import { basemapBoundaryLayer, basemapLayer, concreteCSSColor, coordinateGeometry, coordinateReferenceGrid, fitMapToGeographicData, interactionCommandForRenderedFeatures, joinGeometry, mapInteractionCommand, mapLayer, mapOutlineLayer, mapPointerOptions, normalizeFeatureWeights, removeRendererFrame, sameOriginGeometryURL, updateSelectionSources, verifyGeometryDigest } from './maplibre'

test('MapLibre geometry assets are same-origin and content addressed', async () => {
  expect(sameOriginGeometryURL('/static/geometry/states.geojson', 'https://dash.example/workspaces/sales').href).toBe('https://dash.example/static/geometry/states.geojson')
  expect(() => sameOriginGeometryURL('https://attacker.example/states.geojson', 'https://dash.example/workspaces/sales')).toThrow(/same-origin/)
  await expect(verifyGeometryDigest(new TextEncoder().encode('geometry'), 'sha256:invalid')).rejects.toThrow(/canonical SHA-256/)
  await expect(verifyGeometryDigest(new TextEncoder().encode('geometry'), `sha256:${'0'.repeat(64)}`)).rejects.toThrow(/digest mismatch/)
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
    expect(layer.paint['heatmap-color']).toEqual([
      'interpolate', ['linear'], ['heatmap-density'],
      0, 'rgba(9,105,218,0)',
      0.15, 'rgba(84,174,255,0.28)',
      0.35, 'rgba(84,174,255,0.62)',
      0.6, '#0969da',
      0.85, '#0550ae',
      1, '#033d8b',
    ])
  }
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

function selectableEnvelope(): VisualizationEnvelope {
  return {
    schemaVersion: 1, visualID: 'state-map', rendererID: 'maplibre', specRevision: 'sha256:test', dataRevision: 4,
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
      layers: [{ id: 'states', kind: 'choropleth', geometry: {} as any, join: { dataset: 'primary', field: 'state' }, value: { dataset: 'primary', field: 'value' } }],
      presentation: { legend: 'hidden', showLabels: false, roam: false },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 4, generation: 1, datasets: [{
      id: 'primary', specRevision: 'sha256:test', dataRevision: 4, generation: 1, columns: ['state', 'value', 'customer_secret'], rows: [['SP', 10, 'governed-a'], ['RJ', 20, 'governed-b']], completeness: 'complete',
    }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope
}
