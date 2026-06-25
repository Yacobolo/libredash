export type VisualFocusMount<T extends Element = Element> = {
  element: T
  placeholder: Comment
  sourceParent: Node
  nextSibling: ChildNode | null
  previousSlot?: string | null
}

export function mountVisualFocus<T extends Element>(element: T, target: Node, options: { slot?: string } = {}): VisualFocusMount<T> | null {
  const sourceParent = element.parentNode
  if (!sourceParent) return null

  const previousSlot = options.slot ? element.getAttribute('slot') : undefined
  const placeholder = element.ownerDocument.createComment('ld-visual-focus-placeholder')
  const nextSibling = element.nextSibling
  if (options.slot) element.setAttribute('slot', options.slot)
  sourceParent.insertBefore(placeholder, element)
  target.appendChild(element)

  return { element, placeholder, sourceParent, nextSibling, previousSlot }
}

export function restoreVisualFocus(mount: VisualFocusMount): void {
  const restoreParent = mount.placeholder.parentNode ?? mount.sourceParent
  const restoreBefore = mount.placeholder.parentNode
    ? mount.placeholder
    : mount.nextSibling?.parentNode === restoreParent
      ? mount.nextSibling
      : null

  restoreParent.insertBefore(mount.element, restoreBefore)
  mount.placeholder.remove()
  if (mount.previousSlot !== undefined) {
    if (mount.previousSlot === null) {
      mount.element.removeAttribute('slot')
    } else {
      mount.element.setAttribute('slot', mount.previousSlot)
    }
  }
}

export function visualSourceFromEvent(event: Event): HTMLElement | null {
  for (const target of event.composedPath()) {
    if (target instanceof HTMLElement && isFocusableVisual(target)) return target
  }
  return null
}

function isFocusableVisual(element: HTMLElement): boolean {
  return element.localName === 'ld-echart' || element.localName === 'ld-data-table'
}
