import type { VisualizationEnvelope, VisualizationSpec } from '../../../generated/visualization'

export enum Change {
  None = 0,
  Spec = 1 << 0,
  Data = 1 << 1,
  Selection = 1 << 2,
  Status = 1 << 3,
  All = Spec | Data | Selection | Status,
}

export type RendererCapabilities = Readonly<{
  snapshot: boolean
  windowed: boolean
  interactive: boolean
}>

export interface RendererHandle {
  update(envelope: VisualizationEnvelope, change: Change): void | Promise<void>
  resize(width: number, height: number, devicePixelRatio: number): void
  snapshot(): Promise<Blob>
  dispose(): void
}

export interface RendererAdapter {
  mount(container: HTMLElement, envelope: VisualizationEnvelope): RendererHandle | Promise<RendererHandle>
}

export type RendererRegistration = Readonly<{
  id: string
  version: string
  schemaVersions: readonly number[]
  kinds: readonly VisualizationSpec['kind'][]
  capabilities: RendererCapabilities
  load(): Promise<RendererAdapter>
}>

type LoadedRegistration = RendererRegistration & { adapter?: Promise<RendererAdapter> }

export class RendererRegistry {
  readonly #registrations = new Map<string, LoadedRegistration>()

  register(registration: RendererRegistration): void {
    if (!registration.id || !registration.version || registration.schemaVersions.length === 0 || registration.kinds.length === 0) {
      throw new Error('renderer registration requires identity, version, schema versions, and kinds')
    }
    if (this.#registrations.has(registration.id)) throw new Error(`renderer ${JSON.stringify(registration.id)} is already registered`)
    this.#registrations.set(registration.id, { ...registration })
  }

  resolve(envelope: VisualizationEnvelope): LoadedRegistration {
    const registration = this.#registrations.get(envelope.rendererID)
    if (!registration) throw new Error(`unknown visualization renderer ${JSON.stringify(envelope.rendererID)}`)
    if (!registration.schemaVersions.includes(envelope.schemaVersion)) {
      throw new Error(`renderer ${JSON.stringify(envelope.rendererID)} does not support schema version ${envelope.schemaVersion}`)
    }
    if (!registration.kinds.includes(envelope.spec.kind)) {
      throw new Error(`renderer ${JSON.stringify(envelope.rendererID)} does not support kind ${JSON.stringify(envelope.spec.kind)}`)
    }
    if ((envelope.dataState.kind === 'windowed' || envelope.dataState.kind === 'spatial_windowed') && !registration.capabilities.windowed) {
      throw new Error(`renderer ${JSON.stringify(envelope.rendererID)} does not support windowed data`)
    }
    return registration
  }

  load(registration: LoadedRegistration): Promise<RendererAdapter> {
    registration.adapter ??= registration.load()
    return registration.adapter
  }
}

export type EnvelopeValidator = (value: unknown) => value is VisualizationEnvelope
export type VisualizationObservation = Readonly<{
  stage: 'validation_failure' | 'stale_result_drop' | 'renderer_load' | 'mount' | 'update' | 'resize' | 'dispose' | 'adapter_error' | 'adapter_observation'
  durationMs: number
  rendererID?: string
  kind?: VisualizationSpec['kind']
  visualID?: string
  adapterStage?: string
  assetID?: string
  layerID?: string
  featureCount?: number
}>
export type VisualizationObserver = (observation: VisualizationObservation) => void

export class VisualizationController {
  readonly #registry: RendererRegistry
  readonly #container: HTMLElement
  readonly #validate: EnvelopeValidator
  readonly #observe?: VisualizationObserver
  #envelope?: VisualizationEnvelope
  #handle?: RendererHandle
  #loadGeneration = 0
  #disposed = false
  #pendingResize?: readonly [number, number, number]
  #resizeFrame?: number

  constructor(registry: RendererRegistry, container: HTMLElement, validate: EnvelopeValidator = validateEnvelopeBoundary, observe?: VisualizationObserver) {
    this.#registry = registry
    this.#container = container
    this.#validate = validate
    this.#observe = observe
  }

