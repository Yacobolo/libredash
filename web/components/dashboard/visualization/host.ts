import { LitElement, css, html } from 'lit'
import { property, query, state } from 'lit/decorators.js'
import type { VisualizationEnvelope } from '../../../generated/visualization'
import validateGeneratedEnvelope from '../../../generated/visualization/validate'
import { visualActionStyles } from '../visual-action-styles'
import { visualMenuIcon } from '../visual-menu-icons'
import type { VisualActionDetail } from '../visual-modal'
import { defaultRendererContext, normalizeRendererLocale, VisualizationController, validateEnvelopeBoundary, type RendererContext } from './host-controller'
import { visualizationRegistry } from './registry'
import { adapterObservation } from './telemetry'

export class VisualizationHost extends LitElement {
  @property({ attribute: false }) envelope?: VisualizationEnvelope
  @property({ attribute: false }) openVisualFocus?: (source: HTMLElement, detail: VisualActionDetail) => void
  @query('.renderer') private rendererContainer?: HTMLDivElement
  @state() private error = ''
  @state() private applying = false
  @state() private presented = false
  private controller?: VisualizationController
  private resizeObserver?: ResizeObserver
  private applyGeneration = 0
  private connectionGeneration = 0
  private presentedRendererID = ''
  private focusMirror?: VisualizationHost
  private pendingViewState?: { value: unknown }
  private contextListenersConnected = false
  private reducedMotionMedia?: MediaQueryList

  static styles = [visualActionStyles, css`
    :host, .surface { display: block; width: 100%; height: 100%; min-width: 0; min-height: 0; }
    :host { color: var(--lv-fg-default); background: var(--lv-chart-surface); font-family: var(--fontStack-system); }
    .surface { position: relative; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--lv-chart-surface); }
    .surface.headerless { grid-template-rows: minmax(0, 1fr); }
    .renderer-stage { position: relative; min-width: 0; min-height: 0; overflow: hidden; background: var(--lv-chart-surface); }
    .renderer { display: block; width: 100%; height: 100%; min-width: 0; min-height: 0; overflow: hidden; }
    .lv-kpi-card {
      position: relative;
      display: grid;
      align-content: center;
      box-sizing: border-box;
      width: 100%;
      height: 100%;
      min-height: 0;
      gap: var(--base-size-4);
      padding: var(--base-size-12) var(--base-size-16) var(--base-size-12) var(--base-size-20);
      overflow: hidden;
      background: var(--lv-chart-surface);
    }
    .lv-kpi-card::before {
      content: '';
      position: absolute;
      inset-block: 0;
      inset-inline-start: 0;
      width: var(--base-size-4);
      background: var(--lv-line-muted);
    }
    .lv-kpi-card[data-tone='success']::before { background: var(--lv-fg-success); }
    .lv-kpi-card[data-tone='warning']::before { background: var(--lv-fg-warning); }
    .lv-kpi-card[data-tone='danger']::before { background: var(--lv-fg-danger); }
    .lv-kpi-card[data-tone='ink']::before { background: var(--lv-data-1); }
    .lv-visualization-label {
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-compact);
      text-transform: uppercase;
    }
    .lv-visualization-kpi {
      display: block;
      overflow: hidden;
      color: var(--lv-fg-default);
      font-size: clamp(var(--lv-font-size-title-md, var(--lv-font-size-title-sm)), 2.5vw, var(--lv-font-size-display));
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-none);
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .lv-visualization-note {
      overflow: hidden;
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-body-sm);
      font-weight: var(--lv-font-weight-medium);
      line-height: var(--lv-line-height-compact);
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .initial-loading {
      position: absolute;
      inset: 0;
      z-index: var(--zIndex-sticky);
      display: grid;
      align-content: center;
      justify-items: center;
      gap: var(--base-size-8);
      background: var(--lv-chart-surface);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-body-sm);
      font-weight: var(--lv-font-weight-medium);
    }
    .loading-spinner {
      width: var(--base-size-20);
      height: var(--base-size-20);
      box-sizing: border-box;
      border: var(--base-size-2) solid var(--lv-line-muted);
      border-top-color: var(--lv-fg-link);
      border-radius: var(--lv-radius-full);
      animation: visualization-loading-spin var(--lv-duration-slow) linear infinite;
    }
    @keyframes visualization-loading-spin { to { transform: rotate(360deg); } }
    @media (prefers-reduced-motion: reduce) { .loading-spinner { animation: none; } }
    .toolbar {
      position: relative;
      z-index: var(--zIndex-sticky);
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
      min-height: calc(var(--control-small-size) + var(--base-size-6));
      border-bottom: var(--lv-border-default);
      background: var(--lv-chart-surface);
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
      font-size: var(--lv-font-size-body-md);
      font-weight: var(--lv-font-weight-strong);
      letter-spacing: 0;
      line-height: var(--lv-line-height-compact);
    }
    .error { position: absolute; inset: 0; display: grid; place-items: center; color: var(--lv-fg-danger); padding: 1rem; text-align: center; background: var(--lv-bg-panel); }
    .fallback { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
  `]

