import type { Map as MapLibreMap } from 'maplibre-gl'
import type { VisualizationEnvelope } from '../../../../../generated/visualization'

export type MapObservationStage = 'basemap_load' | 'layer_shape' | 'webgl_context_loss' | 'webgl_context_restored'

export function installWebGLRecovery(
  canvas: EventTarget,
  map: Pick<MapLibreMap, 'resize' | 'triggerRepaint'>,
  observe: (stage: Extract<MapObservationStage, 'webgl_context_loss' | 'webgl_context_restored'>) => void = () => {},
): () => void {
  const lost = (event: Event) => {
    event.preventDefault()
    observe('webgl_context_loss')
  }
  const restored = () => {
    map.resize()
    map.triggerRepaint()
    observe('webgl_context_restored')
  }
  canvas.addEventListener('webglcontextlost', lost)
  canvas.addEventListener('webglcontextrestored', restored)
  return () => {
    canvas.removeEventListener('webglcontextlost', lost)
    canvas.removeEventListener('webglcontextrestored', restored)
  }
}

export function emitMapObservation(
  target: EventTarget,
  stage: MapObservationStage,
  durationMs: number,
  envelope: VisualizationEnvelope,
  detail: Readonly<Record<string, string | number>> = {},
): void {
  target.dispatchEvent(new CustomEvent('ld-map-observation', {
    bubbles: true,
    composed: true,
    detail: { stage, durationMs, visualID: envelope.visualID, rendererID: envelope.rendererID, ...detail },
  }))
}

export function mapNow(): number { return typeof performance === 'undefined' ? Date.now() : performance.now() }

export function removeRendererFrame(container: ParentNode, frame: HTMLElement): void {
  if (frame.parentNode === container) frame.remove()
}

export function waitForMapIdle(map: MapLibreMap): Promise<void> {
  return new Promise((resolve) => {
    map.once('idle', () => resolve())
    map.triggerRepaint()
  })
}
