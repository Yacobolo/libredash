export type ColumnResizeDrag = {
  columnKey: string
  startClientX: number
  startSize: number
  minSize: number
}

export function resizeClientX(event: MouseEvent | TouchEvent): number | null {
  if ('touches' in event) {
    return event.touches[0]?.clientX ?? event.changedTouches[0]?.clientX ?? null
  }
  return event.clientX
}

export function resizePlaneScaleX(plane: HTMLElement): number {
  const rect = plane.getBoundingClientRect()
  return rect.width > 0 && plane.offsetWidth > 0 ? rect.width / plane.offsetWidth : 1
}

export function resizeGuideX(plane: HTMLElement, clientX: number): number {
  const rect = plane.getBoundingClientRect()
  const scaleX = resizePlaneScaleX(plane)
  const localX = scaleX > 0 ? (clientX - rect.left) / scaleX : clientX - rect.left
  return Math.max(0, Math.min(plane.scrollWidth, localX))
}

export function resizedColumnWidth(drag: ColumnResizeDrag, clientX: number, scaleX = 1): number {
  const delta = scaleX > 0 ? (clientX - drag.startClientX) / scaleX : clientX - drag.startClientX
  return Math.max(drag.minSize, Math.round(drag.startSize + delta))
}
