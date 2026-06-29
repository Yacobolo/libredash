import { LitElement, html, svg } from 'lit'
import { property, state } from 'lit/decorators.js'

type GridCellTone = 'default' | 'accent' | 'success' | 'attention' | 'danger' | 'muted'
type GridStatusIcon = 'check' | 'x' | 'clock' | 'dot'

type GridCell = {
  label?: string
  value?: string | number
  tone?: GridCellTone
  icon?: GridStatusIcon
}

type GridColumn = {
  id: string
  header: string
  kind?: 'text' | 'code' | 'expression' | 'badge' | 'status' | 'number' | 'link' | 'tags'
  align?: 'left' | 'right'
  hrefKey?: string
  width?: string
}

type GridRow = Record<string, unknown>
type GridSort = { id: string; desc: boolean }

type Grid = {
  columns?: GridColumn[]
  rows?: GridRow[]
  empty?: string
  minWidth?: string
}

const emptyGrid: Required<Grid> = {
  columns: [],
  rows: [],
  empty: 'No rows to show.',
  minWidth: '0',
}

function cellLabel(value: unknown): string {
  if (value == null || value === '') return '-'
  if (typeof value === 'object' && 'label' in value) {
    const label = (value as GridCell).label ?? (value as GridCell).value
    return label == null || label === '' ? '-' : String(label)
  }
  return String(value)
}

function cellTone(value: unknown): GridCellTone {
  if (typeof value === 'object' && value && 'tone' in value) {
    return (value as GridCell).tone ?? 'default'
  }
  return 'default'
}

function statusIcon(value: unknown, label: string): GridStatusIcon {
  if (typeof value === 'object' && value && 'icon' in value) {
    return (value as GridCell).icon ?? 'dot'
  }
  switch (label.toLowerCase()) {
    case 'succeeded':
      return 'check'
    case 'failed':
      return 'x'
    case 'running':
    case 'queued':
      return 'clock'
    default:
      return 'dot'
  }
}

function sortValue(value: unknown): string | number {
  if (typeof value === 'number') return value
  return cellLabel(value).toLowerCase()
}

class DataGrid extends LitElement {
  @property({ attribute: false }) grid: Grid | null = null
  @property({ attribute: 'grid' }) gridAttribute = ''
  @state() private sorting: GridSort[] = []

  createRenderRoot(): HTMLElement {
    return this
  }

  render() {
    const grid = this.resolvedGrid
    const rows = this.sortedRows(grid.rows)

    if (grid.rows.length === 0) {
      return html`
        <style>
          ${dataGridStyles}
        </style>
        <div class="data-grid-empty">${grid.empty}</div>
      `
    }

    return html`
      <style>
        ${dataGridStyles}
      </style>
      <div class="data-grid-wrap">
        <table class="data-grid" style=${grid.minWidth ? `min-width: ${grid.minWidth}` : ''}>
          <thead>
            <tr>
              ${grid.columns.map((column) => {
                const direction = this.sortDirection(column.id)
                return html`
                  <th style=${column.width ? `width: ${column.width}` : ''} class=${column.align === 'right' ? 'is-right' : ''}>
                    <button
                      type="button"
                      class="data-grid-sort"
                      @click=${() => this.toggleSort(column.id)}
                      aria-label=${`Sort by ${column.header}`}
                    >
                      <span>${column.header}</span>
                      <span class="data-grid-sort-indicator" aria-hidden="true">${direction === 'asc' ? '^' : direction === 'desc' ? 'v' : ''}</span>
                    </button>
                  </th>
                `
              })}
            </tr>
          </thead>
          <tbody>
            ${rows.map((row) => html`
              <tr>
                ${grid.columns.map((column) => {
                  return html`<td class=${column.align === 'right' ? 'is-right' : ''}>${this.renderCell(column, row[column.id], row)}</td>`
                })}
              </tr>
            `)}
          </tbody>
        </table>
      </div>
    `
  }

  private get resolvedGrid(): Required<Grid> {
    if (this.grid) return normalizeGrid(this.grid)
    if (this.gridAttribute) {
      try {
        return normalizeGrid(JSON.parse(this.gridAttribute) as Grid)
      } catch {
        // Datastar sets the property in normal operation. Attribute parsing is only a fallback.
      }
    }
    return emptyGrid
  }

