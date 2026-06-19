import { LitElement, css, html, svg as svgTemplate, type PropertyValues } from 'lit'
import { property, state } from 'lit/decorators.js'

type ReportPage = {
  id: string
  title: string
  href: string
  active?: boolean
}

type ReportSidebarConfig = {
  dashboardId?: string
  dashboardTitle?: string
  pageId?: string
  pageTitle?: string
  pages?: ReportPage[]
}

type HoverTitle = {
  index: string
  title: string
  top: number
  active: boolean
}

const defaultConfig: ReportSidebarConfig = {
  pageTitle: 'Page',
  pages: [],
}

const configConverter = {
  fromAttribute(value: string | null): ReportSidebarConfig {
    if (!value) return defaultConfig
    try {
      return { ...defaultConfig, ...JSON.parse(value) } as ReportSidebarConfig
    } catch {
      return defaultConfig
    }
  },
  toAttribute(value: ReportSidebarConfig): string {
    return JSON.stringify(value ?? defaultConfig)
  },
}

class ReportSidebar extends LitElement {
  @property({ attribute: 'config', converter: configConverter }) config: ReportSidebarConfig = defaultConfig
  @state() private collapsed = storedCollapsed()
  @state() private hoverTitle?: HoverTitle

  static styles = css`
    :host {
      --ld-report-sidebar-width: 144px;
      display: block;
      width: var(--ld-report-sidebar-width);
      height: 100svh;
      min-height: 0;
      overflow: hidden;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
      transition: width 180ms var(--ld-ease-out);
    }

    :host([data-collapsed]) {
      --ld-report-sidebar-width: 38px;
      z-index: 30;
      overflow: visible;
    }

    :host([data-collapsed]) aside {
      overflow: visible;
    }

    aside {
      position: sticky;
      top: 0;
      display: grid;
      width: var(--ld-report-sidebar-width);
      height: 100svh;
      min-height: 0;
      max-height: 100svh;
      grid-template-rows: auto minmax(0, 1fr);
      overflow: hidden;
      border-right: 1px solid color-mix(in srgb, var(--ld-line-muted), transparent 36%);
      background: var(--ld-report-rail-bg);
      transition: width 180ms var(--ld-ease-out);
    }

    header {
      display: grid;
      min-width: 0;
      padding: 10px 8px;
    }

    .top-row {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: 6px;
      justify-content: space-between;
    }

    .collapse svg {
      width: 14px;
      height: 14px;
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .section-title {
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
      text-transform: uppercase;
    }

    .section-title {
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-caption);
    }

    .collapse {
      display: grid;
      width: 24px;
      height: 24px;
      flex: 0 0 auto;
      place-items: center;
      margin-left: auto;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      padding: 0;
    }

    .collapse:hover,
    .collapse:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    nav {
      display: grid;
      align-content: start;
      gap: 2px;
      min-width: 0;
      min-height: 0;
      overflow-x: hidden;
      overflow-y: auto;
      padding: 7px 5px;
      scrollbar-gutter: stable;
      scrollbar-color: var(--ld-scrollbar-thumb) transparent;
      scrollbar-width: thin;
    }

    nav::-webkit-scrollbar {
      width: 6px;
    }

    nav::-webkit-scrollbar-track {
      background: transparent;
    }

    nav::-webkit-scrollbar-thumb {
      border-radius: var(--ld-radius-full);
      background: var(--ld-scrollbar-thumb);
    }

    nav::-webkit-scrollbar-thumb:hover {
      background: var(--ld-scrollbar-thumb-hover);
    }

    a {
      text-decoration: none;
    }

    .page-link {
      position: relative;
      display: grid;
      grid-template-columns: 24px minmax(0, 1fr);
      min-height: 29px;
      align-items: center;
      gap: 6px;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      color: color-mix(in srgb, var(--ld-fg-muted), transparent 8%);
      padding: 0 9px;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .page-link:hover,
    .page-link:focus-visible {
      background: var(--ld-bg-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .page-link[aria-current='page'] {
      border-color: transparent;
      background: var(--ld-bg-hover);
      color: var(--ld-fg-default);
    }

    .page-link[aria-current='page']::before {
      content: '';
      position: absolute;
      inset-block: 7px;
      left: 0;
      width: 2px;
      border-radius: var(--ld-radius-full);
      background: var(--ld-accent);
    }

    .page-index {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      color: color-mix(in srgb, var(--ld-fg-muted), transparent 24%);
      font-size: var(--ld-font-size-caption);
      font-variant-numeric: tabular-nums;
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
    }

    .page-link:hover .page-index,
    .page-link:focus-visible .page-index,
    .page-link[aria-current='page'] .page-index {
      color: var(--ld-fg-default);
    }

    .link-text {
      overflow: hidden;
      min-width: 0;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .page-link:hover .link-text,
    .page-link:focus-visible .link-text,
    .page-link[aria-current='page'] .link-text {
      font-weight: var(--ld-font-weight-strong);
    }

    :host([data-collapsed]) header {
      padding: 8px 5px 6px;
    }

    :host([data-collapsed]) .section-title,
    :host([data-collapsed]) .link-text {
      display: none;
    }

    :host([data-collapsed]) .top-row {
      display: grid;
      justify-items: center;
    }

    :host([data-collapsed]) .collapse {
      margin-left: 0;
    }

    :host([data-collapsed]) nav {
      scrollbar-gutter: auto;
      scrollbar-width: none;
    }

    :host([data-collapsed]) nav::-webkit-scrollbar {
      display: none;
      width: 0;
    }

    :host([data-collapsed]) .page-link {
      grid-template-columns: 24px;
      justify-content: center;
      padding-inline: 0;
    }

    :host([data-collapsed]) .page-index {
      background: transparent;
    }

    :host([data-collapsed]) .page-link:hover .page-index,
    :host([data-collapsed]) .page-link:focus-visible .page-index {
      color: var(--ld-fg-default);
    }

    :host([data-collapsed]) .page-link[aria-current='page'] .page-index {
      color: var(--ld-fg-default);
    }

    :host([data-collapsed]) .page-link {
      min-height: 29px;
    }

    :host([data-collapsed]) .page-link[aria-current='page']::before {
      content: '';
      inset-block: 7px;
      left: 0;
      width: 2px;
    }

    .hover-title {
      display: none;
    }

    :host([data-collapsed]) .hover-title {
      position: absolute;
      z-index: 40;
      left: 7px;
      min-height: 29px;
      max-width: 12rem;
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 0 9px 0 0;
      background: var(--ld-report-rail-bg);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
      pointer-events: none;
      transform: translateY(-50%);
      animation: rail-title-fade-in 90ms var(--ld-ease-out);
      white-space: nowrap;
    }

    :host([data-collapsed]) .hover-title[data-active]::before {
      content: '';
      position: absolute;
      inset-block: 7px;
      left: -2px;
      width: 2px;
      border-radius: var(--ld-radius-full);
      background: var(--ld-accent);
    }

    @keyframes rail-title-fade-in {
      from {
        opacity: 0;
      }

      to {
        opacity: 1;
      }
    }

    .hover-title-index {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      color: var(--ld-fg-default);
      font-variant-numeric: tabular-nums;
      font-weight: var(--ld-font-weight-strong);
    }

    .hover-title-name {
      overflow: hidden;
      text-overflow: ellipsis;
      animation: rail-title-name-fold-out 120ms var(--ld-ease-out);
      transform-origin: left center;
    }

    @keyframes rail-title-name-fold-out {
      from {
        opacity: 0;
        transform: translateX(-4px) scaleX(0.86);
      }

      to {
        opacity: 1;
        transform: translateX(0) scaleX(1);
      }
    }

    .rail-label {
      display: none;
    }

    :host([data-collapsed]) .rail-label {
      display: block;
      margin: 8px auto 10px;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
      line-height: var(--ld-line-height-none);
      text-orientation: mixed;
      text-transform: uppercase;
      transform: rotate(180deg);
      writing-mode: vertical-rl;
    }
  `

