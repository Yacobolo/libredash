import { chromium, type Page } from '@playwright/test'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'

type Sample = {
  interaction: string
  iteration: number
  refreshId: string
  generation: number
  feedbackMs: number
  settlementMs: number
}

type RefreshSummary = {
  refreshId: string
  queryCount: number | null
  cancellationCount: number | null
  stageTimingsMs: Record<string, number>
}

const projectRoot = process.cwd()
const outputPath = Bun.env.LIBREDASH_PERF_OUTPUT ?? join(projectRoot, '.tmp/movielens-performance.json')
const logPath = Bun.env.LIBREDASH_PERF_LOG ?? join(projectRoot, '.tmp/dev-server.log')
const iterations = positiveInteger(Bun.env.LIBREDASH_PERF_ITERATIONS, 5)
const scenarios = [
  { name: 'release_decade', visualId: 'tags_per_rating_by_decade' },
  { name: 'activity_month', visualId: 'activity_by_month' },
  { name: 'rating_bucket', visualId: 'rating_distribution' },
]

export function percentile(values: number[], percentileRank: number): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((left, right) => left - right)
  const index = Math.max(0, Math.ceil((percentileRank / 100) * sorted.length) - 1)
  return round(sorted[Math.min(index, sorted.length - 1)])
}

export function parseRefreshSummaries(log: string, refreshIDs: ReadonlySet<string>): RefreshSummary[] {
  const summaries: RefreshSummary[] = []
  for (const line of log.split('\n')) {
    const summary = parseJSONRefreshSummary(line) ?? parseSlogRefreshSummary(line)
    if (summary && refreshIDs.has(summary.refreshId)) summaries.push(summary)
  }
  return summaries
}

export function logTextAfterCursor(log: Uint8Array, cursor: number): string {
	const start = cursor >= 0 && cursor <= log.byteLength ? cursor : 0
	return new TextDecoder().decode(log.subarray(start))
}

function parseJSONRefreshSummary(line: string): RefreshSummary | null {
  const objectStart = line.indexOf('{')
  if (objectStart < 0) return null
  let value: Record<string, unknown>
  try {
    value = JSON.parse(line.slice(objectStart)) as Record<string, unknown>
  } catch {
    return null
  }
  const event = stringField(value, 'event') || stringField(value, 'message')
  if (event !== 'dashboard_refresh' && event !== 'dashboard refresh') return null
  const refreshId = stringField(value, 'refreshId') || stringField(value, 'refresh_id')
  if (!refreshId) return null
  return {
    refreshId,
    queryCount: numberField(value, 'queryCount', 'query_count'),
    cancellationCount: numberField(value, 'cancellationCount', 'cancellation_count', 'cancellations'),
    stageTimingsMs: timingFields(value.stageTimingsMs ?? value.stage_timings_ms ?? value.stageTimings),
  }
}

function parseSlogRefreshSummary(line: string): RefreshSummary | null {
  const event = slogField(line, 'event') || slogField(line, 'msg')
  if (event !== 'dashboard_refresh' && event !== 'dashboard refresh') return null
  const refreshId = slogField(line, 'refreshId') || slogField(line, 'refresh_id')
  if (!refreshId) return null
  return {
    refreshId,
    queryCount: slogNumberField(line, 'queryCount', 'query_count'),
    cancellationCount: slogNumberField(line, 'cancellationCount', 'cancellation_count', 'cancellations'),
    stageTimingsMs: slogTimingFields(line),
  }
}

function slogField(line: string, name: string): string {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = line.match(new RegExp(`(?:^|\\s)${escaped}=(?:"((?:\\\\.|[^"])*)"|([^\\s]+))`))
  if (!match) return ''
  return match[1] === undefined ? match[2] : match[1].replace(/\\"/g, '"').replace(/\\\\/g, '\\')
}

function slogNumberField(line: string, ...names: string[]): number | null {
  for (const name of names) {
    const raw = slogField(line, name)
    if (!raw) continue
    const value = Number(raw)
    if (Number.isFinite(value)) return value
  }
  return null
}

function slogTimingFields(line: string): Record<string, number> {
	const raw = slogField(line, 'stageTimingsMs') || slogField(line, 'stage_timings_ms') || slogField(line, 'stageTimings')
	const match = raw.match(/^map\[([^\]]*)\]$/)
	if (!match) return {}
  const fields: Record<string, number> = {}
  for (const entry of match[1].trim().split(/\s+/)) {
    const separator = entry.lastIndexOf(':')
    if (separator <= 0) continue
    const value = Number(entry.slice(separator + 1))
    if (Number.isFinite(value)) fields[entry.slice(0, separator)] = value
  }
  return fields
}

