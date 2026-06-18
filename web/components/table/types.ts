export type SortDirection = 'asc' | 'desc'
export type BlockID = 'a' | 'b' | 'c'
export type TableKind = 'data_table' | 'matrix_table' | 'pivot_table'

export interface TableSort {
  key: string
  direction: SortDirection
}

export interface TableColumn {
  key: string
  label: string
  align?: 'left' | 'right'
  role?: 'row_header' | 'measure'
  group?: string
  measure?: string
  columnValue?: string
}

export type TableRow = Record<string, unknown>

export interface TableBlock {
  start: number
  requestSeq: number
  resetVersion: number
  sort: TableSort
  rows: TableRow[]
}

export interface TableSignal {
  version: number
  kind: TableKind
  title: string
  columns: TableColumn[]
  totalRows: number
  availableRows: number
  isCapped: boolean
  rowCap: number
  chunkSize: number
  rowHeight: number
  resetVersion: number
  sort: TableSort
  blocks: Record<BlockID, TableBlock>
  loadingBlock: string
  error: string
}

export interface TableBlockCommand {
  table: string
  block: BlockID | 'all'
  start: number
  count: number
  requestSeq: number
  sort: TableSort
  resetVersion: number
}

export type VisualAction = 'focus' | 'show-data' | 'copy-data' | 'export-csv' | 'clear-selection'
export type VisibleRowSlot = { kind: 'row'; row: TableRow; index: number } | { kind: 'skeleton'; index: number }

export interface ExpectedBlockRequest {
  start: number
  requestSeq: number
  resetVersion: number
  sort: TableSort
}

export type TanStackTableRow = TableRow & {
  __absoluteIndex: number
  __rowKey: string
}

export const blockIDs: BlockID[] = ['a', 'b', 'c']
export const defaultChunkSize = 50
export const defaultRowHeight = 34
export const defaultSort: TableSort = { key: 'purchase_date', direction: 'desc' }
