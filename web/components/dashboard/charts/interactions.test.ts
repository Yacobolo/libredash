import { expect, test } from 'bun:test'
import { chartInteractionDetailForDatum } from './interactions'
import { selectedRows } from './utils'
import type { ChartPayload } from './types'
import {
  applyOptimisticInteraction,
  canonicalSelectionEntriesForSource,
  validateInteractionCommand,
} from '../interaction-selection'

test('chartInteractionDetailForDatum preserves typed scalar values and mapping identity', () => {
  const detail = chartInteractionDetailForDatum({
    id: 'activity_by_month',
    interaction: {
      kind: 'point_selection',
      toggle: true,
      mappings: [
        { field: 'activity_date', grain: 'month', value: 'period', label: 'period_label' },
        { field: 'ratings.rating_bucket', fact: 'ratings', value: 'bucket', label: 'bucket_label' },
      ],
    },
  }, {
    period: '2026-07-01',
    period_label: 'July 2026',
    bucket: 5,
    bucket_label: 'Five stars',
  })

  expect(detail).toEqual({
    sourceKind: 'visual',
    sourceId: 'activity_by_month',
    interactionKind: 'point_selection',
    action: 'set',
    toggle: true,
    mappings: [
      { field: 'activity_date', grain: 'month', value: '2026-07-01', label: 'July 2026' },
      { field: 'ratings.rating_bucket', fact: 'ratings', value: 5, label: 'Five stars' },
    ],
  })
})

test('chartInteractionDetailForDatum preserves false, zero, and null values', () => {
  const payload: ChartPayload = {
    id: 'typed_values',
    interaction: {
      mappings: [
        { field: 'enabled', value: 'enabled' },
        { field: 'score', value: 'score' },
        { field: 'segment', value: 'segment' },
      ],
    },
  }

  expect(chartInteractionDetailForDatum(payload, { enabled: false, score: 0, segment: null })?.mappings).toEqual([
    { field: 'enabled', value: false, label: 'false' },
    { field: 'score', value: 0, label: '0' },
    { field: 'segment', value: null, label: '' },
  ])
})

test('chartInteractionDetailForDatum rejects missing and non-scalar values', () => {
  const payload: ChartPayload = {
    id: 'typed_values',
    interaction: { mappings: [{ field: 'score', value: 'score' }] },
  }

  expect(chartInteractionDetailForDatum(payload, {})).toBeUndefined()
  expect(chartInteractionDetailForDatum(payload, { score: { nested: true } })).toBeUndefined()
})

test('selectedRows includes fact, grain, and scalar type in tuple identity', () => {
  const payload: ChartPayload = {
    interaction: {
      mappings: [
        { field: 'activity_date', grain: 'month', value: 'period' },
        { field: 'rating_bucket', fact: 'ratings', value: 'bucket' },
      ],
    },
    selection: [{
      mappings: [
        { field: 'activity_date', grain: 'month', value: '2026-07-01' },
        { field: 'rating_bucket', fact: 'ratings', value: 1 },
      ],
    }],
    data: [
      { period: '2026-07-01', bucket: 1 },
      { period: '2026-07-01', bucket: '1' },
    ],
  }

  const selection = selectedRows(payload)
  expect(selection.hasSelection).toBe(true)
  expect(selection.isSelected(payload.data?.[0] ?? {})).toBe(true)
  expect(selection.isSelected(payload.data?.[1] ?? {})).toBe(false)

  payload.selection = [{
    mappings: [
      { field: 'activity_date', grain: 'day', value: '2026-07-01' },
      { field: 'rating_bucket', fact: 'ratings', value: 1 },
    ],
  }]
  expect(selectedRows(payload).isSelected(payload.data?.[0] ?? {})).toBe(false)

  payload.selection = [{
    mappings: [
      { field: 'activity_date', grain: 'month', value: '2026-07-01' },
      { field: 'rating_bucket', fact: 'tags', value: 1 },
    ],
  }]
  expect(selectedRows(payload).isSelected(payload.data?.[0] ?? {})).toBe(false)
})

test('canonical selections are projected only to their source component', () => {
  const selections = [
    {
      id: 'visual:ratings:point_selection',
      sourceKind: 'visual',
      sourceId: 'ratings',
      interactionKind: 'point_selection',
      label: 'One star',
      order: 1,
      entries: [{ mappings: [{ field: 'rating_bucket', fact: 'ratings', value: 1 }] }],
    },
    {
      id: 'visual:movies:row_selection',
      sourceKind: 'visual',
      sourceId: 'movies',
      interactionKind: 'row_selection',
      label: 'Movie 1',
      order: 2,
      entries: [{ mappings: [{ field: 'movies.movie_id', fact: 'movies', value: 1 }] }],
    },
  ]

  expect(canonicalSelectionEntriesForSource(selections, 'visual', 'ratings')).toEqual([
    { mappings: [{ field: 'rating_bucket', fact: 'ratings', value: 1 }] },
  ])
  expect(canonicalSelectionEntriesForSource(selections, 'visual', 'movies')).toEqual([
    { mappings: [{ field: 'movies.movie_id', fact: 'movies', value: 1 }] },
  ])
})

test('optimistic selections replace rapidly without allowing an older value to return', () => {
  const configured = {
    kind: 'point_selection',
    toggle: false,
    mappings: [{ field: 'release_decade', value: 'label' }],
    targets: ['rating_count', 'movie_table'],
  }
  const first = {
    sourceKind: 'visual',
    sourceId: 'ratings_by_decade',
    interactionKind: 'point_selection',
    action: 'replace',
    toggle: false,
    mappings: [{ field: 'release_decade', value: '1980s', label: '1980s' }],
  } as const
  const second = {
    ...first,
    mappings: [{ field: 'release_decade', value: '1990s', label: '1990s' }],
  } as const

  expect(validateInteractionCommand(first, configured)).toBe(true)
  const afterFirst = applyOptimisticInteraction([], first)
  const afterSecond = applyOptimisticInteraction(afterFirst, second)

  expect(canonicalSelectionEntriesForSource(afterSecond, 'visual', 'ratings_by_decade')).toEqual([
    { mappings: [{ field: 'release_decade', value: '1990s', label: '1990s' }], label: '1990s' },
  ])
})

test('optimistic validation rejects forged identities and incomplete composite tuples', () => {
  const configured = {
    kind: 'point_selection',
    toggle: true,
    mappings: [
      { field: 'activity_date', grain: 'month', value: 'period' },
      { field: 'ratings.rating_bucket', fact: 'ratings', value: 'bucket' },
    ],
  }
  const valid = {
    sourceKind: 'visual',
    sourceId: 'activity',
    interactionKind: 'point_selection',
    action: 'set',
    toggle: true,
    mappings: [
      { field: 'activity_date', grain: 'month', value: '2026-07-01' },
      { field: 'ratings.rating_bucket', fact: 'ratings', value: 5 },
    ],
  } as const

  expect(validateInteractionCommand(valid, configured)).toBe(true)
  expect(validateInteractionCommand({ ...valid, mappings: valid.mappings.slice(0, 1) }, configured)).toBe(false)
  expect(validateInteractionCommand({
    ...valid,
    mappings: [valid.mappings[0], { ...valid.mappings[1], fact: 'tags' }],
  }, configured)).toBe(false)
  expect(validateInteractionCommand({ ...valid, mappings: [{ ...valid.mappings[0], value: {} }] }, configured)).toBe(false)
})
