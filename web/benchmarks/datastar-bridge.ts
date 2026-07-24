import { html, LitElement, type TemplateResult } from 'lit'
import { jsonAttribute } from '../components/shared/json-attribute'
import { DatastarLit } from '../components/shared/datastar-lit'
import { loadDatastarRuntime } from '../components/shared/datastar-runtime'
import { DatastarWatcher } from '../vendor/ignition/datastar-watcher'

type BridgeVariant = 'legacy' | 'ignition' | 'datastar-lit'
type SignalPayload = Record<string, any>
type BenchmarkMetrics = {
  jsonParseCalls: number
  jsonParseMs: number
  jsonStringifyCalls: number
  jsonStringifyMs: number
  setAttributeCalls: number
  setAttributeMs: number
  hostAttributeMutations: number
  hostChildListMutations: number
  shadowMutations: number
  litUpdates: number
  longTaskCount: number
  longTaskMs: number
}
type BenchmarkResult = BenchmarkMetrics & {
  variant: BridgeVariant
  iterations: number
  warmup: number
  initialMs: number
  updateTotalMs: number
  updateMeanMs: number
  updateP50Ms: number
  updateP95Ms: number
  updateMaxMs: number
  usedJSHeapSize: number | null
  renderedTextLength: number
}

declare global {
  interface Window {
    __DATSTAR_BRIDGE_BENCH__?: { variant: BridgeVariant; bootStart: number }
    __DATSTAR_BRIDGE_BENCH_METRICS__?: BenchmarkMetrics
    runDatastarBridgeBenchmark?: (options?: { iterations?: number; warmup?: number }) => Promise<BenchmarkResult>
  }
}

const emptyPage = {}
const emptyFilterContract = { applicationMode: 'immediate', definitions: {}, bindings: {} }
const emptyFilterState = { revision: 0, appliedControls: {}, draftControls: {}, dirtyBindings: [], defaultsRevision: '' }
const emptyFilterOptionPages = {}
const emptyVisuals = {}
const emptyTables = {}
const emptyStatus = {}

class BenchLegacyPage extends LitElement {
  static properties = {
    page: { attribute: 'page', converter: jsonAttribute(emptyPage) },
    filterContract: { attribute: 'filtercontract', converter: jsonAttribute(emptyFilterContract) },
    filterState: { attribute: 'filterstate', converter: jsonAttribute(emptyFilterState) },
    filterOptionPages: { attribute: 'filteroptionpages', converter: jsonAttribute(emptyFilterOptionPages) },
    visuals: { attribute: 'visuals', converter: jsonAttribute(emptyVisuals) },
    tables: { attribute: 'tables', converter: jsonAttribute(emptyTables) },
    status: { attribute: 'status', converter: jsonAttribute(emptyStatus) },
  }

  declare page: unknown
  declare filterContract: unknown
  declare filterState: unknown
  declare filterOptionPages: unknown
  declare visuals: unknown
  declare tables: unknown
  declare status: unknown

  override render(): TemplateResult {
    return renderSummary(summaryFromPayload(this.payload()))
  }

  override updated(): void {
    countLitUpdate()
  }

  private payload(): SignalPayload {
    return {
      page: this.page ?? emptyPage,
      filterContract: this.filterContract ?? emptyFilterContract,
      filterState: this.filterState ?? emptyFilterState,
      filterOptionPages: this.filterOptionPages ?? emptyFilterOptionPages,
      visuals: this.visuals ?? emptyVisuals,
      tables: this.tables ?? emptyTables,
      status: this.status ?? emptyStatus,
    }
  }
}

class BenchIgnitionPage extends DatastarWatcher(LitElement) {
  protected override morphReactive = false

  override render(): TemplateResult {
    return renderSummary(summaryFromPayload(this.dsRoot as SignalPayload))
  }

  override updated(): void {
    countLitUpdate()
  }
}

class BenchDatastarLitPage extends DatastarLit(LitElement) {
  override render(): TemplateResult {
    return renderSummary(summaryFromPayload({
      page: this.signal('page', emptyPage),
      filterContract: this.signal('filterContract', emptyFilterContract),
      filterState: this.signal('filterState', emptyFilterState),
      filterOptionPages: this.signal('filterOptionPages', emptyFilterOptionPages),
      visuals: this.signal('visuals', emptyVisuals),
      tables: this.signal('tables', emptyTables),
      status: this.signal('status', emptyStatus),
    }))
  }

