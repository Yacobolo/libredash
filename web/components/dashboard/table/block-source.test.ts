import { describe, expect, test } from 'bun:test'

import { emptyTable, normalizeTable, preserveKnownCardinality } from './block-source'

describe('progressive table cardinality', () => {
  test('normalizes an unknown total without hiding the available window', () => {
    const table = normalizeTable({
      ...emptyTable,
      totalRows: 0,
      totalRowsKnown: false,
      availableRows: 10_000,
    })

    expect(table.totalRowsKnown).toBe(false)
    expect(table.availableRows).toBe(10_000)
  })

  test('preserves an exact total across a later scrolling block', () => {
    const previous = normalizeTable({
      ...emptyTable,
      resetVersion: 4,
      totalRows: 1_234,
      totalRowsKnown: true,
      availableRows: 1_234,
      isCapped: false,
    })
    const incoming = normalizeTable({
      ...emptyTable,
      resetVersion: 4,
      totalRows: 0,
      totalRowsKnown: false,
      availableRows: 10_000,
      isCapped: false,
    })

    const merged = preserveKnownCardinality(previous, incoming)
    expect(merged.totalRowsKnown).toBe(true)
    expect(merged.totalRows).toBe(1_234)
    expect(merged.availableRows).toBe(1_234)
  })

  test('does not carry an old total into a reset generation', () => {
    const previous = normalizeTable({
      ...emptyTable,
      resetVersion: 4,
      totalRows: 1_234,
      totalRowsKnown: true,
      availableRows: 1_234,
    })
    const incoming = normalizeTable({
      ...emptyTable,
      resetVersion: 5,
      totalRowsKnown: false,
      availableRows: 10_000,
    })

    expect(preserveKnownCardinality(previous, incoming).totalRowsKnown).toBe(false)
  })

  test('does not replace a known cardinality with a false overshoot total', () => {
    const previous = normalizeTable({
      ...emptyTable,
      resetVersion: 4,
      totalRows: 75,
      totalRowsKnown: true,
      availableRows: 75,
    })
    const incoming = normalizeTable({
      ...emptyTable,
      resetVersion: 4,
      totalRows: 150,
      totalRowsKnown: true,
      availableRows: 150,
      blocks: {
        ...emptyTable.blocks,
        b: { ...emptyTable.blocks.b, start: 150, rows: [] },
      },
    })

    const merged = preserveKnownCardinality(previous, incoming)
    expect(merged.totalRows).toBe(75)
    expect(merged.availableRows).toBe(75)
  })
})