  private renderCell(column: GridColumn, value: unknown, row: GridRow) {
    const label = cellLabel(value)
    switch (column.kind) {
      case 'code':
        return html`<code class="grid-code">${label}</code>`
      case 'expression':
        return html`<code class="grid-expression">${label}</code>`
      case 'badge':
        return label === '-' ? html`<span class="grid-muted">-</span>` : html`<span class=${`grid-badge grid-badge-${cellTone(value)}`}>${label}</span>`
      case 'status':
        return label === '-' ? html`<span class="grid-muted">-</span>` : this.renderStatusCell(value, label)
      case 'number':
        return html`<span class="grid-number">${label}</span>`
      case 'link': {
        const href = column.hrefKey ? cellLabel(row[column.hrefKey]) : ''
        return href && href !== '-' ? html`<a class="grid-link" href=${href}>${label}</a>` : html`<span>${label}</span>`
      }
      case 'tags':
        return Array.isArray(value) && value.length > 0
          ? html`<span class="grid-tags">${value.map((tag) => html`<span>${String(tag)}</span>`)}</span>`
          : html`<span class="grid-muted">-</span>`
      default:
        return html`<span>${label}</span>`
    }
  }

  private renderStatusCell(value: unknown, label: string) {
    const tone = cellTone(value)
    const icon = statusIcon(value, label)
    return html`
      <span class=${`grid-status grid-status-${tone}`}>
        <span class="grid-status-icon" aria-hidden="true">${this.renderStatusIcon(icon)}</span>
        <span>${label}</span>
      </span>
    `
  }

  private renderStatusIcon(icon: GridStatusIcon) {
    switch (icon) {
      case 'check':
        return svg`
          <svg viewBox="0 0 16 16" focusable="false">
            <path fill="currentColor" fill-rule="evenodd" clip-rule="evenodd" d="M8 16A8 8 0 1 0 8 0a8 8 0 0 0 0 16Zm-1.4-4.9L3.8 8.3 5 7.1l1.6 1.6L11 4.3l1.2 1.2-5.6 5.6Z"></path>
          </svg>
        `
      case 'x':
        return svg`
          <svg viewBox="0 0 16 16" focusable="false">
            <path fill="currentColor" fill-rule="evenodd" clip-rule="evenodd" d="M8 16A8 8 0 1 0 8 0a8 8 0 0 0 0 16ZM5.1 4 8 6.9 10.9 4 12 5.1 9.1 8l2.9 2.9-1.1 1.1L8 9.1 5.1 12 4 10.9 6.9 8 4 5.1 5.1 4Z"></path>
          </svg>
        `
      case 'clock':
        return svg`
          <svg viewBox="0 0 16 16" focusable="false">
            <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" stroke-width="2"></circle>
            <path d="M8 4.5v3.8l2.6 1.5" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.6"></path>
          </svg>
        `
      default:
        return svg`
          <svg viewBox="0 0 16 16" focusable="false">
            <circle cx="8" cy="8" r="4" fill="currentColor"></circle>
          </svg>
        `
    }
  }

  private sortDirection(columnID: string): false | 'asc' | 'desc' {
    const sort = this.sorting.find((item) => item.id === columnID)
    if (!sort) return false
    return sort.desc ? 'desc' : 'asc'
  }

  private toggleSort(columnID: string): void {
    const direction = this.sortDirection(columnID)
    if (!direction) {
      this.sorting = [{ id: columnID, desc: false }]
      return
    }
    if (direction === 'asc') {
      this.sorting = [{ id: columnID, desc: true }]
      return
    }
    this.sorting = []
  }

  private sortedRows(rows: GridRow[]): GridRow[] {
    const sort = this.sorting[0]
    if (!sort) return rows
    return [...rows].sort((left, right) => {
      const leftValue = sortValue(left[sort.id])
      const rightValue = sortValue(right[sort.id])
      const result = typeof leftValue === 'number' && typeof rightValue === 'number'
        ? leftValue - rightValue
        : String(leftValue).localeCompare(String(rightValue), undefined, { numeric: true })
      return sort.desc ? -result : result
    })
  }
}

function normalizeGrid(grid: Grid): Required<Grid> {
  return {
    columns: grid.columns ?? [],
    rows: grid.rows ?? [],
    empty: grid.empty ?? emptyGrid.empty,
    minWidth: grid.minWidth ?? emptyGrid.minWidth,
  }
}

