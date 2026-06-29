export type FilterType = 'date_range' | 'multi_select' | 'text'

export type URLParamValue = string | string[]
export type URLParamsShape = Record<string, URLParamValue>

export type FilterDefinition = {
  type: FilterType
  label: string
  dataset: string
  dimension: string
  default?: FilterDefault
  custom?: boolean
  presets?: DatePreset[]
  operator?: string
  values?: { source?: string; limit?: number }
  defaultOperator?: string
  operators?: string[]
  urlParam?: string
  fromURLParam?: string
  toURLParam?: string
  operatorURLParam?: string
}

export type FilterConfigItem = FilterDefinition & { id: string }
export type FilterConfig = FilterConfigItem[]

export type FilterDefault = {
  preset?: string
  from?: string
  to?: string
  operator?: string
  value?: string
  values?: string[]
}

export type DatePreset = {
  value: string
  label: string
  from?: string
  to?: string
  relativeDays?: number
}

export type FilterControl = {
  type: FilterType | string
  operator?: string
  preset?: string
  from?: string
  to?: string
  value?: string
  values?: string[]
}

export type InteractionSelection = {
  label?: string
  entries?: Array<{
    label?: string
    mappings?: Array<{ field?: string; value?: string; label?: string }>
  }>
}

export type FiltersSignal = {
  controls: Record<string, FilterControl>
  selections: InteractionSelection[]
}

export const emptyFilters: FiltersSignal = { controls: {}, selections: [] }

export function filterConfigEntries(config: FilterConfig): Array<[string, FilterDefinition]> {
  return config.map((definition) => [definition.id, definition])
}

export function filterConfigMap(config: FilterConfig): Record<string, FilterDefinition> {
  const mapped: Record<string, FilterDefinition> = {}
  for (const definition of config) {
    mapped[definition.id] = definition
  }
  return mapped
}

export function defaultControl(definition: FilterDefinition): FilterControl {
  switch (definition.type) {
    case 'date_range':
      return { type: 'date_range', preset: definition.default?.preset || 'all', from: definition.default?.from || '', to: definition.default?.to || '' }
    case 'multi_select':
      return { type: 'multi_select', operator: definition.operator || 'in', values: [...(definition.default?.values ?? [])] }
    case 'text':
      return { type: 'text', operator: definition.default?.operator || definition.defaultOperator || 'contains', value: definition.default?.value || '' }
    default:
      return { type: definition.type || '' }
  }
}

export function filtersFromURLParams(config: FilterConfig, filters: FiltersSignal, params: URLParamsShape): FiltersSignal {
  const next: FiltersSignal = {
    controls: { ...(filters.controls ?? {}) },
    selections: [...(filters.selections ?? [])],
  }

  for (const [name, definition] of filterConfigEntries(config)) {
    const base = defaultControl(definition)
    const current = next.controls[name] ?? base
    switch (definition.type) {
      case 'date_range':
        next.controls[name] = dateControlFromParams(definition, current, params)
        break
      case 'multi_select':
        next.controls[name] = {
          ...current,
          type: 'multi_select',
          operator: current.operator || definition.operator || 'in',
          values: definition.urlParam ? paramArray(params[definition.urlParam]).sort() : [...(base.values ?? [])],
        }
        break
      case 'text': {
        const value = definition.urlParam ? paramString(params[definition.urlParam]) : base.value ?? ''
        const operator = definition.operatorURLParam ? paramString(params[definition.operatorURLParam]) : ''
        next.controls[name] = {
          ...current,
          type: 'text',
          operator: operator && (definition.operators ?? []).includes(operator) ? operator : base.operator,
          value,
        }
        break
      }
    }
  }

  return next
}

export function filtersToURLParams(config: FilterConfig, filters: FiltersSignal): URLParamsShape {
  const params: URLParamsShape = {}

  for (const [name, definition] of filterConfigEntries(config)) {
    const control = filters.controls?.[name] ?? defaultControl(definition)
    const base = defaultControl(definition)
    switch (definition.type) {
      case 'date_range':
        if (!definition.urlParam) break
        if (control.from || control.to || control.preset === 'custom') {
          params[definition.urlParam] = 'custom'
          addString(params, definition.fromURLParam, control.from)
          addString(params, definition.toURLParam, control.to)
          break
        }
        if (control.preset && control.preset !== base.preset) {
          params[definition.urlParam] = control.preset
        }
        break
      case 'multi_select':
        if (definition.urlParam && (control.values ?? []).length > 0) {
          params[definition.urlParam] = [...(control.values ?? [])].filter(Boolean).sort()
        }
        break
      case 'text': {
        const value = (control.value ?? '').trim()
        if (!definition.urlParam || !value) break
        params[definition.urlParam] = value
        if (definition.operatorURLParam && control.operator && control.operator !== base.operator) {
          params[definition.operatorURLParam] = control.operator
        }
        break
      }
    }
  }

  return params
}

function dateControlFromParams(definition: FilterDefinition, current: FilterControl, params: URLParamsShape): FilterControl {
  const base = defaultControl(definition)
  const preset = definition.urlParam ? paramString(params[definition.urlParam]) : ''
  const from = definition.fromURLParam ? paramString(params[definition.fromURLParam]) : ''
  const to = definition.toURLParam ? paramString(params[definition.toURLParam]) : ''

  if (from || to) {
    return { ...current, type: 'date_range', preset: 'custom', from, to }
  }
  if (!preset) {
    return base
  }
  if (preset === 'custom') {
    return { ...current, type: 'date_range', preset: 'custom', from: '', to: '' }
  }
  if ((definition.presets ?? []).some((item) => item.value === preset)) {
    return { ...current, type: 'date_range', preset, from: '', to: '' }
  }
  return base
}

function addString(params: URLParamsShape, key: string | undefined, value: string | undefined): void {
  const trimmed = (value ?? '').trim()
  if (key && trimmed) {
    params[key] = trimmed
  }
}

function paramString(value: URLParamValue | undefined): string {
  if (Array.isArray(value)) {
    return value[0] ?? ''
  }
  return (value ?? '').trim()
}

function paramArray(value: URLParamValue | undefined): string[] {
  const values = Array.isArray(value) ? value : value ? [value] : []
  return [...new Set(values.map((item) => item.trim()).filter(Boolean))]
}
