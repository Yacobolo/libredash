import { expect, test } from 'bun:test'
import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import { sampleScreenPoints, spatialSelectionCommand, spatialSelectionGeometry } from './spatial-selection'

const unproject = ({ x, y }: { x: number; y: number }) => ({ longitude: x, latitude: y })

test('MapLibre constructs exact box, lasso, and radius geometries', () => {
  expect(spatialSelectionGeometry('box', [{ x: -50, y: -25 }, { x: -40, y: -15 }], unproject)).toEqual({
    kind: 'box', bounds: { west: -50, south: -25, east: -40, north: -15 },
  })
  expect(spatialSelectionGeometry('lasso', [{ x: -50, y: -25 }, { x: -40, y: -25 }, { x: -45, y: -15 }], unproject)).toEqual({
    kind: 'lasso', points: [{ longitude: -50, latitude: -25 }, { longitude: -40, latitude: -25 }, { longitude: -45, latitude: -15 }],
  })
  const radius = spatialSelectionGeometry('radius', [{ x: -46, y: -23 }, { x: -46, y: -22.9 }], unproject)
  expect(radius?.kind).toBe('radius')
  if (radius?.kind === 'radius') expect(radius.radiusMeters).toBeGreaterThan(11_000)
})

test('MapLibre rejects degenerate and unsafe spatial gestures and bounds lasso size', () => {
  expect(spatialSelectionGeometry('box', [{ x: 1, y: 1 }, { x: 1, y: 2 }], unproject)).toBeUndefined()
  expect(spatialSelectionGeometry('lasso', [{ x: 1, y: 1 }, { x: 2, y: 2 }, { x: 3, y: 3 }], unproject)).toBeUndefined()
  expect(spatialSelectionGeometry('radius', [{ x: 0, y: 0 }, { x: 90, y: 0 }], unproject)).toBeUndefined()
  expect(sampleScreenPoints(Array.from({ length: 1000 }, (_, x) => ({ x, y: x }))).length).toBe(256)
})

test('MapLibre spatial commands carry immutable visual revisions and clear without geometry', () => {
  const envelope = { visualID: 'customers', specRevision: 'sha256:spec', dataRevision: 9 } as VisualizationEnvelope
  const geometry = { kind: 'box', bounds: { west: -50, south: -25, east: -40, north: -15 } } as const
  expect(spatialSelectionCommand(envelope, 'spatial_selection', 'box', geometry)).toEqual({
    visualID: 'customers', specRevision: 'sha256:spec', dataRevision: 9, interactionID: 'spatial_selection', action: 'set', gesture: 'box', geometry,
  })
  expect(spatialSelectionCommand(envelope, 'spatial_selection', 'box')).toEqual({
    visualID: 'customers', specRevision: 'sha256:spec', dataRevision: 9, interactionID: 'spatial_selection', action: 'clear', gesture: 'box',
  })
})