  get envelope(): VisualizationEnvelope | undefined { return this.#envelope }

  async apply(next: VisualizationEnvelope): Promise<boolean> {
    if (this.#disposed) throw new Error('visualization controller is disposed')
    if (!this.#validate(next)) {
      this.#record('validation_failure', 0, next)
      throw new Error('invalid visualization envelope')
    }
    const registration = this.#registry.resolve(next)
    const previous = this.#envelope
    if (previous && previous.specRevision === next.specRevision && next.dataRevision < previous.dataRevision) {
      this.#record('stale_result_drop', 0, next)
      return false
    }
    const change = changes(previous, next)
    if (change === Change.None) return false

    if (!this.#handle || previous?.rendererID !== next.rendererID) {
      this.#handle?.dispose()
      this.#handle = undefined
      const generation = ++this.#loadGeneration
      const loadStarted = now()
      const adapter = await this.#registry.load(registration)
      this.#record('renderer_load', now() - loadStarted, next)
      if (this.#disposed || generation !== this.#loadGeneration) return false
      const mountStarted = now()
      let handle: RendererHandle
      try {
        handle = await adapter.mount(this.#container, next)
      } catch (error) {
        this.#record('adapter_error', now() - mountStarted, next)
        throw error
      }
      this.#record('mount', now() - mountStarted, next)
      if (this.#disposed || generation !== this.#loadGeneration) {
		handle.dispose()
        return false
      }
	  this.#handle = handle
      this.#envelope = next
      this.#flushResize()
      return true
    }

    this.#envelope = next
    const updateStarted = now()
    try {
      await this.#handle.update(next, change)
    } catch (error) {
      this.#record('adapter_error', now() - updateStarted, next)
      throw error
    }
    this.#record('update', now() - updateStarted, next)
    return true
  }

  resize(width: number, height: number, devicePixelRatio = 1): void {
    if (width < 0 || height < 0 || !Number.isFinite(devicePixelRatio) || devicePixelRatio <= 0) return
    this.#pendingResize = [width, height, devicePixelRatio]
    if (this.#resizeFrame !== undefined) return
    if (typeof requestAnimationFrame === 'function') {
      this.#resizeFrame = requestAnimationFrame(() => { this.#resizeFrame = undefined; this.#flushResize() })
    } else {
      queueMicrotask(() => { this.#resizeFrame = undefined; this.#flushResize() })
      this.#resizeFrame = -1
    }
  }

  snapshot(): Promise<Blob> {
    if (!this.#handle) return Promise.reject(new Error('visualization renderer is not mounted'))
    return this.#handle.snapshot()
  }

  dispose(): void {
    if (this.#disposed) return
    this.#disposed = true
    this.#loadGeneration++
    if (this.#resizeFrame !== undefined && this.#resizeFrame >= 0 && typeof cancelAnimationFrame === 'function') cancelAnimationFrame(this.#resizeFrame)
    this.#resizeFrame = undefined
    this.#pendingResize = undefined
    const disposeStarted = now()
    this.#handle?.dispose()
    if (this.#handle) this.#record('dispose', now() - disposeStarted, this.#envelope)
    this.#handle = undefined
    this.#envelope = undefined
  }

  #flushResize(): void {
    if (!this.#handle || !this.#pendingResize) return
    const [width, height, devicePixelRatio] = this.#pendingResize
    this.#pendingResize = undefined
    const started = now()
    this.#handle.resize(width, height, devicePixelRatio)
    this.#record('resize', now() - started, this.#envelope)
  }

  #record(stage: VisualizationObservation['stage'], durationMs: number, envelope?: VisualizationEnvelope): void {
    this.#observe?.({ stage, durationMs, rendererID: envelope?.rendererID, kind: envelope?.spec.kind })
  }
}

function now(): number { return typeof performance === 'undefined' ? Date.now() : performance.now() }

function changes(previous: VisualizationEnvelope | undefined, next: VisualizationEnvelope): Change {
  if (!previous) return Change.All
  let result = Change.None
  if (previous.rendererID !== next.rendererID || previous.specRevision !== next.specRevision) result |= Change.Spec
  if (previous.specRevision !== next.specRevision || previous.dataRevision !== next.dataRevision) result |= Change.Data
  if (!sameJSON(previous.selection, next.selection)) result |= Change.Selection
  if (!sameJSON(previous.status, next.status) || !sameJSON(previous.diagnostics, next.diagnostics)) result |= Change.Status
  return result
}

function sameJSON(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right)
}

// Generated JSON Schema validation is injected by the element in production.
// This structural guard protects direct controller users and enforces the
// revision invariants before any lazy renderer code runs.
export function validateEnvelopeBoundary(value: unknown): value is VisualizationEnvelope {
  if (!value || typeof value !== 'object') return false
  const envelope = value as Partial<VisualizationEnvelope>
  if (envelope.schemaVersion !== 2 || typeof envelope.visualID !== 'string' || envelope.visualID.length === 0) return false
  if (typeof envelope.rendererID !== 'string' || envelope.rendererID.length === 0 || typeof envelope.specRevision !== 'string') return false
  if (!envelope.spec || !envelope.dataState || typeof envelope.dataRevision !== 'number' || envelope.dataRevision < 0) return false
  if (envelope.dataState.specRevision !== envelope.specRevision || envelope.dataState.dataRevision !== envelope.dataRevision) return false
  return Array.isArray(envelope.selection) && !!envelope.status && Array.isArray(envelope.diagnostics)
}