  updated(changed: PropertyValues<this>): void {
    this.toggleAttribute('data-collapsed', this.collapsed)
    if (changed.has('config') || changed.has('collapsed')) {
      this.scrollActivePageIntoView()
    }
  }

  render() {
    const pages = this.config.pages ?? []
    return html`
      <aside aria-label="Report pages">
        <header>
          <div class="top-row">
            <strong class="section-title">Pages</strong>
            <button
              class="collapse"
              type="button"
              aria-label=${this.collapsed ? 'Expand report pages' : 'Collapse report pages'}
              aria-pressed=${String(this.collapsed)}
              title=${this.collapsed ? 'Expand report pages' : 'Collapse report pages'}
              @click=${this.toggleCollapsed}
            >
              ${icon(this.collapsed ? 'chevron-right' : 'chevron-left')}
            </button>
          </div>
        </header>

        <nav aria-label="Report pages" @scroll=${this.hideHoverTitle}>
          <span class="rail-label" aria-hidden="true">Pages</span>
          ${pages.map((page, index) => this.renderPageLink(page, index, pages.length))}
        </nav>
        ${this.collapsed && this.hoverTitle ? html`
          <div
            class="hover-title"
            style=${`top:${this.hoverTitle.top}px`}
            ?data-active=${this.hoverTitle.active}
          >
            <span class="hover-title-index" aria-hidden="true">${this.hoverTitle.index}</span>
            <span class="hover-title-name">${this.hoverTitle.title}</span>
          </div>
        ` : null}
      </aside>
    `
  }

