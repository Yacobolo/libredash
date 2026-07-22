export function visualSourceFromEvent(event: Event): HTMLElement | null {
  for (const target of event.composedPath()) {
    if (target instanceof HTMLElement && isFocusableVisual(target)) return target
  }
  return null
}

function isFocusableVisual(element: HTMLElement): boolean {
  return element.localName === 'lv-visualization-host'
}
