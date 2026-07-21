import { LitElement, css, html } from 'lit'
import { property, query, state } from 'lit/decorators.js'
import type { VisualizationEnvelope } from '../../../generated/visualization'
import validateGeneratedEnvelope from '../../../generated/visualization/validate'
import { visualActionStyles } from '../visual-action-styles'
import { visualMenuIcon } from '../visual-menu-icons'
import { VisualizationController, validateEnvelopeBoundary } from './host-controller'
import { visualizationRegistry } from './registry'
import { adapterObservation } from './telemetry'

export class VisualizationHost extends LitElement {
  @property({ attribute: false }) envelope?: VisualizationEnvelope
  @query('.renderer') private rendererContainer?: HTMLDivElement
  @state() private error = ''
  @state() private applying = false
  private controller?: VisualizationController
  private resizeObserver?: ResizeObserver
  private applyGeneration = 0

  static styles = [visualActionStyles, css`
    :host, .surface { display: block; width: 100%; height: 100%; min-width: 0; min-height: 0; }
    :host { color: var(--ld-fg-default); font-family: var(--fontStack-system); }
    .surface { position: relative; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--ld-chart-surface); }
    .surface.headerless { grid-template-rows: minmax(0, 1fr); }
    .renderer { display: block; width: 100%; min-width: 0; min-height: 0; overflow: hidden; }
    .toolbar {
      position: relative;
      z-index: var(--zIndex-sticky);
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
      min-height: calc(var(--control-small-size) + var(--base-size-6));
      border-bottom: var(--ld-border-default);
      background: var(--ld-chart-surface);
      padding: var(--base-size-6) var(--base-size-8) var(--base-size-4) var(--control-small-paddingInline-normal);
      box-sizing: border-box;
    }
    .toolbar-title { flex: 1 1 auto; min-width: 0; }
    h2 {
      min-width: 0;
      margin: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
      line-height: var(--ld-line-height-compact);
    }
    .error { position: absolute; inset: 0; display: grid; place-items: center; color: var(--ld-fg-danger); padding: 1rem; text-align: center; background: var(--ld-bg-panel); }
    .fallback { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
  `]

  protected firstUpdated(): void {
    if (!this.rendererContainer) return
    this.controller = new VisualizationController(
      visualizationRegistry,
      this.rendererContainer,
      (value): value is VisualizationEnvelope => validateGeneratedEnvelope(value) && validateEnvelopeBoundary(value),
      (detail) => this.dispatchEvent(new CustomEvent('ld-visualization-observation', { bubbles: true, composed: true, detail })),
    )
    this.resizeObserver = new ResizeObserver(([entry]) => {
      if (!entry) return
      this.controller?.resize(entry.contentRect.width, entry.contentRect.height, window.devicePixelRatio || 1)
    })
    this.resizeObserver.observe(this.rendererContainer)
    void this.applyEnvelope()
  }

  protected updated(changed: Map<PropertyKey, unknown>): void {
    if (changed.has('envelope')) void this.applyEnvelope()
  }

  disconnectedCallback(): void {
    this.resizeObserver?.disconnect()
    this.controller?.dispose()
    this.controller = undefined
    super.disconnectedCallback()
  }

  async snapshot(): Promise<Blob> { return this.controller?.snapshot() ?? Promise.reject(new Error('visualization is not mounted')) }

  protected render() {
    const statusError = this.envelope?.status.kind === 'error' ? this.envelope.status.message ?? 'Visualization error' : ''
    const error = this.error || statusError
    const header = this.sharedHeader()
    return html`<div class=${header ? 'surface' : 'surface headerless'}>
      ${header ? html`
        <header class="toolbar">
          <div class="toolbar-title"><h2 data-visualization-title>${this.envelope?.spec.title}</h2></div>
          <div class="visual-actions">
            <button class="icon-action" type="button" data-visualization-id=${this.envelope?.visualID ?? ''} aria-label=${`Expand ${header}`} title=${`Expand ${header}`} @click=${this.expand}>${visualMenuIcon('focus')}</button>
          </div>
        </header>
      ` : null}
      <div class="renderer" role="group" aria-label=${this.envelope?.spec.accessibility.title ?? 'Visualization'} aria-describedby="visualization-fallback" aria-busy=${String(this.applying)} @ld-map-observation=${this.forwardAdapterObservation}></div>
      <div id="visualization-fallback" class="fallback">${this.accessibleFallback()}</div>
      ${error ? html`<div class="error" role="alert">${error}</div>` : null}
    </div>`
  }

  private async applyEnvelope(): Promise<void> {
    if (!this.envelope || !this.controller) return
    const generation = ++this.applyGeneration
    this.applying = true
    try {
      await this.controller.apply(this.envelope)
      if (generation === this.applyGeneration) this.error = ''
    } catch (error) {
      if (generation === this.applyGeneration) this.error = error instanceof Error ? error.message : String(error)
    } finally {
      if (generation === this.applyGeneration) this.applying = false
    }
  }

  private sharedHeader(): 'chart' | 'map' | 'visualization' | undefined {
    const kind = this.envelope?.spec.kind
    if (!kind || kind === 'kpi' || kind === 'table' || kind === 'matrix' || kind === 'pivot') return undefined
    if (kind === 'geographic') return 'map'
    if (kind === 'custom') return 'visualization'
    return 'chart'
  }

  private expand = (): void => {
    const envelope = this.envelope
    const visualType = this.sharedHeader()
    if (!envelope || !visualType) return
    this.dispatchEvent(new CustomEvent('ld-visual-action', {
      bubbles: true,
      composed: true,
      detail: {
        action: 'focus',
        visualType,
        visualId: envelope.visualID,
        title: envelope.spec.title,
        columns: [],
        rows: [],
        selection: envelope.selection.map((entry) => entry.label ?? Object.values(entry.datum.identity).join(' · ')),
      },
    }))
  }

  private forwardAdapterObservation = (event: CustomEvent<unknown>): void => {
    const detail = adapterObservation(event.detail)
    if (!detail) return
    event.stopPropagation()
    this.dispatchEvent(new CustomEvent('ld-visualization-observation', { bubbles: true, composed: true, detail }))
  }

  private accessibleFallback() {
    const envelope = this.envelope
    if (!envelope) return 'Visualization is loading.'
    const status = envelope.status.message ?? envelope.status.kind.replaceAll('_', ' ')
    const summary = envelope.spec.accessibility.summary ?? envelope.spec.accessibility.description
    return `${envelope.spec.accessibility.title}. ${summary}. Status: ${status}.`
  }
}

if (!customElements.get('ld-visualization-host')) customElements.define('ld-visualization-host', VisualizationHost)

declare global { interface HTMLElementTagNameMap { 'ld-visualization-host': VisualizationHost } }
