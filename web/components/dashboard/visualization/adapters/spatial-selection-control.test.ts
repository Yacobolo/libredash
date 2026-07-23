import { expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'

import type { VisualizationEnvelope } from '../../../../generated/visualization'
import { MapSpatialSelectionControl } from './maplibre/spatial-selection-control'

test('MapLibre spatial controls preserve the armed button and keyboard focus across envelope updates', () => {
  const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true })
  const previousDocument = globalThis.document
  const previousWindow = globalThis.window
  Object.defineProperty(globalThis, 'document', { configurable: true, value: dom.window.document })
  Object.defineProperty(globalThis, 'window', { configurable: true, value: dom.window })
  try {
    const canvas = document.createElement('canvas')
    const frame = document.createElement('div')
    frame.append(canvas)
    document.body.append(frame)
    const map = {
      getCanvas: () => canvas,
      on: () => undefined,
      off: () => undefined,
      dragPan: { isEnabled: () => true, enable: () => undefined, disable: () => undefined },
      project: () => ({ x: 0, y: 0 }),
      unproject: () => ({ lng: 0, lat: 0 }),
    }
    const envelope = {
      spec: { kind: 'geographic', spatialInteractions: [{ id: 'area', gestures: ['box', 'lasso', 'radius'] }] },
      spatialSelection: undefined,
    } as unknown as VisualizationEnvelope
    const control = new MapSpatialSelectionControl(map as never, frame, () => undefined)
    frame.append(control.element)
    control.update(envelope)
    const button = control.element.querySelector<HTMLButtonElement>('[aria-label="Select map data with box"]')!

    button.focus()
    expect(document.activeElement).toBe(button)
    button.click()
    expect(control.element.querySelector('[aria-label="Select map data with box"]')).toBe(button)
    expect(button.getAttribute('aria-pressed')).toBe('true')
    expect(document.activeElement).toBe(button)

    control.update({ ...envelope, status: { kind: 'loading' } } as VisualizationEnvelope)
    expect(control.element.querySelector('[aria-label="Select map data with box"]')).toBe(button)
    expect(button.getAttribute('aria-pressed')).toBe('true')
    expect(document.activeElement).toBe(button)
    control.dispose()
  } finally {
    Object.defineProperty(globalThis, 'document', { configurable: true, value: previousDocument })
    Object.defineProperty(globalThis, 'window', { configurable: true, value: previousWindow })
    dom.window.close()
  }
})