export function aggregateSamples(samples: Sample[], summaries: RefreshSummary[]) {
  const stageValues = new Map<string, number[]>()
  const cancellationCounts = summaries.map((summary) => summary.cancellationCount).filter((value): value is number => value !== null)
  for (const summary of summaries) {
    for (const [name, duration] of Object.entries(summary.stageTimingsMs)) {
      stageValues.set(name, [...(stageValues.get(name) ?? []), duration])
    }
  }
  return {
    feedbackMs: timingStats(samples.map((sample) => sample.feedbackMs)),
    settlementMs: timingStats(samples.map((sample) => sample.settlementMs)),
    queryCounts: summaries.map((summary) => summary.queryCount).filter((value): value is number => value !== null),
    cancellations: cancellationCounts.length > 0 ? cancellationCounts.reduce((sum, value) => sum + value, 0) : null,
    stageTimingsMs: Object.fromEntries([...stageValues].sort(([left], [right]) => left.localeCompare(right)).map(([name, values]) => [name, timingStats(values)])),
  }
}

async function main(): Promise<void> {
	const baseURL = await resolveBaseURL()
	const logCursor = await refreshLogCursor()
  const dashboardURL = new URL('/workspaces/movielens/dashboards/ratings-overview/pages/overview', baseURL).toString()
  const browser = await chromium.launch()
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 } })
  const samples: Sample[] = []
  try {
    const response = await page.goto(dashboardURL, { waitUntil: 'domcontentloaded', timeout: 120_000 })
    if (!response?.ok()) throw new Error(`MovieLens dashboard returned status ${response?.status() ?? 'unknown'}`)
    await waitForDashboardIdle(page, 600_000)

    // Warm the compiled runtime, filter-option cache, reader pool, and every fixed interaction shape.
    for (const scenario of scenarios) {
      await runInteraction(page, scenario.visualId, 0)
      await clearSelections(page)
    }

    for (const scenario of scenarios) {
      for (let iteration = 0; iteration < iterations; iteration++) {
        const sample = await runInteraction(page, scenario.visualId, iteration)
        samples.push({ interaction: scenario.name, iteration: iteration + 1, ...sample })
        await clearSelections(page)
      }
    }
  } finally {
    await browser.close()
  }

  const refreshIDs = new Set(samples.map((sample) => sample.refreshId).filter(Boolean))
	const summaries = await readRefreshSummaries(refreshIDs, logCursor)
  const byInteraction = Object.fromEntries(scenarios.map((scenario) => {
    const selected = samples.filter((sample) => sample.interaction === scenario.name)
    const selectedIDs = new Set(selected.map((sample) => sample.refreshId))
    return [scenario.name, aggregateSamples(selected, summaries.filter((summary) => selectedIDs.has(summary.refreshId)))]
  }))
  const result = {
    generatedAt: new Date().toISOString(),
    baseURL,
    iterations,
    samples,
    overall: aggregateSamples(samples, summaries),
    interactions: byInteraction,
    observability: {
      refreshSummariesFound: summaries.length,
      refreshSummariesExpected: refreshIDs.size,
      logPath,
    },
  }
  await mkdir(dirname(outputPath), { recursive: true })
  await writeFile(outputPath, `${JSON.stringify(result, null, 2)}\n`)
  console.log(JSON.stringify(result, null, 2))
}

