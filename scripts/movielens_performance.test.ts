import { expect, test } from 'bun:test'
import { aggregateSamples, logTextAfterCursor, parseRefreshSummaries, percentile } from './movielens_performance'

test('percentile uses nearest-rank values', () => {
  expect(percentile([5, 1, 3, 2, 4], 50)).toBe(3)
  expect(percentile([5, 1, 3, 2, 4], 95)).toBe(5)
  expect(percentile([], 95)).toBe(0)
})

test('log cursor excludes refresh IDs from earlier server processes', () => {
	const previous = 'refresh-1 from previous process\n'
	const current = 'refresh-1 from current process\n'
	const bytes = new TextEncoder().encode(previous + current)
	expect(logTextAfterCursor(bytes, new TextEncoder().encode(previous).byteLength)).toBe(current)
	expect(logTextAfterCursor(bytes, bytes.byteLength + 1)).toBe(previous + current)
})

test('refresh summaries are correlated without accepting raw non-refresh logs', () => {
  const log = [
    '{"event":"dashboard_refresh","refreshId":"keep","queryCount":3,"cancellationCount":1,"stageTimingsMs":{"planning":4,"database":12}}',
    '{"event":"dashboard_refresh","refreshId":"ignore","queryCount":99}',
    'time=now level=INFO msg="unrelated"',
  ].join('\n')
  const summaries = parseRefreshSummaries(log, new Set(['keep']))
  expect(summaries).toEqual([{
    refreshId: 'keep',
    queryCount: 3,
    cancellationCount: 1,
    stageTimingsMs: { planning: 4, database: 12 },
  }])
  expect(aggregateSamples([{
    interaction: 'decade', iteration: 1, refreshId: 'keep', generation: 2, feedbackMs: 40, settlementMs: 200,
  }], summaries)).toEqual({
    feedbackMs: { samples: 1, p50: 40, p95: 40 },
    settlementMs: { samples: 1, p50: 200, p95: 200 },
    queryCounts: [3],
    cancellations: 1,
    stageTimingsMs: {
      database: { samples: 1, p50: 12, p95: 12 },
      planning: { samples: 1, p50: 4, p95: 4 },
    },
  })
})

test('refresh summaries accept the coordinator slog text format', () => {
  const log = [
    'time=2026-07-14T13:00:00.000Z level=INFO msg="dashboard refresh" event=dashboard_refresh refreshId=keep generation=3 queryCount=4 cancellationCount=2 stageTimingsMs="map[admissionWait:3 connectionWait:5 database:17 endToEnd:31 planning:6]" outcome=complete',
    'time=2026-07-14T13:00:01.000Z level=INFO msg="dashboard refresh" event=dashboard_refresh refreshId=ignore queryCount=99 cancellationCount=0 stageTimingsMs=map[endToEnd:1] outcome=complete',
  ].join('\n')

  expect(parseRefreshSummaries(log, new Set(['keep']))).toEqual([{
    refreshId: 'keep',
    queryCount: 4,
    cancellationCount: 2,
    stageTimingsMs: {
      admissionWait: 3,
      connectionWait: 5,
      database: 17,
      endToEnd: 31,
      planning: 6,
    },
  }])
})
