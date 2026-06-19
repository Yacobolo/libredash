import { LitElement, css, html, svg as svgTemplate } from 'lit'
import { property, state } from 'lit/decorators.js'

type NavItem = {
  id: string
  label: string
  href: string
  icon: IconName
  meta?: string
  disabled?: boolean
}

type NavGroup = {
  label: string
  items: NavItem[]
}

type SidebarConfig = {
  active: string
  workspaceTitle?: string
  dashboardTitle?: string
  pageTitle?: string
  modelTitle?: string
  modelId?: string
  dashboardId?: string
  userRole?: string
  compact?: boolean
  groups: NavGroup[]
}

type SidebarStatus = {
  loading?: boolean
  lastUpdated?: string
  error?: string
}

type ThemeMode = 'system' | 'light' | 'dark'

type IconName =
  | 'catalog'
  | 'dashboard'
  | 'model'
  | 'data'
  | 'cache'
  | 'settings'
  | 'system'
  | 'sun'
  | 'moon'
  | 'activity'
  | 'collapse'
  | 'expand'

const defaultConfig: SidebarConfig = {
  active: 'dashboards',
  workspaceTitle: 'LibreDash Workspace',
  groups: [
    { label: 'Workspace', items: [{ id: 'dashboards', label: 'Dashboards', href: '/', icon: 'dashboard' }] },
  ],
}

const configConverter = {
  fromAttribute(value: string | null): SidebarConfig {
    if (!value) return defaultConfig
    try {
      return { ...defaultConfig, ...JSON.parse(value) } as SidebarConfig
    } catch {
      return defaultConfig
    }
  },
  toAttribute(value: SidebarConfig): string {
    return JSON.stringify(value ?? defaultConfig)
  },
}

const statusConverter = {
  fromAttribute(value: string | null): SidebarStatus {
    if (!value) return {}
    try {
      return JSON.parse(value) as SidebarStatus
    } catch {
      return {}
    }
  },
  toAttribute(value: SidebarStatus): string {
    return JSON.stringify(value ?? {})
  },
}

class LibreDashSidebar extends LitElement {
  @property({ attribute: 'config', converter: configConverter }) config: SidebarConfig = defaultConfig
  @property({ attribute: 'status', converter: statusConverter }) status: SidebarStatus = {}
  @state() private mode: ThemeMode = storedThemeMode()
  @state() private collapsed = storedCollapsed()

