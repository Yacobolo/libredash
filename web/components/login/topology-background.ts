import { LitElement, css, html } from 'lit'
import p5 from 'p5'
import topology from '../../vendor/vanta/vanta.topology.js'

declare global {
  interface Window {
    VANTA?: {
      VantaBase?: {
        prototype?: {
          initThree?: () => void
          libreDashP5Only?: boolean
        }
      }
    }
  }
}

type VantaEffect = {
  destroy(): void
}

class LibreDashTopologyBackground extends LitElement {
  private effect?: VantaEffect
  private refreshFrame?: number
  private readonly themeListener = () => this.scheduleEffectRefresh()

  static styles = css`
    :host {
      --topology-accent: var(--ld-accent, var(--bgColor-accent-emphasis));
      --topology-bg: var(--ld-topology-bg, var(--bgColor-inverse));
      position: absolute;
      inset: 0;
      display: block;
      overflow: hidden;
      background: var(--topology-bg);
    }

    .mount {
      position: absolute;
      inset: 0;
      overflow: hidden;
      background: var(--topology-bg);
    }

    .mount canvas {
      visibility: visible !important;
    }
  `

  connectedCallback() {
    super.connectedCallback()
    document.addEventListener('libredash-theme-applied', this.themeListener)
  }

  disconnectedCallback() {
    document.removeEventListener('libredash-theme-applied', this.themeListener)
    if (this.refreshFrame) window.cancelAnimationFrame(this.refreshFrame)
    this.destroyEffect()
    super.disconnectedCallback()
  }

  firstUpdated() {
    this.startEffect()
  }

  updated() {
    this.startEffect()
  }

  render() {
    return html`<div class="mount" aria-hidden="true"></div>`
  }

  private scheduleEffectRefresh() {
    if (this.refreshFrame) return
    this.refreshFrame = window.requestAnimationFrame(() => {
      this.refreshFrame = undefined
      this.destroyEffect()
      window.requestAnimationFrame(() => this.startEffect())
    })
  }

  private startEffect() {
    const mount = this.renderRoot.querySelector<HTMLElement>('.mount')
    if (!mount || this.effect) return

    skipUnusedThreeInit()
    const color = cssColor('--bgColor-accent-emphasis', '--ld-accent')
    const backgroundColor = cssColor('--ld-topology-bg', '--bgColor-inverse')
    if (!color || !backgroundColor) return

    this.effect = topology({
      el: mount,
      p5,
      color,
      backgroundColor,
      mouseControls: true,
      touchControls: true,
      gyroControls: false,
      minHeight: 200,
      minWidth: 200,
      scale: 1,
      scaleMobile: 1,
    }) as VantaEffect
  }

  private destroyEffect() {
    this.effect?.destroy()
    this.effect = undefined
  }
}

function skipUnusedThreeInit() {
  const prototype = window.VANTA?.VantaBase?.prototype
  if (!prototype || prototype.libreDashP5Only) return
  prototype.initThree = () => {}
  prototype.libreDashP5Only = true
}

function cssColor(variableName: string, fallbackVariableName: string) {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(variableName).trim()
  const fallback = getComputedStyle(document.documentElement).getPropertyValue(fallbackVariableName).trim()
  return normalizeColor(raw) ?? normalizeColor(fallback)
}

function normalizeColor(value: string) {
  const color = value.trim()
  if (!color) return null

  const hex = color.match(/^#([\da-f]{3}|[\da-f]{6})$/i)
  if (hex) {
    const value = hex[1]
    return value.length === 3
      ? `#${value[0]}${value[0]}${value[1]}${value[1]}${value[2]}${value[2]}`.toLowerCase()
      : `#${value}`.toLowerCase()
  }

  const rgb = color.match(/^rgba?\(\s*([\d.]+)(?:,|\s+)\s*([\d.]+)(?:,|\s+)\s*([\d.]+)/i)
  if (rgb) return `#${toHex(rgb[1])}${toHex(rgb[2])}${toHex(rgb[3])}`

  const srgb = color.match(/^color\(\s*srgb\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)/i)
  if (srgb) {
    return `#${toHex(Number(srgb[1]) * 255)}${toHex(Number(srgb[2]) * 255)}${toHex(Number(srgb[3]) * 255)}`
  }

  return null
}

function toHex(value: string | number) {
  return Math.round(Number(value)).toString(16).padStart(2, '0')
}

customElements.define('ld-topology-background', LibreDashTopologyBackground)
