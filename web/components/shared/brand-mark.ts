import { LitElement, css } from 'lit'
import { Aperture } from 'lucide'
import { lucideIcon } from './lucide-icons'

export const leapViewBrandName = 'LeapView'

class LeapViewBrandMark extends LitElement {
  static styles = css`
    :host {
      display: inline-grid;
      width: var(--lv-brand-mark-size, var(--base-size-28));
      height: var(--lv-brand-mark-size, var(--base-size-28));
      flex: 0 0 auto;
      place-items: center;
      color: inherit;
    }

    :host([large]) {
      --lv-brand-mark-size: var(--base-size-40);
    }

    svg {
      width: 100%;
      height: 100%;
    }
  `

  render() {
    return lucideIcon(Aperture, { size: 28, strokeWidth: 1.8 })
  }
}

if (!customElements.get('lv-brand-mark')) customElements.define('lv-brand-mark', LeapViewBrandMark)

declare global {
  interface HTMLElementTagNameMap {
    'lv-brand-mark': LeapViewBrandMark
  }
}
