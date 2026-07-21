import type { FeatureCollection } from 'geojson'
import type { GeoJSONSource } from 'maplibre-gl'
import type { VisualizationEnvelope, VisualizationGeographicLayer } from '../../../../../generated/visualization'
import type { OptimisticInteractionCommand } from '../../../interaction-selection'
import { clearInteractionCommand, interactionCommandForRowIndex } from '../../interaction-command'
import { coordinateGeometry, joinGeometry, pathGeometry } from './data'
import { applyFeatureScales } from './layers'
import type { RenderedFeatureLocator } from './overlays'

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
      : layer.spec.kind === 'path'
        ? pathGeometry(envelope, layer.spec)
        : layer.spec.kind === 'reference' && layer.geometry
          ? layer.geometry
          : coordinateGeometry(envelope, layer.spec)
    const source = getSource(layer.sourceID)
    if (!source) continue
    source.setData(applyFeatureScales(data, layer.spec))
    updated++
  }
  return updated
}
