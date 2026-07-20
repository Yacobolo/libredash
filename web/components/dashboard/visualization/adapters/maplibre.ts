import type { VisualizationEnvelope, VisualizationGeographicLayer, VisualizationGeometryAsset } from '../../../../generated/visualization'
import { Map as MapLibre, type GeoJSONSource, type Map as MapLibreMap, type MapMouseEvent, type MapOptions } from 'maplibre-gl'
import type { Feature, FeatureCollection, Geometry, Position } from 'geojson'
import type { OptimisticInteractionCommand } from '../../interaction-selection'
import { Change, type RendererAdapter, type RendererHandle } from '../host-controller'
import { clearInteractionCommand, interactionCommandForRowIndex } from '../interaction-command'
import { MapSelectionControl } from './map-selection-control'

export const adapter: RendererAdapter = {
  async mount(container, envelope) {
    const frame = document.createElement('div'); frame.style.cssText = 'position:relative;width:100%;height:100%;overflow:hidden;background:var(--ld-chart-surface,var(--ld-bg-panel,#fff))'
    const surface = document.createElement('div'); surface.style.cssText = 'position:absolute;inset:0'
    const attribution = document.createElement('div'); attribution.dataset.mapAttribution = ''; attribution.setAttribute('role', 'note'); attribution.setAttribute('aria-label', 'Map attribution')
    attribution.style.cssText = 'position:absolute;right:6px;bottom:6px;z-index:1;max-width:calc(100% - 12px);padding:2px 5px;border-radius:4px;background:color-mix(in srgb,var(--ld-bg-panel,#fff) 88%,transparent);color:var(--ld-fg-muted,#57606a);font:10px/1.3 var(--ld-font-family-ui,system-ui);pointer-events:none;text-align:right'
    frame.append(surface, attribution); container.replaceChildren(frame)
    const pointerOptions = mapPointerOptions(envelope)
    const backgroundColor = getComputedStyle(frame).backgroundColor || '#f6f8fa'
    const map = new MapLibre({
      container: surface,
      style: { version: 8, sources: {}, layers: [{ id: '__ld-background', type: 'background', paint: { 'background-color': backgroundColor } }] },
      attributionControl: false,
      canvasContextAttributes: { preserveDrawingBuffer: true },
      ...pointerOptions,
    })
    await new Promise<void>((resolve, reject) => { map.once('load', () => resolve()); map.once('error', (event) => reject(event.error)) })
    const handle = new MapLibreHandle(container, frame, map, attribution)
    try {
      await handle.update(envelope)
      return handle
    } catch (error) {
      handle.dispose()
      throw error
    }
  },
}

export function mapPointerOptions(envelope: VisualizationEnvelope): Pick<MapOptions, 'interactive' | 'scrollZoom' | 'boxZoom' | 'dragRotate' | 'dragPan' | 'keyboard' | 'doubleClickZoom' | 'touchZoomRotate' | 'touchPitch'> {
  const geographic = envelope.spec.kind === 'geographic'
  const roam = envelope.spec.kind === 'geographic' ? envelope.spec.presentation.roam : false
  const selectable = geographic && envelope.spec.interactions.some((candidate) => candidate.kind === 'select')
  return {
    interactive: roam || selectable,
    scrollZoom: roam,
    boxZoom: roam,
    dragRotate: roam,
    dragPan: roam,
    keyboard: roam,
    doubleClickZoom: roam,
    touchZoomRotate: roam,
    touchPitch: roam,
  }
}

