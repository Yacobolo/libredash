import { LitElement, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import {
  TableController,
  createCoreRowModel,
  createSortedRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from '@tanstack/lit-table'

type GridCellTone = 'default' | 'accent' | 'success' | 'attention' | 'muted'

type GridCell = {
  label?: string
  value?: string | number
  tone?: GridCellTone
}

type GridColumn = {
  id: string
  header: string
  kind?: 'text' | 'code' | 'expression' | 'badge' | 'number' | 'link' | 'tags'
  align?: 'left' | 'right'
  hrefKey?: string
  width?: string
}

type GridRow = Record<string, unknown>

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

function applyUpdater<T>(updater: unknown, current: T): T {
  return typeof updater === 'function' ? (updater as (old: T) => T)(current) : updater as T
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

function sortValue(value: unknown): string | number {
  if (typeof value === 'number') return value
  return cellLabel(value).toLowerCase()
}

class DataGrid extends LitElement {
  @property({ type: Object }) grid: Grid | null = null
  @property({ attribute: 'data-grid' }) dataGrid = '{}'
  @state() private sorting: SortingState = []
  private tableController = new TableController<GridRow>(this)

  createRenderRoot(): HTMLElement {
    return this
  }

  render() {
    const grid = this.resolvedGrid
    const columns: ColumnDef<GridRow, unknown>[] = grid.columns.map((column) => ({
      id: column.id,
      accessorFn: (row) => row[column.id],
      header: column.header,
      cell: (info) => this.renderCell(column, info.getValue(), info.row.original),
      sortingFn: (left, right, columnID) => {
        const leftValue = sortValue(left.original[columnID])
        const rightValue = sortValue(right.original[columnID])
        if (typeof leftValue === 'number' && typeof rightValue === 'number') return leftValue - rightValue
        return String(leftValue).localeCompare(String(rightValue), undefined, { numeric: true })
      },
      meta: column,
    }))
    const table = this.tableController.table({
      data: grid.rows,
      columns,
      state: { sorting: this.sorting },
      onSortingChange: (updater) => {
        this.sorting = applyUpdater(updater, this.sorting)
      },
      getCoreRowModel: createCoreRowModel(),
      getSortedRowModel: createSortedRowModel(),
    })

    if (grid.rows.length === 0) {
      return html`<div class="data-grid-empty">${grid.empty}</div>`
    }

    return html`
      <div class="data-grid-wrap">
        <table class="data-grid" style=${grid.minWidth ? `min-width: ${grid.minWidth}` : ''}>
          <thead>
            ${table.getHeaderGroups().map((headerGroup) => html`
              <tr>
                ${headerGroup.headers.map((header) => {
                  const column = header.column.columnDef.meta as GridColumn | undefined
                  const direction = header.column.getIsSorted()
                  return html`
                    <th style=${column?.width ? `width: ${column.width}` : ''} class=${column?.align === 'right' ? 'is-right' : ''}>
                      <button
                        type="button"
                        class="data-grid-sort"
                        @click=${header.column.getToggleSortingHandler()}
                        aria-label=${`Sort by ${cellLabel(header.column.columnDef.header)}`}
                      >
                        <span>${flexRender(header.column.columnDef.header, header.getContext())}</span>
                        <span class="data-grid-sort-indicator" aria-hidden="true">${direction === 'asc' ? '^' : direction === 'desc' ? 'v' : ''}</span>
                      </button>
                    </th>
                  `
                })}
              </tr>
            `)}
          </thead>
          <tbody>
            ${table.getRowModel().rows.map((row) => html`
              <tr>
                ${row.getVisibleCells().map((cell) => {
                  const column = cell.column.columnDef.meta as GridColumn | undefined
                  return html`<td class=${column?.align === 'right' ? 'is-right' : ''}>${flexRender(cell.column.columnDef.cell, cell.getContext())}</td>`
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
    const gridAttribute = this.getAttribute('grid')
    for (const source of [this.dataGrid, gridAttribute]) {
      if (!source) continue
      try {
        return normalizeGrid(JSON.parse(source) as Grid)
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
}

function normalizeGrid(grid: Grid): Required<Grid> {
  return {
    columns: grid.columns ?? [],
    rows: grid.rows ?? [],
    empty: grid.empty ?? emptyGrid.empty,
    minWidth: grid.minWidth ?? emptyGrid.minWidth,
  }
}

customElements.define('ld-data-grid', DataGrid)
