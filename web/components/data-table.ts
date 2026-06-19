import { LitElement, css, html, nothing } from 'lit'
import { createRef, ref, type Ref } from 'lit/directives/ref.js'
import {
  TableController,
  callMemoOrStaticFn,
  columnPinningFeature,
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  createCoreRowModel,
  flexRender,
  rowSelectionFeature,
  rowSortingFeature,
  tableFeatures,
  type ColumnDef,
  type ColumnSizingState,
  type ColumnVisibilityState,
  type RowSelectionState,
  type SortingState,
} from '@tanstack/lit-table'
import {
  column_getCanHide,
  column_getIsLastColumn,
  column_getIsPinned,
  column_getStart,
  column_getIsVisible,
  column_getToggleVisibilityHandler,
  row_getVisibleCells,
  table_getAllLeafColumns,
} from '@tanstack/table-core/static-functions'
import { visualMenuIcon } from './visual-menu-icons'
import { defaultDirection, formatCell, rowKey } from './table/format'
import { blockStartsForAll, emptyBlocks, emptyTable, sameSort, sortedBlockRows, tableConverter } from './table/block-source'
import {
  blockIDs,
  defaultChunkSize,
  defaultRowHeight,
  defaultSort,
  type BlockID,
  type ExpectedBlockRequest,
  type SortDirection,
  type TableBlock,
  type TableBlockCommand,
  type TableColumn,
  type TableFormattingRule,
  type TableRow,
  type TableSignal,
  type TanStackTableRow,
  type VisualAction,
  type VisibleRowSlot,
} from './table/types'

type ResizeDrag = {
  columnKey: string
  startClientX: number
  startSize: number
  minSize: number
}

const dataTableFeatures = tableFeatures({
  columnPinningFeature,
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  rowSelectionFeature,
  rowSortingFeature,
})

const groupHeaderHeight = 26

function defaultColumnSize(column: TableColumn): number {
  const configuredWidth = Number(column.width)
  if (Number.isFinite(configuredWidth) && configuredWidth > 0) return configuredWidth
  const widths: Record<string, number> = {
    order_id: 240,
    purchase_date: 126,
    status: 128,
    state: 78,
    category: 210,
    revenue: 130,
    review_score: 104,
    delivery_days: 108,
  }
  if (widths[column.key]) return widths[column.key]
  if (column.align === 'right') return 120
  return 140
}

function tableTone(value: string | undefined, fallback = 'accent'): string {
  const normalized = String(value || fallback).toLowerCase().replace(/[^a-z0-9_-]/g, '')
  return normalized || fallback
}

function toneColor(value: string | undefined, fallback = 'accent'): string {
  switch (tableTone(value, fallback)) {
    case 'success':
    case 'green':
      return 'var(--ld-fg-success)'
    case 'danger':
    case 'red':
      return 'var(--ld-fg-danger)'
    case 'warning':
    case 'yellow':
      return 'var(--ld-fg-warning)'
    case 'muted':
    case 'gray':
      return 'var(--ld-fg-muted)'
    default:
      return 'var(--ld-fg-link)'
  }
}

function numericValue(value: unknown): number | null {
  const next = Number(value)
  return Number.isFinite(next) ? next : null
}

function ruleMatches(value: unknown, rule: TableFormattingRule): boolean {
  const next = numericValue(value)
  if (next === null) return false
  if (typeof rule.min === 'number' && next < rule.min) return false
  if (typeof rule.max === 'number' && next > rule.max) return false
  return true
}

function scalePercent(value: unknown, rule: TableFormattingRule): number {
  const next = numericValue(value)
  if (next === null) return 0
  const min = typeof rule.min === 'number' ? rule.min : 0
  const max = typeof rule.max === 'number' && rule.max > min ? rule.max : Math.max(min + 1, next)
  return Math.max(0, Math.min(100, ((next - min) / (max - min)) * 100))
}

function badgeRule(column: TableColumn): TableFormattingRule | undefined {
  return column.formatting?.find((rule) => rule.kind === 'badge')
}

function dataBarRule(column: TableColumn): TableFormattingRule | undefined {
  return column.formatting?.find((rule) => rule.kind === 'data_bar')
}

function backgroundRule(value: unknown, column: TableColumn): TableFormattingRule | undefined {
  return column.formatting?.find((rule) => rule.kind === 'background_scale' && ruleMatches(value, rule))
}

function textColorRule(value: unknown, column: TableColumn): TableFormattingRule | undefined {
  return column.formatting?.find((rule) => rule.kind === 'text_color' && ruleMatches(value, rule))
}

function applyUpdater<T>(updater: unknown, current: T): T {
  return typeof updater === 'function' ? (updater as (old: T) => T)(current) : updater as T
}

function columnVisible(columnID: string, visibility: ColumnVisibilityState): boolean {
  return visibility[columnID] !== false
}

function visibleHeaders(table: any, visibility: ColumnVisibilityState): any[] {
  const groups = table.getHeaderGroups?.() ?? []
  const headers = groups[groups.length - 1]?.headers ?? []
  return headers.filter((header: any) => columnVisible(header.column.id, visibility))
}

function allTableColumns(table: any): any[] {
  return table.getAllLeafColumns?.() ?? callMemoOrStaticFn(table, 'getAllLeafColumns', table_getAllLeafColumns)
}

function visibleColumnsFromHeaders(headers: any[], columns: TableColumn[]): TableColumn[] {
  return headers
    .map((header) => header.column.columnDef.meta?.column ?? columns.find((item) => item.key === header.column.id))
    .filter(Boolean) as TableColumn[]
}

function visibleCellsForRow(row: any, visibility: ColumnVisibilityState): any[] {
  const cells = row?.getVisibleCells?.() ?? callMemoOrStaticFn(row, 'getVisibleCells', row_getVisibleCells)
  return cells.filter((cell: any) => columnVisible(cell.column.id, visibility))
}

function columnIsVisible(column: any, fallback: ColumnVisibilityState): boolean {
  if (column.id in fallback) return columnVisible(column.id, fallback)
  return column?.getIsVisible?.() ?? callMemoOrStaticFn(column, 'getIsVisible', column_getIsVisible) ?? true
}

function columnCanHide(column: any): boolean {
  return column?.getCanHide?.() ?? callMemoOrStaticFn(column, 'getCanHide', column_getCanHide) ?? true
}

function columnVisibilityHandler(column: any, fallback: (checked: boolean) => void): (event: Event) => void {
  const handler = column?.getToggleVisibilityHandler?.() ?? callMemoOrStaticFn(column, 'getToggleVisibilityHandler', column_getToggleVisibilityHandler)
  return (event: Event) => {
    if (typeof handler === 'function') handler(event)
    fallback((event.currentTarget as HTMLInputElement).checked)
  }
}

