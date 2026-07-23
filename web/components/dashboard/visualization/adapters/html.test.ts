import { expect, test } from 'bun:test'

import type { VisualizationEnvelope } from '../../../../generated/visualization'
import { defaultRendererContext } from '../host-controller'
import { kpiText } from './html'

test('HTML KPI values use the field formatting contract', () => {
  const envelope = {
    schemaVersion: 3, visualID: 'revenue', rendererID: 'html', specRevision: 'sha256:test', dataRevision: 1,
    spec: {
      kind: 'kpi', title: 'Revenue', datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Revenue', format: { kind: 'currency', currency: 'BRL' } }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' }, accessibility: { title: 'Revenue', description: 'Revenue' }, interactions: [],
      value: { dataset: 'primary', field: 'value' }, presentation: { trend: 'positive', tone: 'success' },
    },
    dataState: { kind: 'inline', specRevision: 'sha256:test', dataRevision: 1, generation: 1, datasets: [{ id: 'primary', specRevision: 'sha256:test', dataRevision: 1, generation: 1, columns: ['value'], rows: [[1234.5]], completeness: 'complete' }] },
    selection: [], status: { kind: 'ready' }, diagnostics: [],
  } as VisualizationEnvelope

  expect(kpiText(envelope)).toBe('R$1,234.50')
  expect(kpiText(envelope, { ...defaultRendererContext, locale: 'pt-BR' })).toBe('R$\u00a01.234,50')
})