class MapLibreHandle implements RendererHandle {
  private sourceIDs: string[] = []
  private layerIDs: string[] = []
  private dynamicLayers: Array<{ spec: VisualizationGeographicLayer; sourceID: string; geometry?: FeatureCollection }> = []
  private selectableLayerIDs: string[] = []
  private basemapIDs?: Readonly<{ fill: string; boundary: string }>
  private envelope?: VisualizationEnvelope
  private selectionControl?: MapSelectionControl
  private updateQueue: Promise<void> = Promise.resolve()
  private disposed = false
  private readonly handleThemeApplied = () => this.applyTheme()
  constructor(private readonly container: HTMLElement, private readonly frame: HTMLElement, private readonly map: MapLibreMap, private readonly attribution: HTMLElement) {
    document.addEventListener('libredash-theme-applied', this.handleThemeApplied)
    this.map.on('click', this.handleClick)
    this.map.on('mousemove', this.handlePointerMove)
    this.map.on('mouseout', this.handlePointerLeave)
  }
  update(envelope: VisualizationEnvelope, change: Change = Change.All): Promise<void> {
    if (this.disposed) return Promise.resolve()
    const pending = this.updateQueue.then(() => this.applyUpdate(envelope, change))
    this.updateQueue = pending.catch(() => {})
    return pending
  }
  private async applyUpdate(envelope: VisualizationEnvelope, change: Change): Promise<void> {
    if (this.disposed) return
    if (envelope.spec.kind !== 'geographic') throw new Error(`MapLibre cannot render ${envelope.spec.kind}`)
    this.envelope = envelope
    this.updateSelectionControl(envelope)
    if ((change & (Change.Spec | Change.Data)) === 0) {
      if ((change & Change.Selection) !== 0) this.updateSelectionData(envelope)
      return
    }
    this.removeOwnedMapData()
    this.sourceIDs = []
    this.layerIDs = []
    this.dynamicLayers = []
    this.selectableLayerIDs = []
    this.basemapIDs = undefined
    const collections: FeatureCollection[] = []
    const coordinateCollections: FeatureCollection[] = []
    const attributions = new Set<string>()
    if (envelope.spec.presentation.basemap) {
      await this.addBasemap(envelope.spec.presentation.basemap)
      if (this.disposed) return
      attributions.add(envelope.spec.presentation.basemap.attribution)
    }
    for (const layer of envelope.spec.layers) {
      const collection = await this.addLayer(envelope, layer)
      if (this.disposed) return
      collections.push(collection)
      if (layer.kind !== 'choropleth') coordinateCollections.push(collection)
      if (layer.geometry?.attribution) attributions.add(layer.geometry.attribution)
    }
    if (!envelope.spec.presentation.basemap) this.addCoordinateReferenceGrid(coordinateCollections)
    this.attribution.textContent = [...attributions].join(' · ')
    this.attribution.hidden = attributions.size === 0
    fitMapToGeographicData(this.map, collections)
    if (this.disposed) return
    await waitForMapIdle(this.map)
  }
  resize(): void { this.map.resize() }
  async snapshot(): Promise<Blob> {
    await waitForMapIdle(this.map)
    const canvas = this.map.getCanvas()
    return new Promise((resolve, reject) => canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error('MapLibre snapshot failed')), 'image/png'))
  }
  dispose(): void {
    if (this.disposed) return
    this.disposed = true
    document.removeEventListener('libredash-theme-applied', this.handleThemeApplied)
    this.map.off('click', this.handleClick)
    this.map.off('mousemove', this.handlePointerMove)
    this.map.off('mouseout', this.handlePointerLeave)
    this.selectionControl?.dispose()
    this.map.remove()
    removeRendererFrame(this.container, this.frame)
  }

  private async addBasemap(asset: VisualizationGeometryAsset): Promise<void> {
    const data = await this.loadGeometry(asset)
    if (this.disposed) return
    let id = '__ld-basemap'
    while (this.map.getSource(id) || this.map.getLayer(id)) id += '-'
    const boundaryID = `${id}-boundaries`
    const colors = this.currentBasemapColors()
    this.map.addSource(id, { type: 'geojson', data })
    this.map.addLayer(basemapLayer(id, colors))
    this.map.addLayer(basemapBoundaryLayer(boundaryID, id, colors.boundary))
    this.sourceIDs.push(id)
    this.layerIDs.push(id, boundaryID)
    this.basemapIDs = { fill: id, boundary: boundaryID }
  }

  private addCoordinateReferenceGrid(collections: FeatureCollection[]): void {
    const data = coordinateReferenceGrid(collections)
    if (data.features.length === 0) return
    let id = '__ld-coordinate-reference'
    while (this.map.getSource(id) || this.map.getLayer(id)) id += '-'
    this.map.addSource(id, { type: 'geojson', data })
    this.map.addLayer({
      id,
      source: id,
      type: 'line',
      paint: { 'line-color': '#8c959f', 'line-opacity': 0.22, 'line-width': 1, 'line-dasharray': [2, 3] },
    }, this.sourceIDs[0])
    this.sourceIDs.push(id)
    this.layerIDs.push(id)
  }

  private async addLayer(envelope: VisualizationEnvelope, layer: VisualizationGeographicLayer): Promise<FeatureCollection> {
    let data: FeatureCollection
    let geometry: FeatureCollection | undefined
    if (layer.kind === 'choropleth') {
      if (!layer.geometry || !layer.join) throw new Error(`choropleth layer ${JSON.stringify(layer.id)} requires geometry and join`)
      geometry = await this.loadGeometry(layer.geometry)
      if (this.disposed) return { type: 'FeatureCollection', features: [] }
      data = joinGeometry(envelope, layer, geometry)
    } else {
      data = coordinateGeometry(envelope, layer)
    }
    data = normalizeFeatureWeights(data)
    const id = `ld-${layer.id}`
    this.map.addSource(id, { type: 'geojson', data })
    this.map.addLayer(mapLayer(id, layer.kind))
    this.sourceIDs.push(id)
    this.layerIDs.push(id)
    if (layer.kind === 'choropleth') {
      const outlineID = `${id}-selected-outline`
      this.map.addLayer(mapOutlineLayer(outlineID, id))
      this.layerIDs.push(outlineID)
    }
    if (layer.kind === 'point' || layer.kind === 'choropleth') this.selectableLayerIDs.push(id)
    this.dynamicLayers.push({ spec: layer, sourceID: id, geometry })
    return data
  }

  private updateSelectionData(envelope: VisualizationEnvelope): void {
    updateSelectionSources(envelope, this.dynamicLayers, (sourceID) => this.map.getSource(sourceID) as GeoJSONSource | undefined)
    this.map.triggerRepaint()
  }

  private removeOwnedMapData(): void {
    for (const id of [...this.layerIDs].reverse()) if (this.map.getLayer(id)) this.map.removeLayer(id)
    for (const id of [...this.sourceIDs].reverse()) if (this.map.getSource(id)) this.map.removeSource(id)
  }

  private updateSelectionControl(envelope: VisualizationEnvelope): void {
    const selectable = envelope.spec.interactions.some((candidate) => candidate.kind === 'select')
    if (!selectable) {
      this.selectionControl?.dispose()
      this.selectionControl = undefined
      return
    }
    this.selectionControl ??= new MapSelectionControl((command) => this.dispatchInteraction(command))
    if (!this.selectionControl.element.isConnected) this.frame.append(this.selectionControl.element)
    this.selectionControl.update(envelope)
  }

  private dispatchInteraction(command: OptimisticInteractionCommand): void {
    this.container.dispatchEvent(new CustomEvent('ld-interaction-select', { bubbles: true, composed: true, detail: command }))
  }

  private readonly handleClick = (event: MapMouseEvent) => {
    if (!this.envelope || this.selectableLayerIDs.length === 0) return
    const features = this.map.queryRenderedFeatures(event.point, { layers: this.selectableLayerIDs })
    const command = mapInteractionCommand(this.envelope, features, this.selectableLayerIDs)
    if (command) this.dispatchInteraction(command)
  }

  private readonly handlePointerMove = (event: MapMouseEvent) => {
    if (!this.envelope || this.selectableLayerIDs.length === 0) return
    const features = this.map.queryRenderedFeatures(event.point, { layers: this.selectableLayerIDs })
    this.map.getCanvas().style.cursor = interactionCommandForRenderedFeatures(this.envelope, features, this.selectableLayerIDs) ? 'pointer' : ''
  }

  private readonly handlePointerLeave = () => { this.map.getCanvas().style.cursor = '' }

  private async loadGeometry(asset: VisualizationGeometryAsset): Promise<FeatureCollection> {
    return loadGeometryAsset(asset, location.href)
  }

  private applyTheme(): void {
    this.map.setPaintProperty('__ld-background', 'background-color', getComputedStyle(this.frame).backgroundColor || '#ffffff')
    if (this.basemapIDs && this.map.getLayer(this.basemapIDs.fill) && this.map.getLayer(this.basemapIDs.boundary)) {
      const colors = this.currentBasemapColors()
      this.map.setPaintProperty(this.basemapIDs.fill, 'fill-color', colors.land)
      this.map.setPaintProperty(this.basemapIDs.boundary, 'line-color', colors.boundary)
    }
    this.map.triggerRepaint()
  }

  private currentBasemapColors(): BasemapColors {
    return {
      boundary: resolveCSSColor(this.frame, 'var(--ld-line-emphasis,#8c959f)', '#8c959f'),
      land: resolveCSSColor(this.frame, 'var(--ld-bg-control-hover,#eaeef2)', '#eaeef2'),
    }
  }
}

