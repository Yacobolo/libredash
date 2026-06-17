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

  static styles = css`
    :host {
      --ld-report-sidebar-width: 144px;
      display: block;
      width: var(--ld-report-sidebar-width);
      height: 100svh;
      min-height: 0;
      overflow: hidden;
      color: var(--fgColor-default);
      font-family: var(--fontStack-system);
      transition: width 180ms var(--ld-ease-out);
    }

    :host([data-collapsed]) {
      --ld-report-sidebar-width: 38px;
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
      border-right: 1px solid color-mix(in srgb, var(--borderColor-muted), transparent 36%);
      background: color-mix(in srgb, var(--bgColor-muted), var(--bgColor-default) 56%);
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
      color: var(--fgColor-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-950);
      letter-spacing: 0;
      text-transform: uppercase;
    }

    .section-title {
      color: var(--fgColor-default);
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
      color: var(--fgColor-muted);
      cursor: pointer;
      padding: 0;
    }

    .collapse:hover,
    .collapse:focus-visible {
      border-color: var(--borderColor-muted);
      background: var(--bgColor-muted);
      color: var(--fgColor-default);
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
    }

    a {
      text-decoration: none;
    }

    .page-link {
      position: relative;
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      min-height: 30px;
      align-items: center;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      color: var(--fgColor-muted);
      padding: 0 9px;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-800);
    }

    .page-link:hover,
    .page-link:focus-visible {
      background: var(--bgColor-muted);
      color: var(--fgColor-default);
      outline: 0;
    }

    .page-link[aria-current='page'] {
      border-color: transparent;
      background: color-mix(in srgb, var(--bgColor-muted), var(--bgColor-default) 30%);
      color: var(--fgColor-default);
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
      display: none;
    }

    .link-text {
      overflow: hidden;
      min-width: 0;
      text-overflow: ellipsis;
      white-space: nowrap;
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

    :host([data-collapsed]) .page-link {
      grid-template-columns: 24px;
      justify-content: center;
      padding-inline: 0;
    }

    :host([data-collapsed]) .page-index {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      color: var(--fgColor-muted);
      background: transparent;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-850);
      line-height: var(--ld-line-height-none);
    }

    :host([data-collapsed]) .page-link:hover .page-index,
    :host([data-collapsed]) .page-link:focus-visible .page-index {
      color: var(--fgColor-default);
    }

    :host([data-collapsed]) .page-link[aria-current='page'] .page-index {
      color: var(--fgColor-default);
    }

    :host([data-collapsed]) .page-link {
      min-height: 28px;
    }

    :host([data-collapsed]) .page-link[aria-current='page']::before {
      content: '';
      inset-block: 6px;
      left: 0;
      width: 2px;
    }

    .rail-label {
      display: none;
    }

    :host([data-collapsed]) .rail-label {
      display: block;
      margin: 8px auto 10px;
      color: var(--fgColor-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-950);
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

        <nav aria-label="Report pages">
          <span class="rail-label" aria-hidden="true">Pages</span>
          ${pages.map((page, index) => this.renderPageLink(page, index))}
        </nav>
      </aside>
    `
  }

  private renderPageLink(page: ReportPage, index: number) {
    const active = Boolean(page.active || page.id === this.config.pageId)
    const title = page.title || page.id
    return html`
      <a class="page-link" href=${page.href} aria-current=${active ? 'page' : 'false'} title=${title}>
        <span class="page-index" aria-hidden="true">${index + 1}</span>
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
}

function storedCollapsed(): boolean {
  try {
    return localStorage.getItem('libredash-report-sidebar-collapsed') === 'true'
  } catch {
    return false
  }
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
