import { expect, test } from 'bun:test'

import type { VisualizationEnvelope } from '../../../generated/visualization'
import { Change, RendererRegistry, VisualizationController, type RendererHandle } from './host-controller'

function envelope(dataRevision: number, specRevision = 'sha256:spec', rendererID = 'test'): VisualizationEnvelope {
  return {
    schemaVersion: 2,
    visualID: 'revenue',
    rendererID,
    specRevision,
    dataRevision,
    spec: {
      kind: 'kpi',
      title: 'Revenue',
      datasets: [{ id: 'primary', fields: [{ id: 'value', role: 'measure', dataType: 'decimal', nullable: false, label: 'Revenue' }] }],
      dataBudget: { maxRows: 1, requiredCompleteness: 'complete' },
      accessibility: { title: 'Revenue', description: 'Current revenue' },
      interactions: [],
      value: { dataset: 'primary', field: 'value' },
      presentation: { trend: 'neutral' },
    },
    dataState: { kind: 'inline', specRevision, dataRevision, generation: 1, datasets: [] },
    selection: [],
    status: { kind: 'ready' },
    diagnostics: [],
  }
}

test('controller mounts lazily, rejects stale revisions, and disposes deterministically', async () => {
  const updates: Change[] = []
  const observations: string[] = []
  let mounts = 0
  let disposals = 0
  const handle: RendererHandle = {
    update: (_value, change) => updates.push(change),
    resize: () => {},
    snapshot: async () => new Blob(),
    dispose: () => { disposals++ },
  }
  const registry = new RendererRegistry()
  registry.register({
    id: 'test', version: '1.0.0', schemaVersions: [2], kinds: ['kpi'], capabilities: { snapshot: true, windowed: false, interactive: false },
    load: async () => ({ mount: () => { mounts++; return handle } }),
  })
  const controller = new VisualizationController(registry, {} as HTMLElement, undefined, (value) => observations.push(value.stage))

  expect(await controller.apply(envelope(2))).toBe(true)
  expect(await controller.apply(envelope(1))).toBe(false)
  expect(await controller.apply(envelope(3))).toBe(true)
  expect(mounts).toBe(1)
  expect(updates).toEqual([Change.Data])
  expect(observations).toContain('renderer_load')
  expect(observations).toContain('mount')
  expect(observations).toContain('stale_result_drop')
  expect(observations).toContain('update')

  controller.dispose()
  controller.dispose()
  expect(disposals).toBe(1)
  expect(observations).toContain('dispose')
})

test('controller coalesces resize and applies the latest size after a lazy mount', async () => {
  const sizes: number[][] = []
  let release!: (adapter: { mount: () => RendererHandle }) => void
  const loading = new Promise<{ mount: () => RendererHandle }>((resolve) => { release = resolve })
  const handle: RendererHandle = {
    update: () => {}, resize: (...size) => sizes.push(size), snapshot: async () => new Blob(), dispose: () => {},
  }
  const registry = new RendererRegistry()
  registry.register({
    id: 'test', version: '1.0.0', schemaVersions: [2], kinds: ['kpi'], capabilities: { snapshot: true, windowed: false, interactive: false },
    load: () => loading,
  })
  const controller = new VisualizationController(registry, {} as HTMLElement)
  const applying = controller.apply(envelope(1))
  controller.resize(100, 80, 1)
  controller.resize(240, 160, 2)
  release({ mount: () => handle })
  await applying
  await Promise.resolve()
  expect(sizes).toEqual([[240, 160, 2]])
})

test('registry rejects duplicate IDs and unsupported capabilities fail closed', async () => {
  const registry = new RendererRegistry()
  const registration = {
    id: 'test', version: '1.0.0', schemaVersions: [2] as const, kinds: ['table'] as const,
    capabilities: { snapshot: false, windowed: true, interactive: true },
    load: async () => ({ mount: () => { throw new Error('not reached') } }),
  }
  registry.register(registration)
  expect(() => registry.register(registration)).toThrow()

  const controller = new VisualizationController(registry, {} as HTMLElement)
  await expect(controller.apply(envelope(1))).rejects.toThrow(/does not support kind/)
})

test('a superseded asynchronous mount cannot dispose the winning renderer', async () => {
  const releases = new Map<number, (handle: RendererHandle) => void>()
  const disposed: number[] = []
  const handles = (revision: number): RendererHandle => ({
    update: () => {}, resize: () => {}, snapshot: async () => new Blob([String(revision)]), dispose: () => { disposed.push(revision) },
  })
  const registry = new RendererRegistry()
  registry.register({
    id: 'test', version: '1.0.0', schemaVersions: [2], kinds: ['kpi'], capabilities: { snapshot: true, windowed: false, interactive: false },
    load: async () => ({ mount: (_container, value) => new Promise<RendererHandle>((resolve) => { releases.set(value.dataRevision, resolve) }) }),
  })
  const controller = new VisualizationController(registry, {} as HTMLElement)
  const first = controller.apply(envelope(1))
  await Promise.resolve()
  const second = controller.apply(envelope(2))
  await Promise.resolve()
  releases.get(2)?.(handles(2))
  await second
  releases.get(1)?.(handles(1))
  await first

  expect(await (await controller.snapshot()).text()).toBe('2')
  expect(disposed).toEqual([1])
})

test('repeated mount and dispose cycles release every renderer exactly once', async () => {
  let mounts = 0
  let disposals = 0
  const registry = new RendererRegistry()
  registry.register({
    id: 'test', version: '1.0.0', schemaVersions: [2], kinds: ['kpi'], capabilities: { snapshot: true, windowed: false, interactive: false },
    load: async () => ({ mount: () => {
      mounts++
      return { update: () => {}, resize: () => {}, snapshot: async () => new Blob(), dispose: () => { disposals++ } }
    } }),
  })
  for (let cycle = 0; cycle < 100; cycle++) {
    const controller = new VisualizationController(registry, {} as HTMLElement)
    await controller.apply(envelope(cycle + 1))
    controller.dispose()
    controller.dispose()
  }
  expect(mounts).toBe(100)
  expect(disposals).toBe(100)
})
