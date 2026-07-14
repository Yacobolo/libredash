export type InteractionSelectionValue = string | number | boolean | null

export interface InteractionMappingIdentity {
  field: string
  fact?: string
  grain?: string
}

export interface InteractionMapping extends InteractionMappingIdentity {
  value: string
  label?: string
}

export interface InteractionSelectionMapping extends InteractionMappingIdentity {
  value: InteractionSelectionValue
  label?: string
}

export interface InteractionSelectionEntry {
  mappings?: InteractionSelectionMapping[]
  label?: string
}

export interface CanonicalInteractionSelection {
  sourceKind: string
  sourceId: string
  entries?: InteractionSelectionEntry[]
}

export function canonicalSelectionEntriesForSource(
  selections: readonly CanonicalInteractionSelection[] | undefined,
  sourceKind: 'visual' | 'table',
  sourceId: string,
): InteractionSelectionEntry[] {
  if (!sourceId) return []
  return (selections ?? [])
    .filter((selection) => selection.sourceKind === sourceKind && selection.sourceId === sourceId)
    .flatMap((selection) => selection.entries ?? [])
}

export function interactionSelectionValue(value: unknown): InteractionSelectionValue | undefined {
  if (value === null) return null
  if (typeof value === 'string' || typeof value === 'boolean') return value
  if (typeof value === 'number' && Number.isFinite(value)) return value
  return undefined
}

export function interactionMappingIdentityEqual(
  left: InteractionMappingIdentity,
  right: InteractionMappingIdentity,
): boolean {
  return left.field === right.field
    && (left.fact ?? '') === (right.fact ?? '')
    && (left.grain ?? '') === (right.grain ?? '')
}

export function interactionMappingKey(mapping: InteractionMappingIdentity, value: InteractionSelectionValue): string {
  return JSON.stringify([mapping.field, mapping.fact ?? null, mapping.grain ?? null, value])
}

export function interactionSelectionLabel(value: InteractionSelectionValue): string {
  return value === null ? '' : String(value)
}
