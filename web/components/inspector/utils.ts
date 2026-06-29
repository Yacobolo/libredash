/**
 * Utility functions for Datastar Inspector
 */

import type { SignalObject } from './types.js'

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function countSignals(obj: unknown, count = 0): number {
  if (typeof obj !== 'object' || obj === null) return count + 1
  for (const value of Object.values(obj as Record<string, unknown>)) {
    count = countSignals(value, count)
  }
  return count
}

export function parseFilterPattern(filterText: string): RegExp {
  if (filterText.startsWith('/') && filterText.lastIndexOf('/') > 0) {
    const lastSlash = filterText.lastIndexOf('/')
    const pattern = filterText.slice(1, lastSlash)
    const flags = filterText.slice(lastSlash + 1)
    try {
      return new RegExp(pattern, flags || 'i')
    } catch {
      return new RegExp(escapeRegex(filterText), 'i')
    }
  }

  if (filterText.includes('*')) {
    const pattern = escapeRegex(filterText).replace(/\\\*/g, '.*')
    return new RegExp(pattern, 'i')
  }

  return new RegExp(escapeRegex(filterText), 'i')
}

export function filterObject(
  obj: Record<string, unknown>,
  regex: RegExp,
  path = ''
): Record<string, unknown> {
  const result: Record<string, unknown> = {}

  for (const [key, value] of Object.entries(obj)) {
    const fullPath = path ? `${path}.${key}` : key

    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
      const filtered = filterObject(value as Record<string, unknown>, regex, fullPath)
      if (Object.keys(filtered).length > 0) {
        result[key] = filtered
      }
    } else if (regex.test(fullPath) || regex.test(String(value))) {
      result[key] = value
    }
  }

  return result
}

export function findChangedPaths(
  oldObj: SignalObject,
  newObj: SignalObject,
  prefix = ''
): Set<string> {
  const changed = new Set<string>()

  for (const [key, newValue] of Object.entries(newObj)) {
    const path = prefix ? `${prefix}.${key}` : key
    const oldValue = oldObj[key]

    if (typeof newValue === 'object' && newValue !== null && !Array.isArray(newValue)) {
      if (typeof oldValue === 'object' && oldValue !== null && !Array.isArray(oldValue)) {
        const nestedChanged = findChangedPaths(
          oldValue as SignalObject,
          newValue as SignalObject,
          path
        )
        nestedChanged.forEach((p) => changed.add(p))
      } else {
        changed.add(path)
      }
    } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
      changed.add(path)
    }
  }

  for (const key of Object.keys(oldObj)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (!(key in newObj)) {
      changed.add(path)
    }
  }

  return changed
}
