import type { VisualizationEnvelope } from '../../../../../generated/visualization'
import { formatValue } from '../../format'
import { geographicDataset } from './data'

export type RenderedFeatureLocator = Readonly<{ layer?: { id?: string }; properties?: Record<string, unknown> | null }>

export function mapTooltipEntries(envelope: VisualizationEnvelope, features: readonly RenderedFeatureLocator[]): Array<{ label: string; value: string }> {
  if (envelope.spec.kind !== 'geographic') return []
  for (const feature of features) {
    const datasetID = feature.properties?.__ld_dataset, rowIndex = feature.properties?.__ld_row_index, layerID = feature.properties?.__ld_layer_id
    if (typeof datasetID !== 'string' || typeof rowIndex !== 'number' || !Number.isInteger(rowIndex) || rowIndex < 0 || typeof layerID !== 'string') continue
    const dataset = geographicDataset(envelope, datasetID)
    const layer = envelope.spec.layers.find((candidate) => candidate.id === layerID)
    const row = dataset?.rows[rowIndex]
    if (!dataset || !layer || !row) continue
    const fields = layer.tooltip.length ? layer.tooltip : layer.label ? [layer.label] : []
    return fields.flatMap((reference) => {
      if (reference.dataset !== datasetID) return []
      const column = dataset.columns.indexOf(reference.field)
      if (column < 0 || column >= row.length) return []
      const schema = envelope.spec.datasets.find((candidate) => candidate.id === datasetID)
      const field = schema?.fields.find((candidate) => candidate.id === reference.field)
      const raw = row[column]
      let value: string
      try { value = field?.format ? formatValue('en-US', field.format, raw) : raw == null ? '—' : String(raw) } catch { value = raw == null ? '—' : String(raw) }
      return [{ label: field?.label ?? reference.field, value }]
    })
  }
  return []
}

export function mapAccessibleData(envelope: VisualizationEnvelope, limit = 100): {
  columns: Array<{ id: string; label: string }>
  rows: string[][]
  totalRows: number
} {
  if (envelope.spec.kind !== 'geographic' || limit < 1) return { columns: [], rows: [], totalRows: 0 }
  const schema = envelope.spec.datasets[0]
  if (!schema) return { columns: [], rows: [], totalRows: 0 }
  const dataset = geographicDataset(envelope, schema.id)
  if (!dataset) return { columns: [], rows: [], totalRows: 0 }
  const fieldIDs: string[] = []
  const add = (reference?: { dataset: string; field: string }) => {
    if (reference?.dataset === schema.id && !fieldIDs.includes(reference.field)) fieldIDs.push(reference.field)
  }
  for (const layer of envelope.spec.layers) {
    for (const reference of layer.tooltip) add(reference)
    if (layer.tooltip.length > 0) continue
    add(layer.label)
    if (layer.kind === 'choropleth') { add(layer.join); add(layer.value); add(layer.category) }
    if (layer.kind === 'point') { add(layer.latitude); add(layer.longitude); add(layer.value); add(layer.category) }
    if (layer.kind === 'heat' || layer.kind === 'density') { add(layer.latitude); add(layer.longitude); add(layer.value) }
    if (layer.kind === 'path') { add(layer.path); add(layer.order); add(layer.latitude); add(layer.longitude); add(layer.value); add(layer.category) }
  }
  if (fieldIDs.length === 0) for (const field of schema.fields.slice(0, 3)) add({ dataset: schema.id, field: field.id })
  const columns = fieldIDs.flatMap((id) => {
    const field = schema.fields.find((candidate) => candidate.id === id)
    return field ? [{ id, label: field.label }] : []
  })
  const indexes = columns.map((column) => dataset.columns.indexOf(column.id))
  const fields = columns.map((column) => schema.fields.find((field) => field.id === column.id))
  const rows = dataset.rows.slice(0, Math.min(limit, dataset.rows.length)).map((row) => indexes.map((index, columnIndex) => {
    const raw = index >= 0 ? row[index] : null
    const field = fields[columnIndex]
    try { return field?.format ? formatValue('en-US', field.format, raw) : raw == null ? '—' : String(raw) } catch { return raw == null ? '—' : String(raw) }
  }))
  return { columns, rows, totalRows: dataset.rows.length }
}