  static styles = css`
    :host {
      --ld-sidebar-width: 248px;
      display: block;
      width: var(--ld-sidebar-width);
      min-height: 100svh;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
      transition: width 180ms var(--ld-ease-out);
    }

    :host([data-collapsed]) {
      --ld-sidebar-width: 48px;
    }

    aside {
      position: sticky;
      top: 0;
      display: grid;
      width: var(--ld-sidebar-width);
      min-height: 100svh;
      grid-template-rows: auto minmax(0, 1fr) auto;
      background: var(--ld-sidebar-bg);
      transition: width 180ms var(--ld-ease-out);
    }

    .brand {
      display: grid;
      gap: 10px;
      padding: 12px 12px 11px;
    }

    .brand-row {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: 10px;
    }

    .name {
      overflow: hidden;
      min-width: 0;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-lg);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
    }

    .collapse-button {
      display: grid;
      width: 26px;
      height: 26px;
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

    .collapse-button:hover,
    .collapse-button:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .collapse-button:disabled {
      cursor: default;
      opacity: 0.7;
    }

    .collapse-button:disabled:hover {
      border-color: var(--ld-line-default);
      color: var(--ld-fg-muted);
    }

    nav {
      display: grid;
      align-content: start;
      gap: 9px;
      min-height: 0;
      overflow: auto;
      padding: 10px 7px;
      border-bottom: var(--ld-border-muted);
    }

    .nav-group {
      display: grid;
      gap: 5px;
    }

    a,
    button {
      font: inherit;
    }

    .nav-item {
      position: relative;
      display: grid;
      grid-template-columns: 26px minmax(0, 1fr) auto;
      min-height: 34px;
      align-items: center;
      gap: 8px;
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      color: var(--ld-fg-muted);
      padding: 0 9px;
      text-decoration: none;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-medium);
    }

    .nav-text {
      display: grid;
      gap: 0;
      min-width: 0;
    }

    .nav-text strong {
      overflow: hidden;
      color: inherit;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .nav-item:hover,
    .nav-item:focus-visible {
      background: var(--ld-bg-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .nav-item[aria-current='page'] {
      border-color: transparent;
      background: var(--ld-bg-hover);
      color: var(--ld-fg-default);
    }

    .nav-item[aria-current='page']::before {
      content: '';
      position: absolute;
      inset-block: 7px;
      left: 0;
      width: 2px;
      border-radius: var(--ld-radius-full);
      background: var(--ld-accent);
    }

    .nav-item.disabled {
      cursor: not-allowed;
      opacity: 0.48;
    }

    .nav-icon {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      border-radius: var(--ld-radius-default);
      background: transparent;
    }

    .nav-item[aria-current='page'] .nav-icon {
      background: color-mix(in srgb, var(--ld-fg-muted), transparent 88%);
      color: var(--ld-fg-default);
    }

    svg {
      width: 15px;
      height: 15px;
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .footer {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 6px;
      align-items: center;
      padding: 8px 7px 10px;
      border-top: var(--ld-border-muted);
      background: transparent;
    }

    .user-card {
      display: grid;
      grid-template-columns: 28px minmax(0, 1fr);
      min-height: 34px;
      align-items: center;
      gap: 8px;
      border-radius: var(--ld-radius-default);
      color: var(--ld-fg-default);
      padding: 0 7px;
    }

    .user-card:hover {
      background: var(--ld-bg-hover);
    }

    .avatar {
      display: grid;
      width: 24px;
      height: 24px;
      place-items: center;
      border-radius: 50%;
      background: color-mix(in srgb, var(--ld-fg-muted), transparent 78%);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
    }

    .user-text {
      display: grid;
      gap: 1px;
      min-width: 0;
    }

    .user-name,
    .user-role {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .user-name {
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .user-role {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .actions {
      display: flex;
      gap: 5px;
      align-items: center;
      justify-content: end;
    }

    .theme-button {
      display: inline-flex;
      width: 32px;
      height: 30px;
      min-height: 30px;
      align-items: center;
      justify-content: center;
      gap: 7px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: transparent;
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .theme-button:hover,
    .theme-button:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .theme-button {
      border-color: var(--ld-line-default);
      background: transparent;
      color: var(--ld-fg-default);
    }

    :host([data-collapsed]) .brand {
      justify-items: center;
      gap: 0;
      padding: 10px 6px;
    }

    :host([data-collapsed]) .brand-row {
      display: grid;
      justify-items: center;
      gap: 7px;
    }

    :host([data-collapsed]) .name,
    :host([data-collapsed]) .nav-group-label,
    :host([data-collapsed]) .nav-text,
    :host([data-collapsed]) .user-text {
      display: none;
    }

    :host([data-collapsed]) .collapse-button {
      margin-left: 0;
    }

    :host([data-collapsed]) nav {
      gap: 9px;
      padding: 9px 5px;
    }

    :host([data-collapsed]) .nav-group {
      justify-items: center;
      gap: 8px;
    }

    :host([data-collapsed]) .nav-item {
      width: 36px;
      min-height: 36px;
      grid-template-columns: 1fr;
      justify-items: center;
      gap: 0;
      padding: 0;
    }

    :host([data-collapsed]) .nav-icon {
      width: 28px;
      height: 28px;
    }

    :host([data-collapsed]) .nav-item[aria-current='page']::before {
      content: none;
    }

    :host([data-collapsed]) .footer {
      grid-template-columns: 1fr;
      padding: 8px 5px 9px;
    }

    :host([data-collapsed]) .actions {
      display: grid;
      justify-content: center;
      justify-items: center;
    }

    :host([data-collapsed]) .theme-button {
      width: 34px;
      min-height: 34px;
      height: 34px;
      padding: 0;
    }

    :host([data-collapsed]) .user-card {
      grid-template-columns: 1fr;
      justify-items: center;
      padding: 0;
    }

    @media (max-width: 640px) {
      :host,
      :host([data-collapsed]) {
        --ld-sidebar-width: 100%;
        width: 100%;
      }

      aside {
        position: static;
        width: 100%;
        min-height: auto;
        grid-template-rows: auto;
      }

      .brand {
        padding: 12px;
      }

      nav {
        display: flex;
        overflow-x: auto;
      }

      .nav-group {
        min-width: max-content;
      }

      .footer {
        display: none;
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('libredash-theme-applied', this.onThemeApplied as EventListener)
    this.mode = storedThemeMode()
    this.syncCollapsedState()
  }

  disconnectedCallback(): void {
    document.removeEventListener('libredash-theme-applied', this.onThemeApplied as EventListener)
    super.disconnectedCallback()
  }

  private onThemeApplied = (event: CustomEvent<{ mode: ThemeMode }>): void => {
    this.mode = normalizeThemeMode(event.detail?.mode)
  }

  private changeTheme(mode: ThemeMode): void {
    this.dispatchEvent(new CustomEvent('libredash-theme-change', {
      detail: { mode },
      bubbles: true,
      composed: true,
    }))
  }

  protected updated(): void {
    this.syncCollapsedState()
  }

  private syncCollapsedState(): void {
    if (this.effectiveCollapsed) {
      this.setAttribute('data-collapsed', '')
      this.style.setProperty('--ld-sidebar-width', '48px')
    } else {
      this.removeAttribute('data-collapsed')
      this.style.setProperty('--ld-sidebar-width', '248px')
    }
  }

  private toggleCollapsed(): void {
    if (this.config.compact) return
    this.collapsed = !this.collapsed
    try {
      localStorage.setItem('libredash-sidebar-collapsed', String(this.collapsed))
    } catch {
      // Ignore storage failures; the current session state still updates.
    }
    this.dispatchEvent(new CustomEvent('ld-sidebar-collapse', {
      detail: { collapsed: this.collapsed },
      bubbles: true,
      composed: true,
    }))
  }

  render() {
    const collapsed = this.effectiveCollapsed
    return html`
      <aside aria-label="LibreDash workspace">
        <header class="brand">
          <div class="brand-row">
            <span class="name">LibreDash</span>
            <button
              class="collapse-button"
              type="button"
              aria-label=${collapsed ? 'Expand navigation' : 'Collapse navigation'}
              aria-pressed=${String(collapsed)}
              title=${this.config.compact ? 'Workspace navigation is compact on report pages' : collapsed ? 'Expand navigation' : 'Collapse navigation'}
              ?disabled=${this.config.compact}
              @click=${this.toggleCollapsed}
            >
              ${icon(collapsed ? 'expand' : 'collapse')}
            </button>
          </div>
        </header>