  protected firstUpdated(): void {
    this.connectContextListeners()
    this.ensureController()
  }

  connectedCallback(): void {
    super.connectedCallback()
    const generation = ++this.connectionGeneration
    if (!this.hasUpdated || this.controller) return
    queueMicrotask(() => {
      if (generation === this.connectionGeneration && this.isConnected) {
        this.connectContextListeners()
        this.ensureController()
      }
    })
  }

  private ensureController(): void {
    if (this.controller || !this.rendererContainer) return
    this.controller = new VisualizationController(
      visualizationRegistry,
      this.rendererContainer,
      (value): value is VisualizationEnvelope => validateGeneratedEnvelope(value) && validateEnvelopeBoundary(value),
      (detail) => this.dispatchEvent(new CustomEvent('lv-visualization-observation', { bubbles: true, composed: true, detail })),
    )
    this.resizeObserver = new ResizeObserver(([entry]) => {
      if (!entry) return
      this.controller?.resize(entry.contentRect.width, entry.contentRect.height, window.devicePixelRatio || 1)
    })
    this.resizeObserver.observe(this.rendererContainer)
    if (this.pendingViewState) {
      this.controller.restoreViewState(this.pendingViewState.value)
      this.pendingViewState = undefined
    }
    void this.applyEnvelope()
  }

  protected updated(changed: Map<PropertyKey, unknown>): void {
    if (changed.has('envelope')) {
      if (this.focusMirror) this.focusMirror.envelope = this.envelope
      void this.applyEnvelope()
    }
  }

  disconnectedCallback(): void {
    const generation = ++this.connectionGeneration
    super.disconnectedCallback()
    // A synchronous DOM move fires disconnected/connected callbacks even though
    // the visual remains live. Defer teardown so transient moves retain renderer
    // state; a host that stays detached is still disposed in the same microtask.
    queueMicrotask(() => {
      if (generation !== this.connectionGeneration || this.isConnected) return
      this.resizeObserver?.disconnect()
      this.resizeObserver = undefined
      this.disconnectContextListeners()
      this.controller?.dispose()
      this.controller = undefined
      this.presented = false
      this.presentedRendererID = ''
    })
  }

  async snapshot(): Promise<Blob> { return this.controller?.snapshot() ?? Promise.reject(new Error('visualization is not mounted')) }

  setFocusMirror(mirror?: VisualizationHost): void {
    this.focusMirror = mirror
    if (mirror) {
      mirror.envelope = this.envelope
      const state = this.controller?.captureViewState()
      if (state !== undefined) mirror.restoreViewState(state)
    }
  }

  private restoreViewState(state: unknown): void {
    if (this.controller) {
      this.controller.restoreViewState(state)
      return
    }
    this.pendingViewState = { value: state }
  }

  protected render() {
    const statusError = this.envelope?.status.kind === 'error' ? this.envelope.status.message ?? 'Visualization error' : ''
    const error = this.error || statusError
    const header = this.sharedHeader()
    const showInitialLoading = !this.presented && !error
    const loadingLabel = `Loading ${header ?? 'visualization'}…`
    return html`<div class=${header ? 'surface' : 'surface headerless'}>
      ${header ? html`
        <header class="toolbar">
          <div class="toolbar-title"><h2 data-visualization-title>${this.envelope?.spec.title}</h2></div>
          <div class="visual-actions">
            <button class="icon-action" type="button" data-visualization-expand data-visualization-id=${this.envelope?.visualID ?? ''} aria-label=${`Expand ${header}`} title=${`Expand ${header}`} @click=${this.expand}>${visualMenuIcon('focus')}</button>
          </div>
        </header>
      ` : null}
      <div class="renderer-stage" aria-busy=${String(this.applying)}>
        <div class="renderer" role="group" aria-label=${this.envelope?.spec.accessibility.title ?? 'Visualization'} aria-describedby="visualization-fallback" aria-busy=${String(this.applying)} aria-hidden=${String(!this.presented)} ?inert=${!this.presented} @lv-map-observation=${this.forwardAdapterObservation}></div>
        ${showInitialLoading ? html`<div class="initial-loading" data-visualization-loading role="status" aria-live="polite">
          <span class="loading-spinner" aria-hidden="true"></span>
          <span>${loadingLabel}</span>
        </div>` : null}
      </div>
      <div id="visualization-fallback" class="fallback">${this.accessibleFallback()}</div>
      ${error ? html`<div class="error" role="alert">${error}</div>` : null}
    </div>`
  }

