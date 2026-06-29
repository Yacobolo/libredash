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
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    footer {
      display: flex;
      min-height: var(--control-medium-size);
      height: var(--control-medium-size);
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-12);
      border-top: var(--ld-border-muted);
      box-sizing: border-box;
      padding: 0 calc(var(--base-size-16) + var(--base-size-2));
    }

    .status {
      display: inline-flex;
      min-width: 0;
      align-items: center;
      gap: var(--base-size-8);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      white-space: nowrap;
    }

    .dot {
      width: var(--base-size-6);
      height: var(--base-size-6);
      flex: 0 0 auto;
      border-radius: var(--ld-radius-full);
      background: var(--ld-fg-success);
    }

    .status.loading .dot {
      background: var(--ld-fg-warning);
    }

    .status.error .dot {
      background: var(--ld-fg-danger);
    }

    ld-report-zoom {
      flex: 0 1 auto;
    }

    @media (max-width: 560px) {
      footer {
        height: auto;
        min-height: var(--control-medium-size);
        align-items: flex-start;
        flex-direction: column;
        gap: var(--base-size-8);
        padding-block: var(--base-size-8);
        padding-inline: var(--base-size-12);
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
