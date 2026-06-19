import { LitElement, css, html, nothing } from 'lit'
import { state } from 'lit/decorators.js'

type VisualActionName = 'focus' | 'show-data' | 'copy-data' | 'export-csv' | 'clear-selection'

type VisualColumn = {
  key: string
  label: string
  align?: 'left' | 'right'
}

type VisualRow = Record<string, unknown>

type VisualActionDetail = {
  action: VisualActionName
  visualType: 'chart' | 'table'
  visualId: string
  title: string
  columns: VisualColumn[]
  rows: VisualRow[]
  selection: string[]
  chart?: Record<string, unknown>
  table?: Record<string, unknown>
}

type ModalMode = 'focus' | 'show-data'

class VisualModal extends LitElement {
  @state() private mode: ModalMode | '' = ''
  @state() private detail: VisualActionDetail | null = null
  @state() private notice = ''

  static styles = css`
    :host {
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    .backdrop {
      position: fixed;
      inset: 0;
      z-index: 80;
      display: grid;
      place-items: center;
      background: var(--ld-modal-backdrop);
      padding: var(--base-size-28);
    }

    .dialog {
      display: grid;
      width: min(1120px, 100%);
      max-height: min(760px, calc(100vh - 56px));
      min-height: 420px;
      grid-template-rows: auto minmax(0, 1fr);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-panel);
      background: var(--ld-bg-overlay);
      box-shadow: var(--ld-shadow-floating-lg);
      overflow: hidden;
    }

    header {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: var(--ld-space-lg);
      border-bottom: var(--ld-border-default);
      padding: var(--ld-space-md) var(--ld-space-lg);
    }

    .title {
      min-width: 0;
    }

    .eyebrow {
      margin: 0 0 var(--borderRadius-small);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
      text-transform: uppercase;
    }

    h2 {
      margin: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-lg);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .actions {
      display: flex;
      flex: 0 0 auto;
      align-items: center;
      gap: var(--ld-space-sm);
    }

    button {
      min-height: var(--ld-control-small);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0 var(--ld-space-md);
      font: inherit;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    button:hover,
    button:focus-visible {
      background: var(--ld-bg-control-hover);
      outline: 0;
    }

    .close {
      width: calc(var(--ld-control-small) + var(--base-size-2));
      padding: 0;
      font-size: var(--ld-font-size-body-lg);
      line-height: var(--ld-line-height-none);
    }

    .body {
      min-height: 0;
      overflow: hidden;
      background: var(--ld-chart-surface);
    }

    .focus-chart,
    .focus-table {
      height: 100%;
      min-height: 0;
    }

    .data-shell {
      display: grid;
      height: 100%;
      min-height: 0;
      grid-template-rows: auto minmax(0, 1fr);
    }

    .data-summary {
      border-bottom: var(--ld-border-default);
      color: var(--ld-fg-muted);
      padding: var(--ld-space-md) var(--ld-space-lg);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .data-scroll {
      min-height: 0;
      overflow: auto;
    }

    table {
      width: 100%;
      border-collapse: collapse;
      font-size: var(--ld-font-size-body-md);
    }

    th,
    td {
      max-width: 21.25rem;
      border-bottom: var(--ld-border-muted);
      padding: var(--ld-space-md) var(--ld-space-lg);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      text-align: left;
    }

    th {
      position: sticky;
      top: 0;
      z-index: 1;
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      text-transform: uppercase;
    }

    td.right,
    th.right {
      text-align: right;
      font-variant-numeric: tabular-nums;
    }

    .empty {
      display: grid;
      height: 100%;
      place-items: center;
      color: var(--ld-fg-muted);
      font-weight: var(--ld-font-weight-strong);
    }

    .notice {
      position: fixed;
      right: calc(var(--base-size-16) + var(--base-size-2));
      bottom: calc(var(--base-size-48) + var(--base-size-2));
      z-index: 90;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-overlay);
      box-shadow: var(--ld-shadow-floating-sm);
      color: var(--ld-fg-default);
      padding: var(--ld-space-md) var(--ld-space-lg);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    window.addEventListener('ld-visual-action', this.handleVisualAction as EventListener)
    window.addEventListener('keydown', this.handleKeydown)
  }

  disconnectedCallback(): void {
    window.removeEventListener('ld-visual-action', this.handleVisualAction as EventListener)
    window.removeEventListener('keydown', this.handleKeydown)
    super.disconnectedCallback()
  }

  render() {
    return html`
      ${this.detail && this.mode ? this.renderDialog(this.detail, this.mode) : nothing}
      ${this.notice ? html`<div class="notice" role="status">${this.notice}</div>` : nothing}
    `
  }

  private renderDialog(detail: VisualActionDetail, mode: ModalMode) {
    return html`
      <div class="backdrop" @click=${this.closeFromBackdrop}>
        <section class="dialog" role="dialog" aria-modal="true" aria-label=${detail.title}>
          <header>
            <div class="title">
              <p class="eyebrow">${mode === 'focus' ? 'Focus mode' : 'Show data'} · ${detail.visualType}</p>
              <h2>${detail.title}</h2>
            </div>
            <div class="actions">
              <button type="button" @click=${() => this.copy(detail)}>Copy</button>
              <button type="button" @click=${() => this.exportCSV(detail)}>Export CSV</button>
              <button class="close" type="button" aria-label="Close visual modal" @click=${this.close}>×</button>
            </div>
          </header>
          <div class="body">
            ${mode === 'focus' ? this.renderFocus(detail) : this.renderData(detail)}
          </div>
        </section>
      </div>
    `
  }

  private renderFocus(detail: VisualActionDetail) {
    if (detail.visualType === 'chart') {
      const chart = detail.chart ?? chartPayloadFromRows(detail)
      return html`<ld-echart class="focus-chart" .chart=${chart}></ld-echart>`
    }
    return html`<div class="focus-table">${this.renderData(detail)}</div>`
  }

  private renderData(detail: VisualActionDetail) {
    const columns = detail.columns ?? []
    const rows = detail.rows ?? []
    if (columns.length === 0 || rows.length === 0) return html`<div class="empty">No visual data</div>`
    return html`
      <div class="data-shell">
        <div class="data-summary">${rows.length.toLocaleString()} row${rows.length === 1 ? '' : 's'} from current visual data</div>
        <div class="data-scroll">
          <table>
            <thead>
              <tr>
                ${columns.map((column) => html`<th class=${column.align === 'right' ? 'right' : ''}>${column.label}</th>`)}
              </tr>
            </thead>
            <tbody>
              ${rows.map((row) => html`
                <tr>
                  ${columns.map((column) => html`<td class=${column.align === 'right' ? 'right' : ''} title=${stringValue(row[column.key])}>${stringValue(row[column.key])}</td>`)}
                </tr>
              `)}
            </tbody>
          </table>
        </div>
      </div>
    `
  }

  private handleVisualAction = (event: CustomEvent<VisualActionDetail>): void => {
    const detail = event.detail
    if (!detail || detail.action === 'clear-selection') return
    if (detail.action === 'copy-data') {
      void this.copy(detail)
      return
    }
    if (detail.action === 'export-csv') {
      this.exportCSV(detail)
      return
    }
    if (detail.action === 'focus' || detail.action === 'show-data') {
      this.detail = detail
      this.mode = detail.action === 'focus' ? 'focus' : 'show-data'
    }
  }

  private handleKeydown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape') this.close()
  }

  private closeFromBackdrop = (event: Event): void => {
    if (event.target === event.currentTarget) this.close()
  }

  private close = (): void => {
    this.mode = ''
    this.detail = null
  }

  private async copy(detail: VisualActionDetail): Promise<void> {
    const text = toDelimited(detail, '\t')
    try {
      await navigator.clipboard.writeText(text)
      this.flash('Copied visual data')
    } catch {
      this.fallbackCopy(text)
      this.flash('Copied visual data')
    }
  }

  private fallbackCopy(text: string): void {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.append(area)
    area.select()
    document.execCommand('copy')
    area.remove()
  }

  private exportCSV(detail: VisualActionDetail): void {
    const blob = new Blob([toDelimited(detail, ',')], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${slug(detail.title || detail.visualId || 'visual')}.csv`
    document.body.append(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    this.flash('Downloaded CSV')
  }