  private async applyEnvelope(): Promise<void> {
    if (!this.envelope || !this.controller) return
    if (this.presentedRendererID !== this.envelope.rendererID) {
      this.presentedRendererID = this.envelope.rendererID
      this.presented = false
    }
    const generation = ++this.applyGeneration
    this.applying = true
    try {
      await this.controller.apply(this.envelope, this.rendererContext())
      if (generation === this.applyGeneration) {
        this.error = ''
        this.presented = true
      }
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
    const detail: VisualActionDetail = {
      action: 'focus',
      visualType,
      visualId: envelope.visualID,
      title: envelope.spec.title,
      columns: [],
      rows: [],
      selection: envelope.selection.map((entry) => entry.label ?? Object.values(entry.datum.identity).join(' · ')),
    }
    this.openFocus(detail)
  }

  private openFocus(detail: VisualActionDetail): void {
    if (this.openVisualFocus) {
      this.openVisualFocus(this, detail)
      return
    }
    this.dispatchEvent(new CustomEvent('lv-visual-action', {
      bubbles: true,
      composed: true,
      detail,
    }))
  }

  private forwardAdapterObservation = (event: CustomEvent<unknown>): void => {
    const detail = adapterObservation(event.detail)
    if (!detail) return
    event.stopPropagation()
    this.dispatchEvent(new CustomEvent('lv-visualization-observation', { bubbles: true, composed: true, detail }))
  }

  private connectContextListeners(): void {
    if (this.contextListenersConnected) return
    this.contextListenersConnected = true
    document.addEventListener('leapview-theme-applied', this.handleRendererContextChange)
    this.reducedMotionMedia = window.matchMedia?.('(prefers-reduced-motion: reduce)')
    this.reducedMotionMedia?.addEventListener?.('change', this.handleRendererContextChange)
  }

  private disconnectContextListeners(): void {
    if (!this.contextListenersConnected) return
    this.contextListenersConnected = false
    document.removeEventListener('leapview-theme-applied', this.handleRendererContextChange)
    this.reducedMotionMedia?.removeEventListener?.('change', this.handleRendererContextChange)
    this.reducedMotionMedia = undefined
  }

  private readonly handleRendererContextChange = (): void => { void this.applyEnvelope() }

  private rendererContext(): RendererContext {
    const target = this.rendererContainer
    if (!target) return defaultRendererContext
    const styles = getComputedStyle(target)
    const color = (name: string, fallback: string): string => styles.getPropertyValue(name).trim() || fallback
    const colorScheme = document.documentElement.style.colorScheme.trim()
    const theme = colorScheme === 'dark' || (colorScheme !== 'light' && window.matchMedia?.('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light'
    return {
      locale: normalizeRendererLocale(document.documentElement.lang || 'en'),
      theme,
      reducedMotion: this.reducedMotionMedia?.matches ?? true,
      devicePixelRatio: window.devicePixelRatio || 1,
      fontFamily: styles.fontFamily || defaultRendererContext.fontFamily,
      colors: {
        foreground: color('--lv-fg-default', defaultRendererContext.colors.foreground),
        muted: color('--lv-chart-axis', defaultRendererContext.colors.muted),
        grid: color('--lv-chart-grid', defaultRendererContext.colors.grid),
        surface: color('--lv-chart-surface', defaultRendererContext.colors.surface),
        accent: color('--lv-fg-accent', defaultRendererContext.colors.accent),
        success: color('--lv-fg-success', defaultRendererContext.colors.success),
        attention: color('--lv-fg-warning', defaultRendererContext.colors.attention),
        danger: color('--lv-fg-danger', defaultRendererContext.colors.danger),
        data: Array.from({ length: 8 }, (_, index) => color(`--lv-data-${index + 1}`, defaultRendererContext.colors.data[index]!)),
      },
    }
  }

  private accessibleFallback() {
    const envelope = this.envelope
    if (!envelope) return 'Visualization is loading.'
    const status = envelope.status.message ?? envelope.status.kind.replaceAll('_', ' ')
    const summary = envelope.spec.accessibility.summary ?? envelope.spec.accessibility.description
    return `${envelope.spec.accessibility.title}. ${summary}. Status: ${status}.`
  }
}

if (!customElements.get('lv-visualization-host')) customElements.define('lv-visualization-host', VisualizationHost)

declare global { interface HTMLElementTagNameMap { 'lv-visualization-host': VisualizationHost } }
