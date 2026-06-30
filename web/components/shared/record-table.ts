import { LitElement, html, svg, nothing, type TemplateResult } from 'lit'
import { property, state } from 'lit/decorators.js'
import {
  ArrowUpDown,
  BarChart3,
  Boxes,
  Braces,
  CheckCircle2,
  Clock3,
  Database,
  ExternalLink,
  FileText,
  Filter,
  Gauge,
  KeyRound,
  Layers3,
  ListTree,
  Server,
  Sigma,
  Table2,
  Waves,
  XCircle,
} from 'lucide'
import {
  TableController,
  createCoreRowModel,
  createSortedRowModel,
  rowSortingFeature,
  tableFeatures,
  type ColumnDef,
  type SortingState,
} from '@tanstack/lit-table'
import { lucideIcon } from './lucide-icons'

type RecordCellTone = 'default' | 'accent' | 'success' | 'attention' | 'danger' | 'muted'
type RecordStatusIcon = 'check' | 'x' | 'clock' | 'dot'

type RecordCell = {
  label?: string
  value?: string | number
  description?: string
  href?: string
  icon?: string
  tone?: RecordCellTone
  action?: string
}

type RecordAction = {
  label: string
  href?: string
  icon?: string
  action?: string
  disabled?: boolean
}

type RecordColumn = {
  id: string
  header: string
  kind?: 'text' | 'code' | 'expression' | 'badge' | 'status' | 'number' | 'link' | 'tags' | 'entity' | 'button' | 'actions'
  align?: 'left' | 'right'
  hrefKey?: string
  width?: string
  sortable?: boolean
}

type RecordRow = Record<string, unknown>
type RecordTablePayload = {
  columns?: RecordColumn[]
  rows?: RecordRow[]
  empty?: string
  minWidth?: string
}

type RecordTableVariant = 'minimal' | 'primary' | 'compact'

const recordTableFeatures = tableFeatures({ rowSortingFeature, sortedRowModel: createSortedRowModel() })

const emptyRecordTable: Required<RecordTablePayload> = {
  columns: [],
  rows: [],
  empty: 'No rows to show.',
  minWidth: '0',
}

function cellLabel(value: unknown): string {
  if (value == null || value === '') return '-'
  if (typeof value === 'object' && 'label' in value) {
    const label = (value as RecordCell).label ?? (value as RecordCell).value
    return label == null || label === '' ? '-' : String(label)
  }
  return String(value)
}

function cellDescription(value: unknown): string {
  return typeof value === 'object' && value && 'description' in value ? String((value as RecordCell).description ?? '') : ''
}

function cellHref(column: RecordColumn, value: unknown, row: RecordRow): string {
  if (typeof value === 'object' && value && 'href' in value) return String((value as RecordCell).href ?? '')
  return column.hrefKey ? cellLabel(row[column.hrefKey]) : ''
}

function cellIcon(value: unknown): string {
  return typeof value === 'object' && value && 'icon' in value ? String((value as RecordCell).icon ?? '') : ''
}

function cellTone(value: unknown): RecordCellTone {
  if (typeof value === 'object' && value && 'tone' in value) {
    return (value as RecordCell).tone ?? 'default'
  }
  return 'default'
}

function cellAction(value: unknown): string {
  return typeof value === 'object' && value && 'action' in value ? String((value as RecordCell).action ?? '') : ''
}

