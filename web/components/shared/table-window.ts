export type VirtualRowRange = {
  first: number
  last: number
}

export function virtualRowRange(totalRows: number, viewportTop: number, viewportHeight: number, rowHeight: number, overscan = 2): VirtualRowRange {
  const safeTotalRows = Math.max(0, totalRows)
  const safeRowHeight = Math.max(1, rowHeight)
  if (safeTotalRows <= 0) return { first: 0, last: 0 }
  const first = Math.max(0, Math.floor(viewportTop / safeRowHeight) - overscan)
  const visibleCount = Math.max(1, Math.ceil((viewportHeight || safeRowHeight) / safeRowHeight) + overscan * 2)
  return { first, last: Math.min(safeTotalRows, first + visibleCount) }
}

export function tableWindowOffsetForViewport(viewportTop: number, rowHeight: number, limit: number): number {
  const safeRowHeight = Math.max(1, rowHeight)
  const safeLimit = Math.max(1, limit)
  const firstVisible = Math.max(0, Math.floor(viewportTop / safeRowHeight))
  return Math.max(0, Math.floor(firstVisible / safeLimit) * safeLimit)
}
