import type { ChartDatum, ChartPayload } from './types'

export type ChartInteractionDetail = {
  sourceKind: 'visual'
  sourceId: string
  interactionKind: string
  action: 'set'
  toggle: boolean
  mappings: Array<{ field: string; value: string; label: string }>
}

export function chartInteractionDetailForDatum(payload: ChartPayload, datum: ChartDatum): ChartInteractionDetail | undefined {
  const interaction = payload.interaction
  const mappings = interaction?.mappings ?? []
  if (!payload.id || mappings.length === 0) return undefined
  const commandMappings = mappings.map((mapping) => {
    const value = stringFromDatum(datum, mapping.value)
    return {
      field: mapping.field,
      value,
      label: stringFromDatum(datum, mapping.label || mapping.value) || value,
    }
  })
  if (commandMappings.some((mapping) => !mapping.field || !mapping.value)) return undefined
  return {
    sourceKind: 'visual',
    sourceId: payload.id,
    interactionKind: interaction?.kind || 'point_selection',
    action: 'set',
    toggle: interaction?.toggle !== false,
    mappings: commandMappings,
  }
}

function stringFromDatum(datum: ChartDatum, key: string): string {
  const value = datum[key]
  if (value === undefined || value === null) return ''
  return String(value)
}
