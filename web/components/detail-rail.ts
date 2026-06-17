const storageKey = 'libredash.metricDetailRail'

class DetailRail extends HTMLElement {
  private button?: HTMLButtonElement
  private collapsed = false

  connectedCallback(): void {
    this.collapsed = this.savedState()
    this.ensureToggle()
    this.sync()
  }

  private ensureToggle(): void {
    if (this.button) return
    const header = this.querySelector<HTMLElement>('[data-metric-info-header]')
    if (!header) return
    this.button = document.createElement('button')
    this.button.type = 'button'
    this.button.className = [
      'inline-flex',
      'size-7',
      'items-center',
      'justify-center',
      'rounded-small',
      'border',
      'border-transparent',
      'bg-transparent',
      'p-0',
      'text-fg-muted',
      'transition-colors',
      'duration-fast',
      'hover:border-outline-muted',
      'hover:bg-control-hover',
      'hover:text-fg-default',
      'focus-visible:border-outline-accent',
      'focus-visible:bg-control-hover',
      'focus-visible:text-fg-default',
      'focus-visible:outline-0',
    ].join(' ')
    this.button.addEventListener('click', () => this.toggle())
    header.append(this.button)
  }

  private toggle(): void {
    this.collapsed = !this.collapsed
    try {
      window.localStorage.setItem(storageKey, this.collapsed ? 'collapsed' : 'expanded')
    } catch {
      // The rail still works if storage is unavailable.
    }
    this.sync()
  }

  private sync(): void {
    this.toggleAttribute('data-rail-collapsed', this.collapsed)
    this.classList.toggle('grid-cols-metric-workspace-collapsed', this.collapsed)
    this.classList.toggle('grid-cols-metric-workspace', !this.collapsed)

    const body = this.querySelector<HTMLElement>('[data-metric-info-body]')
    body?.classList.toggle('hidden', this.collapsed)

    const sidebar = this.querySelector<HTMLElement>('[data-metric-info-sidebar]')
    sidebar?.classList.toggle('border-l', !this.collapsed)
    sidebar?.classList.toggle('items-start', this.collapsed)
    sidebar?.classList.toggle('justify-center', this.collapsed)

    const title = this.querySelector<HTMLElement>('[data-metric-info-header] h2')
    title?.classList.toggle('hidden', this.collapsed)

    if (this.collapsed) {
      document.documentElement.setAttribute('data-metric-detail-rail', 'collapsed')
    } else {
      document.documentElement.removeAttribute('data-metric-detail-rail')
    }
    if (!this.button) return
    this.button.setAttribute('aria-expanded', String(!this.collapsed))
    this.button.setAttribute('aria-label', this.collapsed ? 'Expand details' : 'Collapse details')
    this.button.title = this.collapsed ? 'Expand details' : 'Collapse details'
    this.button.innerHTML = this.collapsed ? detailsIcon() : collapseIcon()
  }

  private savedState(): boolean {
    try {
      return window.localStorage.getItem(storageKey) === 'collapsed'
    } catch {
      return false
    }
  }
}

function detailsIcon(): string {
  return '<svg class="size-4" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"></path><path d="M14 2v6h6"></path><path d="M8 13h8"></path><path d="M8 17h6"></path></svg>'
}

function collapseIcon(): string {
  return '<svg class="size-4" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"></path></svg>'
}

customElements.define('ld-detail-rail', DetailRail)