async function runInteraction(page: Page, visualId: string, datumOffset: number): Promise<Omit<Sample, 'interaction' | 'iteration'>> {
  const before = await dashboardStatus(page)
  const startedAt = performance.now()
  await page.locator('ld-dashboard-page').evaluate((element: any, input) => {
    const chart = element.shadowRoot?.querySelector(`ld-echart[visual-id="${input.visualId}"]`) as any
    const payload = chart?.chart
    const rows = payload?.data ?? []
    if (rows.length === 0) throw new Error(`visual ${input.visualId} has no data`)
    const datum = rows[input.datumOffset % rows.length]
    const mappings = (payload.interaction?.mappings ?? []).map((mapping: any) => {
      const value = datum[mapping.value]
      if (value === undefined || (typeof value === 'object' && value !== null)) throw new Error(`visual ${input.visualId} mapping ${mapping.field} has no scalar value`)
      const labelValue = mapping.label ? datum[mapping.label] : value
      return {
        field: mapping.field,
        ...(mapping.fact !== undefined ? { fact: mapping.fact } : {}),
        ...(mapping.grain !== undefined ? { grain: mapping.grain } : {}),
        value,
        label: labelValue === null ? '' : String(labelValue),
      }
    })
    element.dispatchEvent(new CustomEvent('ld-interaction-select', {
      bubbles: true,
      composed: true,
      detail: {
        sourceKind: 'visual',
        sourceId: input.visualId,
        interactionKind: payload.interaction?.kind || 'point_selection',
        action: 'replace',
        toggle: false,
        mappings,
      },
    }))
  }, { visualId, datumOffset })

  const feedback = await waitForStatus(page, (status) => status.generation > before.generation && status.loading, 10_000)
  const feedbackMs = performance.now() - startedAt
  const settled = await waitForStatus(page, (status) => status.generation === feedback.generation && !status.loading, 600_000)
  return {
    refreshId: settled.refreshId,
    generation: settled.generation,
    feedbackMs: round(feedbackMs),
    settlementMs: round(performance.now() - startedAt),
  }
}

async function clearSelections(page: Page): Promise<void> {
  const status = await dashboardStatus(page)
  await page.locator('ld-dashboard-page').evaluate((element: Element) => {
    element.dispatchEvent(new CustomEvent('ld-selection-clear', { bubbles: true, composed: true }))
  })
  await waitForStatus(page, (next) => next.generation > status.generation && !next.loading, 600_000)
}

async function waitForDashboardIdle(page: Page, timeoutMs: number): Promise<void> {
  await page.waitForSelector('ld-dashboard-page')
  await waitForStatus(page, (status) => status.generation > 0 && !status.loading, timeoutMs)
}

type DashboardStatusSnapshot = { refreshId: string; generation: number; loading: boolean }

async function dashboardStatus(page: Page): Promise<DashboardStatusSnapshot> {
  return page.locator('ld-dashboard-page').evaluate((element: any) => ({
    refreshId: String(element.status?.refreshId ?? ''),
    generation: Number(element.status?.generation ?? 0),
    loading: Boolean(element.status?.loading),
  }))
}

async function waitForStatus(page: Page, predicate: (status: DashboardStatusSnapshot) => boolean, timeoutMs: number): Promise<DashboardStatusSnapshot> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const status = await dashboardStatus(page)
    if (predicate(status)) return status
    await page.waitForTimeout(10)
  }
  throw new Error(`timed out after ${timeoutMs}ms waiting for dashboard refresh state`)
}

async function resolveBaseURL(): Promise<string> {
  if (Bun.env.LIBREDASH_BASE_URL) return Bun.env.LIBREDASH_BASE_URL
  try {
    const port = (await readFile(join(projectRoot, '.tmp/dev-server.port'), 'utf8')).trim()
    if (/^\d+$/.test(port)) return `http://localhost:${port}`
  } catch {}
  return 'http://localhost:8195'
}

async function refreshLogCursor(): Promise<number> {
	try {
		return (await readFile(logPath)).byteLength
	} catch {
		return 0
	}
}

async function readRefreshSummaries(refreshIDs: ReadonlySet<string>, cursor: number): Promise<RefreshSummary[]> {
	try {
		const log = await readFile(logPath)
		return parseRefreshSummaries(logTextAfterCursor(log, cursor), refreshIDs)
	} catch {
    return []
  }
}

function timingStats(values: number[]) {
  return { samples: values.length, p50: percentile(values, 50), p95: percentile(values, 95) }
}

function positiveInteger(raw: string | undefined, fallback: number): number {
  const value = Number(raw)
  return Number.isInteger(value) && value > 0 ? value : fallback
}

function stringField(value: Record<string, unknown>, name: string): string {
  return typeof value[name] === 'string' ? value[name] as string : ''
}

function numberField(value: Record<string, unknown>, ...names: string[]): number | null {
  for (const name of names) {
    if (typeof value[name] === 'number' && Number.isFinite(value[name])) return value[name] as number
  }
  return null
}

function timingFields(value: unknown): Record<string, number> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, number] => typeof entry[1] === 'number' && Number.isFinite(entry[1])))
}

function round(value: number): number {
  return Math.round(value * 100) / 100
}

if (import.meta.main) await main()
