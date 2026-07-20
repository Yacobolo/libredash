import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'
import { X } from 'lucide'
import { lucideIcon } from './lucide-icons'

const focusableSelector = [
  'a[href]:not([tabindex="-1"])',
  'button:not([disabled]):not([tabindex="-1"])',
  'input:not([disabled]):not([tabindex="-1"])',
  'select:not([disabled]):not([tabindex="-1"])',
  'textarea:not([disabled]):not([tabindex="-1"])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

class LibreDashDrawer extends LitElement {
  @property({ type: Boolean, reflect: true }) open = false
  @property({ type: Boolean, reflect: true }) modal = true
  @property() label = 'Drawer'
  @property({ reflect: true }) size: 'default' | 'wide' = 'default'

  static styles = css`
    :host {
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    button {
      font: inherit;
    }

    .overlay {
      position: fixed;
      inset: 0;
      z-index: calc(var(--z-index-inspector) - 1);
      display: flex;
      justify-content: flex-end;
      background: var(--ld-modal-backdrop);
    }

    .overlay-nonmodal {
      background: transparent;
      pointer-events: none;
    }

    .overlay-nonmodal .drawer {
      pointer-events: auto;
    }

    .drawer {
      display: grid;
      width: min(30rem, 100vw);
      max-width: 100vw;
      height: 100svh;
      grid-template-rows: auto minmax(0, 1fr);
      overflow: hidden;
      border-left: var(--ld-border-default);
      background: var(--ld-bg-panel);
      box-shadow: var(--ld-shadow-floating-lg);
      animation: drawer-slide-in var(--ld-transition-fast);
    }

    :host([size='wide']) .drawer {
      width: min(34rem, 100vw);
    }

    .header {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: start;
      gap: var(--base-size-16);
      border-bottom: var(--ld-border-muted);
      padding: var(--base-size-16) var(--base-size-20);
    }

    .title-block {
      min-width: 0;
    }

    .close {
      display: inline-flex;
      width: var(--ld-control-medium);
      height: var(--ld-control-medium);
      flex: 0 0 auto;
      align-items: center;
      justify-content: center;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      padding: 0;
      transition:
        color var(--ld-transition-fast),
        background-color var(--ld-transition-fast),
        border-color var(--ld-transition-fast);
    }

    .close:hover,
    .close:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .body {
      min-height: 0;
      overflow: auto;
      padding: var(--base-size-20);
    }

    .icon {
      display: inline-flex;
      width: var(--ld-icon-sm);
      height: var(--ld-icon-sm);
      align-items: center;
      justify-content: center;
      color: currentColor;
    }

    @media (max-width: 44rem) {
      .drawer {
        width: 100vw;
        border-left: 0;
      }
    }

    @media (prefers-reduced-motion: reduce) {
      .drawer {
        animation-duration: 1ms;
      }
    }

    @keyframes drawer-slide-in {
      from {
        transform: translateX(var(--base-size-16));
        opacity: .96;
      }
      to {
        transform: translateX(0);
        opacity: 1;
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    window.addEventListener('keydown', this.handleWindowKeyDown)
  }

  disconnectedCallback(): void {
    window.removeEventListener('keydown', this.handleWindowKeyDown)
    super.disconnectedCallback()
  }

  render() {
    if (!this.open) return nothing
    return html`
      <div class=${this.modal ? 'overlay' : 'overlay overlay-nonmodal'} @click=${this.handleOverlayClick}>
        <aside
          class="drawer"
          role="dialog"
          aria-modal=${this.modal ? 'true' : nothing}
          aria-label=${this.label}
          @keydown=${this.handleKeyDown}
        >
          <header class="header">
            <div class="title-block">
              <slot name="title"></slot>
              <slot name="subtitle"></slot>
            </div>
            <button class="close" type="button" aria-label=${`Close ${this.label}`} @click=${this.close}>
              <span class="icon" aria-hidden="true">${lucideIcon(X, { size: 16 })}</span>
            </button>
          </header>
          <div class="body">
            <slot></slot>
          </div>
        </aside>
      </div>
    `
  }

  focusFirst(): void {
    window.setTimeout(() => {
      this.focusableElements()[0]?.focus()
    }, 0)
  }

  private readonly close = (): void => {
    this.dispatchEvent(new CustomEvent('ld-drawer-close', { bubbles: true, composed: true }))
  }

  private readonly handleOverlayClick = (event: Event): void => {
    if (this.modal && event.target === event.currentTarget) this.close()
  }

  private readonly handleWindowKeyDown = (event: KeyboardEvent): void => {
    if (!this.open || event.defaultPrevented || event.key !== 'Escape') return
    event.preventDefault()
    this.close()
  }

  private readonly handleKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape') {
      event.preventDefault()
      this.close()
      return
    }
    if (event.key !== 'Tab' || !this.modal) return
    const focusable = this.focusableElements()
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    const root = this.getRootNode() as Document | ShadowRoot
    const active = 'activeElement' in root ? root.activeElement : document.activeElement
    if (event.shiftKey && active === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && active === last) {
      event.preventDefault()
      first.focus()
    }
  }

  private focusableElements(): HTMLElement[] {
    const bodySlot = this.renderRoot.querySelector<HTMLSlotElement>('slot:not([name])')
    const titleSlot = this.renderRoot.querySelector<HTMLSlotElement>('slot[name="title"]')
    const subtitleSlot = this.renderRoot.querySelector<HTMLSlotElement>('slot[name="subtitle"]')
    const slotted = [titleSlot, subtitleSlot, bodySlot].flatMap((slot) => slot?.assignedElements({ flatten: true }) ?? [])
    const nested = slotted.flatMap((element) => Array.from(element.querySelectorAll<HTMLElement>(focusableSelector)))
    const direct = slotted.filter((element): element is HTMLElement => element instanceof HTMLElement && element.matches(focusableSelector))
    const close = this.renderRoot.querySelector<HTMLElement>('.close')
    return [...direct, ...nested, ...(close ? [close] : [])]
  }
}

if (!customElements.get('ld-drawer')) customElements.define('ld-drawer', LibreDashDrawer)

declare global {
  interface HTMLElementTagNameMap {
    'ld-drawer': LibreDashDrawer
  }
}