const geometryCache = new Map<string, Promise<FeatureCollection>>()

async function loadGeometryAsset(asset: VisualizationGeometryAsset, baseURL: string): Promise<FeatureCollection> {
  const url = sameOriginGeometryURL(asset.url, baseURL)
  const key = `${url.href}\0${asset.digest}`
  let pending = geometryCache.get(key)
  if (!pending) {
    pending = (async () => {
      const response = await fetch(url, { credentials: 'same-origin', redirect: 'error' })
      if (!response.ok) throw new Error(`geometry asset ${JSON.stringify(asset.id)} returned ${response.status}`)
      const bytes = new Uint8Array(await response.arrayBuffer())
      await verifyGeometryDigest(bytes, asset.digest)
      const value = JSON.parse(new TextDecoder().decode(bytes)) as Partial<FeatureCollection>
      if (value.type !== 'FeatureCollection' || !Array.isArray(value.features)) throw new Error(`geometry asset ${JSON.stringify(asset.id)} is not a GeoJSON FeatureCollection`)
      return value as FeatureCollection
    })()
    geometryCache.set(key, pending)
    void pending.catch(() => { if (geometryCache.get(key) === pending) geometryCache.delete(key) })
  }
  return pending
}

type BasemapColors = Readonly<{ boundary: string; land: string }>

