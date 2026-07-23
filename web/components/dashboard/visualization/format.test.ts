import { expect, test } from 'bun:test'
import type { VisualizationFormat } from '../../../generated/visualization'
import { formatValue } from './format'

test('TypeScript formatting matches shared Go fixtures', async () => {
  const fixtures = await Bun.file(new URL('../../../../api/visualization/conformance/formatting.json', import.meta.url)).json() as Array<{ locale: string; format: VisualizationFormat; value: unknown; expected: string }>
  for (const fixture of fixtures) expect(formatValue(fixture.locale, fixture.format, fixture.value)).toBe(fixture.expected)
})

test('formatting fails closed for unsupported locale and currency', () => {
  expect(() => formatValue('de-DE', { kind: 'number' }, 1)).toThrow()
  expect(() => formatValue('en-US', { kind: 'currency', currency: 'XYZ' }, 1)).toThrow()
})
