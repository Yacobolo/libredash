import type { VisualizationEnvelope, VisualizationSpatialBounds } from '../../../../../generated/visualization'

export type MapSpatialWindowRequest = {
  visualID: string; specRevision: string; dataRevision: number; requestSeq: number; resetVersion: number
  bounds: VisualizationSpatialBounds; zoom: number; width: number; height: number; windowID: string
}

const maximumViewportDimension = 16_384

export function spatialWindowRequest(
  envelope: VisualizationEnvelope,
  bounds: VisualizationSpatialBounds,
  zoom: number,
  width: number,
  height: number,
  requestSeq: number,
): MapSpatialWindowRequest | undefined {
  if (envelope.dataState.kind !== 'spatial_windowed' || !envelope.dataState.window) return undefined
  if (!Number.isInteger(requestSeq) || requestSeq <= 0 || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0 || !Number.isFinite(zoom)) return undefined
  const normalizedBounds = normalizeSpatialBounds(bounds)
  if (!normalizedBounds) return undefined
  width = Math.min(maximumViewportDimension, Math.max(1, Math.round(width)))
  height = Math.min(maximumViewportDimension, Math.max(1, Math.round(height)))
  const normalizedZoom = Math.max(0, Math.min(24, zoom))
  const windowID = `${normalizedBounds.west.toFixed(6)},${normalizedBounds.south.toFixed(6)},${normalizedBounds.east.toFixed(6)},${normalizedBounds.north.toFixed(6)}@${normalizedZoom.toFixed(3)}:${width}x${height}`
  return {
    visualID: envelope.visualID,
    specRevision: envelope.specRevision,
    dataRevision: envelope.dataRevision,
    requestSeq,
    resetVersion: envelope.dataState.resetVersion,
    bounds: normalizedBounds,
    zoom: normalizedZoom,
    width,
    height,
    windowID,
  }
}

export function nextSpatialRequestSequence(envelope: VisualizationEnvelope, localSequence: number): number {
  const serverSequence = envelope.dataState.kind === 'spatial_windowed' ? envelope.dataState.window?.requestSeq ?? 0 : 0
  return Math.max(0, localSequence, serverSequence) + 1
}

export function spatialWindowAlreadyCurrent(envelope: VisualizationEnvelope, request: MapSpatialWindowRequest): boolean {
  if (envelope.dataState.kind !== 'spatial_windowed' || !envelope.dataState.window) return false
  return envelope.dataState.resetVersion === request.resetVersion && envelope.dataState.window.id === request.windowID
}

function normalizeSpatialBounds(bounds: VisualizationSpatialBounds): VisualizationSpatialBounds | undefined {
  const values = [bounds.west, bounds.south, bounds.east, bounds.north]
  if (!values.every(Number.isFinite) || bounds.south < -90 || bounds.south > 90 || bounds.north < -90 || bounds.north > 90 || bounds.south >= bounds.north) return undefined
  if (bounds.west >= -180 && bounds.west <= 180 && bounds.east >= -180 && bounds.east <= 180) {
    return bounds.west === bounds.east ? undefined : { ...bounds }
  }
  const span = bounds.east - bounds.west
  if (span <= 0) return undefined
  if (span >= 360) return { west: -180, south: bounds.south, east: 180, north: bounds.north }
  const west = wrapLongitude(bounds.west)
  const east = wrapLongitude(bounds.east)
  if (west === east) return undefined
  return { west, south: bounds.south, east, north: bounds.north }
}

function wrapLongitude(value: number): number {
  const wrapped = ((value + 180) % 360 + 360) % 360 - 180
  return Object.is(wrapped, -0) ? 0 : wrapped
}
