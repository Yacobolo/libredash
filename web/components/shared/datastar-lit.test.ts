import { afterEach, expect, test } from 'bun:test'
import { DatastarLit, setDatastarLitRuntimeForTests } from './datastar-lit'

type Dispose = () => void

class FakeElement {
  isUpdatePending = true
  requestUpdates = 0
  rendered: unknown[] = []

  connectedCallback(): void {}
  disconnectedCallback(): void {}

  requestUpdate(): void {
    this.requestUpdates++
    this.isUpdatePending = true
  }

  performUpdate(): void {
    this.isUpdatePending = false
    this.rendered.push((this as FakeElement & { render(): unknown }).render())
  }
}

class TestElement extends DatastarLit(FakeElement as never) {
  render(): unknown {
    return {
      title: this.signal('dashboard.title', 'Untitled'),
      missing: this.signal('dashboard.missing', 'fallback'),
      root: this.signals.dashboard,
    }
  }
}

afterEach(() => {
  setDatastarLitRuntimeForTests(null)
})

test('DatastarLit renders from injected signal state', () => {
  const state = { dashboard: { title: 'Orders' } }
  setDatastarLitRuntimeForTests(fakeRuntime(state))

  const element = new TestElement()
  element.performUpdate()

  expect(element.rendered).toEqual([{
    title: 'Orders',
    missing: 'fallback',
    root: { title: 'Orders' },
  }])
})

test('DatastarLit schedules one Lit update when a tracked signal changes', () => {
  const state = { dashboard: { title: 'Orders' } }
  const effects: Array<() => void> = []
  setDatastarLitRuntimeForTests(fakeRuntime(state, effects))

  const element = new TestElement()
  element.performUpdate()
  expect(effects).toHaveLength(1)
  expect(element.requestUpdates).toBe(0)

  state.dashboard.title = 'Revenue'
  effects[0]()

  expect(element.requestUpdates).toBe(1)
  element.performUpdate()
  expect(element.rendered.at(-1)).toEqual({
    title: 'Revenue',
    missing: 'fallback',
    root: { title: 'Revenue' },
  })
})

test('DatastarLit tracks signal root additions during cold hydration', () => {
  const state: Record<string, unknown> = {}
  const effects: Array<() => void> = []
  let rootReads = 0
  setDatastarLitRuntimeForTests({
    get root() {
      rootReads++
      return state
    },
    getPath(path: string) {
      return path.split('.').reduce<unknown>((value, key) => {
        return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined
      }, state)
    },
    effect(fn: () => void) {
      effects.push(fn)
      fn()
      return () => {
        const index = effects.indexOf(fn)
        if (index >= 0) effects.splice(index, 1)
      }
    },
  })

  const element = new TestElement()
  element.performUpdate()
  expect(rootReads).toBeGreaterThan(0)
  expect(element.rendered.at(-1)).toEqual({
    title: 'Untitled',
    missing: 'fallback',
    root: undefined,
  })

  state.dashboard = { title: 'Revenue' }
  effects[0]()
  expect(element.requestUpdates).toBe(1)
  element.performUpdate()
  expect(element.rendered.at(-1)).toEqual({
    title: 'Revenue',
    missing: 'fallback',
    root: { title: 'Revenue' },
  })
})

test('DatastarLit disconnect disposes the render effect', () => {
  const disposers: Dispose[] = []
  setDatastarLitRuntimeForTests({
    root: { dashboard: { title: 'Orders' } },
    getPath(path) {
      return path.split('.').reduce<unknown>((value, key) => {
        return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined
      }, this.root)
    },
    effect(fn) {
      fn()
      const dispose = () => {}
      disposers.push(dispose)
      return () => {
        const index = disposers.indexOf(dispose)
        if (index >= 0) disposers.splice(index, 1)
      }
    },
  })

  const element = new TestElement()
  element.performUpdate()
  expect(disposers).toHaveLength(1)

  element.disconnectedCallback()
  expect(disposers).toHaveLength(0)
})

function fakeRuntime(state: Record<string, unknown>, effects: Array<() => void> = []) {
  return {
    root: state,
    getPath(path: string) {
      return path.split('.').reduce<unknown>((value, key) => {
        return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined
      }, state)
    },
    effect(fn: () => void) {
      effects.push(fn)
      fn()
      return () => {
        const index = effects.indexOf(fn)
        if (index >= 0) effects.splice(index, 1)
      }
    },
  }
}
