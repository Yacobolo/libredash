import type { VisualizationEnvelope, VisualizationSpatialBounds } from '../../../../../generated/visualization'

export type MapSpatialWindowRequest = {
  visualID: string; specRevision: string; dataRevision: number; requestSeq: number; resetVersion: number
  bounds: VisualizationSpatialBounds; zoom: number; width: number; height: number; windowID: string
}

export function spatialWindowRequest(
  envelope: VisualizationEnvelope,
  bounds: VisualizationSpatialBounds,
  zoom: number,
  width: number,
  height: number,
  requestSeq: number,
): MapSpatialWindowRequest | undefined {
  if (envelope.dataState.kind !== 'spatial_windowed') return undefined
  const values = [bounds.west, bounds.south, bounds.east, bounds.north, zoom]
  if (!values.every(Number.isFinite) || bounds.west < -180 || bounds.west > 180 || bounds.east < -180 || bounds.east > 180 || bounds.south < -90 || bounds.south > 90 || bounds.north < -90 || bounds.north > 90 || bounds.south > bounds.north) return undefined
  width = Math.max(1, Math.round(width)); height = Math.max(1, Math.round(height))
  const normalizedZoom = Math.max(0, Math.min(24, zoom))
  const windowID = `${bounds.west.toFixed(6)},${bounds.south.toFixed(6)},${bounds.east.toFixed(6)},${bounds.north.toFixed(6)}@${normalizedZoom.toFixed(3)}:${width}x${height}`
  return {
    visualID: envelope.visualID,
    specRevision: envelope.specRevision,
    dataRevision: envelope.dataRevision,
    requestSeq,
    resetVersion: envelope.dataState.resetVersion,
    bounds,
    zoom: normalizedZoom,
    width,
    height,
    windowID,
  }
}
