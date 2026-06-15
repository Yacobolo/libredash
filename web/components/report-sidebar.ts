import { LitElement, css, html, nothing, svg as svgTemplate } from 'lit'
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
  modelId?: string
  modelTitle?: string
  modelHref?: string
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
      min-height: 100svh;
      color: var(--fgColor-default);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
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
      min-height: 100svh;
      grid-template-rows: auto minmax(0, 1fr) auto;
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

    .page-initial,
    .model-glyph {
      display: grid;
      width: 24px;
      height: 24px;
      flex: 0 0 auto;
      place-items: center;
      border-radius: 7px;
      background: transparent;
      color: var(--fgColor-muted);
      font-size: 0.62rem;
      font-weight: 900;
    }

    .model-glyph svg,
    .collapse svg {
      width: 14px;
      height: 14px;
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .section-title,
    .model-label {
      overflow: hidden;
      color: var(--fgColor-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 0.58rem;
      font-weight: 950;
      letter-spacing: 0;
      text-transform: uppercase;
    }

    .section-title {
      color: var(--fgColor-default);
      font-size: 0.64rem;
    }

    .collapse {
      display: grid;
      width: 24px;
      height: 24px;
      flex: 0 0 auto;
      place-items: center;
      margin-left: auto;
      border: 1px solid transparent;
      border-radius: 6px;
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
      overflow: auto;
      padding: 7px 5px;
    }

    a {
      text-decoration: none;
    }

    .page-link,
    .model-link {
      position: relative;
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      min-height: 30px;
      align-items: center;
      border: 1px solid transparent;
      border-radius: 6px;
      color: var(--fgColor-muted);
      padding: 0 9px;
      font-size: 0.7rem;
      font-weight: 800;
    }

    .page-link:hover,
    .page-link:focus-visible,
    .model-link:hover,
    .model-link:focus-visible {
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
      border-radius: 999px;
      background: var(--ld-accent);
    }

    .page-initial {
      display: none;
    }

    .link-text {
      overflow: hidden;
      min-width: 0;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    footer {
      display: grid;
      gap: 4px;
      border-top: 1px solid var(--borderColor-muted);
      padding: 6px 5px 7px;
    }

    .model-link {
      min-height: 28px;
      font-size: 0.66rem;
      font-weight: 750;
    }

    .model-glyph {
      display: none;
    }

    :host([data-collapsed]) header {
      padding: 8px 5px 6px;
    }

    :host([data-collapsed]) .section-title,
    :host([data-collapsed]) .link-text,
    :host([data-collapsed]) .model-label {
      display: none;
    }

    :host([data-collapsed]) .top-row {
      display: grid;
      justify-items: center;
    }

    :host([data-collapsed]) .collapse {
      margin-left: 0;
    }

    :host([data-collapsed]) .page-link,
    :host([data-collapsed]) .model-link {
      grid-template-columns: 24px;
      justify-content: center;
      padding-inline: 0;
    }

    :host([data-collapsed]) .page-initial,
    :host([data-collapsed]) .model-glyph {
      display: grid;
    }

    :host([data-collapsed]) .page-link {
      min-height: 28px;
    }

    :host([data-collapsed]) .page-link[aria-current='page']::before {
      content: none;
    }

    .rail-label {
      display: none;
    }

    :host([data-collapsed]) .rail-label {
      display: block;
      margin: 8px auto 10px;
      color: var(--fgColor-muted);
      font-size: 0.56rem;
      font-weight: 950;
      letter-spacing: 0;
      line-height: 1;
      text-orientation: mixed;
      text-transform: uppercase;
      transform: rotate(180deg);
      writing-mode: vertical-rl;
    }
  `

  updated(): void {
    this.toggleAttribute('data-collapsed', this.collapsed)
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
          ${pages.map((page) => this.renderPageLink(page))}
        </nav>

        <footer>
          <span class="model-label">Model</span>
          ${this.config.modelHref ? html`
            <a class="model-link" href=${this.config.modelHref} title=${this.config.modelTitle || 'Semantic model'}>
              <span class="model-glyph">${icon('model')}</span>
              <span class="link-text">${this.config.modelTitle || this.config.modelId || 'Model'}</span>
            </a>
          ` : nothing}
        </footer>
      </aside>
    `
  }

  private renderPageLink(page: ReportPage) {
    const active = Boolean(page.active || page.id === this.config.pageId)
    const title = page.title || page.id
    return html`
      <a class="page-link" href=${page.href} aria-current=${active ? 'page' : 'false'} title=${title}>
        <span class="page-initial" aria-hidden="true">${initials(title)}</span>
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
}

function initials(value: string): string {
  const words = value.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return 'P'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return words.slice(0, 2).map((word) => word[0]).join('').toUpperCase()
}

function storedCollapsed(): boolean {
  try {
    return localStorage.getItem('libredash-report-sidebar-collapsed') === 'true'
  } catch {
    return false
  }
}

function icon(name: 'model' | 'chevron-left' | 'chevron-right') {
  switch (name) {
    case 'model':
      return iconSvg(svgTemplate`<ellipse cx="12" cy="5" rx="8" ry="3"></ellipse><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5"></path><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"></path>`)
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