const dataGridStyles = `
  ld-data-grid {
    display: block;
    min-width: 0;
    max-width: 100%;
  }

  ld-data-grid .data-grid-wrap {
    width: 100%;
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    border-top: var(--ld-border-muted);
    border-bottom: var(--ld-border-muted);
  }

  ld-data-grid .data-grid {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
  }

  ld-data-grid .data-grid th,
  ld-data-grid .data-grid td {
    border-bottom: var(--borderWidth-default) solid color-mix(in srgb, var(--ld-line-muted), transparent 28%);
    padding: var(--base-size-8);
    text-align: left;
    vertical-align: top;
  }

  ld-data-grid .data-grid th {
    background: transparent;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    letter-spacing: 0.03em;
    text-transform: uppercase;
  }

  ld-data-grid .data-grid td {
    color: var(--ld-fg-default);
    font-size: var(--ld-font-size-body-md);
    line-height: var(--ld-line-height-normal);
    font-weight: var(--ld-font-weight-regular);
  }

  ld-data-grid .data-grid th.is-right,
  ld-data-grid .data-grid td.is-right {
    text-align: right;
  }

  ld-data-grid .data-grid tbody tr:last-child td {
    border-bottom: 0;
  }

  ld-data-grid .data-grid tbody tr {
    transition: background-color var(--motion-transition-hover);
  }

  ld-data-grid .data-grid tbody tr:hover {
    background: var(--ld-bg-hover);
  }

  ld-data-grid .data-grid-sort {
    display: inline-flex;
    width: 100%;
    min-width: 0;
    align-items: center;
    justify-content: space-between;
    gap: var(--base-size-6);
    border: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    padding: 0;
    font: inherit;
    letter-spacing: inherit;
    text-align: inherit;
    text-transform: inherit;
  }

  ld-data-grid .data-grid-sort:hover,
  ld-data-grid .data-grid-sort:focus-visible {
    color: var(--ld-fg-default);
    outline: 0;
  }

  ld-data-grid .data-grid-sort-indicator {
    min-width: var(--base-size-8);
    color: var(--ld-fg-link);
    text-align: right;
  }

  ld-data-grid .grid-code,
  ld-data-grid .grid-expression {
    font-family: var(--fontStack-monospace);
  }

  ld-data-grid .grid-code {
    display: inline-flex;
    max-width: 100%;
    overflow: hidden;
    color: var(--ld-fg-default);
    padding: 0;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-medium);
  }

  ld-data-grid .grid-expression {
    display: block;
    overflow: hidden;
    color: var(--ld-fg-default);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-regular);
  }

  ld-data-grid .grid-badge {
    display: inline-flex;
    min-height: var(--base-size-20);
    align-items: center;
    gap: var(--base-size-4);
    border-radius: var(--borderRadius-full);
    padding: 0 var(--base-size-8);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
  }

  ld-data-grid .grid-badge-success {
    border: var(--borderWidth-default) solid var(--ld-line-success-muted);
    background: var(--ld-bg-success-muted);
    color: var(--ld-fg-default);
  }

  ld-data-grid .grid-badge-accent {
    border: var(--borderWidth-default) solid var(--ld-line-accent-muted);
    background: var(--ld-bg-accent-muted);
    color: var(--ld-fg-default);
  }

  ld-data-grid .grid-badge-attention {
    border: var(--borderWidth-default) solid var(--ld-line-warning-muted);
    background: var(--ld-bg-warning-muted);
    color: var(--ld-fg-default);
  }

  ld-data-grid .grid-badge-muted,
  ld-data-grid .grid-badge-default {
    border: var(--ld-border-muted);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
  }

  ld-data-grid .grid-status {
    display: inline-flex;
    align-items: center;
    gap: var(--base-size-6);
    color: var(--ld-fg-default);
    font-weight: var(--ld-font-weight-medium);
    white-space: nowrap;
  }

  ld-data-grid .grid-status-icon {
    display: inline-flex;
    width: var(--base-size-16);
    height: var(--base-size-16);
    flex: none;
    align-items: center;
    justify-content: center;
    color: var(--ld-fg-muted);
  }

  ld-data-grid .grid-status-icon svg {
    display: block;
    width: var(--base-size-16);
    height: var(--base-size-16);
  }

  ld-data-grid .grid-status-success .grid-status-icon {
    color: var(--ld-fg-success);
  }

  ld-data-grid .grid-status-danger .grid-status-icon {
    color: var(--ld-fg-danger);
  }

  ld-data-grid .grid-status-attention .grid-status-icon {
    color: var(--ld-fg-warning);
  }

  ld-data-grid .grid-status-accent .grid-status-icon {
    color: var(--ld-fg-link);
  }

  ld-data-grid .grid-status-muted .grid-status-icon,
  ld-data-grid .grid-status-default .grid-status-icon {
    color: var(--ld-fg-muted);
  }

  ld-data-grid .grid-number {
    font-variant-numeric: tabular-nums;
  }

  ld-data-grid .grid-link {
    color: var(--ld-fg-link);
    font-weight: var(--ld-font-weight-medium);
    text-decoration: none;
  }

  ld-data-grid .grid-link:hover,
  ld-data-grid .grid-link:focus-visible {
    text-decoration: underline;
    outline: 0;
  }

  ld-data-grid .grid-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--base-size-4);
  }

  ld-data-grid .grid-tags span {
    display: inline-flex;
    min-height: var(--base-size-20);
    align-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--borderRadius-full);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
    padding: 0 var(--base-size-8);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    text-transform: uppercase;
  }

  ld-data-grid .grid-muted,
  ld-data-grid .data-grid-empty {
    color: var(--ld-fg-muted);
  }

  ld-data-grid .data-grid-empty {
    border-top: var(--ld-border-muted);
    border-bottom: var(--ld-border-muted);
    padding: var(--base-size-20) 0;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-regular);
  }
`

customElements.define('ld-data-grid', DataGrid)
