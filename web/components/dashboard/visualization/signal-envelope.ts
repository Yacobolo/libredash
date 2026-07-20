import type {
  DashboardVisualizationSignal,
  VisualizationDataState,
  VisualizationEnvelope,
} from '../../../generated/signals'

type CachedDataState = {
  encoded: string
  value: VisualizationDataState
}

/**
 * Reconstructs canonical visualization envelopes while keeping large data
 * frames opaque to Datastar's recursively reactive signal graph.
 */
export class DashboardVisualizationSignalDecoder {
  private readonly dataStates = new Map<string, CachedDataState>()

  decodeAll(signals: Record<string, DashboardVisualizationSignal>): Record<string, VisualizationEnvelope> {
    const envelopes: Record<string, VisualizationEnvelope> = {}
    const active = new Set(Object.keys(signals))
    for (const [id, signal] of Object.entries(signals)) {
      const envelope = this.decode(signal)
      if (envelope) envelopes[id] = envelope
    }
    for (const id of this.dataStates.keys()) {
      if (!active.has(id)) this.dataStates.delete(id)
    }
    return envelopes
  }

  decode(signal: DashboardVisualizationSignal): VisualizationEnvelope | undefined {
    let dataState = this.dataStates.get(signal.visualID)
    if (!dataState || dataState.encoded !== signal.dataStateJson) {
      const decoded = decodeDataState(signal.dataStateJson)
      if (!decoded) return undefined
      dataState = { encoded: signal.dataStateJson, value: decoded }
      this.dataStates.set(signal.visualID, dataState)
    }
    const { dataStateJson: _, ...envelope } = signal
    return { ...envelope, dataState: dataState.value }
  }
}

function decodeDataState(encoded: string): VisualizationDataState | undefined {
  try {
    const value = JSON.parse(encoded)
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
    return value as VisualizationDataState
  } catch {
    return undefined
  }
}
