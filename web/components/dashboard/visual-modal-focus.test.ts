import { expect, test } from 'bun:test'
import { mountVisualFocus, restoreVisualFocus, visualSourceFromEvent } from './visual-modal-focus'

class FakeDocument {
  createComment(value: string): FakeNode {
    return new FakeNode(`#comment:${value}`)
  }
}

class FakeNode {
  children: FakeNode[] = []
  parentNode: FakeNode | null = null
  ownerDocument = fakeDocument
  private attributes = new Map<string, string>()

  constructor(readonly name: string) {}

  get nextSibling(): FakeNode | null {
    if (!this.parentNode) return null
    const index = this.parentNode.children.indexOf(this)
    return index >= 0 ? this.parentNode.children[index + 1] ?? null : null
  }

  appendChild(node: FakeNode): FakeNode {
    this.detach(node)
    this.children.push(node)
    node.parentNode = this
    return node
  }

  insertBefore(node: FakeNode, before: FakeNode | null): FakeNode {
    this.detach(node)
    const index = before ? this.children.indexOf(before) : -1
    if (index >= 0) {
      this.children.splice(index, 0, node)
    } else {
      this.children.push(node)
    }
    node.parentNode = this
    return node
  }

  remove(): void {
    if (!this.parentNode) return
    this.parentNode.children = this.parentNode.children.filter((child) => child !== this)
    this.parentNode = null
  }

  getAttribute(name: string): string | null {
    return this.attributes.get(name) ?? null
  }

  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value)
  }

  removeAttribute(name: string): void {
    this.attributes.delete(name)
  }

  private detach(node: FakeNode): void {
    if (!node.parentNode) return
    node.parentNode.children = node.parentNode.children.filter((child) => child !== node)
    node.parentNode = null
  }
}

const fakeDocument = new FakeDocument()

function childNames(node: FakeNode): string[] {
  return node.children.map((child) => child.name)
}

test('mountVisualFocus moves the source element into the target and leaves a placeholder', () => {
  const source = new FakeNode('source')
  const after = new FakeNode('after')
  const parent = new FakeNode('parent')
  const target = new FakeNode('target')
  parent.appendChild(source)
  parent.appendChild(after)

  const mount = mountVisualFocus(source as unknown as Element, target as unknown as Node)

  expect(mount).toBeTruthy()
  expect(target.children[0]).toBe(source)
  expect(source.parentNode).toBe(target)
  expect(childNames(parent)).toEqual(['#comment:lv-visual-focus-placeholder', 'after'])
})

test('mountVisualFocus can slot the source as a light DOM child of the modal host', () => {
  const source = new FakeNode('source')
  const parent = new FakeNode('parent')
  const modalHost = new FakeNode('lv-visual-modal')
  parent.appendChild(source)

  const mount = mountVisualFocus(source as unknown as Element, modalHost as unknown as Node, { slot: 'focus-visual' })

  expect(mount).toBeTruthy()
  expect(source.parentNode).toBe(modalHost)
  expect(source.getAttribute('slot')).toBe('focus-visual')
  expect(childNames(parent)).toEqual(['#comment:lv-visual-focus-placeholder'])

  restoreVisualFocus(mount)

  expect(source.parentNode).toBe(parent)
  expect(source.getAttribute('slot')).toBe(null)
  expect(childNames(parent)).toEqual(['source'])
})

test('restoreVisualFocus restores a previous slot value after fullscreen focus', () => {
  const source = new FakeNode('source')
  const parent = new FakeNode('parent')
  const modalHost = new FakeNode('lv-visual-modal')
  source.setAttribute('slot', 'dashboard-cell')
  parent.appendChild(source)

  const mount = mountVisualFocus(source as unknown as Element, modalHost as unknown as Node, { slot: 'focus-visual' })
  expect(mount).toBeTruthy()
  expect(source.getAttribute('slot')).toBe('focus-visual')

  restoreVisualFocus(mount)

  expect(source.parentNode).toBe(parent)
  expect(source.getAttribute('slot')).toBe('dashboard-cell')
})

test('restoreVisualFocus restores the same source element before the placeholder', () => {
  const before = new FakeNode('before')
  const source = new FakeNode('source')
  const after = new FakeNode('after')
  const parent = new FakeNode('parent')
  const target = new FakeNode('target')
  parent.appendChild(before)
  parent.appendChild(source)
  parent.appendChild(after)
  const mount = mountVisualFocus(source as unknown as Element, target as unknown as Node)
  expect(mount).toBeTruthy()

  restoreVisualFocus(mount)

  expect(source.parentNode).toBe(parent)
  expect(childNames(parent)).toEqual(['before', 'source', 'after'])
  expect(childNames(target)).toEqual([])
})

test('restoreVisualFocus falls back to the original next sibling when placeholder is gone', () => {
  const source = new FakeNode('source')
  const after = new FakeNode('after')
  const parent = new FakeNode('parent')
  const target = new FakeNode('target')
  parent.appendChild(source)
  parent.appendChild(after)
  const mount = mountVisualFocus(source as unknown as Element, target as unknown as Node)
  expect(mount).toBeTruthy()
  mount.placeholder.remove()

  restoreVisualFocus(mount)

  expect(childNames(parent)).toEqual(['source', 'after'])
  expect(childNames(target)).toEqual([])
})

test('focus can move from one source element to another after restoring the first', () => {
  const first = new FakeNode('first')
  const firstParent = new FakeNode('first-parent')
  const second = new FakeNode('second')
  const secondParent = new FakeNode('second-parent')
  const target = new FakeNode('target')
  firstParent.appendChild(first)
  secondParent.appendChild(second)
  const firstMount = mountVisualFocus(first as unknown as Element, target as unknown as Node)
  expect(firstMount).toBeTruthy()

  restoreVisualFocus(firstMount)
  const secondMount = mountVisualFocus(second as unknown as Element, target as unknown as Node)

  expect(secondMount).toBeTruthy()
  expect(childNames(firstParent)).toEqual(['first'])
  expect(childNames(secondParent)).toEqual(['#comment:lv-visual-focus-placeholder'])
  expect(childNames(target)).toEqual(['second'])
})

test('visualSourceFromEvent returns the first focusable visual in the composed path', () => {
  const originalHTMLElement = globalThis.HTMLElement
  class TestHTMLElement {
    constructor(readonly localName: string) {}
  }
  globalThis.HTMLElement = TestHTMLElement as unknown as typeof HTMLElement
  try {
    const button = new TestHTMLElement('button')
    const chart = new TestHTMLElement('lv-echart')
    const table = new TestHTMLElement('lv-report-table')
    const event = { composedPath: () => [button, chart, table] } as unknown as Event

    expect(visualSourceFromEvent(event)).toBe(chart)
  } finally {
    globalThis.HTMLElement = originalHTMLElement
  }
})