export function basemapLayer(id: string, colors: BasemapColors): any {
  return { id, source: id, type: 'fill', paint: { 'fill-color': colors.land, 'fill-opacity': 1 } }
}

export function basemapBoundaryLayer(id: string, source: string, boundary: string): any {
  return { id, source, type: 'line', paint: { 'line-color': boundary, 'line-opacity': 0.92, 'line-width': 1.5 } }
}

export function removeRendererFrame(container: ParentNode, frame: HTMLElement): void {
  if (frame.parentNode === container) frame.remove()
}

function resolveCSSColor(container: HTMLElement, value: string, fallback: string): string {
  const probe = document.createElement('span')
  probe.style.color = value
  probe.hidden = true
  container.append(probe)
  const color = getComputedStyle(probe).color
  probe.remove()
  return concreteCSSColor(color, fallback)
}

export function concreteCSSColor(resolved: string, fallback: string): string {
  return resolved.trim() || fallback
}

function waitForMapIdle(map: MapLibreMap): Promise<void> {
  return new Promise((resolve) => {
    map.once('idle', () => resolve())
    map.triggerRepaint()
  })
}

export function joinGeometry(envelope: VisualizationEnvelope, layer: VisualizationGeographicLayer, geometry: FeatureCollection): FeatureCollection {
  if (envelope.dataState.kind !== 'inline' || !layer.join) return geometry
  const join = layer.join
  const dataset = envelope.dataState.datasets.find((candidate) => candidate.id === join.dataset)
  if (!dataset) return geometry
  const joinIndex = dataset.columns.indexOf(join.field)
  const valueIndex = layer.value ? dataset.columns.indexOf(layer.value.field) : -1
  const values = new Map(dataset.rows.map((row, rowIndex) => [String(row[joinIndex]), {
    value: valueIndex >= 0 ? row[valueIndex] : 1,
    selected: rowIsSelected(envelope, dataset.id, dataset.columns, row),
    rowIndex,
  }]))
  const features: Feature<Geometry>[] = geometry.features.map((feature) => {
    const matched = values.get(String(feature.id ?? feature.properties?.id))
    return { ...feature, properties: {
      ...feature.properties,
      __ld_value: matched?.value ?? null,
      __ld_selected: matched?.selected ?? false,
      __ld_has_selection: envelope.selection.length > 0,
      ...(matched ? rowLocator(dataset.id, matched.rowIndex, layer.id) : {}),
    } }
  })
  return { ...geometry, features }
}

