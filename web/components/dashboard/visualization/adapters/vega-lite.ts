import type { VisualizationEnvelope } from '../../../../generated/visualization'
import validateVegaLite from '../../../../generated/vega-lite/validate'
import { interactionSelectionLabel, interactionSelectionValue, type OptimisticInteractionCommand } from '../../interaction-selection'
import type { RendererAdapter, RendererHandle } from '../host-controller'

export const adapter: RendererAdapter = {
  mount(container, envelope) {
    const iframe = document.createElement('iframe')
    iframe.setAttribute('sandbox', 'allow-scripts')
    iframe.setAttribute('title', envelope.spec.accessibility.title)
    iframe.style.cssText = 'border:0;width:100%;height:100%'
    const origin = location.origin.replace(/["&<>]/g, '')
    iframe.srcdoc = `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src ${origin}; style-src 'unsafe-inline'; img-src data: blob:"><style>html,body,#view{width:100%;height:100%;margin:0}</style><div id="view"></div><script type="module" src="/static/vega-sandbox.js"></script>`
    container.replaceChildren(iframe)
    return new VegaLiteHandle(container, iframe, envelope)
  },
}

class VegaLiteHandle implements RendererHandle {
  private readonly channel = new MessageChannel()
  private readonly ready: Promise<void>
  private requestID = 0
  private pendingSnapshots = new Map<number, { resolve(value: Blob): void; reject(error: Error): void }>()
  private envelope: VisualizationEnvelope
  constructor(private readonly container: HTMLElement, private readonly iframe: HTMLIFrameElement, envelope: VisualizationEnvelope) {
    this.envelope = envelope
    this.channel.port1.onmessage = (event) => this.receive(event.data)
    this.ready = new Promise((resolve, reject) => {
      iframe.addEventListener('load', () => {
        iframe.contentWindow?.postMessage({ kind: 'connect' }, '*', [this.channel.port2])
        resolve()
      }, { once: true })
      iframe.addEventListener('error', () => reject(new Error('Vega-Lite sandbox failed to load')), { once: true })
    })
    void this.update(envelope)
  }
  async update(envelope: VisualizationEnvelope): Promise<void> {
    if (envelope.spec.kind !== 'custom' || envelope.spec.engine !== 'vega_lite') throw new Error('Vega-Lite adapter requires a custom vega_lite specification')
    const program = assertSafeVegaLiteProgram(envelope.spec.program, envelope.spec.datasets.flatMap((dataset) => dataset.fields.map((field) => field.id)))
    await verifyProgramDigest(envelope.spec.program, envelope.spec.programDigest)
    this.envelope = envelope
    await this.ready
    this.channel.port1.postMessage({ kind: 'render', program, datasets: inlineRecords(envelope) })
  }
  resize(width: number, height: number): void { this.channel.port1.postMessage({ kind: 'resize', width, height }) }
  async snapshot(): Promise<Blob> {
    await this.ready
    const id = ++this.requestID
    const result = new Promise<Blob>((resolve, reject) => this.pendingSnapshots.set(id, { resolve, reject }))
    this.channel.port1.postMessage({ kind: 'snapshot', id })
    return result
  }
  dispose(): void {
    for (const pending of this.pendingSnapshots.values()) pending.reject(new Error('Vega-Lite renderer disposed'))
    this.pendingSnapshots.clear(); this.channel.port1.close(); this.iframe.remove(); this.container.replaceChildren()
  }
  private receive(message: { kind?: string; id?: number; blob?: Blob; error?: string; datum?: Record<string, unknown> }): void {
    if (message.kind === 'error') this.container.dispatchEvent(new CustomEvent('lv-visualization-renderer-error', { bubbles: true, composed: true, detail: { error: message.error ?? 'Vega-Lite error' } }))
    if (message.kind === 'interaction' && message.datum) {
      const command = interactionCommandForDatum(this.envelope, message.datum)
      if (command) this.container.dispatchEvent(new CustomEvent('lv-interaction-select', { bubbles: true, composed: true, detail: command }))
    }
    if (message.kind !== 'snapshot' || message.id === undefined) return
    const pending = this.pendingSnapshots.get(message.id); if (!pending) return
    this.pendingSnapshots.delete(message.id)
    if (message.blob) pending.resolve(message.blob); else pending.reject(new Error(message.error ?? 'Vega-Lite snapshot failed'))
  }
}

export function interactionCommandForDatum(envelope: VisualizationEnvelope, datum: Record<string, unknown>): OptimisticInteractionCommand | undefined {
  const interaction = envelope.spec.interactions.find((candidate) => candidate.kind === 'select')
  if (!interaction || interaction.mappings.length === 0) return undefined
  const mappings = interaction.mappings.map((mapping) => {
    const value = interactionSelectionValue(datum[mapping.source.field])
    const label = interactionSelectionValue(datum[mapping.label?.field ?? mapping.source.field])
    if (value === undefined || label === undefined) return undefined
    return {
      field: mapping.targetFieldID,
      ...(mapping.targetFactID ? { fact: mapping.targetFactID } : {}),
      ...(mapping.grain ? { grain: mapping.grain } : {}),
      value, label: interactionSelectionLabel(label),
    }
  })
  if (mappings.some((mapping) => mapping === undefined)) return undefined
  return { sourceKind: 'visual', sourceId: envelope.visualID, interactionKind: interaction.id, action: 'set', toggle: interaction.mode === 'multiple', mappings: mappings as OptimisticInteractionCommand['mappings'] }
}

export function assertSafeVegaLiteProgram(source: string, fields: string[]): object {
  let program: unknown
  try { program = JSON.parse(source) } catch { throw new Error('Vega-Lite program must be valid JSON') }
  const allowedFields = new Set(fields)
  walk(program, '', allowedFields)
	if (!validateVegaLite(program)) throw new Error('program does not match the pinned Vega-Lite schema')
  return program as object
}

function walk(value: unknown, path: string, fields: Set<string>): void {
  if (Array.isArray(value)) { value.forEach((item, index) => walk(item, `${path}/${index}`, fields)); return }
  if (!value || typeof value !== 'object') return
  for (const [key, child] of Object.entries(value)) {
    if (['url', 'href', 'expr', 'calculate', 'transform', 'params', 'datasets', 'values'].includes(key)) throw new Error(`Vega-Lite property ${path}/${key} is not allowed`)
    if (key === 'field' && (typeof child !== 'string' || !fields.has(child))) throw new Error(`Vega-Lite field ${JSON.stringify(child)} is not in the compiled dataset schema`)
    walk(child, `${path}/${key}`, fields)
  }
}

function inlineRecords(envelope: VisualizationEnvelope): Record<string, Record<string, unknown>[]> {
  if (envelope.dataState.kind !== 'inline') return {}
  return Object.fromEntries(envelope.dataState.datasets.map((dataset) => [dataset.id, dataset.rows.map((row) => Object.fromEntries(dataset.columns.map((column, index) => [column, row[index]])))]))
}

async function verifyProgramDigest(source: string, declared: string): Promise<void> {
  const bytes = new TextEncoder().encode(source)
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))
  const actual = `sha256:${Array.from(digest, (value) => value.toString(16).padStart(2, '0')).join('')}`
  if (actual !== declared) throw new Error('Vega-Lite program digest mismatch')
}