function statusIcon(value: unknown, label: string): RecordStatusIcon {
  if (typeof value === 'object' && value && 'icon' in value) {
    return ((value as RecordCell).icon as RecordStatusIcon | undefined) ?? 'dot'
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

function sortPrimitive(value: unknown): string | number {
  if (typeof value === 'number') return value
  return cellLabel(value).toLowerCase()
}

function normalizeTable(table: RecordTablePayload): Required<RecordTablePayload> {
  return {
    columns: table.columns ?? [],
    rows: table.rows ?? [],
    empty: table.empty ?? emptyRecordTable.empty,
    minWidth: table.minWidth ?? emptyRecordTable.minWidth,
  }
}

function applyUpdater<T>(updater: unknown, current: T): T {
  return typeof updater === 'function' ? (updater as (old: T) => T)(current) : updater as T
}

function columnAlignClass(column: RecordColumn): string {
  return column.align === 'right' || column.kind === 'number' ? 'is-right' : ''
}

class RecordTable extends LitElement {
  @property({ attribute: false }) table: RecordTablePayload | null = null
  @property({ attribute: 'table' }) tableAttribute = ''
  @property() variant: RecordTableVariant = 'minimal'
  @state() private sorting: SortingState = []

  private tableController = new TableController<typeof recordTableFeatures, RecordRow>(this)

  createRenderRoot(): HTMLElement {
    return this
  }

  render() {
    const table = this.resolvedTable
    const model = this.tanstackTable(table)
    const rows = model.getRowModel().rows.map((row: any) => row.original as RecordRow)

    if (table.rows.length === 0) {
      return html`
        <style>${recordTableStyles}</style>
        <div class="record-table-empty">${table.empty}</div>
      `
    }

    return html`
      <style>${recordTableStyles}</style>
      <div class=${`record-table-wrap variant-${this.variant}`}>
        <table class="record-table" style=${table.minWidth ? `min-width: ${table.minWidth}` : ''}>
          <thead>
            <tr>
              ${table.columns.map((column) => {
                const direction = this.sortDirection(column.id)
                const sortable = column.sortable !== false && column.kind !== 'actions'
                return html`
                  <th style=${column.width ? `width: ${column.width}` : ''} class=${columnAlignClass(column)}>
                    <button
                      type="button"
                      class="record-table-sort"
                      aria-label=${`Sort by ${column.header}`}
                      aria-sort=${direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : 'none'}
                      ?disabled=${!sortable}
                      @click=${() => sortable ? this.toggleSort(column.id) : undefined}
                    >
                      <span>${column.header}</span>
                      <span class=${direction ? 'record-table-sort-indicator is-active' : 'record-table-sort-indicator'} aria-hidden="true">${sortable ? this.sortIndicator(direction) : nothing}</span>
                    </button>
                  </th>
                `
              })}
            </tr>
          </thead>
          <tbody>
            ${rows.map((row) => html`
              <tr>
                ${table.columns.map((column) => html`
                  <td class=${columnAlignClass(column)}>
                    ${this.renderCell(column, row[column.id], row)}
                  </td>
                `)}
              </tr>
            `)}
          </tbody>
        </table>
      </div>
    `
  }

  private get resolvedTable(): Required<RecordTablePayload> {
    if (this.table) return normalizeTable(this.table)
    if (this.tableAttribute) {
      try {
        return normalizeTable(JSON.parse(this.tableAttribute) as RecordTablePayload)
      } catch {
        // Datastar sets the property in normal operation. Attribute parsing is only a fallback.
      }
    }
    return emptyRecordTable
  }

  private tanstackTable(table: Required<RecordTablePayload>) {
    return this.tableController.table({
      features: recordTableFeatures,
      columns: this.columnDefs(table.columns),
      data: table.rows,
      getCoreRowModel: createCoreRowModel(),
      enableSorting: true,
      renderFallbackValue: '-',
      state: { sorting: this.sorting },
      onSortingChange: (updater: unknown) => {
        this.sorting = applyUpdater(updater, this.sorting)
      },
    } as any) as any
  }

  private columnDefs(columns: RecordColumn[]): Array<ColumnDef<RecordRow, unknown>> {
    return columns.map((column) => ({
      id: column.id,
      accessorFn: (row: RecordRow) => row[column.id],
      header: column.header,
      cell: (info: any) => this.renderCell(column, info.getValue(), info.row.original),
      enableSorting: column.sortable !== false && column.kind !== 'actions',
      sortingFn: (left: any, right: any, columnID: string) => {
        const leftValue = sortPrimitive(left.original[columnID])
        const rightValue = sortPrimitive(right.original[columnID])
        return typeof leftValue === 'number' && typeof rightValue === 'number'
          ? leftValue - rightValue
          : String(leftValue).localeCompare(String(rightValue), undefined, { numeric: true })
      },
      meta: { column },
    })) as Array<ColumnDef<RecordRow, unknown>>
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

  private renderCell(column: RecordColumn, value: unknown, row: RecordRow): TemplateResult | string {
    const label = cellLabel(value)
    switch (column.kind) {
      case 'code':
        return label === '-' ? html`<span class="record-muted">-</span>` : html`<code class="record-code">${label}</code>`
      case 'expression':
        return label === '-' ? html`<span class="record-muted">-</span>` : html`<code class="record-expression">${label}</code>`
      case 'badge':
        return label === '-' ? html`<span class="record-muted">-</span>` : html`<span class=${`record-badge record-badge-${cellTone(value)}`}>${label}</span>`
      case 'status':
        return label === '-' ? html`<span class="record-muted">-</span>` : this.renderStatusCell(value, label)
      case 'number':
        return label === '-' ? html`<span class="record-muted">-</span>` : html`<span class="record-number">${label}</span>`
      case 'link':
        return this.renderLink(column, value, row)
      case 'tags':
        return Array.isArray(value) && value.length > 0
          ? html`<span class="record-tags">${value.map((tag) => html`<span>${String(tag)}</span>`)}</span>`
          : html`<span class="record-muted">-</span>`
      case 'entity':
        return this.renderEntity(column, value, row)
      case 'button':
        return this.renderButton(column, value, row)
      case 'actions':
        return this.renderActions(value, row)
      default:
        return label === '-' ? html`<span class="record-muted">-</span>` : html`<span>${label}</span>`
    }
  }

  private renderLink(column: RecordColumn, value: unknown, row: RecordRow) {
    const href = cellHref(column, value, row)
    const label = cellLabel(value)
    if (label === '-') return html`<span class="record-muted">-</span>`
    return href && href !== '-' ? html`<a class="record-link" href=${href}>${label}</a>` : html`<span>${label}</span>`
  }

  private renderEntity(column: RecordColumn, value: unknown, row: RecordRow) {
    const href = cellHref(column, value, row)
    const label = cellLabel(value)
    const description = cellDescription(value)
    const content = html`
      <span class="record-entity">
        ${this.renderIcon(cellIcon(value), `record-entity-icon record-icon-${iconToken(cellIcon(value))}`)}
        <span class="record-entity-copy">
          <span class="record-entity-label">${label}</span>
          ${description ? html`<span class="record-entity-description">${description}</span>` : nothing}
        </span>
      </span>
    `
    return href && href !== '-' ? html`<a class="record-entity-link" href=${href}>${content}</a>` : content
  }

  private renderButton(column: RecordColumn, value: unknown, row: RecordRow) {
    const label = cellLabel(value)
    const action = cellAction(value)
    return html`
      <button type="button" class="record-button-cell" @click=${() => this.emitAction(action || column.id, row)}>
        ${this.renderIcon(cellIcon(value), 'record-button-icon')}
        <span>${label}</span>
      </button>
    `
  }

  private renderActions(value: unknown, row: RecordRow) {
    const actions = Array.isArray(value) ? value as RecordAction[] : []
    return html`
      <span class="record-actions">
        ${actions.map((action) => action.href
          ? html`
            <a class="record-icon-action" href=${action.href} title=${action.label} aria-label=${action.label}>
              ${this.renderIcon(action.icon || 'external', '')}
            </a>
          `
          : html`
            <button
              type="button"
              class="record-icon-action"
              title=${action.label}
              aria-label=${action.label}
              ?disabled=${action.disabled}
              @click=${() => this.emitAction(action.action || action.label, row)}
            >
              ${this.renderIcon(action.icon || 'external', '')}
            </button>
          `)}
      </span>
    `
  }

  private renderStatusCell(value: unknown, label: string) {
    const tone = cellTone(value)
    const icon = statusIcon(value, label)
    return html`
      <span class=${`record-status record-status-${tone}`}>
        <span class="record-status-icon" aria-hidden="true">${this.renderStatusIcon(icon)}</span>
        <span>${label}</span>
      </span>
    `
  }

  private renderStatusIcon(icon: RecordStatusIcon) {
    switch (icon) {
      case 'check':
        return lucideIcon(CheckCircle2, { size: 16, strokeWidth: 2 })
      case 'x':
        return lucideIcon(XCircle, { size: 16, strokeWidth: 2 })
      case 'clock':
        return lucideIcon(Clock3, { size: 16, strokeWidth: 2 })
      default:
        return svg`<svg viewBox="0 0 16 16" focusable="false"><circle cx="8" cy="8" r="4" fill="currentColor"></circle></svg>`
    }
  }

  private renderIcon(name: string, className: string): TemplateResult {
    const icon = iconForName(name)
    return html`<span class=${className} aria-hidden="true">${lucideIcon(icon, { size: 16, strokeWidth: 1.75 })}</span>`
  }

  private emitAction(action: string, row: RecordRow): void {
    this.dispatchEvent(new CustomEvent('ld-record-table-action', {
      bubbles: true,
      composed: true,
      detail: { action, row },
    }))
  }

  private sortIndicator(direction: false | 'asc' | 'desc'): TemplateResult {
    if (direction === 'asc') return html`<span>↑</span>`
    if (direction === 'desc') return html`<span>↓</span>`
    return lucideIcon(ArrowUpDown, { size: 12, strokeWidth: 2 })
  }
}

function iconForName(name: string): any {
  switch (name) {
    case 'catalog':
    case 'connection':
    case 'database':
      return Database
    case 'dashboard':
      return BarChart3
    case 'model_table':
    case 'semantic_table':
    case 'table':
      return Table2
    case 'semantic_model':
      return Boxes
    case 'source':
    case 'schema':
      return Server
    case 'field':
      return KeyRound
    case 'measure':
      return Sigma
    case 'filter':
      return Filter
    case 'visual':
      return Gauge
    case 'page':
      return FileText
    case 'relationship':
    case 'lineage':
      return ListTree
    case 'view':
      return Waves
    case 'code':
      return Braces
    case 'open':
    case 'external':
      return ExternalLink
    case 'details':
      return FileText
    default:
      return Layers3
  }
}

function iconToken(name: string): string {
  return String(name || 'default').toLowerCase().replace(/[^a-z0-9_-]/g, '-')
}

const recordTableStyles = `
  ld-record-table {
    display: block;
    min-width: 0;
    max-width: 100%;
  }

  ld-record-table .record-table-wrap {
    width: 100%;
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    border-top: var(--ld-border-muted);
    border-bottom: var(--ld-border-muted);
  }

  ld-record-table .record-table-wrap.variant-primary,
  ld-record-table .record-table-wrap.variant-compact {
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel);
  }

  ld-record-table .record-table {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
  }

  ld-record-table .record-table th,
  ld-record-table .record-table td {
    border-bottom: var(--borderWidth-default, 1px) solid color-mix(in srgb, var(--ld-line-muted), transparent 28%);
    padding: var(--base-size-8);
    text-align: left;
    vertical-align: top;
  }

  ld-record-table .record-table th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--ld-bg-panel);
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    letter-spacing: 0.03em;
    text-transform: uppercase;
  }

  ld-record-table .variant-primary .record-table th,
  ld-record-table .variant-compact .record-table th {
    padding: var(--base-size-12) var(--base-size-16);
    background: var(--ld-bg-panel-muted);
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-strong);
  }

  ld-record-table .variant-compact .record-table th {
    padding: var(--base-size-8) var(--base-size-12);
    font-size: var(--ld-font-size-caption);
  }

  ld-record-table .record-table td {
    color: var(--ld-fg-default);
    font-size: var(--ld-font-size-body-md);
    line-height: var(--ld-line-height-normal);
    font-weight: var(--ld-font-weight-regular);
  }

  ld-record-table .variant-primary .record-table td {
    padding: var(--base-size-12) var(--base-size-16);
    font-size: var(--ld-font-size-body-md);
    vertical-align: top;
  }

  ld-record-table .variant-compact .record-table td {
    padding: var(--base-size-8) var(--base-size-12);
    font-size: var(--ld-font-size-body-sm);
    vertical-align: middle;
  }

  ld-record-table .variant-primary .record-table tbody tr {
    min-height: 4rem;
  }

  ld-record-table .record-table th.is-right,
  ld-record-table .record-table td.is-right {
    text-align: right;
  }

  ld-record-table .record-table tbody tr:last-child td {
    border-bottom: 0;
  }

  ld-record-table .record-table tbody tr {
    transition: background-color var(--motion-transition-hover, 120ms ease);
  }

  ld-record-table .record-table tbody tr:hover {
    background: var(--ld-bg-hover, var(--ld-bg-panel-muted));
  }

  ld-record-table .variant-primary .record-table tbody tr:hover,
  ld-record-table .variant-compact .record-table tbody tr:hover {
    background: color-mix(in srgb, var(--ld-bg-panel-muted), transparent 35%);
  }

  ld-record-table .record-table-sort {
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

  ld-record-table .record-table-sort:hover,
  ld-record-table .record-table-sort:focus-visible {
    color: var(--ld-fg-default);
    outline: 0;
  }

  ld-record-table .record-table-sort-indicator {
    display: inline-flex;
    min-width: var(--base-size-16);
    justify-content: flex-end;
    color: var(--ld-fg-muted);
    opacity: 0;
  }

  ld-record-table .record-table-sort:hover .record-table-sort-indicator,
  ld-record-table .record-table-sort:focus-visible .record-table-sort-indicator,
  ld-record-table .record-table-sort-indicator.is-active {
    opacity: 1;
  }

  ld-record-table .record-code,
  ld-record-table .record-expression {
    font-family: var(--fontStack-monospace);
  }

  ld-record-table .record-code {
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

  ld-record-table .record-expression {
    display: block;
    overflow: hidden;
    color: var(--ld-fg-default);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-regular);
  }

  ld-record-table .record-badge {
    display: inline-flex;
    min-height: var(--base-size-20);
    align-items: center;
    gap: var(--base-size-4);
    border-radius: var(--borderRadius-full, var(--ld-radius-full));
    padding: 0 var(--base-size-8);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
  }

  ld-record-table .record-badge-success {
    border: var(--borderWidth-default, 1px) solid var(--ld-line-success-muted, var(--ld-line-muted));
    background: var(--ld-bg-success-muted, var(--ld-bg-panel-muted));
    color: var(--ld-fg-default);
  }

  ld-record-table .record-badge-accent {
    border: var(--borderWidth-default, 1px) solid var(--ld-line-accent-muted, var(--ld-line-muted));
    background: var(--ld-bg-accent-muted, var(--ld-bg-panel-muted));
    color: var(--ld-fg-default);
  }

  ld-record-table .record-badge-attention {
    border: var(--borderWidth-default, 1px) solid var(--ld-line-warning-muted, var(--ld-line-muted));
    background: var(--ld-bg-warning-muted, var(--ld-bg-panel-muted));
    color: var(--ld-fg-default);
  }

  ld-record-table .record-badge-muted,
  ld-record-table .record-badge-default {
    border: var(--ld-border-muted);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
  }

  ld-record-table .record-status {
    display: inline-flex;
    align-items: center;
    gap: var(--base-size-6);
    color: var(--ld-fg-default);
    font-weight: var(--ld-font-weight-medium);
    white-space: nowrap;
  }

  ld-record-table .record-status-icon {
    display: inline-flex;
    width: var(--base-size-16);
    height: var(--base-size-16);
    flex: none;
    align-items: center;
    justify-content: center;
    color: var(--ld-fg-muted);
  }

  ld-record-table .record-status-icon svg,
  ld-record-table .record-entity-icon svg,
  ld-record-table .record-button-icon svg,
  ld-record-table .record-icon-action svg {
    display: block;
    width: var(--base-size-16);
    height: var(--base-size-16);
  }

  ld-record-table .record-status-success .record-status-icon {
    color: var(--ld-fg-success);
  }

  ld-record-table .record-status-danger .record-status-icon {
    color: var(--ld-fg-danger);
  }

  ld-record-table .record-status-attention .record-status-icon {
    color: var(--ld-fg-warning);
  }

  ld-record-table .record-status-accent .record-status-icon {
    color: var(--ld-fg-link);
  }

  ld-record-table .record-number {
    font-variant-numeric: tabular-nums;
  }

  ld-record-table .record-link,
  ld-record-table .record-entity-link {
    color: var(--ld-fg-link);
    font-weight: var(--ld-font-weight-medium);
    text-decoration: none;
  }

  ld-record-table .record-entity-link {
    display: grid;
    width: 100%;
    max-width: 100%;
  }

  ld-record-table .record-link:hover,
  ld-record-table .record-link:focus-visible,
  ld-record-table .record-entity-link:hover .record-entity-label,
  ld-record-table .record-entity-link:focus-visible .record-entity-label {
    text-decoration: underline;
    outline: 0;
  }

  ld-record-table .record-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--base-size-4);
  }

  ld-record-table .record-tags span {
    display: inline-flex;
    min-height: var(--base-size-20);
    align-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--borderRadius-full, var(--ld-radius-full));
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
    padding: 0 var(--base-size-8);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    text-transform: uppercase;
  }

  ld-record-table .record-entity,
  ld-record-table .record-button-cell {
    display: grid;
    width: 100%;
    max-width: 100%;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: start;
    gap: var(--base-size-8);
  }

  ld-record-table .record-entity-icon,
  ld-record-table .record-button-icon {
    display: inline-flex;
    width: var(--control-medium-size, 32px);
    height: var(--control-medium-size, 32px);
    align-items: center;
    justify-content: center;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
  }

  ld-record-table .variant-primary .record-entity-icon {
    background: var(--ld-bg-control, var(--ld-bg-panel-muted));
  }

  ld-record-table .variant-primary .record-icon-dashboard {
    border-color: var(--ld-asset-dashboard-border, var(--ld-line-muted));
    background: var(--ld-asset-dashboard-bg, var(--ld-bg-panel-muted));
    color: var(--ld-asset-dashboard-accent, var(--ld-fg-muted));
  }

  ld-record-table .variant-primary .record-icon-semantic_model {
    border-color: var(--ld-asset-semantic-model-border, var(--ld-line-muted));
    background: var(--ld-asset-semantic-model-bg, var(--ld-bg-panel-muted));
    color: var(--ld-asset-semantic-model-accent, var(--ld-fg-muted));
  }

  ld-record-table .variant-primary .record-icon-model_table,
  ld-record-table .variant-primary .record-icon-semantic_table {
    border-color: var(--ld-asset-model-table-border, var(--ld-line-muted));
    background: var(--ld-asset-model-table-bg, var(--ld-bg-panel-muted));
    color: var(--ld-asset-model-table-accent, var(--ld-fg-muted));
  }

  ld-record-table .variant-primary .record-icon-source {
    border-color: var(--ld-asset-source-border, var(--ld-line-muted));
    background: var(--ld-asset-source-bg, var(--ld-bg-panel-muted));
    color: var(--ld-asset-source-accent, var(--ld-fg-muted));
  }

  ld-record-table .record-entity-copy {
    display: grid;
    min-width: 0;
    gap: var(--base-size-4);
  }

  ld-record-table .record-entity-label {
    display: block;
    max-width: 100%;
    overflow-wrap: anywhere;
    color: var(--ld-fg-default);
    font-weight: var(--ld-font-weight-strong);
    white-space: normal;
  }

  ld-record-table .variant-primary .record-entity-label {
    font-size: var(--ld-font-size-title-sm);
    line-height: var(--ld-line-height-compact);
  }

  ld-record-table .record-entity-description {
    display: block;
    max-width: 100%;
    overflow-wrap: anywhere;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-regular);
    line-height: var(--ld-line-height-compact);
    white-space: normal;
  }

  ld-record-table .record-button-cell {
    border: 0;
    background: transparent;
    color: var(--ld-fg-default);
    cursor: pointer;
    padding: 0;
    font: inherit;
    text-align: left;
  }

  ld-record-table .record-button-cell:hover span:last-child,
  ld-record-table .record-button-cell:focus-visible span:last-child {
    color: var(--ld-fg-link);
    text-decoration: underline;
  }

  ld-record-table .record-actions {
    display: inline-flex;
    justify-content: flex-end;
    gap: var(--base-size-6);
  }

  ld-record-table .record-icon-action {
    display: inline-flex;
    width: var(--control-medium-size, 32px);
    height: var(--control-medium-size, 32px);
    align-items: center;
    justify-content: center;
    border: var(--ld-border-transparent, 1px solid transparent);
    border-radius: var(--ld-radius-default);
    background: transparent;
    color: var(--ld-fg-muted);
    text-decoration: none;
    cursor: pointer;
  }

  ld-record-table .record-icon-action:hover,
  ld-record-table .record-icon-action:focus-visible {
    border-color: var(--ld-line-muted);
    background: var(--ld-bg-control-hover, var(--ld-bg-panel-muted));
    color: var(--ld-fg-default);
    outline: 0;
  }

  ld-record-table .record-muted,
  ld-record-table .record-table-empty {
    color: var(--ld-fg-muted);
  }

  ld-record-table .record-table-empty {
    border-top: var(--ld-border-muted);
    border-bottom: var(--ld-border-muted);
    padding: var(--base-size-20) 0;
    font-size: var(--ld-font-size-body-md);
    font-weight: var(--ld-font-weight-regular);
  }
`

customElements.define('ld-record-table', RecordTable)
