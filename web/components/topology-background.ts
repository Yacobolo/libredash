import { LitElement, css, html } from 'lit'
import p5 from 'p5'
import topology from '../vendor/vanta/vanta.topology.js'

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
  private readonly themeListener = () => this.refreshEffect()

  static styles = css`
    :host {
      --topology-accent: var(--ld-accent, var(--bgColor-accent-emphasis, #0969da));
      --topology-bg: #070b12;
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

  private refreshEffect() {
    this.destroyEffect()
    window.requestAnimationFrame(() => this.startEffect())
  }

  private startEffect() {
    const mount = this.renderRoot.querySelector<HTMLElement>('.mount')
    if (!mount || this.effect) return

    skipUnusedThreeInit()
    this.effect = topology({
      el: mount,
      p5,
      color: cssColor('--bgColor-accent-emphasis', '#0969da'),
      backgroundColor: '#070b12',
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

function cssColor(variableName: string, fallback: string) {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(variableName).trim()
  return normalizeColor(raw || fallback, fallback)
}

function normalizeColor(value: string, fallback: string) {
  if (/^#[\da-f]{6}$/i.test(value)) return value
  const rgb = value.match(/^rgba?\((\d+),\s*(\d+),\s*(\d+)/i)
  if (!rgb) return fallback
  return `#${toHex(rgb[1])}${toHex(rgb[2])}${toHex(rgb[3])}`
}

function toHex(value: string) {
  return Number(value).toString(16).padStart(2, '0')
}

customElements.define('ld-topology-background', LibreDashTopologyBackground)
