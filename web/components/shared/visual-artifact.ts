import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { defineElementOnce } from './lazy-registry'

class VisualArtifact extends LitElement {
  @property() kind: string = ''
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
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-chart-surface, var(--ld-bg-panel));
      box-shadow: var(--shadow-resting-small);
    }

    ld-echart,
    ld-data-table {
      display: block;
      width: 100%;
      height: 100%;
    }

    .state {
      display: grid;
      height: 100%;
      min-height: 8rem;
      place-items: center;
      padding: var(--ld-space-lg);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      text-align: center;
    }
  `

  protected updated(changed: Map<string, unknown>) {
    if (changed.has('kind') || changed.has('payload')) {
      void this.ensureRenderer()
    }
  }

  render() {
    if (this.kind !== 'chart' && this.kind !== 'table') {
      return this.renderState(`Unsupported artifact: ${this.kind || 'unknown'}`)
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
    if (this.kind === 'chart') {
      return html`
        <div class="artifact chart">
          <ld-echart visual-id=${this.artifactId} .chart=${this.payload}></ld-echart>
        </div>
      `
    }
    return html`
      <div class="artifact table">
        <ld-data-table table-id=${this.artifactId} .table=${this.payload}></ld-data-table>
      </div>
    `
  }

  private renderState(message: string) {
    return html`<div class="artifact"><div class="state">${message}</div></div>`
  }

  private async ensureRenderer() {
    const kind = this.kind
    const token = ++this.loadToken
    this.rendererReady = false
    this.rendererError = ''
    if (!this.payload || (kind !== 'chart' && kind !== 'table')) return
    try {
      if (kind === 'chart') {
        await defineElementOnce('ld-echart', () => import('../dashboard/charts/echart'))
      } else {
        await defineElementOnce('ld-data-table', () => import('../dashboard/table/data-table'))
      }
      if (token !== this.loadToken) return
      this.rendererReady = true
    } catch (error) {
      if (token !== this.loadToken) return
      this.rendererError = error instanceof Error ? error.message : 'Artifact renderer failed to load.'
    }
  }
}

if (!customElements.get('ld-visual-artifact')) customElements.define('ld-visual-artifact', VisualArtifact)