export function coordinateGeometry(envelope: VisualizationEnvelope, layer: VisualizationGeographicLayer): FeatureCollection {
  if (envelope.dataState.kind !== 'inline' || !layer.latitude || !layer.longitude) return { type: 'FeatureCollection', features: [] }
  const dataset = envelope.dataState.datasets.find((candidate) => candidate.id === layer.latitude?.dataset && candidate.id === layer.longitude?.dataset)
  if (!dataset) return { type: 'FeatureCollection', features: [] }
  const latitudeIndex = dataset.columns.indexOf(layer.latitude.field)
  const longitudeIndex = dataset.columns.indexOf(layer.longitude.field)
  const valueIndex = layer.value ? dataset.columns.indexOf(layer.value.field) : -1
  const features: Feature<Geometry>[] = []
  for (let index = 0; index < dataset.rows.length; index++) {
    const row = dataset.rows[index]!
    const latitude = row[latitudeIndex], longitude = row[longitudeIndex]
    if (typeof latitude !== 'number' || !Number.isFinite(latitude) || latitude < -90 || latitude > 90 || typeof longitude !== 'number' || !Number.isFinite(longitude) || longitude < -180 || longitude > 180) continue
    features.push({ type: 'Feature', id: index, geometry: { type: 'Point', coordinates: [longitude, latitude] }, properties: {
      __ld_value: valueIndex >= 0 ? row[valueIndex] : 1,
      __ld_selected: rowIsSelected(envelope, dataset.id, dataset.columns, row),
      __ld_has_selection: envelope.selection.length > 0,
      ...(layer.kind === 'point' ? rowLocator(dataset.id, index, layer.id) : {}),
    } })
  }
  return { type: 'FeatureCollection', features }
}

export function mapLayer(id: string, kind: VisualizationGeographicLayer['kind']): any {
  if (kind === 'choropleth') return { id, source: id, type: 'fill', paint: { 'fill-color': ['case', ['==', ['get', '__ld_value'], null], '#d8dee4', ['interpolate', ['linear'], ['get', '__ld_weight'], 0, '#ddf4ff', 0.5, '#54aeff', 1, '#0550ae']], 'fill-opacity': ['case', ['get', '__ld_selected'], 1, ['get', '__ld_has_selection'], 0.4, 0.82], 'fill-outline-color': '#ffffff' } }
  if (kind === 'point') return { id, source: id, type: 'circle', paint: { 'circle-radius': ['case', ['get', '__ld_selected'], 13, ['interpolate', ['linear'], ['get', '__ld_weight'], 0, 5, 1, 10]], 'circle-color': '#0969da', 'circle-stroke-color': '#ffffff', 'circle-stroke-width': ['case', ['get', '__ld_selected'], 2.5, 1.5], 'circle-opacity': ['case', ['get', '__ld_selected'], 1, ['get', '__ld_has_selection'], 0.3, 0.78] } }
  return { id, source: id, type: 'heatmap', paint: {
    'heatmap-weight': ['*', ['get', '__ld_weight'], ['case', ['get', '__ld_selected'], 1, 0.75]],
    'heatmap-intensity': kind === 'density' ? 1.35 : 1,
    'heatmap-radius': kind === 'density' ? 24 : 32,
    'heatmap-opacity': 0.86,
    'heatmap-color': ['interpolate', ['linear'], ['heatmap-density'], 0, 'rgba(9,105,218,0)', 0.15, 'rgba(84,174,255,0.28)', 0.35, 'rgba(84,174,255,0.62)', 0.6, '#0969da', 0.85, '#0550ae', 1, '#033d8b'],
  } }
}

