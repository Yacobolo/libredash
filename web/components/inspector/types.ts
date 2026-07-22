/**
 * Type definitions for Datastar Inspector
 */

/**
 * Persisted inspector state (stored in sessionStorage)
 */
export interface InspectorState {
  /** Whether the panel is expanded */
  expanded: boolean
  /** Current filter text */
  filter: string
  /** Expanded tree paths */
  expandedPaths?: string[]
  /** User-positioned collapsed launcher coordinates */
  togglePosition?: InspectorPosition
  /** User-positioned expanded panel coordinates */
  panelPosition?: InspectorPosition
}

export interface InspectorPosition {
  x: number
  y: number
}

/**
 * Signal data structure (recursive key-value)
 */
export type SignalValue = string | number | boolean | null | SignalValue[] | SignalObject

export interface SignalObject {
  [key: string]: SignalValue
}

export interface PageStreamSignalLeaf {
  path: string
  displayPath: string
  value: unknown
}

export interface PageStreamSignalChange {
  id: number
  traceEventId: number
  timestamp: string
  streamId: string
  path: string
  displayPath: string
  operation: 'set' | 'removed'
  value?: unknown
  generation?: number
  sequence: number
  origin?: string
  correlationId?: string
}

export interface PageStreamSignalsResponse {
  streamId: string
  state: SignalObject
  leaves: PageStreamSignalLeaf[]
  history: PageStreamSignalChange[]
  nextAfter: number
}
