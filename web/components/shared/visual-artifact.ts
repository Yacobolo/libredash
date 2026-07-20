import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { defineElementOnce } from './lazy-registry'

class VisualArtifact extends LitElement {
  @property() type: string = ''
  @property({ attribute: 'artifact-id' }) artifactId = ''
  @property({ attribute: false }) payload: unknown = null
  @state() private rendererReady = false
  @state() private rendererError = ''
  private loadToken = 0

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 0;
    }

    *,
    *::before,
    *::after {
      box-sizing: border-box;
    }

    .artifact {
      display: block;
      width: 100%;
      height: 100%;
      min-width: 0;
      overflow: hidden;
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: var(--lv-chart-surface, var(--lv-bg-panel));
      box-shadow: var(--shadow-resting-small);
    }

    lv-echart,
    lv-report-table {
      display: block;
      width: 100%;
      height: 100%;
    }

    .state {
      display: grid;
      height: 100%;
      min-height: 8rem;
      place-items: center;
      padding: var(--lv-space-lg);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-body-sm);
      text-align: center;
    }
  `

  protected updated(changed: Map<string, unknown>) {
    if (changed.has('type') || changed.has('payload')) {
      void this.ensureRenderer()
    }
  }

  render() {
    if (!this.type) {
      return this.renderState('Unsupported artifact: unknown')
    }
    if (!this.payload) {
      return this.renderState('Artifact data is unavailable.')
    }
    if (this.rendererError) {
      return this.renderState(this.rendererError)
    }
    if (!this.rendererReady) {
      return this.renderState('Loading artifact...')
    }
    if (!isTabularVisualType(this.type)) {
      return html`
        <div class="artifact chart">
          ${this.type === 'kpi'
            ? html`<lv-kpi-card visual-id=${this.artifactId} .visual=${this.payload}></lv-kpi-card>`
            : html`<lv-echart visual-id=${this.artifactId} .chart=${this.payload}></lv-echart>`}
        </div>
      `
    }
    return html`
      <div class="artifact table">
        <lv-report-table table-id=${this.artifactId} .table=${this.payload}></lv-report-table>
      </div>
    `
  }

  private renderState(message: string) {
    return html`<div class="artifact"><div class="state">${message}</div></div>`
  }

  private async ensureRenderer() {
    const visualType = this.type
    const token = ++this.loadToken
    this.rendererReady = false
    this.rendererError = ''
    if (!this.payload || !visualType) return
    try {
      if (visualType === 'kpi') {
        await defineElementOnce('lv-kpi-card', () => import('../dashboard/charts/echart'))
      } else if (!isTabularVisualType(visualType)) {
        await defineElementOnce('lv-echart', () => import('../dashboard/charts/echart'))
      } else {
        await defineElementOnce('lv-report-table', () => import('../dashboard/table/report-table'))
      }
      if (token !== this.loadToken) return
      this.rendererReady = true
    } catch (error) {
      if (token !== this.loadToken) return
      this.rendererError = error instanceof Error ? error.message : 'Artifact renderer failed to load.'
    }
  }
}

function isTabularVisualType(type: string): boolean {
  return type === 'table' || type === 'matrix' || type === 'pivot'
}

if (!customElements.get('lv-visual-artifact')) customElements.define('lv-visual-artifact', VisualArtifact)
