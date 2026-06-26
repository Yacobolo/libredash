import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import {
  Activity,
  Database,
  Layers,
  LayoutDashboard,
  MessagesSquare,
  Monitor,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Plug,
  Settings,
  Sun,
  TableProperties,
  type IconNode,
} from 'lucide'
import { lucideIcon } from './lucide-icons'

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
  | 'chat'
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
      transition: width var(--motion-transition-stateChange);
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
      transition: width var(--motion-transition-stateChange);
    }

    .brand {
      display: grid;
      gap: var(--base-size-12);
      padding: var(--base-size-12);
    }

    .brand-row {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: var(--base-size-12);
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
      width: calc(var(--control-xsmall-size) + var(--base-size-2));
      height: calc(var(--control-xsmall-size) + var(--base-size-2));
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
      background: var(--control-bgColor-hover);
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
      gap: var(--base-size-8);
      min-height: 0;
      overflow: auto;
      padding: var(--base-size-8);
      border-bottom: var(--ld-border-muted);
    }

    .nav-group {
      display: grid;
      gap: var(--base-size-4);
    }

    a,
    button {
      font: inherit;
    }

    .nav-item {
      position: relative;
      display: grid;
      grid-template-columns: calc(var(--control-xsmall-size) + var(--base-size-2)) minmax(0, 1fr) auto;
      min-height: calc(var(--control-medium-size) + var(--base-size-2));
      align-items: center;
      gap: var(--base-size-8);
      border: var(--ld-border-transparent);
      border-radius: var(--ld-radius-default);
      color: var(--ld-fg-muted);
      padding: 0 var(--control-xsmall-paddingInline-normal);
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
      inset-block: var(--base-size-8);
      left: 0;
      width: var(--base-size-2);
      border-radius: var(--ld-radius-full);
      background: var(--ld-accent);
    }

    .nav-item.disabled {
      cursor: not-allowed;
      opacity: var(--opacity-disabled);
    }

    .nav-icon {
      display: grid;
      width: var(--control-xsmall-size);
      height: var(--control-xsmall-size);
      place-items: center;
      border-radius: var(--ld-radius-default);
      background: transparent;
    }

    .nav-item[aria-current='page'] .nav-icon {
      background: color-mix(in srgb, var(--ld-fg-muted), transparent 88%);
      color: var(--ld-fg-default);
    }

    svg {
      width: var(--base-size-16);
      height: var(--base-size-16);
      fill: none;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
    }

    .footer {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: var(--base-size-6);
      align-items: center;
      padding: var(--base-size-8);
      border-top: var(--ld-border-muted);
      background: transparent;
    }

    .user-card {
      display: grid;
      grid-template-columns: var(--control-small-size) minmax(0, 1fr);
      min-height: calc(var(--control-medium-size) + var(--base-size-2));
      align-items: center;
      gap: var(--base-size-8);
      border-radius: var(--ld-radius-default);
      color: var(--ld-fg-default);
      padding: 0 var(--control-xsmall-paddingInline-normal);
    }

    .user-card:hover {
      background: var(--ld-bg-hover);
    }

    .avatar {
      display: grid;
      width: var(--control-xsmall-size);
      height: var(--control-xsmall-size);
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
      gap: var(--base-size-2);
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
      gap: var(--base-size-4);
      align-items: center;
      justify-content: end;
    }

    .theme-button {
      display: inline-flex;
      width: var(--control-medium-size);
      height: var(--control-medium-size);
      min-height: var(--control-medium-size);
      align-items: center;
      justify-content: center;
      gap: var(--base-size-8);
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
      background: var(--control-bgColor-hover);
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
      padding: var(--base-size-8) var(--base-size-6);
    }

    :host([data-collapsed]) .brand-row {
      display: grid;
      justify-items: center;
      gap: var(--base-size-8);
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
      gap: var(--base-size-8);
      padding: var(--base-size-8) var(--base-size-4);
    }

    :host([data-collapsed]) .nav-group {
      justify-items: center;
      gap: var(--base-size-8);
    }

    :host([data-collapsed]) .nav-item {
      width: var(--base-size-36);
      min-height: var(--base-size-36);
      grid-template-columns: 1fr;
      justify-items: center;
      gap: 0;
      padding: 0;
    }

    :host([data-collapsed]) .nav-icon {
      width: var(--control-small-size);
      height: var(--control-small-size);
    }

    :host([data-collapsed]) .nav-item[aria-current='page']::before {
      content: none;
    }

    :host([data-collapsed]) .footer {
      grid-template-columns: 1fr;
      padding: var(--base-size-8) var(--base-size-4);
    }

    :host([data-collapsed]) .actions {
      display: grid;
      justify-content: center;
      justify-items: center;
    }

    :host([data-collapsed]) .theme-button {
      width: calc(var(--control-medium-size) + var(--base-size-2));
      min-height: calc(var(--control-medium-size) + var(--base-size-2));
      height: calc(var(--control-medium-size) + var(--base-size-2));
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
        min-height: auto;
      }

      aside {
        position: static;
        width: 100%;
        min-height: auto;
        grid-template-rows: auto;
      }

      .brand {
        padding: var(--base-size-12);
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
  const icons: Record<IconName, IconNode> = {
    catalog: Layers,
    dashboard: LayoutDashboard,
    chat: MessagesSquare,
    model: Database,
    data: Plug,
    cache: TableProperties,
    settings: Settings,
    system: Monitor,
    sun: Sun,
    moon: Moon,
    activity: Activity,
    collapse: PanelLeftClose,
    expand: PanelLeftOpen,
  }

  return lucideIcon(icons[name])
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
