import test from 'node:test'
import assert from 'node:assert/strict'
import {
  UI_ROW_SELECTION_FIELD,
  buildRowSelectionCommand,
  rowClickSelectionAction,
  rowIsSelected,
  rowSelectionFromEntries,
  selectionLabels,
  selectedRowCount,
} from './selection'
import type { InteractionConfig, InteractionSelectionEntry, TableRow } from './types'

const semanticInteraction: InteractionConfig = {
  kind: 'row_selection',
  toggle: true,
  mappings: [
    { field: 'orders.order_id', value: 'order_id' },
    { field: 'orders.status', value: 'status', label: 'status_label' },
  ],
}

test('buildRowSelectionCommand emits configured semantic mappings', () => {
  const command = buildRowSelectionCommand({
    sourceId: 'orders_table',
    interaction: semanticInteraction,
    key: 'o1',
    row: { order_id: 'o1', status: 'delivered', status_label: 'Delivered' },
    selectionAction: { action: 'replace', toggle: false },
  })

  assert.deepEqual(command, {
    sourceKind: 'table',
    sourceId: 'orders_table',
    interactionKind: 'row_selection',
    action: 'replace',
    toggle: false,
    mappings: [
      { field: 'orders.order_id', value: 'o1', label: 'o1' },
      { field: 'orders.status', value: 'delivered', label: 'Delivered' },
    ],
  })
})

test('buildRowSelectionCommand emits UI-only row key mappings without semantic mappings', () => {
  const command = buildRowSelectionCommand({
    sourceId: 'orders_table',
    interaction: { kind: 'row_selection', toggle: true, mappings: [] },
    key: 'row-42',
    row: { label: 'ignored' },
    selectionAction: { action: 'set', toggle: true },
  })

  assert.deepEqual(command, {
    sourceKind: 'table',
    sourceId: 'orders_table',
    interactionKind: 'row_selection',
    action: 'set',
    toggle: true,
    mappings: [{ field: UI_ROW_SELECTION_FIELD, value: 'row-42', label: 'row-42' }],
  })
})

test('buildRowSelectionCommand rejects incomplete semantic mapping payloads', () => {
  const command = buildRowSelectionCommand({
    sourceId: 'orders_table',
    interaction: semanticInteraction,
    key: 'o1',
    row: { order_id: 'o1', status_label: 'Delivered' },
    selectionAction: { action: 'replace', toggle: false },
  })

  assert.equal(command, null)
})

test('rowClickSelectionAction keeps the table gesture matrix server-driven', () => {
  assert.deepEqual(rowClickSelectionAction({ selected: false, selectedCount: 0, metaKey: false, ctrlKey: false }), {
    action: 'replace',
    toggle: false,
  })
  assert.deepEqual(rowClickSelectionAction({ selected: true, selectedCount: 1, metaKey: false, ctrlKey: false }), {
    action: 'set',
    toggle: true,
  })
  assert.deepEqual(rowClickSelectionAction({ selected: true, selectedCount: 3, metaKey: false, ctrlKey: false }), {
    action: 'replace',
    toggle: false,
  })
  assert.deepEqual(rowClickSelectionAction({ selected: false, selectedCount: 1, metaKey: false, ctrlKey: true }), {
    action: 'set',
    toggle: true,
  })
  assert.deepEqual(rowClickSelectionAction({ selected: false, selectedCount: 1, metaKey: true, ctrlKey: false }), {
    action: 'set',
    toggle: true,
  })
})

test('rowSelectionFromEntries projects semantic selection entries to loaded rows', () => {
  const rows: Array<{ row: TableRow; key: string }> = [
    { key: 'o1', row: { order_id: 'o1', status: 'delivered' } },
    { key: 'o2', row: { order_id: 'o2', status: 'shipped' } },
  ]
  const selection: InteractionSelectionEntry[] = [
    {
      mappings: [
        { field: 'orders.order_id', value: 'o2', label: 'o2' },
        { field: 'orders.status', value: 'shipped', label: 'Shipped' },
      ],
      label: 'o2',
    },
  ]

  assert.deepEqual(rowSelectionFromEntries(rows, semanticInteraction, selection), { o2: true })
  assert.equal(rowIsSelected(rows[1].row, rows[1].key, semanticInteraction, selection), true)
})

test('rowSelectionFromEntries projects UI-only row-key entries to loaded rows', () => {
  const rows: Array<{ row: TableRow; key: string }> = [
    { key: 'row-1', row: { label: 'First' } },
    { key: 'row-2', row: { label: 'Second' } },
  ]
  const selection: InteractionSelectionEntry[] = [
    { mappings: [{ field: UI_ROW_SELECTION_FIELD, value: 'row-1', label: 'row-1' }], label: 'row-1' },
  ]

  assert.deepEqual(rowSelectionFromEntries(rows, { kind: 'row_selection', mappings: [] }, selection), { 'row-1': true })
  assert.equal(rowIsSelected(rows[1].row, rows[1].key, { kind: 'row_selection', mappings: [] }, selection), false)
})

test('selected count and labels come only from server selection entries', () => {
  const selection: InteractionSelectionEntry[] = [
    { label: 'Delivered' },
    { mappings: [{ field: 'orders.order_id', value: 'o2', label: 'o2' }] },
  ]

  assert.equal(selectedRowCount(selection), 2)
  assert.deepEqual(selectionLabels(selection), ['Delivered', 'o2'])
  assert.equal(selectedRowCount(undefined), 0)
  assert.deepEqual(selectionLabels(undefined), [])
})
