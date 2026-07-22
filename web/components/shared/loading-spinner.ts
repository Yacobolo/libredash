import { LitElement, css } from 'lit'
import { Aperture } from 'lucide'
import { lucideIcon } from './lucide-icons'

class LeapViewLoadingSpinner extends LitElement {
  static styles = css`
    :host {
      display: inline-grid;
      width: var(--lv-spinner-size, var(--lv-spinner-size-md));
      height: var(--lv-spinner-size, var(--lv-spinner-size-md));
      flex: 0 0 auto;
      color: inherit;
      place-items: center;
    }

    svg {
      width: 100%;
      height: 100%;
      transform-origin: center;
      animation: lv-aperture-spin var(--lv-spinner-duration) linear infinite;
    }

    @keyframes lv-aperture-spin {
      to {
        transform: rotate(360deg);
      }
    }

    @media (prefers-reduced-motion: reduce) {
      svg {
        animation: none;
      }
    }
  `

  render() {
    return lucideIcon(Aperture, { size: 24, strokeWidth: 1.8 })
  }
}

if (!customElements.get('lv-loading-spinner')) customElements.define('lv-loading-spinner', LeapViewLoadingSpinner)

declare global {
  interface HTMLElementTagNameMap {
    'lv-loading-spinner': LeapViewLoadingSpinner
  }
}
