import { expect, test } from 'bun:test'
import type { VisualizationEnvelope } from '../../../../generated/visualization'
import { assertSafeVegaLiteProgram, interactionCommandForDatum } from './vega-lite'

test('Vega-Lite policy accepts compiled fields and rejects network and expression surfaces', () => {
  expect(assertSafeVegaLiteProgram(JSON.stringify({ mark: 'bar', data: { name: 'primary' }, encoding: { x: { field: 'month' }, y: { field: 'revenue' } } }), ['month', 'revenue'])).toBeTruthy()
  expect(() => assertSafeVegaLiteProgram(JSON.stringify({ data: { url: 'https://example.test/data.json' }, mark: 'bar' }), [])).toThrow(/not allowed/)
  expect(() => assertSafeVegaLiteProgram(JSON.stringify({ data: { name: 'primary' }, transform: [{ calculate: 'datum.x', as: 'y' }] }), ['x'])).toThrow(/not allowed/)
  expect(() => assertSafeVegaLiteProgram(JSON.stringify({ mark: 'bar', encoding: { x: { field: 'secret' } } }), ['public'])).toThrow(/compiled dataset schema/)
	expect(() => assertSafeVegaLiteProgram(JSON.stringify({ mark: 'not-a-vega-lite-mark', data: { name: 'primary' } }), [])).toThrow(/pinned Vega-Lite schema/)
})

test('Vega-Lite interactions return through compiled host mappings only', () => {
  const envelope = {
    visualID: 'custom_orders', spec: { interactions: [{ id: 'point_selection', kind: 'select', mode: 'multiple', mappings: [{ source: { dataset: 'primary', field: 'status' }, targetFieldID: 'orders.status', targetFactID: 'orders' }] }] },
  } as VisualizationEnvelope
  expect(interactionCommandForDatum(envelope, { status: 'delivered', rendererRowKey: { forged: true } })).toEqual({
    sourceKind: 'visual', sourceId: 'custom_orders', interactionKind: 'point_selection', action: 'set', toggle: true,
    mappings: [{ field: 'orders.status', fact: 'orders', value: 'delivered', label: 'delivered' }],
  })
  expect(interactionCommandForDatum(envelope, { status: { forged: true } })).toBeUndefined()
})