  private flash(message: string): void {
    this.notice = message
    window.setTimeout(() => {
      if (this.notice === message) this.notice = ''
    }, 1800)
  }
}

function chartPayloadFromRows(detail: VisualActionDetail): Record<string, unknown> {
  return {
    id: detail.visualId,
    title: detail.title,
    type: 'bar',
    data: detail.rows.map((row) => ({
      label: stringValue(row.label),
      series: stringValue(row.series),
      value: Number(row.value) || 0,
    })),
  }
}

function toDelimited(detail: VisualActionDetail, delimiter: ',' | '\t'): string {
  const columns = detail.columns ?? []
  const rows = detail.rows ?? []
  return [
    columns.map((column) => escapeCell(column.label, delimiter)).join(delimiter),
    ...rows.map((row) => columns.map((column) => escapeCell(row[column.key], delimiter)).join(delimiter)),
  ].join('\n')
}

function escapeCell(value: unknown, delimiter: ',' | '\t'): string {
  const text = stringValue(value)
  if (delimiter === '\t') return text.replace(/\t/g, ' ').replace(/\r?\n/g, ' ')
  if (!/[",\r\n]/.test(text)) return text
  return `"${text.replace(/"/g, '""')}"`
}

function stringValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  return String(value)
}

function slug(value: string): string {
  const normalized = value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  return normalized || 'visual'
}

customElements.define('ld-visual-modal', VisualModal)
