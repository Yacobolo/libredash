import { expect, test } from 'bun:test'
import type { DashboardVisualizationSignal } from '../../../generated/signals'
import { DashboardVisualizationSignalDecoder } from './signal-envelope'

test('dashboard visualization signals keep large frames opaque and reconstruct canonical envelopes', () => {
  const rows = Array.from({ length: 20_000 }, (_, index) => [`zip-${index}`, index])
  const signal = visualizationSignal(JSON.stringify({
    specRevision: 'spec-1', dataRevision: 1, generation: 1, kind: 'inline',
    datasets: [{ id: 'primary', specRevision: 'spec-1', dataRevision: 1, generation: 1, columns: ['zip', 'value'], rows, completeness: 'complete' }],
  }))
  const decoder = new DashboardVisualizationSignalDecoder()

  const envelope = decoder.decode(signal)

  expect(typeof signal.dataStateJson).toBe('string')
  expect((envelope?.dataState as any).datasets[0].rows).toHaveLength(20_000)
})

test('dashboard visualization signal decoder reuses data on status-only patches and fails closed', () => {
  const decoder = new DashboardVisualizationSignalDecoder()
  const encoded = JSON.stringify({
    specRevision: 'spec-1', dataRevision: 1, generation: 1, kind: 'inline',
    datasets: [{ id: 'primary', specRevision: 'spec-1', dataRevision: 1, generation: 1, columns: ['value'], rows: [[1]], completeness: 'complete' }],
  })
  const ready = decoder.decode(visualizationSignal(encoded))
  const loading = decoder.decode({ ...visualizationSignal(encoded), status: { kind: 'loading' } })

  expect(loading?.dataState).toBe(ready?.dataState)
  expect(loading?.status.kind).toBe('loading')
  expect(decoder.decode(visualizationSignal('{invalid'))).toBeUndefined()
})

function visualizationSignal(dataStateJson: string): DashboardVisualizationSignal {
  return {
    schemaVersion: 2,
    visualID: 'map',
    rendererID: 'maplibre',
    specRevision: 'spec-1',
    dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Map', datasets: [], dataBudget: { maxRows: 20_000, requiredCompleteness: 'complete' },
      accessibility: { title: 'Map', description: 'Map' }, interactions: [],
      value: { dataset: 'primary', field: 'value' }, presentation: { trend: 'neutral' },
    },
    dataStateJson,
    selection: [],
    status: { kind: 'ready' },
    diagnostics: [],
  }
}