export function mapOutlineLayer(id: string, source: string): any {
  return {
    id, source, type: 'line',
    filter: ['==', ['get', '__ld_selected'], true],
    paint: { 'line-color': '#bf3989', 'line-opacity': 1, 'line-width': 3 },
  }
}

function rowLocator(datasetID: string, rowIndex: number, layerID: string): Record<string, string | number> {
  return { __ld_dataset: datasetID, __ld_row_index: rowIndex, __ld_layer_id: layerID }
}

type RenderedFeatureLocator = Readonly<{ layer?: { id?: string }; properties?: Record<string, unknown> | null }>

export function interactionCommandForRenderedFeatures(
  envelope: VisualizationEnvelope,
  features: readonly RenderedFeatureLocator[],
  selectableLayerIDs: readonly string[],
) {
  const selectable = new Set(selectableLayerIDs)
  for (const feature of features) {
    const renderedLayerID = feature.layer?.id
    const datasetID = feature.properties?.__ld_dataset
    const rowIndex = feature.properties?.__ld_row_index
    const authoredLayerID = feature.properties?.__ld_layer_id
    if (typeof renderedLayerID !== 'string' || !selectable.has(renderedLayerID)) continue
    if (renderedLayerID !== `ld-${authoredLayerID}` || typeof datasetID !== 'string' || typeof rowIndex !== 'number') continue
    const command = interactionCommandForRowIndex(envelope, datasetID, rowIndex)
    if (command) return command
  }
  return undefined
}

export function mapInteractionCommand(
  envelope: VisualizationEnvelope,
  features: readonly RenderedFeatureLocator[],
  selectableLayerIDs: readonly string[],
): OptimisticInteractionCommand | undefined {
  return interactionCommandForRenderedFeatures(envelope, features, selectableLayerIDs)
    ?? (envelope.selection.length > 0 ? clearInteractionCommand(envelope) : undefined)
}

export function updateSelectionSources(
  envelope: VisualizationEnvelope,
  layers: readonly { spec: VisualizationGeographicLayer; sourceID: string; geometry?: FeatureCollection }[],
  getSource: (sourceID: string) => Pick<GeoJSONSource, 'setData'> | undefined,
): number {
  let updated = 0
  for (const layer of layers) {
    const data = layer.spec.kind === 'choropleth' && layer.geometry
      ? joinGeometry(envelope, layer.spec, layer.geometry)
      : coordinateGeometry(envelope, layer.spec)
    const source = getSource(layer.sourceID)
    if (!source) continue
    source.setData(normalizeFeatureWeights(data))
    updated++
  }
  return updated
}

export function normalizeFeatureWeights(data: FeatureCollection): FeatureCollection {
  const values = data.features.map((feature) => feature.properties?.__ld_value).filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
  const minimum = values.length > 0 ? Math.min(...values) : 0
  const maximum = values.length > 0 ? Math.max(...values) : 0
  const span = maximum - minimum
  return {
    ...data,
    features: data.features.map((feature) => {
      const value = feature.properties?.__ld_value
      const weight = typeof value !== 'number' || !Number.isFinite(value) ? 0 : span === 0 ? (value === 0 ? 0 : 1) : (value - minimum) / span
      return { ...feature, properties: { ...feature.properties, __ld_weight: weight } }
    }),
  }
}

type GeographicViewport = { fitBounds(bounds: [[number, number], [number, number]], options: { padding: number; duration: number; maxZoom: number }): unknown }