  private renderPageLink(page: ReportPage, index: number, pageCount: number) {
    const active = Boolean(page.active || page.id === this.config.pageId)
    const pageNumber = formatPageNumber(index, pageCount)
    const title = page.title || page.id
    return html`
      <a
        class="page-link"
        href=${page.href}
        aria-current=${active ? 'page' : 'false'}
        aria-label=${title}
        @mouseenter=${(event: MouseEvent) => this.showHoverTitle(event, title, pageNumber, active)}
        @mouseleave=${this.hideHoverTitle}
        @focus=${(event: FocusEvent) => this.showHoverTitle(event, title, pageNumber, active)}
        @blur=${this.hideHoverTitle}
      >
        <span class="page-index" aria-hidden="true">${pageNumber}</span>
        <span class="link-text">${title}</span>
      </a>
    `
  }

  private toggleCollapsed = () => {
    this.collapsed = !this.collapsed
    try {
      localStorage.setItem('libredash-report-sidebar-collapsed', String(this.collapsed))
    } catch {
      // Session state still updates when storage is unavailable.
    }
  }

  private scrollActivePageIntoView(): void {
    requestAnimationFrame(() => {
      const active = this.renderRoot.querySelector<HTMLElement>('.page-link[aria-current="page"]')
      active?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    })
  }

  private showHoverTitle(event: MouseEvent | FocusEvent, title: string, index: string, active: boolean): void {
    if (!this.collapsed) return
    const target = event.currentTarget
    const aside = this.renderRoot.querySelector<HTMLElement>('aside')
    if (!(target instanceof HTMLElement) || !aside) return
    const targetRect = target.getBoundingClientRect()
    const asideRect = aside.getBoundingClientRect()
    this.hoverTitle = {
      index,
      title,
      top: targetRect.top - asideRect.top + targetRect.height / 2,
      active,
    }
  }

  private hideHoverTitle = (): void => {
    this.hoverTitle = undefined
  }
}

function storedCollapsed(): boolean {
  try {
    return localStorage.getItem('libredash-report-sidebar-collapsed') === 'true'
  } catch {
    return false
  }
}

function formatPageNumber(index: number, pageCount: number): string {
  const pageNumber = String(index + 1)
  return pageCount >= 10 ? pageNumber.padStart(2, '0') : pageNumber
}

function icon(name: 'chevron-left' | 'chevron-right') {
  switch (name) {
    case 'chevron-left':
      return iconSvg(svgTemplate`<path d="m15 18-6-6 6-6"></path>`)
    case 'chevron-right':
      return iconSvg(svgTemplate`<path d="m9 18 6-6-6-6"></path>`)
  }
}

function iconSvg(content: unknown) {
  return svgTemplate`<svg viewBox="0 0 24 24" aria-hidden="true">${content}</svg>`
}

customElements.define('ld-report-sidebar', ReportSidebar)
