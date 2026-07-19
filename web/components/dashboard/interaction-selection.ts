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
  id?: string
  sourceKind: string
  sourceId: string
  interactionKind?: string
  entries?: InteractionSelectionEntry[]
  label?: string
  order?: number
}

export interface InteractionConfigLike {
  kind?: string
  toggle?: boolean
  mappings?: InteractionMapping[]
  targets?: string[]
}

export interface OptimisticInteractionCommand {
  sourceKind: 'visual'
  sourceId: string
  interactionKind: string
  action: 'set' | 'replace' | 'clear'
  toggle: boolean
  mappings: InteractionSelectionMapping[]
}

export function canonicalSelectionEntriesForSource(
  selections: readonly CanonicalInteractionSelection[] | undefined,
  sourceKind: 'visual',
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

export function validateInteractionCommand(
  command: OptimisticInteractionCommand,
  configured: InteractionConfigLike | undefined,
): boolean {
  if (!configured || !command.sourceId || command.sourceKind !== 'visual') return false
  if (!['set', 'replace', 'clear'].includes(command.action)) return false
  if (command.interactionKind !== (configured.kind || 'point_selection')) return false
  if (command.action === 'clear') return command.mappings.length === 0

  const mappings = configured.mappings ?? []
  if (mappings.length === 0) {
    return command.interactionKind === 'row_selection'
      && command.mappings.length === 1
      && command.mappings[0]?.field === '__libredash.rowKey'
      && !command.mappings[0]?.fact
      && !command.mappings[0]?.grain
      && validCommandMapping(command.mappings[0])
  }
  if (command.mappings.length !== mappings.length) return false
  return mappings.every((mapping) => {
    const matches = command.mappings.filter((candidate) => interactionMappingIdentityEqual(candidate, mapping))
    return matches.length === 1 && validCommandMapping(matches[0])
  })
}

export function applyOptimisticInteraction(
  selections: readonly CanonicalInteractionSelection[] | undefined,
  command: OptimisticInteractionCommand,
): CanonicalInteractionSelection[] {
  const selectionID = `${command.sourceKind}:${command.sourceId}:${command.interactionKind}`
  const next: CanonicalInteractionSelection[] = []
  let maxOrder = 0
  let changed = false
  for (const selection of selections ?? []) {
    maxOrder = Math.max(maxOrder, selection.order ?? 0)
    const sameSource = selection.id === selectionID || (
      selection.sourceKind === command.sourceKind
      && selection.sourceId === command.sourceId
      && selection.interactionKind === command.interactionKind
    )
    if (!sameSource) {
      next.push(copyCanonicalSelection(selection))
      continue
    }
    changed = true
    if (command.action === 'clear') continue
    const entries = command.action === 'replace'
      ? updateOptimisticEntries([], command.mappings, false)
      : updateOptimisticEntries(selection.entries ?? [], command.mappings, command.toggle)
    if (entries.length === 0) continue
    next.push({
      ...copyCanonicalSelection(selection),
      id: selectionID,
      entries,
      label: selectionEntriesLabel(entries),
    })
  }
  if (!changed && command.action !== 'clear') {
    const entries = updateOptimisticEntries([], command.mappings, false)
    if (entries.length > 0) {
      next.push({
        id: selectionID,
        sourceKind: command.sourceKind,
        sourceId: command.sourceId,
        interactionKind: command.interactionKind,
        entries,
        label: selectionEntriesLabel(entries),
        order: maxOrder + 1,
      })
    }
  }
  return next
}

function updateOptimisticEntries(
  existing: readonly InteractionSelectionEntry[],
  mappings: readonly InteractionSelectionMapping[],
  toggle: boolean,
): InteractionSelectionEntry[] {
  if (mappings.length === 0) return []
  const entry: InteractionSelectionEntry = {
    mappings: mappings.map((mapping) => ({
      ...mapping,
      label: mapping.label || optimisticValueLabel(mapping.value),
    })),
  }
  entry.label = selectionEntryLabel(entry)
  const next: InteractionSelectionEntry[] = []
  let found = false
  for (const candidate of existing) {
    if (selectionEntryKey(candidate) === selectionEntryKey(entry)) {
      found = true
      if (toggle) continue
    }
    next.push(copySelectionEntry(candidate))
  }
  if (!found) next.push(entry)
  return next
}

function selectionEntryKey(entry: InteractionSelectionEntry): string {
  return JSON.stringify((entry.mappings ?? [])
    .map((mapping) => interactionMappingKey(mapping, mapping.value))
    .sort())
}

function selectionEntryLabel(entry: InteractionSelectionEntry): string {
  return (entry.mappings ?? [])
    .map((mapping) => mapping.label || optimisticValueLabel(mapping.value))
    .filter(Boolean)
    .join(', ')
}

function selectionEntriesLabel(entries: readonly InteractionSelectionEntry[]): string {
  return entries.map((entry) => entry.label || selectionEntryLabel(entry)).filter(Boolean).join(', ')
}

function copySelectionEntry(entry: InteractionSelectionEntry): InteractionSelectionEntry {
  return { ...entry, mappings: (entry.mappings ?? []).map((mapping) => ({ ...mapping })) }
}

function copyCanonicalSelection(selection: CanonicalInteractionSelection): CanonicalInteractionSelection {
  return { ...selection, entries: (selection.entries ?? []).map(copySelectionEntry) }
}

function validCommandMapping(mapping: InteractionSelectionMapping | undefined): boolean {
  return Boolean(mapping)
    && typeof mapping?.field === 'string'
    && (mapping.fact === undefined || typeof mapping.fact === 'string')
    && (mapping.grain === undefined || typeof mapping.grain === 'string')
    && (mapping.label === undefined || typeof mapping.label === 'string')
    && interactionSelectionValue(mapping.value) !== undefined
}

function optimisticValueLabel(value: InteractionSelectionValue): string {
  return value === null ? 'null' : String(value)
}
