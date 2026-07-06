import { filtersFromURLParams, type FilterConfig, type FiltersSignal, type URLParamsShape } from './filter-url'
import { loadDatastarRuntime } from '../../shared/datastar-runtime'

const dataStarURLSyncEvent = 'datastar-url-params-sync'

type DatastarRuntime = {
  effect(fn: () => void): () => void
  getPath<T = unknown>(path: string): T | undefined
}

type URLSyncDetail = {
  params: URLParamsShape
  url: string
}

function normalizeURLParams(value: unknown): URLParamsShape {
  const record = typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {}
  const out: URLParamsShape = {}

  for (const [key, raw] of Object.entries(record)) {
    if (Array.isArray(raw)) {
      const seen = new Set<string>()
      out[key] = raw.flatMap((item) => {
        if (typeof item !== 'string') return []
        const trimmed = item.trim()
        if (!trimmed || seen.has(trimmed)) return []
        seen.add(trimmed)
        return [trimmed]
      })
      continue
    }

    out[key] = typeof raw === 'string' ? raw.trim() : ''
  }

  return out
}

function toQueryString(value: unknown): string {
  const params = normalizeURLParams(value)
  const search = new URLSearchParams()

  for (const [key, raw] of Object.entries(params)) {
    if (Array.isArray(raw)) {
      for (const item of raw) search.append(key, item)
      continue
    }
    if (raw) search.set(key, raw)
  }

  return search.toString()
}

function toURL(path: string, value: unknown): string {
  const query = toQueryString(value)
  return query ? `${path}?${query}` : path
}

function readLocation(shape: unknown): URLParamsShape {
  const base = normalizeURLParams(shape)
  const url = new URL(window.location.href)
  const next: URLParamsShape = {}

  for (const [key, raw] of Object.entries(base)) {
    if (Array.isArray(raw)) {
      next[key] = url.searchParams.getAll(key).map((item) => item.trim()).filter(Boolean)
      continue
    }
    next[key] = url.searchParams.get(key)?.trim() ?? raw
  }

  return next
}

function emit(params: URLParamsShape): URLParamsShape {
  window.dispatchEvent(new CustomEvent<URLSyncDetail>(dataStarURLSyncEvent, {
    detail: {
      params,
      url: `${window.location.pathname}${window.location.search}`,
    },
  }))
  return params
}

function updateHistory(method: 'pushState' | 'replaceState', value: unknown, path = window.location.pathname): string {
  const next = toURL(path, value)
  const current = `${window.location.pathname}${window.location.search}`
  if (next !== current) {
    window.history[method]({}, '', next)
  }
  return next
}

function replace(value: unknown, path = window.location.pathname): string {
  return updateHistory('replaceState', value, path)
}

function push(value: unknown, path = window.location.pathname): string {
  return updateHistory('pushState', value, path)
}

let popstateBound = false

function bindPopstate(fallback: unknown): void {
  if (popstateBound) return
  popstateBound = true
  window.addEventListener('popstate', () => {
    emit(readLocation(fallback))
  })
}

async function bindSignalPopstate(): Promise<void> {
  try {
    const runtime = await loadDatastarRuntime() as DatastarRuntime
    let dispose: (() => void) | null = null
    let bound = false
    dispose = runtime.effect(() => {
      if (bound) return
      const fallback = normalizeURLParams(runtime.getPath('urlParamShape'))
      if (Object.keys(fallback).length === 0) return
      bindPopstate(fallback)
      bound = true
      dispose?.()
      dispose = null
    })
  } catch (error) {
    console.error('LibreDash URL sync failed to bind Datastar signals', error)
  }
}

const datastarURLSync = {
  bindPopstate,
  push,
  replace,
}

const libreDashFilterURL = {
  fromParams(config: FilterConfig, filters: FiltersSignal, params: URLParamsShape): FiltersSignal {
    return filtersFromURLParams(config, filters, params)
  },
}

declare global {
  interface Window {
    DatastarURLSync?: typeof datastarURLSync
    LibreDashFilterURL?: typeof libreDashFilterURL
  }
}

window.DatastarURLSync = datastarURLSync
window.LibreDashFilterURL = libreDashFilterURL

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => void bindSignalPopstate(), { once: true })
} else {
  void bindSignalPopstate()
}
