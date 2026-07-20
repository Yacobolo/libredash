import { afterEach, expect, test } from 'bun:test'
import { checkSignalContract } from './signal-contract'

const originalLocation = globalThis.location
const originalWarn = console.warn

afterEach(() => {
  Object.defineProperty(globalThis, 'location', { value: originalLocation, configurable: true })
  console.warn = originalWarn
})

test('checkSignalContract skips missing signal roots before hydration', () => {
  const warnings: unknown[][] = []
  Object.defineProperty(globalThis, 'location', { value: { hostname: 'localhost', search: '' }, configurable: true })
  console.warn = (...args: unknown[]) => warnings.push(args)

  checkSignalContract('route page', null, { kind: 'required' })
  checkSignalContract('route page', undefined, { kind: 'required' })

  expect(warnings).toHaveLength(0)
})

test('checkSignalContract warns for partially hydrated signal roots', () => {
  const warnings: unknown[][] = []
  Object.defineProperty(globalThis, 'location', { value: { hostname: 'localhost', search: '' }, configurable: true })
  console.warn = (...args: unknown[]) => warnings.push(args)

  checkSignalContract('route page', { kind: 'catalog' }, { kind: 'required', dashboards: 'required' })

  expect(warnings).toEqual([['[LeapView] route page is missing signal fields: dashboards']])
})
