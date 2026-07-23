import { expect, test } from 'bun:test'

import type { VisualizationEnvelope } from '../../generated/visualization'
import { visualizationSelectionEntries, type CanonicalInteractionSelection } from './interaction-selection'

test('canonical visual selections project back to renderer-independent datum references', () => {
  const envelope = {
    visualID: 'customers', dataRevision: 7,
    spec: { interactions: [{
      id: 'point_selection', kind: 'select',
      mappings: [
        { source: { dataset: 'primary', field: 'customer_id' }, targetFieldID: 'customers.customer_id', targetFactID: 'customers' },
        { source: { dataset: 'primary', field: 'state' }, targetFieldID: 'customers.state', targetFactID: 'customers' },
      ],
    }] },
  } as VisualizationEnvelope
  const selections: CanonicalInteractionSelection[] = [{
    sourceKind: 'visual', sourceId: 'customers', interactionKind: 'point_selection',
    entries: [{ label: 'Customer 2', mappings: [
      { field: 'customers.state', fact: 'customers', value: 'RJ' },
      { field: 'customers.customer_id', fact: 'customers', value: 'c-2' },
    ] }],
  }]

  expect(visualizationSelectionEntries(envelope, selections)).toEqual([{
    datum: { dataset: 'primary', dataRevision: 7, identity: { customer_id: 'c-2', state: 'RJ' } },
    label: 'Customer 2',
  }])
  expect(visualizationSelectionEntries(envelope, [{ ...selections[0]!, sourceId: 'other' }])).toEqual([])
})