  override updated(): void {
    countLitUpdate()
  }
}

customElements.define('bench-legacy-page', BenchLegacyPage)
customElements.define('bench-ignition-page', BenchIgnitionPage)
customElements.define('bench-datastar-lit-page', BenchDatastarLitPage)

window.runDatastarBridgeBenchmark = async (options = {}) => {
  const variant = window.__DATSTAR_BRIDGE_BENCH__?.variant
  if (!variant) throw new Error('missing benchmark variant')
  const tag = `bench-${variant}-page`
  const element = document.querySelector(tag) as LitElement | null
  if (!element) throw new Error(`missing ${tag}`)

  await customElements.whenDefined(tag)
  await flushElementWithFrame(element)
  const initialMs = performance.now() - (window.__DATSTAR_BRIDGE_BENCH__?.bootStart ?? performance.now())

  const iterations = options.iterations ?? 120
  const warmup = options.warmup ?? 20
  const runtime = await loadDatastarRuntime() as { mergePatch(patch: Record<string, unknown>): void }
  const observer = observeMutations(element)

  for (let i = 0; i < warmup; i++) {
    runtime.mergePatch(updatePatch(i))
    await flushElementWithFrame(element)
  }

  resetMetrics()
  const durations: number[] = []
  for (let i = 0; i < iterations; i++) {
    const start = performance.now()
    runtime.mergePatch(updatePatch(i + warmup))
    await flushElement(element)
    durations.push(performance.now() - start)
  }
  await flushElementWithFrame(element)
  observer.disconnect()

  const metrics = window.__DATSTAR_BRIDGE_BENCH_METRICS__ ?? emptyMetrics()
  const sorted = [...durations].sort((a, b) => a - b)
  const renderedTextLength = element.shadowRoot?.textContent?.replace(/\s+/g, ' ').trim().length ?? 0
  return {
    variant,
    iterations,
    warmup,
    initialMs,
    updateTotalMs: sum(durations),
    updateMeanMs: sum(durations) / durations.length,
    updateP50Ms: percentile(sorted, 0.5),
    updateP95Ms: percentile(sorted, 0.95),
    updateMaxMs: sorted.at(-1) ?? 0,
    usedJSHeapSize: browserHeapSize(),
    renderedTextLength,
    ...metrics,
  }
}

function renderSummary(summary: Record<string, unknown>): TemplateResult {
  return html`
    <section>
      <h1>${summary.title}</h1>
      <dl>
        <dt>filters</dt><dd>${summary.filters}</dd>
        <dt>options</dt><dd>${summary.options}</dd>
        <dt>visuals</dt><dd>${summary.visuals}</dd>
        <dt>visual points</dt><dd>${summary.visualPoints}</dd>
        <dt>tables</dt><dd>${summary.tables}</dd>
        <dt>table rows</dt><dd>${summary.tableRows}</dd>
        <dt>status</dt><dd>${summary.status}</dd>
      </dl>
    </section>
  `
}

function summaryFromPayload(payload: SignalPayload): Record<string, unknown> {
  const visuals = Object.values(payload.visuals ?? {}) as Array<Record<string, any>>
  const tables = Object.values(payload.tables ?? {}) as Array<Record<string, any>>
  const filterOptionPages = Object.values(payload.filterOptionPages ?? {}) as Array<{ items?: unknown[] }>
  return {
    title: payload.page?.title ?? '',
    filters: Object.keys(payload.filterContract?.bindings ?? {}).length,
    options: filterOptionPages.reduce((total, page) => total + (page.items?.length ?? 0), 0),
    visuals: visuals.length,
    visualPoints: visuals.reduce((total, visual) => total + (visual.data?.length ?? 0), 0),
    tables: tables.length,
    tableRows: tables.reduce((total, table) => total + tableRows(table), 0),
    status: payload.status?.lastUpdated ?? '',
  }
}

function tableRows(table: Record<string, any>): number {
  return Object.values(table.blocks ?? {}).reduce((total: number, block: any) => total + (block.rows?.length ?? 0), 0)
}

