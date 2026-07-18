export type FlowPoint = { x: number; y: number }

export type FlowFieldSettings = {
  width: number
  height: number
  lineCount: number
  pointCount: number
  spacing: number
  amplitude: number
  frequency: number
  phase: number
  twist: number
  bulge: number
  morph: number
  spinSpeed: number
  speed: number
  rotate: number
  edgeFade: number
}

export const flowFieldSettings: Readonly<FlowFieldSettings> = Object.freeze({
  width: 1720,
  height: 1080,
  lineCount: 24,
  pointCount: 81,
  spacing: 4.9,
  amplitude: 163,
  frequency: 0.75,
  phase: 0.14,
  twist: 0.41,
  bulge: 0.66,
  morph: 0.5,
  spinSpeed: 0.7,
  speed: 0.66,
  rotate: -28,
  edgeFade: 0.25,
})

export function generateFlowLinePoints(
  line: number,
  time: number,
  settings: Readonly<FlowFieldSettings> = flowFieldSettings,
): FlowPoint[] {
  const centerLine = (settings.lineCount - 1) / 2
  const linePhase = settings.phase * (line - centerLine)
  const angularFrequency = settings.frequency * Math.PI * 2
  const animationTime = time * settings.speed
  const rotation = (settings.rotate * Math.PI) / 180
  const rotationCosine = Math.cos(rotation)
  const rotationSine = Math.sin(rotation)
  const lineOffset =
    ((settings.lineCount <= 1 ? 0.5 : line / (settings.lineCount - 1)) - 0.5) *
    settings.lineCount *
    settings.spacing

  return Array.from({ length: settings.pointCount }, (_, index) => {
    const progress = index / Math.max(1, settings.pointCount - 1)
    const envelope = 1 + settings.bulge * Math.sin(progress * Math.PI)
    const primary = Math.sin(angularFrequency * progress + linePhase)
    const spin = Math.sin(angularFrequency * 0.5 * progress - linePhase * 2 + settings.spinSpeed * animationTime)
    const morph =
      Math.sin(3.7 * progress + 0.2 * line) *
      Math.cos(2.1 * progress + 0.13 * line)
    const wave = (primary + settings.twist * spin + settings.morph * morph) * 0.5

    const x = progress * settings.width
    const y = settings.height / 2 + lineOffset + wave * envelope * settings.amplitude
    if (rotation === 0) return { x, y }

    const centeredX = x - settings.width / 2
    const centeredY = y - settings.height / 2
    return {
      x: settings.width / 2 + centeredX * rotationCosine - centeredY * rotationSine,
      y: settings.height / 2 + centeredX * rotationSine + centeredY * rotationCosine,
    }
  })
}

export function flowCoverTransform(
  viewportWidth: number,
  viewportHeight: number,
  settings: Pick<FlowFieldSettings, 'width' | 'height'> = flowFieldSettings,
): { scale: number; offsetX: number; offsetY: number } {
  const scale = Math.max(viewportWidth / settings.width, viewportHeight / settings.height)
  return {
    scale,
    offsetX: (viewportWidth - settings.width * scale) / 2,
    offsetY: (viewportHeight - settings.height * scale) / 2,
  }
}