export function fitMapToGeographicData(map: GeographicViewport, collections: FeatureCollection[]): boolean {
  const extent = geographicExtent(collections)
  if (!extent) return false
  let [[west, south], [east, north]] = extent
  if (west === east) { west -= 0.01; east += 0.01 }
  if (south === north) { south -= 0.01; north += 0.01 }
  map.fitBounds([[west, south], [east, north]], { padding: 24, duration: 0, maxZoom: 10 })
  return true
}

export function coordinateReferenceGrid(collections: FeatureCollection[]): FeatureCollection {
  const extent = geographicExtent(collections)
  if (!extent) return { type: 'FeatureCollection', features: [] }
  const [[west, south], [east, north]] = extent
  const longitudeStep = referenceGridStep(east - west)
  const latitudeStep = referenceGridStep(north - south)
  const features: Feature<Geometry>[] = []
  for (let longitude = Math.ceil(west / longitudeStep) * longitudeStep; longitude <= east; longitude += longitudeStep) {
    features.push({ type: 'Feature', geometry: { type: 'LineString', coordinates: [[longitude, south], [longitude, north]] }, properties: {} })
  }
  for (let latitude = Math.ceil(south / latitudeStep) * latitudeStep; latitude <= north; latitude += latitudeStep) {
    features.push({ type: 'Feature', geometry: { type: 'LineString', coordinates: [[west, latitude], [east, latitude]] }, properties: {} })
  }
  return { type: 'FeatureCollection', features }
}

function referenceGridStep(span: number): number {
  const target = Math.max(span / 5, 0.0001)
  const magnitude = 10 ** Math.floor(Math.log10(target))
  const normalized = target / magnitude
  const interval = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return interval * magnitude
}

function geographicExtent(collections: FeatureCollection[]): [[number, number], [number, number]] | undefined {
  let west = Infinity, south = Infinity, east = -Infinity, north = -Infinity
  const include = (position: Position) => {
    const [longitude, latitude] = position
    if (typeof longitude !== 'number' || !Number.isFinite(longitude) || longitude < -180 || longitude > 180 || typeof latitude !== 'number' || !Number.isFinite(latitude) || latitude < -90 || latitude > 90) return
    west = Math.min(west, longitude); east = Math.max(east, longitude); south = Math.min(south, latitude); north = Math.max(north, latitude)
  }
  const coordinates = (value: unknown): void => {
    if (!Array.isArray(value)) return
    if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') { include(value as Position); return }
    for (const child of value) coordinates(child)
  }
  const geometry = (value: Geometry | null): void => {
    if (!value) return
    if (value.type === 'GeometryCollection') { for (const child of value.geometries) geometry(child); return }
    coordinates(value.coordinates)
  }
  for (const collection of collections) for (const feature of collection.features) geometry(feature.geometry)
  return [west, south, east, north].every(Number.isFinite) ? [[west, south], [east, north]] : undefined
}

function rowIsSelected(envelope: VisualizationEnvelope, datasetID: string, columns: string[], row: unknown[]): boolean {
  if (envelope.selection.length === 0) return false
  return envelope.selection.some(({ datum }) => {
    if (datum.dataset !== datasetID || datum.dataRevision !== envelope.dataRevision) return false
    return Object.entries(datum.identity).every(([field, value]) => {
      const index = columns.indexOf(field)
      return index >= 0 && Object.is(row[index], value)
    })
  })
}

export function sameOriginGeometryURL(value: string, base: string): URL {
  const url = new URL(value, base)
  if (url.origin !== new URL(base).origin) throw new Error('geometry asset must be same-origin')
  return url
}

export async function verifyGeometryDigest(bytes: Uint8Array, declared: string): Promise<void> {
  if (!/^sha256:[0-9a-f]{64}$/.test(declared)) throw new Error('geometry asset digest must be canonical SHA-256')
  const input = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', input))
  const actual = `sha256:${Array.from(digest, (value) => value.toString(16).padStart(2, '0')).join('')}`
  if (actual !== declared) throw new Error(`geometry asset digest mismatch: got ${actual}`)
}