class DataTable extends LitElement {
  static properties = {
    tableId: { attribute: 'table-id' },
    table: { attribute: 'table', converter: tableConverter },
    selectedRowId: { state: true },
    selectedCellKey: { state: true },
    viewportTop: { state: true },
    viewportHeight: { state: true },
    columnVisibility: { state: true },
    columnSizing: { state: true },
    rowSelection: { state: true },
    hoveredRowId: { state: true },
    resizeGuideX: { state: true },
  }

  tableId = 'orders'
  table: TableSignal = emptyTable
  private selectedRowId = ''
  private selectedCellKey = ''
  private viewportTop = 0
  private viewportHeight = 0
  private columnVisibility: ColumnVisibilityState = {}
  private columnSizing: ColumnSizingState = {}
  private rowSelection: RowSelectionState = {}
  private hoveredRowId = ''
  private resizeGuideX = -1
  private lastResetVersion = -1
  private shouldResetScroll = false
  private requestSeq = 0
  private scrollFrame = 0
  private jumpTimer = 0
  private pendingJumpStart = 0
  private expectedBlocks = new Map<BlockID, ExpectedBlockRequest>()
  private latestAcceptedSeq = new Map<BlockID, number>()
  private blockCache: Record<BlockID, TableBlock> = emptyBlocks()
  private bodyViewportRef: Ref<HTMLDivElement> = createRef()
  private resizeObserver?: ResizeObserver
  private resizeGuideFrame = 0
  private resizeDrag?: ResizeDrag
  private tableController = new TableController<typeof dataTableFeatures, TanStackTableRow>(this)
  private handleOutsidePointerDown = (event: PointerEvent) => {
    const details = this.renderRoot.querySelector<HTMLDetailsElement>('.visual-options')
    if (!details?.open) return
    if (!event.composedPath().includes(details)) details.removeAttribute('open')
  }
  private handleDocumentKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    this.renderRoot.querySelector<HTMLDetailsElement>('.visual-options')?.removeAttribute('open')
  }
  private handleResizeGuideMove = (event: MouseEvent | TouchEvent) => {
    this.scheduleResizeGuideUpdate(event)
  }
  private handleResizeGuideEnd = () => {
    this.clearResizeGuide()
  }

  static styles = css`
    :host {
      display: block;
      height: 100%;
      min-height: 0;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    .shell {
      display: flex;
      flex-direction: column;
      height: 100%;
      min-height: 0;
      min-width: 0;
      background: var(--ld-chart-surface);
      isolation: isolate;
    }

    .toolbar {
      position: relative;
      z-index: 40;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      min-height: 34px;
      border-bottom: var(--ld-border-default);
      background: var(--ld-chart-surface);
      padding: 6px 8px 5px 10px;
    }

    .toolbar::after {
      content: '';
      position: absolute;
      inset-inline: 0;
      bottom: -2px;
      z-index: 1;
      height: 3px;
      background: inherit;
      pointer-events: none;
    }

    .toolbar > div {
      position: relative;
      z-index: 2;
      flex: 1 1 auto;
      min-width: 0;
    }

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

    .visual-options {
      position: relative;
      z-index: 2;
      flex: 0 0 auto;
    }

    .visual-options summary {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-tight);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      font-size: var(--ld-font-size-body-lg);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
      list-style: none;
    }

    .visual-options summary::-webkit-details-marker {
      display: none;
    }

    .visual-options summary:hover,
    .visual-options summary:focus-visible,
    .visual-options[open] summary {
      border-color: var(--ld-line-default);
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .menu {
      position: absolute;
      top: calc(100% + 4px);
      right: 0;
      z-index: 30;
      display: grid;
      width: 176px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-overlay);
      box-shadow: var(--ld-shadow-floating-sm);
      padding: 4px;
    }

    .menu button {
      display: flex;
      align-items: center;
      gap: 8px;
      min-height: 27px;
      border: 0;
      border-radius: var(--ld-radius-tight);
      background: transparent;
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0 8px;
      font: inherit;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-align: left;
    }

    .menu svg {
      flex: 0 0 auto;
      width: 14px;
      height: 14px;
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .menu button:hover,
    .menu button:focus-visible {
      background: var(--ld-bg-panel-muted);
      outline: 0;
    }

    .menu button:disabled {
      cursor: default;
      opacity: 0.48;
    }

    .menu button:disabled:hover {
      background: transparent;
    }

    .menu-divider {
      height: 1px;
      margin: 4px 2px;
      background: var(--ld-line-muted);
    }

    .column-menu {
      display: grid;
      gap: 3px;
      padding: 2px;
    }

    .column-menu > span {
      padding: 2px 6px;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      text-transform: uppercase;
    }

    .column-menu label {
      display: flex;
      align-items: center;
      gap: 7px;
      min-height: 24px;
      border-radius: var(--ld-radius-tight);
      cursor: pointer;
      padding: 0 6px;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .column-menu label:hover {
      background: var(--ld-bg-hover);
    }

    .column-menu input {
      accent-color: var(--ld-fg-link);
    }

    .error {
      border-bottom: var(--ld-border-danger);
      background: var(--ld-bg-danger-muted);
      color: var(--ld-fg-danger);
      padding: 9px 12px;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
    }

    .head,
    .group-head,
    .row {
      display: grid;
      grid-template-columns: var(--ld-table-columns);
      width: var(--ld-table-width, 1080px);
      min-width: var(--ld-table-width, 1080px);
    }

    .group-head {
      position: sticky;
      top: 0;
      z-index: 30;
      border-bottom: var(--ld-border-default);
      background: color-mix(in srgb, var(--ld-bg-panel-muted), var(--ld-chart-surface) 34%);
      color: var(--ld-fg-muted);
    }

    .group-cell {
      display: flex;
      align-items: center;
      min-width: 0;
      min-height: var(--ld-group-head-height, 26px);
      overflow: hidden;
      border-right: var(--ld-border-default);
      background: inherit;
      padding: 0 9px;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
      text-transform: uppercase;
    }

    .group-cell.measure-group {
      justify-content: center;
      color: var(--ld-fg-default);
    }

    .group-cell:last-child {
      border-right: 0;
    }

    .head {
      position: sticky;
      top: var(--ld-head-top, 0px);
      z-index: 28;
      border-bottom: var(--ld-border-emphasis);
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-muted);
      box-shadow: inset 0 -1px 0 var(--ld-line-emphasis);
    }

    .header-cell,
    .cell {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .header-cell {
      position: relative;
      border-right: var(--ld-border-default);
      background: var(--ld-bg-panel-muted);
    }

    .header-cell:last-child {
      border-right: 0;
    }

    .header-cell.pinned-left,
    .group-cell.pinned-left,
    .cell.pinned-left {
      position: sticky;
      left: calc(var(--ld-pin-left, 0px) - 1px);
      overflow: visible;
      border-right: 0;
      background: var(--ld-chart-surface);
      box-shadow: none;
    }

    .header-cell.pinned-left-edge::after,
    .group-cell.pinned-left-edge::after,
    .cell.pinned-left-edge::after {
      content: '';
      position: absolute;
      inset-block: 0;
      left: 100%;
      z-index: 1;
      width: 10px;
      border-left: 1px solid var(--ld-line-default);
      background: inherit;
      pointer-events: none;
    }

    .header-cell.pinned-left {
      z-index: 36;
      background: var(--ld-bg-panel-muted);
    }

    .group-cell.pinned-left {
      z-index: 38;
      background: color-mix(in srgb, var(--ld-bg-panel-muted), var(--ld-chart-surface) 34%);
    }

    .cell.pinned-left {
      z-index: 12;
    }

    .header-cell.pinned-left > .header-button,
    .cell.pinned-left > *,
    .group-cell.pinned-left > * {
      position: relative;
      z-index: 2;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .cell-value {
      display: block;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    button.header-button {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      width: 100%;
      min-height: 34px;
      border: 0;
      border-bottom: var(--borderWidth-thick) solid transparent;
      background: transparent;
      color: inherit;
      cursor: pointer;
      padding: 0 9px;
      font: inherit;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
      text-align: left;
      text-transform: uppercase;
    }

    button.header-button:hover,
    button.header-button:focus-visible {
      background: color-mix(in srgb, var(--ld-fg-link), transparent 92%);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .sort {
      display: inline-grid;
      min-width: 18px;
      place-items: center;
      color: var(--ld-fg-link);
      font-size: var(--ld-font-size-body-md);
      opacity: 0;
    }

    .sorted .sort {
      opacity: 1;
    }

    .column-resizer {
      position: absolute;
      inset-block: 5px;
      right: -3px;
      z-index: 3;
      width: 6px;
      cursor: col-resize;
    }

    .column-resizer::after {
      content: '';
      position: absolute;
      inset-block: 3px;
      left: 2px;
      width: 2px;
      border-radius: var(--ld-radius-full);
      background: transparent;
    }

    .header-cell:hover .column-resizer::after,
    .column-resizer.resizing::after {
      background: var(--ld-fg-link);
    }

    .table-frame {
      position: relative;
      display: flex;
      flex: 1 1 auto;
      flex-direction: column;
      min-height: 0;
      min-width: 0;
      margin-top: -1px;
      overflow: hidden;
      border-top: 1px solid var(--ld-line-default);
      background: var(--ld-chart-surface);
    }

    .table-scrollport {
      position: relative;
      flex: 1 1 auto;
      overflow: auto;
      min-height: 0;
      min-width: 0;
      background: var(--ld-chart-surface);
      overscroll-behavior: none;
      scrollbar-gutter: stable;
    }

    .table-plane {
      position: relative;
      isolation: isolate;
      width: var(--ld-table-width, 1080px);
      min-width: var(--ld-table-width, 1080px);
    }

    .canvas {
      position: relative;
      z-index: 0;
      width: var(--ld-table-width, 1080px);
      min-width: var(--ld-table-width, 1080px);
    }

    .grid-lines {
      position: absolute;
      inset: 0;
      z-index: 0;
      pointer-events: none;
    }

    .grid-line {
      position: absolute;
      top: 0;
      bottom: 0;
      width: 1px;
      background: var(--ld-line-muted);
    }

    .resize-guide {
      position: absolute;
      top: 0;
      bottom: 0;
      left: var(--ld-resize-guide-x, -9999px);
      z-index: 45;
      width: 0;
      border-left: 2px solid var(--ld-fg-link);
      box-shadow: 0 0 0 1px color-mix(in srgb, var(--ld-fg-link), transparent 74%);
      pointer-events: none;
    }

    .row {
      position: absolute;
      inset-inline: 0;
      z-index: 1;
      height: var(--ld-row-height, 34px);
      background: var(--ld-chart-surface);
      color: var(--ld-fg-default);
    }

    .zebra .row:nth-child(even) {
      background: color-mix(in srgb, var(--ld-table-stripe), var(--ld-chart-surface) 18%);
    }

    .grid-rows .row,
    .grid-full .row {
      border-bottom: var(--ld-border-muted);
    }

    .row:hover {
      background: color-mix(in srgb, var(--ld-fg-link), transparent 91%);
    }

    .row.hovered {
      background: color-mix(in srgb, var(--ld-fg-link), transparent 91%);
    }

    .row.selected {
      background: color-mix(in srgb, var(--ld-fg-link), transparent 86%);
      box-shadow: inset 3px 0 0 var(--ld-fg-link);
    }

    .row.skeleton-row {
      pointer-events: none;
    }

    .row.skeleton-row:hover {
      background: var(--ld-chart-surface);
    }

    .cell {
      display: flex;
      align-items: center;
      min-width: 0;
      border: 0;
      background: transparent;
      color: inherit;
      cursor: default;
      font: inherit;
      padding: 0 9px;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
      text-align: left;
    }

    .density-compact .cell {
      padding: 0 7px;
      font-size: var(--ld-font-size-caption);
    }

    .density-spacious .cell {
      padding: 0 12px;
      font-size: var(--ld-font-size-body-lg);
    }

    .grid-columns .cell,
    .grid-full .cell {
      border-right: var(--ld-border-muted);
    }

    .cell:last-child {
      border-right: 0;
    }

    .cell.active {
      outline: var(--ld-border-width-focus) solid var(--ld-fg-link);
      outline-offset: -2px;
      background: color-mix(in srgb, var(--ld-fg-link), transparent 88%);
    }

    .skeleton-cell {
      cursor: default;
    }

    .cell.has-background {
      background: color-mix(in srgb, var(--ld-cell-bg-color), transparent var(--ld-cell-bg-fade, 78%));
    }

    .cell.has-data-bar {
      position: relative;
      isolation: isolate;
    }

    .cell-data-bar {
      position: absolute;
      inset-block: 5px;
      left: 6px;
      z-index: -1;
      width: var(--ld-cell-bar-width, 0%);
      border-radius: var(--ld-radius-tight);
      background: color-mix(in srgb, var(--ld-cell-bar-color, var(--ld-fg-link)), transparent 74%);
    }

    .cell-badge {
      display: inline-flex;
      max-width: 100%;
      align-items: center;
      justify-content: center;
      overflow: hidden;
      border: 1px solid currentColor;
      border-radius: var(--ld-radius-full);
      padding: 1px 7px;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      line-height: 1.45;
    }

    .cell-badge.tone-success {
      background: var(--ld-bg-success-muted);
      color: var(--ld-fg-success);
    }

    .cell-badge.tone-danger {
      background: var(--ld-bg-danger-muted);
      color: var(--ld-fg-danger);
    }

    .cell-badge.tone-warning {
      background: var(--ld-bg-warning-muted);
      color: var(--ld-fg-warning);
    }

    .cell-badge.tone-muted {
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-muted);
    }

    .cell-badge.tone-accent,
    .cell-badge.tone-blue {
      background: var(--ld-bg-accent-muted);
      color: var(--ld-fg-link);
    }

    .grid-none .grid-lines,
    .grid-rows .grid-lines {
      display: none;
    }

    .skeleton-line {
      display: block;
      width: min(76%, 140px);
      height: 9px;
      overflow: hidden;
      border-radius: var(--ld-radius-full);
      background: linear-gradient(
        90deg,
        var(--ld-bg-panel-muted) 0%,
        color-mix(in srgb, var(--ld-fg-muted), transparent 82%) 45%,
        var(--ld-bg-panel-muted) 90%
      );
      background-size: 220% 100%;
      animation: shimmer 1.15s ease-in-out infinite;
      opacity: 0.78;
    }

    .skeleton-cell:nth-child(2n) .skeleton-line {
      width: min(58%, 120px);
    }

    .right {
      justify-content: end;
      font-variant-numeric: tabular-nums;
    }

    .empty {
      display: grid;
      min-height: 240px;
      place-items: center;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-lg);
      font-weight: var(--ld-font-weight-strong);
    }

    .loading {
      position: absolute;
      inset-inline: 0;
      top: 0;
      z-index: 2;
      height: 3px;
      overflow: hidden;
      background: var(--ld-bg-accent-muted);
    }

    .loading::after {
      content: '';
      display: block;
      width: 34%;
      height: 100%;
      background: var(--ld-fg-link);
      animation: load 900ms ease-in-out infinite;
    }

    .footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      min-height: 34px;
      border-top: var(--ld-border-default);
      background: var(--ld-bg-panel-muted);
      padding: 6px 10px;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .footer span {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .footer span:last-child {
      flex: 0 0 auto;
      margin-left: auto;
      text-align: right;
    }

    .footer strong {
      color: var(--ld-fg-default);
      font-weight: var(--ld-font-weight-strong);
    }

    @keyframes load {
      0% { transform: translateX(-100%); }
      100% { transform: translateX(310%); }
    }

    @keyframes shimmer {
      0% { background-position: 120% 0; }
      100% { background-position: -120% 0; }
    }

    @media (max-width: 760px) {
      .shell {
        min-height: 360px;
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('pointerdown', this.handleOutsidePointerDown)
    document.addEventListener('keydown', this.handleDocumentKeyDown)
  }

  firstUpdated(): void {
    const viewport = this.bodyViewportRef.value
    if (!viewport) return
    this.viewportHeight = viewport.clientHeight
    this.resizeObserver = new ResizeObserver(() => {
      this.viewportHeight = viewport.clientHeight
      this.scheduleEnsureBlocksForScroll()
    })
    this.resizeObserver.observe(viewport)
    this.scheduleEnsureBlocksForScroll()
  }

  disconnectedCallback(): void {
    document.removeEventListener('pointerdown', this.handleOutsidePointerDown)
    document.removeEventListener('keydown', this.handleDocumentKeyDown)
    this.resizeObserver?.disconnect()
    if (this.scrollFrame) cancelAnimationFrame(this.scrollFrame)
    this.clearResizeGuide()
    this.clearJumpTimer()
    super.disconnectedCallback()
  }

  willUpdate(): void {
    if (this.lastResetVersion !== this.table.resetVersion) {
      this.lastResetVersion = this.table.resetVersion
      this.blockCache = emptyBlocks()
      this.shouldResetScroll = true
      this.expectedBlocks.clear()
      this.latestAcceptedSeq.clear()
      this.clearJumpTimer()
      this.selectedRowId = ''
      this.selectedCellKey = ''
    }
    this.mergeIncomingBlocks()
    if (this.selectedRowId && !this.loadedRows.some((item) => rowKey(item.row, item.index) === this.selectedRowId)) {
      this.selectedRowId = ''
      this.selectedCellKey = ''
    }
  }

  updated(): void {
    if (this.shouldResetScroll) {
      this.shouldResetScroll = false
      queueMicrotask(() => {
        const viewport = this.bodyViewportRef.value
        if (!viewport) return
        viewport.scrollTop = 0
        viewport.scrollLeft = 0
        this.viewportTop = 0
        this.viewportHeight = viewport.clientHeight
        this.scheduleEnsureBlocksForScroll()
      })
    }
  }

  get columns(): TableColumn[] {
    return Array.isArray(this.table?.columns) ? this.table.columns : []
  }

  get loadedRows(): Array<{ row: TableRow; index: number }> {
    return sortedBlockRows(this.blocks, this.availableRows)
  }

  get visibleRows(): VisibleRowSlot[] {
    if (this.availableRows <= 0) return []
    const rowMap = new Map(this.loadedRows.map((item) => [item.index, item.row]))
    const first = Math.max(0, Math.floor(this.viewportTop / this.rowHeight) - 2)
    const visibleCount = Math.max(1, Math.ceil((this.viewportHeight || this.rowHeight) / this.rowHeight) + 4)
    const last = Math.min(this.availableRows, first + visibleCount)
    const rows: VisibleRowSlot[] = []
    for (let index = first; index < last; index++) {
      const row = rowMap.get(index)
      rows.push(row ? { kind: 'row', row, index } : { kind: 'skeleton', index })
    }
    return rows
  }

  get visibleLoading(): boolean {
    return this.visibleRows.some((row) => row.kind === 'skeleton') || this.expectedBlocks.size > 0
  }

  get availableRows(): number {
    return Math.max(0, this.table.availableRows ?? 0)
  }

  get blocks(): Record<BlockID, TableBlock> {
    return this.blockCache
  }

  get chunkSize(): number {
    return Math.max(1, this.table.chunkSize || defaultChunkSize)
  }

  get rowHeight(): number {
    return Math.max(1, this.table.rowHeight || defaultRowHeight)
  }

  private gridTemplateFor(columns: TableColumn[]): string {
    return this.columnPixelWidths(columns).map((size) => `${size}px`).join(' ')
  }

  private tableWidthFor(columns: TableColumn[]): number {
    return this.columnPixelWidths(columns).reduce((sum, size) => sum + size, 0)
  }

  private columnLineOffsetsFor(columns: TableColumn[]): number[] {
    const widths = this.columnPixelWidths(columns)
    let offset = 0
    return widths.slice(0, -1).map((width) => {
      offset += width
      return offset
    })
  }

  private columnPixelWidths(columns: TableColumn[]): number[] {
    return columns.map((column) => this.columnPixelWidth(column))
  }

  private columnPixelWidth(column: TableColumn): number {
    return Math.max(this.minColumnSize(column), this.columnSizing[column.key] ?? defaultColumnSize(column) ?? 140)
  }

  private minColumnSize(column: TableColumn): number {
    return column.key === 'order_id' || column.key === 'category' ? 160 : 64
  }

  private tanstackRowsForSlots(slots: VisibleRowSlot[]): TanStackTableRow[] {
    return slots.filter((slot): slot is Extract<VisibleRowSlot, { kind: 'row' }> => slot.kind === 'row').map(({ row, index }) => ({
      ...row,
      __absoluteIndex: index,
      __rowKey: rowKey(row, index),
    }))
  }

  private columnsForTanStack(): TableColumn[] {
    return this.columns
  }

  private groupHeaderSegments(headers: any[], force = false): Array<{ label: string; span: number; rowHeader: boolean; column: any }> {
    if (!force && !headers.some((header) => header.column.columnDef.meta?.column?.group)) return []
    const segments: Array<{ label: string; span: number; rowHeader: boolean; column: any }> = []
    for (const header of headers) {
      const column = header.column.columnDef.meta?.column as TableColumn | undefined
      if (!column) continue
      const rowHeader = column.role === 'row_header'
      const label = rowHeader ? '' : column.group || ''
      const previous = segments[segments.length - 1]
      if (previous && previous.label === label && previous.rowHeader === rowHeader) {
        previous.span++
        continue
      }
      segments.push({ label, span: 1, rowHeader, column: header.column })
    }
    return segments
  }

  private tanstackColumnDefs(): Array<ColumnDef<TanStackTableRow, unknown>> {
    return this.columnsForTanStack().map((column) => ({
      id: column.key,
      accessorKey: column.key,
      header: column.label,
      cell: (info: any) => formatCell(info.getValue(), column),
      size: defaultColumnSize(column),
      minSize: this.minColumnSize(column),
      enableResizing: true,
      meta: { align: column.align, column },
    })) as Array<ColumnDef<TanStackTableRow, unknown>>
  }

  private tanstackTable(rows: TanStackTableRow[]) {
    const pinnedColumns = this.columns.filter((column) => column.role === 'row_header').map((column) => column.key)
    const sorting: SortingState = this.table.sort?.key
      ? [{ id: this.table.sort.key, desc: this.table.sort.direction === 'desc' }]
      : []
    return this.tableController.table(
      {
        features: dataTableFeatures,
        columns: this.tanstackColumnDefs(),
        data: rows,
        getRowId: (row) => row.__rowKey,
        getCoreRowModel: createCoreRowModel(),
        manualSorting: true,
        manualFiltering: true,
        manualPagination: true,
        enableRowSelection: true,
        enableMultiRowSelection: false,
        columnResizeMode: 'onEnd',
        renderFallbackValue: '-',
        state: {
          sorting,
          columnVisibility: this.columnVisibility,
          columnSizing: this.columnSizing,
          columnPinning: { left: pinnedColumns, right: [] },
          rowSelection: this.rowSelection,
        },
        onColumnVisibilityChange: (updater: unknown) => {
          this.columnVisibility = applyUpdater(updater, this.columnVisibility)
        },
        onColumnSizingChange: (updater: unknown) => {
          this.columnSizing = applyUpdater(updater, this.columnSizing)
        },
        onRowSelectionChange: (updater: unknown) => {
          this.rowSelection = applyUpdater(updater, this.rowSelection)
          this.selectedRowId = Object.keys(this.rowSelection).find((key) => this.rowSelection[key]) ?? ''
          if (!this.selectedRowId) this.selectedCellKey = ''
        },
      } as any,
    ) as any
  }

  handleScroll(event: Event): void {
    const target = event.currentTarget as HTMLDivElement
    this.viewportTop = target.scrollTop
    this.viewportHeight = target.clientHeight
    this.scheduleEnsureBlocksForScroll()
  }

  sortColumn(column: TableColumn): void {
    const current = this.table?.sort ?? defaultSort
    const direction: SortDirection = current.key === column.key
      ? current.direction === 'asc' ? 'desc' : 'asc'
      : defaultDirection(column)
    this.emitBlock('all', 0, { key: column.key, direction }, this.table.resetVersion + 1)
  }

  selectCell(row: TableRow, column: TableColumn, absoluteIndex: number): void {
    const key = rowKey(row, absoluteIndex)
    this.selectedRowId = key
    this.selectedCellKey = `${key}:${column.key}`
  }

  private selectRow(key: string): void {
    this.selectedRowId = key
    this.rowSelection = { [key]: true }
    this.selectedCellKey = ''
  }

  private columnPinPosition(column: any): false | 'left' | 'right' {
    return column?.getIsPinned?.() ?? callMemoOrStaticFn(column, 'getIsPinned', column_getIsPinned) ?? false
  }

  private isLastLeftPinnedColumn(column: any): boolean {
    return Boolean(column?.getIsLastColumn?.('left') ?? callMemoOrStaticFn(column, 'getIsLastColumn', column_getIsLastColumn, 'left'))
  }

  private pinnedCellClass(column: any): string {
    const pinPosition = this.columnPinPosition(column)
    if (pinPosition !== 'left') return ''
    return `pinned-left ${this.isLastLeftPinnedColumn(column) ? 'pinned-left-edge' : ''}`
  }

  private pinnedCellStyle(column: any): string {
    if (this.columnPinPosition(column) !== 'left') return ''
    const offset = column.getStart?.('left') ?? callMemoOrStaticFn(column, 'getStart', column_getStart, 'left') ?? 0
    return `--ld-pin-left:${Math.max(0, Number(offset) || 0)}px`
  }

  private beginColumnResize(event: MouseEvent | TouchEvent, header: any): void {
    event.preventDefault()
    event.stopPropagation()
    const clientX = this.resizeClientX(event)
    const column = header.column.columnDef.meta?.column as TableColumn | undefined
    if (clientX === null || !column) return
    this.resizeDrag = {
      columnKey: column.key,
      startClientX: clientX,
      startSize: this.columnPixelWidth(column),
      minSize: this.minColumnSize(column),
    }
    this.scheduleResizeGuideUpdate(event)
    document.addEventListener('mousemove', this.handleResizeGuideMove)
    document.addEventListener('mouseup', this.handleResizeGuideEnd, { once: true })
    document.addEventListener('touchmove', this.handleResizeGuideMove, { passive: true })
    document.addEventListener('touchend', this.handleResizeGuideEnd, { once: true })
    document.addEventListener('touchcancel', this.handleResizeGuideEnd, { once: true })
  }

  private scheduleResizeGuideUpdate(event: MouseEvent | TouchEvent): void {
    const clientX = this.resizeClientX(event)
    if (clientX === null) return
    if (this.resizeGuideFrame) cancelAnimationFrame(this.resizeGuideFrame)
    this.resizeGuideFrame = requestAnimationFrame(() => {
      this.resizeGuideFrame = 0
      const plane = this.renderRoot.querySelector<HTMLElement>('.table-plane')
      if (!plane) return
      const rect = plane.getBoundingClientRect()
      const scaleX = rect.width > 0 && plane.offsetWidth > 0 ? rect.width / plane.offsetWidth : 1
      const localX = scaleX > 0 ? (clientX - rect.left) / scaleX : clientX - rect.left
      this.resizeGuideX = Math.max(0, Math.min(plane.scrollWidth, localX))
      if (this.resizeDrag) {
        const delta = scaleX > 0 ? (clientX - this.resizeDrag.startClientX) / scaleX : clientX - this.resizeDrag.startClientX
        const nextSize = Math.max(this.resizeDrag.minSize, Math.round(this.resizeDrag.startSize + delta))
        this.columnSizing = { ...this.columnSizing, [this.resizeDrag.columnKey]: nextSize }
      }
    })
  }

  private resizeClientX(event: MouseEvent | TouchEvent): number | null {
    if ('touches' in event) {
      return event.touches[0]?.clientX ?? event.changedTouches[0]?.clientX ?? null
    }
    return event.clientX
  }

  private clearResizeGuide(): void {
    document.removeEventListener('mousemove', this.handleResizeGuideMove)
    document.removeEventListener('mouseup', this.handleResizeGuideEnd)
    document.removeEventListener('touchmove', this.handleResizeGuideMove)
    document.removeEventListener('touchend', this.handleResizeGuideEnd)
    document.removeEventListener('touchcancel', this.handleResizeGuideEnd)
    if (this.resizeGuideFrame) cancelAnimationFrame(this.resizeGuideFrame)
    this.resizeGuideFrame = 0
    this.resizeDrag = undefined
    this.resizeGuideX = -1
  }

  private renderGroupHeaderRows(headers: any[], force = false) {
    const groupHeaders = this.groupHeaderSegments(headers, force)
    if (!groupHeaders.length) return nothing
    return html`
      <div class="group-head" role="row">
        ${groupHeaders.map((group) => html`
          <div
            class=${`group-cell ${group.rowHeader ? 'row-header' : 'measure-group'} ${this.pinnedCellClass(group.column)}`}
            role="columnheader"
            style=${`grid-column:span ${group.span};${this.pinnedCellStyle(group.column)}`}
          >
            <span class="cell-value">${group.label}</span>
          </div>
        `)}
      </div>
    `
  }

  private renderHeaderRow(headers: any[]) {
    return html`
      <div class="head" role="row">
        ${headers.map((header: any) => {
          const column = header.column.columnDef.meta?.column as TableColumn | undefined
          if (!column) return nothing
          const sorted = this.table?.sort?.key === header.column.id
          const sortMark = this.table?.sort?.direction === 'asc' ? '↑' : '↓'
          return html`
            <div
              class=${`header-cell ${column.role === 'row_header' ? 'row-header' : ''} ${this.pinnedCellClass(header.column)} ${sorted ? 'sorted' : ''}`}
              role="columnheader"
              style=${this.pinnedCellStyle(header.column)}
            >
              <button class="header-button" type="button" @click=${() => this.sortColumn(column)}>
                <span>${flexRender(header.column.columnDef.header, header.getContext())}</span>
                <span class="sort">${sortMark}</span>
              </button>
              ${header.column.getCanResize?.() ? html`
                <span
                  class=${`column-resizer ${this.resizeDrag?.columnKey === column.key ? 'resizing' : ''}`}
                  @mousedown=${(event: MouseEvent) => this.beginColumnResize(event, header)}
                  @touchstart=${(event: TouchEvent) => this.beginColumnResize(event, header)}
                ></span>
              ` : nothing}
            </div>
          `
        })}
      </div>
    `
  }

  private renderSkeletonSegment(headers: any[], index: number) {
    return html`
      <div
        class="row skeleton-row"
        role="row"
        aria-busy="true"
        style=${`top:${index * this.rowHeight}px`}
      >
        ${headers.map((header: any) => {
          const column = header.column.columnDef.meta?.column as TableColumn | undefined
          if (!column) return nothing
          return html`
            <span
              class=${`cell skeleton-cell ${column.role === 'row_header' ? 'row-header' : ''} ${this.pinnedCellClass(header.column)} ${column.align === 'right' ? 'right' : ''}`}
              role="cell"
              style=${this.pinnedCellStyle(header.column)}
            >
              <span class="skeleton-line"></span>
            </span>
          `
        })}
      </div>
    `
  }

  private cellStyle(row: TableRow, column: TableColumn, pinnedColumn: any): string {
    const styles = [this.pinnedCellStyle(pinnedColumn)].filter(Boolean)
    const value = row[column.key]
    const background = backgroundRule(value, column)
    if (background) {
      const percent = scalePercent(value, background)
      const color = toneColor(background.highColor || background.background || background.color, 'warning')
      styles.push(`--ld-cell-bg-color:${color}`)
      styles.push(`--ld-cell-bg-fade:${Math.max(66, 92 - Math.round(percent * 0.22))}%`)
    }
    const text = textColorRule(value, column)
    if (text?.color) styles.push(`color:${toneColor(text.color)}`)
    const bar = dataBarRule(column)
    if (bar) {
      styles.push(`--ld-cell-bar-width:${scalePercent(value, bar)}%`)
      styles.push(`--ld-cell-bar-color:${toneColor(bar.color || bar.highColor || 'accent')}`)
    }
    return styles.join(';')
  }

  private cellClass(column: TableColumn, cellKey: string, row: TableRow, pinnedColumn: any): string {
    const value = row[column.key]
    return [
      'cell',
      column.align === 'right' ? 'right' : '',
      column.role === 'row_header' ? 'row-header' : '',
      this.pinnedCellClass(pinnedColumn),
      cellKey === this.selectedCellKey ? 'active' : '',
      backgroundRule(value, column) ? 'has-background' : '',
      dataBarRule(column) ? 'has-data-bar' : '',
    ].filter(Boolean).join(' ')
  }

  private renderCellValue(row: TableRow, column: TableColumn, formatted: unknown) {
    const value = row[column.key]
    const badge = badgeRule(column)
    if (badge?.values) {
      const tone = badge.values[String(value)] ?? badge.values[String(value).toLowerCase()]
      if (tone) {
        return html`<span class=${`cell-badge tone-${tableTone(tone)}`}>${formatted}</span>`
      }
    }
    return html`${formatted}`
  }

  private renderRowSegment(cells: any[], row: TableRow, index: number, key: string) {
    const selected = key === this.selectedRowId
    const hovered = key === this.hoveredRowId
    return html`
      <div
        class=${`row ${selected ? 'selected' : ''} ${hovered ? 'hovered' : ''}`}
        role="row"
        aria-selected=${selected ? 'true' : 'false'}
        style=${`top:${index * this.rowHeight}px`}
        @mouseenter=${() => { this.hoveredRowId = key }}
        @mouseleave=${() => { if (this.hoveredRowId === key) this.hoveredRowId = '' }}
        @click=${() => this.selectRow(key)}
      >
        ${cells.map((cell: any) => {
          const column = cell.column.columnDef.meta?.column ?? this.columns.find((item) => item.key === cell.column.id)
          if (!column) return nothing
          const cellKey = `${key}:${cell.column.id}`
          const formatted = flexRender(cell.column.columnDef.cell, cell.getContext())
          return html`
            <button
              class=${this.cellClass(column, cellKey, row, cell.column)}
              role="cell"
              title=${String(row[cell.column.id] ?? '')}
              style=${this.cellStyle(row, column, cell.column)}
              @click=${(event: Event) => {
                event.stopPropagation()
                this.selectCell(row, column, index)
              }}
            >
              ${dataBarRule(column) ? html`<span class="cell-data-bar" aria-hidden="true"></span>` : nothing}
              <span class="cell-value">${this.renderCellValue(row, column, formatted)}</span>
            </button>
          `
        })}
      </div>
    `
  }

  render() {
    const visibleRows = this.visibleRows
    const tanstack = this.tanstackTable(this.tanstackRowsForSlots(visibleRows))
    const headers = visibleHeaders(tanstack, this.columnVisibility)
    const columns = visibleColumnsFromHeaders(headers, this.columns)
    const columnModels = allTableColumns(tanstack)
    const tanstackRows = new Map((tanstack.getRowModel?.().rows ?? []).map((row: any) => [row.id, row]))
    const totalHeight = this.availableRows * this.rowHeight
    const hasGroupHeaderRow = headers.some((header: any) => header.column.columnDef.meta?.column?.group)
    const rowRange = this.rowRangeText()
    const selectedText = this.selectedRowId ? '1 row selected' : 'No selection'
    const loading = Boolean(this.table.loadingBlock) || this.visibleLoading
    const gridTemplate = this.gridTemplateFor(columns)
    const tableWidth = this.tableWidthFor(columns)
    const columnLineOffsets = this.columnLineOffsetsFor(columns)
    const shellStyle = [
      `--ld-table-columns:${gridTemplate}`,
      `--ld-table-width:${tableWidth}px`,
      `--ld-row-height:${this.rowHeight}px`,
      `--ld-group-head-height:${groupHeaderHeight}px`,
      `--ld-head-top:${hasGroupHeaderRow ? groupHeaderHeight : 0}px`,
    ].join(';')
    const style = this.table.style
    const shellClass = [
      'shell',
      `density-${style.density}`,
      `grid-${style.grid}`,
      style.zebra ? 'zebra' : '',
    ].filter(Boolean).join(' ')

    return html`
      <section class=${shellClass} style=${shellStyle}>
        <div class="toolbar">
          <div>
            <h2>${this.table?.title ?? 'Orders'}</h2>
          </div>
          <details class="visual-options">
            <summary aria-label="Visual options" title="Visual options">⋮</summary>
            <div class="menu" role="menu">
              <button type="button" role="menuitem" @click=${() => this.runAction('focus')}>${visualMenuIcon('focus')}<span>Focus mode</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('show-data')}>${visualMenuIcon('show-data')}<span>Show data</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('copy-data')}>${visualMenuIcon('copy-data')}<span>Copy data</span></button>
              <button type="button" role="menuitem" @click=${() => this.runAction('export-csv')}>${visualMenuIcon('export-csv')}<span>Export CSV</span></button>
              <button type="button" role="menuitem" ?disabled=${!this.selectedRowId} @click=${() => this.runAction('clear-selection')}>${visualMenuIcon('clear-selection')}<span>Clear selection</span></button>
              <div class="menu-divider"></div>
              <div class="column-menu" @click=${(event: Event) => event.stopPropagation()}>
                <span>Columns</span>
                ${columnModels.map((column: any) => {
                  const checked = columnIsVisible(column, this.columnVisibility)
                  return html`
                    <label>
                      <input
                        type="checkbox"
                        .checked=${checked}
                        ?disabled=${!columnCanHide(column) || checked && columns.length <= 1}
                        @change=${columnVisibilityHandler(column, (next) => {
                          this.columnVisibility = { ...this.columnVisibility, [column.id]: next }
                        })}
                      />
                      ${column.columnDef.header}
                    </label>
                  `
                })}
              </div>
            </div>
          </details>
        </div>
        ${this.table?.error ? html`<div class="error">${this.table.error}</div>` : nothing}
        <div class="table-frame" role="table" aria-label=${this.table?.title ?? 'Orders'}>
          ${loading ? html`<div class="loading" aria-hidden="true"></div>` : nothing}
          <div class="table-scrollport" ${ref(this.bodyViewportRef)} @scroll=${this.handleScroll}>
            <div class="table-plane">
              ${this.resizeGuideX >= 0 ? html`<span class="resize-guide" style=${`--ld-resize-guide-x:${this.resizeGuideX}px`}></span>` : nothing}
              ${this.renderGroupHeaderRows(headers)}
              ${this.renderHeaderRow(headers)}
              ${this.availableRows === 0 && !loading ? html`<div class="empty">Waiting for table data</div>` : html`
                <div class="canvas" role="rowgroup" style=${`height:${totalHeight}px`}>
                  <div class="grid-lines" aria-hidden="true">
                    ${columnLineOffsets.map((offset) => html`<span class="grid-line" style=${`left:${offset}px`}></span>`)}
                  </div>
                  ${visibleRows.map((slot) => {
                    if (slot.kind === 'skeleton') return this.renderSkeletonSegment(headers, slot.index)
                    const key = rowKey(slot.row, slot.index)
                    const tanstackRow = tanstackRows.get(key)
                    return this.renderRowSegment(tanstackRow ? visibleCellsForRow(tanstackRow, this.columnVisibility) : [], slot.row, slot.index, key)
                  })}
                </div>
              `}
            </div>
          </div>
        </div>
        <div class="footer">
          <span><strong>${rowRange}</strong>${this.visibleLoading ? html` · loading` : nothing}${this.table.isCapped ? html` · browsing first ${this.table.rowCap.toLocaleString()}` : nothing}</span>
          <span>${selectedText}</span>
        </div>
      </section>
    `
  }

  private ensureBlocksForScroll(): void {
    if (this.availableRows <= 0) return
    const currentStart = Math.floor(Math.floor(this.viewportTop / this.rowHeight) / this.chunkSize) * this.chunkSize
    const desired = this.desiredStarts(currentStart)
    const desiredSet = new Set(desired)
    const loadedStarts = new Set(blockIDs.map((id) => this.blocks[id]?.start ?? -1))
    const expectedStarts = new Set([...this.expectedBlocks.values()].map((request) => request.start))
    const missingStarts = desired.filter((start) => !loadedStarts.has(start) && !expectedStarts.has(start))

    if (missingStarts.length > 1 || !loadedStarts.has(currentStart) && !expectedStarts.has(currentStart)) {
      this.scheduleJumpBlock(currentStart)
      return
    }

    this.clearJumpTimer()
    const usedBlocks = new Set<BlockID>()

    for (const start of missingStarts) {
      const block = this.reusableBlock(desiredSet, usedBlocks)
      if (!block) continue
      usedBlocks.add(block)
      this.emitBlock(block, start, this.table.sort, this.table.resetVersion)
    }
  }

  private scheduleEnsureBlocksForScroll(): void {
    if (this.scrollFrame) return
    this.scrollFrame = requestAnimationFrame(() => {
      this.scrollFrame = 0
      this.ensureBlocksForScroll()
    })
  }

  private scheduleJumpBlock(start: number): void {
    if (this.jumpTimer && this.pendingJumpStart === start) return
    this.pendingJumpStart = start
    this.requestUpdate()
    this.clearJumpTimer()
    this.jumpTimer = window.setTimeout(() => {
      this.jumpTimer = 0
      this.emitBlock('all', this.pendingJumpStart, this.table.sort, this.table.resetVersion)
    }, 75)
  }

  private clearJumpTimer(): void {
    if (!this.jumpTimer) return
    clearTimeout(this.jumpTimer)
    this.jumpTimer = 0
  }

  private desiredStarts(currentStart: number): number[] {
    const starts = currentStart <= 0
      ? [0, this.chunkSize, this.chunkSize * 2]
      : [Math.max(0, currentStart - this.chunkSize), currentStart, currentStart + this.chunkSize]
    return starts.filter((start, index, all) => start < this.availableRows && all.indexOf(start) === index)
  }

  private reusableBlock(desiredStarts: Set<number>, usedBlocks: Set<BlockID>): BlockID | undefined {
    return blockIDs.find((id) => !usedBlocks.has(id) && !desiredStarts.has(this.blocks[id]?.start ?? -1))
      ?? blockIDs.find((id) => !usedBlocks.has(id))
  }

  private emitBlock(block: BlockID | 'all', start: number, sort = this.table.sort, resetVersion = this.table.resetVersion): void {
    const count = this.chunkSize
    const requestSeq = ++this.requestSeq
    if (block === 'all') {
      this.expectedBlocks.clear()
      const starts = this.allBlockStarts(start)
      blockIDs.forEach((id, index) => {
        const expectedStart = starts[index]
        this.expectedBlocks.set(id, { start: expectedStart, requestSeq, resetVersion, sort })
      })
    } else {
      this.expectedBlocks.set(block, { start, requestSeq, resetVersion, sort })
    }
    this.requestUpdate()
    this.dispatchEvent(new CustomEvent<TableBlockCommand>('ld-table-window-change', {
      bubbles: true,
      composed: true,
      detail: {
        table: this.tableId || 'orders',
        block,
        start,
        count,
        requestSeq,
        sort,
        resetVersion,
      },
    }))
  }

  private allBlockStarts(start: number): number[] {
    const currentStart = Math.max(0, Math.floor(start / this.chunkSize) * this.chunkSize)
    if (currentStart <= 0) return [0, this.chunkSize, this.chunkSize * 2]
    return [Math.max(0, currentStart - this.chunkSize), currentStart, currentStart + this.chunkSize]
  }

  private rowRangeText(): string {
    if (!this.table.totalRows || !this.availableRows) return 'No rows'
    const firstIndex = Math.min(this.availableRows - 1, Math.max(0, Math.floor(this.viewportTop / this.rowHeight)))
    const visibleRows = Math.max(1, Math.ceil((this.viewportHeight || this.rowHeight) / this.rowHeight))
    const lastIndex = Math.min(this.availableRows, firstIndex + visibleRows)
    return `${(firstIndex + 1).toLocaleString()}-${lastIndex.toLocaleString()} of ${this.table.totalRows.toLocaleString()}`
  }

  private mergeIncomingBlocks(): void {
    const defaults = emptyBlocks()
    for (const id of blockIDs) {
      const incoming = this.table.blocks[id]
      if (!incoming) continue
      if (!this.shouldAcceptBlock(id, incoming)) continue
      const defaultBlock = defaults[id]
      const carriesRows = incoming.rows.length > 0
      const carriesNonDefaultStart = incoming.start !== defaultBlock.start
      const cacheIsEmpty = this.blockCache[id].rows.length === 0
      if (carriesRows || carriesNonDefaultStart || cacheIsEmpty) {
        this.blockCache[id] = { ...incoming, rows: incoming.rows }
        if (incoming.requestSeq > 0) this.latestAcceptedSeq.set(id, incoming.requestSeq)
        const expected = this.expectedBlocks.get(id)
        if (expected && this.blockMatchesExpected(incoming, expected)) {
          this.expectedBlocks.delete(id)
        }
      }
    }
  }

  private shouldAcceptBlock(id: BlockID, incoming: TableBlock): boolean {
    const expected = this.expectedBlocks.get(id)
    if (expected) return this.blockMatchesExpected(incoming, expected)

    if (incoming.requestSeq > 0) {
      const lastAcceptedSeq = this.latestAcceptedSeq.get(id) ?? 0
      return incoming.requestSeq >= lastAcceptedSeq
        && incoming.resetVersion === this.table.resetVersion
        && sameSort(incoming.sort, this.table.sort)
    }

    return incoming.resetVersion === 0
      || incoming.resetVersion === this.table.resetVersion
      && sameSort(incoming.sort, this.table.sort)
  }

  private blockMatchesExpected(block: TableBlock, expected: ExpectedBlockRequest): boolean {
    return block.start === expected.start
      && block.requestSeq === expected.requestSeq
      && block.resetVersion === expected.resetVersion
      && sameSort(block.sort, expected.sort)
  }

  private runAction(action: VisualAction): void {
    this.renderRoot.querySelector<HTMLDetailsElement>('.visual-options')?.removeAttribute('open')
    if (action === 'clear-selection') {
      this.selectedRowId = ''
      this.selectedCellKey = ''
    }
    this.dispatchEvent(
      new CustomEvent('ld-visual-action', {
        bubbles: true,
        composed: true,
        detail: {
          action,
          visualType: 'table',
          visualId: this.tableId || 'orders',
          title: this.table?.title ?? 'Orders',
          columns: this.columns,
          rows: this.exportRows(),
          selection: this.selectedRowId ? [this.selectedRowId] : [],
          table: {
            ...(this.table ?? emptyTable),
            blocks: this.blocks,
            rows: this.exportRows(),
            columns: this.columns,
          },
        },
      }),
    )
  }

  private exportRows(): TableRow[] {
    return this.loadedRows.map(({ row }) => {
      const next: TableRow = {}
      for (const column of this.columns) {
        next[column.key] = formatCell(row[column.key], column)
      }
      return next
    })
  }
}

customElements.define('ld-data-table', DataTable)
