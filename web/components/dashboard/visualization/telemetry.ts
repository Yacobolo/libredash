import type { VisualizationObservation } from './host-controller'

const adapterStages = new Set(['basemap_load', 'layer_shape', 'webgl_context_loss', 'webgl_context_restored'])

export function adapterObservation(value: unknown): VisualizationObservation | undefined {
  if (!value || typeof value !== 'object') return undefined
  const detail = value as Record<string, unknown>
  if (typeof detail.stage !== 'string' || !adapterStages.has(detail.stage)) return undefined
  if (typeof detail.durationMs !== 'number' || !Number.isFinite(detail.durationMs) || detail.durationMs < 0) return undefined
  if (typeof detail.visualID !== 'string' || typeof detail.rendererID !== 'string') return undefined
  return {
    stage: 'adapter_observation',
    adapterStage: detail.stage,
    durationMs: detail.durationMs,
    visualID: detail.visualID,
    rendererID: detail.rendererID,
    ...(typeof detail.assetID === 'string' ? { assetID: detail.assetID } : {}),
    ...(typeof detail.layerID === 'string' ? { layerID: detail.layerID } : {}),
    ...(typeof detail.featureCount === 'number' && Number.isFinite(detail.featureCount) ? { featureCount: detail.featureCount } : {}),
  }
}
