import type { TableColumn, TableRow } from './types'

export function formatCell(value: unknown, column: TableColumn): string {
  if (value === null || value === undefined || value === '') return '-'
  const format = column.format || inferredFormat(column)
  if (format === 'currency' && Number.isFinite(Number(value))) {
    return `R$ ${Number(value).toLocaleString(undefined, { maximumFractionDigits: 2 })}`
  }
  if (format === 'integer' && Number.isFinite(Number(value))) {
    return Number(value).toLocaleString(undefined, { maximumFractionDigits: 0 })
  }
  if (format === 'decimal' && Number.isFinite(Number(value))) {
    return Number(value).toFixed(2)
  }
  if (format === 'days' && Number.isFinite(Number(value))) {
    return `${Number(value)}d`
  }
  if (Number.isFinite(Number(value)) && column.align === 'right') {
    return Number(value).toLocaleString(undefined, { maximumFractionDigits: 2 })
  }
  return String(value)
}

function inferredFormat(column: TableColumn): string {
  if (column.key === 'revenue' || column.measure === 'revenue') return 'currency'
  if (column.key === 'review_score') return 'decimal'
  if (column.key === 'delivery_days') return 'days'
  return ''
}

export function defaultDirection(column: TableColumn): 'asc' | 'desc' {
  return ['revenue', 'review_score', 'delivery_days', 'purchase_date'].includes(column.key) || column.role === 'measure' ? 'desc' : 'asc'
}

export function rowKey(row: TableRow, fallback: number): string {
  const id = row.order_id
  if (typeof id === 'string' && id) return id
  const rowID = row.__rowKey
  if (typeof rowID === 'string' && rowID) return rowID
  return String(fallback)
}