        <nav aria-label="Primary">
          ${this.config.groups.map((group) => html`
            <section class="nav-group" aria-label=${group.label}>
              ${group.items.map((item) => item.disabled ? this.renderDisabledItem(item) : this.renderLink(item))}
            </section>
          `)}
        </nav>

        <footer class="footer">
          <div class="user-card" title="Jacob Nielsen">
            <span class="avatar" aria-hidden="true">JN</span>
            <span class="user-text">
              <strong class="user-name">Jacob Nielsen</strong>
              <span class="user-role">${this.config.userRole ?? 'Local workspace'}</span>
            </span>
          </div>
          <div class="actions">
            <button class="theme-button" type="button" aria-label=${this.themeLabel()} title=${this.themeTitle()} @click=${() => this.changeTheme(this.nextTheme())}>
              ${icon(this.themeIcon())}
            </button>
          </div>
        </footer>
      </aside>
    `
  }

  private get effectiveCollapsed(): boolean {
    return Boolean(this.config.compact || this.collapsed)
  }

  private nextTheme(): ThemeMode {
    if (this.mode === 'system') return 'light'
    if (this.mode === 'light') return 'dark'
    return 'system'
  }

  private themeLabel(): string {
    if (this.mode === 'system') return 'System'
    if (this.mode === 'light') return 'Light'
    return 'Dark'
  }

  private themeTitle(): string {
    const next = this.nextTheme()
    const nextLabel = next === 'system' ? 'System preference' : next === 'light' ? 'Light mode' : 'Dark mode'
    return `${this.themeLabel()} theme. Switch to ${nextLabel}.`
  }

  private themeIcon(): IconName {
    if (this.mode === 'system') return 'system'
    if (this.mode === 'light') return 'sun'
    return 'moon'
  }

  private renderLink(item: NavItem) {
    const current = item.id === this.config.active
    const label = item.meta ? `${item.label}: ${item.meta}` : item.label
    return html`
      <a class="nav-item" href=${item.href} aria-current=${current ? 'page' : 'false'} aria-label=${label} title=${label}>
        <span class="nav-icon">${icon(item.icon)}</span>
        <span class="nav-text">
          <strong>${item.label}</strong>
        </span>
      </a>
    `
  }

  private renderDisabledItem(item: NavItem) {
    const label = item.meta ? `${item.label}: ${item.meta}` : item.label
    return html`
      <span class="nav-item disabled" aria-disabled="true" aria-label=${label} title=${label}>
        <span class="nav-icon">${icon(item.icon)}</span>
        <span class="nav-text">
          <strong>${item.label}</strong>
        </span>
      </span>
    `
  }
}

function icon(name: IconName) {
  switch (name) {
    case 'catalog':
      return iconSvg(svgTemplate`<rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect>`)
    case 'dashboard':
      return iconSvg(svgTemplate`<path d="M3 3v18h18"></path><path d="M8 17V9"></path><path d="M13 17V5"></path><path d="M18 17v-6"></path>`)
    case 'model':
      return iconSvg(svgTemplate`<ellipse cx="12" cy="5" rx="8" ry="3"></ellipse><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5"></path><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"></path>`)
    case 'data':
      return iconSvg(svgTemplate`<path d="M3 4h18"></path><path d="M3 10h18"></path><path d="M3 16h18"></path><path d="M8 4v16"></path><path d="M16 4v16"></path>`)
    case 'cache':
      return iconSvg(svgTemplate`<path d="M4 7h16"></path><path d="M4 12h16"></path><path d="M4 17h16"></path><path d="M7 4v16"></path><path d="M17 4v16"></path>`)
    case 'settings':
      return iconSvg(svgTemplate`<path d="M12 15.5A3.5 3.5 0 1 0 12 8a3.5 3.5 0 0 0 0 7.5Z"></path><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h.1a1.7 1.7 0 0 0 .9-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 .9 1.5h.1a1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v.1a1.7 1.7 0 0 0 1.5.9H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5.9Z"></path>`)
    case 'system':
      return iconSvg(svgTemplate`<rect x="3" y="4" width="18" height="13" rx="2"></rect><path d="M8 21h8"></path><path d="M12 17v4"></path><path d="M8 8h8"></path><path d="M8 12h5"></path>`)
    case 'sun':
      return iconSvg(svgTemplate`<circle cx="12" cy="12" r="4"></circle><path d="M12 2v2"></path><path d="M12 20v2"></path><path d="m4.9 4.9 1.4 1.4"></path><path d="m17.7 17.7 1.4 1.4"></path><path d="M2 12h2"></path><path d="M20 12h2"></path><path d="m6.3 17.7-1.4 1.4"></path><path d="m19.1 4.9-1.4 1.4"></path>`)
    case 'moon':
      return iconSvg(svgTemplate`<path d="M20.9 13.4A8 8 0 0 1 10.6 3.1 8.9 8.9 0 1 0 20.9 13.4Z"></path>`)
    case 'activity':
      return iconSvg(svgTemplate`<path d="M22 12h-4l-3 8L9 4l-3 8H2"></path>`)
    case 'collapse':
      return iconSvg(svgTemplate`<rect x="3" y="4" width="18" height="16" rx="2"></rect><path d="M9 4v16"></path><path d="m16 10-2 2 2 2"></path>`)
    case 'expand':
      return iconSvg(svgTemplate`<rect x="3" y="4" width="18" height="16" rx="2"></rect><path d="M9 4v16"></path><path d="m14 10 2 2-2 2"></path>`)
  }
}

function iconSvg(content: unknown) {
  return svgTemplate`<svg viewBox="0 0 24 24" aria-hidden="true">${content}</svg>`
}

function storedCollapsed(): boolean {
  try {
    return localStorage.getItem('libredash-sidebar-collapsed') === 'true'
  } catch {
    return false
  }
}

function storedThemeMode(): ThemeMode {
  try {
    return normalizeThemeMode(localStorage.getItem('libredash-color-mode') || document.documentElement.dataset.colorMode)
  } catch {
    return normalizeThemeMode(document.documentElement.dataset.colorMode)
  }
}

function normalizeThemeMode(mode: string | null | undefined): ThemeMode {
  if (mode === 'light' || mode === 'dark') return mode
  return 'system'
}

customElements.define('ld-sidebar', LibreDashSidebar)
