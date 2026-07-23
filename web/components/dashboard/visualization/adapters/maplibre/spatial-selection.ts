import type {
  VisualizationEnvelope,
  VisualizationSpatialCoordinate,
  VisualizationSpatialSelectionGeometry,
  VisualizationSpatialSelectionGesture,
  VisualizationSpatialSelectionCommand,
} from '../../../../../generated/visualization'

export type ScreenPoint = Readonly<{ x: number; y: number }>
export type Unproject = (point: ScreenPoint) => VisualizationSpatialCoordinate

const maximumLassoPoints = 256
const earthMeanRadiusMeters = 6_371_008.8

export function spatialSelectionCommand(
  envelope: VisualizationEnvelope,
  interactionID: string,
  gesture: VisualizationSpatialSelectionGesture,
  geometry?: VisualizationSpatialSelectionGeometry,
): VisualizationSpatialSelectionCommand {
  return {
    visualID: envelope.visualID,
    specRevision: envelope.specRevision,
    dataRevision: envelope.dataRevision,
    interactionID,
    action: geometry ? 'set' : 'clear',
    gesture,
    ...(geometry ? { geometry } : {}),
  }
}

export function spatialSelectionGeometry(
  gesture: VisualizationSpatialSelectionGesture,
  points: readonly ScreenPoint[],
  unproject: Unproject,
): VisualizationSpatialSelectionGeometry | undefined {
  if (points.length < 2) return undefined
  if (gesture === 'box') {
    const start = validCoordinate(unproject(points[0]))
    const end = validCoordinate(unproject(points.at(-1)!))
    if (!start || !end) return undefined
    const south = Math.min(start.latitude, end.latitude)
    const north = Math.max(start.latitude, end.latitude)
    if (south === north || start.longitude === end.longitude) return undefined
    return { kind: 'box', bounds: { west: start.longitude, south, east: end.longitude, north } }
  }
  if (gesture === 'radius') {
    const center = validCoordinate(unproject(points[0]))
    const edge = validCoordinate(unproject(points.at(-1)!))
    if (!center || !edge) return undefined
    const radiusMeters = haversineMeters(center, edge)
    if (!Number.isFinite(radiusMeters) || radiusMeters <= 0 || radiusMeters > 5_000_000) return undefined
    return { kind: 'radius', center, radiusMeters }
  }
  const sampled = sampleScreenPoints(points, maximumLassoPoints)
  const coordinates = sampled.map((point) => validCoordinate(unproject(point)))
  if (coordinates.some((point) => !point)) return undefined
  const polygon = coordinates as VisualizationSpatialCoordinate[]
  if (polygon.length < 3 || !hasArea(polygon)) return undefined
  const longitudes = polygon.map((point) => point.longitude)
  if (Math.max(...longitudes) - Math.min(...longitudes) >= 180) return undefined
  return { kind: 'lasso', points: polygon }
}

export function sampleScreenPoints(points: readonly ScreenPoint[], maximum = maximumLassoPoints): ScreenPoint[] {
  if (points.length <= maximum) return points.map(({ x, y }) => ({ x, y }))
  const result: ScreenPoint[] = []
  for (let index = 0; index < maximum; index++) {
    const source = Math.round(index * (points.length - 1) / (maximum - 1))
    result.push({ x: points[source].x, y: points[source].y })
  }
  return result
}

function validCoordinate(value: VisualizationSpatialCoordinate): VisualizationSpatialCoordinate | undefined {
  if (!Number.isFinite(value.longitude) || !Number.isFinite(value.latitude)) return undefined
  if (value.longitude < -180 || value.longitude > 180 || value.latitude < -90 || value.latitude > 90) return undefined
  return { longitude: value.longitude, latitude: value.latitude }
}

function haversineMeters(start: VisualizationSpatialCoordinate, end: VisualizationSpatialCoordinate): number {
  const radians = (value: number) => value * Math.PI / 180
  const latitudeDelta = radians(end.latitude - start.latitude)
  const longitudeDelta = radians(end.longitude - start.longitude)
  const a = Math.sin(latitudeDelta / 2) ** 2
    + Math.cos(radians(start.latitude)) * Math.cos(radians(end.latitude)) * Math.sin(longitudeDelta / 2) ** 2
  return 2 * earthMeanRadiusMeters * Math.asin(Math.sqrt(a))
}

function hasArea(points: readonly VisualizationSpatialCoordinate[]): boolean {
  let twiceArea = 0
  for (let index = 0; index < points.length; index++) {
    const current = points[index]
    const next = points[(index + 1) % points.length]
    twiceArea += current.longitude * next.latitude - next.longitude * current.latitude
  }
  return Math.abs(twiceArea) > 1e-12
}