function updatePatch(iteration: number): Record<string, unknown> {
  const visual = `visual_${iteration % 8}`
  const table = `table_${iteration % 4}`
  return {
    page: { title: `Benchmark Dashboard ${iteration}` },
    status: { lastUpdated: `iteration-${iteration}`, loading: iteration % 9 === 0 },
    filterOptionPages: {
      fb_state: { bindingKey: 'fb_state', items: optionList('state', iteration, 8) },
      fb_category: { bindingKey: 'fb_category', items: optionList('category', iteration, 12) },
    },
    visuals: {
      [visual]: {
        data: Array.from({ length: 24 }, (_, index) => ({
          label: `Bucket ${index}`,
          value: iteration * 10 + index,
          group: index % 3 === 0 ? 'primary' : 'other',
        })),
      },
    },
    tables: {
      [table]: {
        resetVersion: iteration,
        blocks: {
          a: {
            start: 0,
            requestSeq: iteration,
            rows: Array.from({ length: 32 }, (_, index) => ({
              order_id: `order-${iteration}-${index}`,
              status: index % 2 === 0 ? 'delivered' : 'shipped',
              state: index % 3 === 0 ? 'SP' : 'RJ',
              category: `category-${index % 6}`,
            })),
          },
        },
      },
    },
  }
}

function optionList(prefix: string, iteration: number, count: number): Array<{ value: { kind: 'string'; value: string }; label: string; selected: boolean; available: boolean }> {
  return Array.from({ length: count }, (_, index) => ({
    value: { kind: 'string', value: `${prefix}-${iteration}-${index}` },
    label: `${prefix.toUpperCase()} ${iteration}-${index}`,
    selected: false,
    available: true,
  }))
}

async function flushElement(element: LitElement): Promise<void> {
  await element.updateComplete
  await Promise.resolve()
  await element.updateComplete
}

async function flushElementWithFrame(element: LitElement): Promise<void> {
  await flushElement(element)
  await new Promise((resolve) => requestAnimationFrame(() => resolve(undefined)))
  await element.updateComplete
}

function observeMutations(element: LitElement): MutationObserver {
  const observer = new MutationObserver((records) => {
    const metrics = window.__DATSTAR_BRIDGE_BENCH_METRICS__
    if (!metrics) return
    for (const record of records) {
      if (record.target === element && record.type === 'attributes') metrics.hostAttributeMutations++
      if (record.target === element && record.type === 'childList') metrics.hostChildListMutations++
      if (record.target !== element) metrics.shadowMutations++
    }
  })
  observer.observe(element, { attributes: true, childList: true })
  if (element.shadowRoot) {
    observer.observe(element.shadowRoot, { childList: true, subtree: true, characterData: true })
  }
  return observer
}

function resetMetrics(): void {
  const existing = window.__DATSTAR_BRIDGE_BENCH_METRICS__ ?? emptyMetrics()
  window.__DATSTAR_BRIDGE_BENCH_METRICS__ = {
    ...emptyMetrics(),
    longTaskCount: existing.longTaskCount,
    longTaskMs: existing.longTaskMs,
  }
}

function emptyMetrics(): BenchmarkMetrics {
  return {
    jsonParseCalls: 0,
    jsonParseMs: 0,
    jsonStringifyCalls: 0,
    jsonStringifyMs: 0,
    setAttributeCalls: 0,
    setAttributeMs: 0,
    hostAttributeMutations: 0,
    hostChildListMutations: 0,
    shadowMutations: 0,
    litUpdates: 0,
    longTaskCount: 0,
    longTaskMs: 0,
  }
}

function countLitUpdate(): void {
  const metrics = window.__DATSTAR_BRIDGE_BENCH_METRICS__
  if (metrics) metrics.litUpdates++
}

function sum(values: number[]): number {
  return values.reduce((total, value) => total + value, 0)
}

function percentile(sortedValues: number[], fraction: number): number {
  if (sortedValues.length === 0) return 0
  const index = Math.min(sortedValues.length - 1, Math.floor((sortedValues.length - 1) * fraction))
  return sortedValues[index]
}

function browserHeapSize(): number | null {
  const memory = (performance as Performance & { memory?: { usedJSHeapSize?: number } }).memory
  return typeof memory?.usedJSHeapSize === 'number' ? memory.usedJSHeapSize : null
}
