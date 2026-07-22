import type { VisualizationEnvelope, VisualizationFieldRef } from '../../../../generated/visualization'
import { defaultRendererContext, type RendererAdapter, type RendererContext, type RendererHandle } from '../host-controller'
import { formatValue } from '../format'

export const adapter: RendererAdapter = {
  mount(container, envelope, context) { return new HTMLHandle(container, envelope, context) },
}

class HTMLHandle implements RendererHandle {
  constructor(private readonly container: HTMLElement, envelope: VisualizationEnvelope, context: RendererContext) { this.update(envelope, 0, context) }
  update(envelope: VisualizationEnvelope, _change: number, context: RendererContext): void {
    this.container.replaceChildren()
    const article = document.createElement('article')
    article.className = 'ld-kpi-card'
    article.setAttribute('aria-label', envelope.spec.accessibility.title)
    if (envelope.spec.kind === 'kpi') article.dataset.tone = envelope.spec.presentation.tone
    const label = document.createElement('div')
    label.className = 'ld-visualization-label'
    label.textContent = envelope.spec.title
    const value = document.createElement('strong')
    value.className = 'ld-visualization-kpi'
    value.textContent = kpiText(envelope, context)
    article.append(label, value)
    if (envelope.spec.kind === 'kpi' && envelope.spec.presentation.note) {
      const note = document.createElement('small'); note.className = 'ld-visualization-note'; note.textContent = envelope.spec.presentation.note; article.append(note)
    }
    this.container.append(article)
  }
  resize(): void {}
  async snapshot(): Promise<Blob> { return new Blob([this.container.textContent ?? ''], { type: 'text/plain' }) }
  dispose(): void { this.container.replaceChildren() }
}

export function kpiText(envelope: VisualizationEnvelope, context: RendererContext = defaultRendererContext): string {
  const spec = envelope.spec
  if (spec.kind !== 'kpi') return '—'
  const value = scalar(envelope, spec.value)
  const field = spec.datasets.find((dataset) => dataset.id === spec.value.dataset)?.fields.find((candidate) => candidate.id === spec.value.field)
  if (field?.format) return formatValue(context.locale, field.format, value)
  return value === null || value === undefined ? '—' : String(value)
}

function scalar(envelope: VisualizationEnvelope, ref: VisualizationFieldRef): unknown {
  if (envelope.dataState.kind !== 'inline') return undefined
  const dataset = envelope.dataState.datasets.find((candidate) => candidate.id === ref.dataset)
  const index = dataset?.columns.indexOf(ref.field) ?? -1
  return index >= 0 ? dataset?.rows[0]?.[index] : undefined
}
