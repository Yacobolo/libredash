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
  target.dispatchEvent(new CustomEvent('lv-map-observation', {
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
  return waitForMapEvent(map, ['idle'], 10_000)
}

// A renderer is ready once MapLibre has painted the configured style and data
// layers. Waiting for `idle` here is incorrect: that event also requires every
// requested basemap tile to settle, so a slow or unavailable tile can leave the
// visualization host permanently busy even though a useful frame is visible.
export function waitForMapRender(map: MapLibreMap): Promise<void> {
  return waitForMapEvent(map, ['idle', 'render'], 2_000)
}

function waitForMapEvent(map: MapLibreMap, events: Array<'idle' | 'render'>, timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    let timer: ReturnType<typeof setTimeout> | undefined
    const finish = () => {
      if (timer !== undefined) clearTimeout(timer)
      for (const event of events) map.off(event, finish)
      resolve()
    }
    for (const event of events) map.once(event, finish)
    timer = setTimeout(finish, timeoutMs)
    map.triggerRepaint()
  })
}
