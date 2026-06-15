import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'

type FooterStatus = {
  loading?: boolean
  lastUpdated?: string
  error?: string
}

const statusConverter = {
  fromAttribute(value: string | null): FooterStatus {
    if (!value) return {}
    try {
      return JSON.parse(value) as FooterStatus
    } catch {
      return {}
    }
  },
  toAttribute(value: FooterStatus): string {
    return JSON.stringify(value ?? {})
  },
}

class ReportFooter extends LitElement {
  @property({ attribute: 'status', converter: statusConverter }) status: FooterStatus = {}

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      color: var(--fgColor-default);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    }

    footer {
      display: flex;
      min-height: 32px;
      height: 32px;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      border-top: 1px solid var(--borderColor-muted);
      box-sizing: border-box;
      padding: 0 24px;
    }

    .status {
      display: inline-flex;
      min-width: 0;
      align-items: center;
      gap: 7px;
      color: var(--fgColor-muted);
      font-size: 0.66rem;
      font-weight: 850;
      white-space: nowrap;
    }

    .dot {
      width: 7px;
      height: 7px;
      flex: 0 0 auto;
      border-radius: 999px;
      background: var(--fgColor-success);
    }

    .status.loading .dot {
      background: var(--fgColor-attention);
    }

    .status.error .dot {
      background: var(--fgColor-danger);
    }

    ld-report-zoom {
      flex: 0 0 auto;
    }

    @media (max-width: 860px) {
      footer {
        height: auto;
        min-height: 32px;
        align-items: flex-start;
        flex-direction: column;
        gap: 8px;
        padding-block: 8px;
        padding-inline: 12px;
      }
    }
  `

  render() {
    const statusClass = [
      'status',
      this.status.loading ? 'loading' : '',
      this.status.error ? 'error' : '',
    ].filter(Boolean).join(' ')

    return html`
      <footer part="footer">
        <div class=${statusClass} title=${this.status.error || ''}>
          <span class="dot" aria-hidden="true"></span>
          <span>${this.statusText()}</span>
        </div>
        <ld-report-zoom></ld-report-zoom>
      </footer>
    `
  }

  private statusText(): string {
    if (this.status.error) return 'Refresh failed'
    if (this.status.loading) return 'Refreshing'
    if (this.status.lastUpdated) return `Updated ${this.status.lastUpdated}`
    return 'Not refreshed'
  }
}

customElements.define('ld-report-footer', ReportFooter)
