import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import {
  Activity,
  Database,
  Layers,
  LayoutDashboard,
  Menu,
  MessagesSquare,
  Monitor,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Plug,
  Settings,
  Sun,
  TableProperties,
  X,
  type IconNode,
} from 'lucide'
import { lucideIcon } from '../shared/lucide-icons'

type NavItem = {
  id: string
  label: string
  href: string
  icon: string
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
  primaryAction?: SidebarAction
  history?: SidebarHistory
  groups: NavGroup[]
}

type SidebarAction = {
  label: string
  href: string
  icon: IconName
}

type SidebarHistory = {
  label: string
  emptyText?: string
  items: SidebarHistoryItem[]
}

type SidebarHistoryItem = {
  id: string
  title: string
  href: string
  active?: boolean
  pending?: boolean
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
  | 'menu'
  | 'close'
  | 'plus'

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
  @state() private mobileOpen = false
  private mobileMediaQuery?: MediaQueryList

  static styles = css`
    :host {
      --ld-sidebar-width: var(--ld-sidebar-width-expanded);
      display: block;
      width: var(--ld-sidebar-width);
      min-height: 100svh;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
      transition: width var(--motion-transition-stateChange);
    }

    :host([data-collapsed]) {
      --ld-sidebar-width: var(--ld-sidebar-width-collapsed);
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
      width: var(--ld-button-height-xs);
      height: var(--ld-button-height-xs);
      flex: 0 0 auto;
      place-items: center;
      margin-left: auto;
      border: var(--borderWidth-default) solid var(--ld-button-invisible-border-rest);
      border-radius: var(--ld-button-radius);
      background: var(--ld-button-invisible-bg-rest);
      color: var(--ld-button-invisible-icon-rest);
      cursor: pointer;
      padding: 0;
    }

    .collapse-button:hover,
    .collapse-button:focus-visible {
      border-color: var(--ld-button-invisible-border-hover);
      background: var(--ld-button-invisible-bg-hover);
      color: var(--ld-fg-default);
      outline: var(--focus-outline);
      outline-offset: var(--focus-outline-offset);
    }

    .collapse-button:disabled {
      cursor: default;
      opacity: 0.7;
    }

    .collapse-button:disabled:hover {
      border-color: var(--ld-button-invisible-border-rest);
      color: var(--fgColor-disabled);
    }

    .mobile-menu-button,
    .mobile-close-button,
    .mobile-backdrop,
    .mobile-drawer-header {
      display: none;
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
      gap: var(--base-size-2);
    }

    .primary-action {
      margin-bottom: var(--base-size-4);
    }

    .primary-action .nav-item {
      min-height: var(--control-medium-size);
      border-color: transparent;
      background: transparent;
      color: var(--ld-fg-default);
      font-weight: var(--ld-font-weight-strong);
    }

    .primary-action .nav-item:hover,
    .primary-action .nav-item:focus-visible {
      border-color: transparent;
      background: var(--control-bgColor-hover);
      color: var(--ld-fg-default);
    }

    .primary-action .nav-icon {
      width: calc(var(--control-xsmall-size) + var(--base-size-2));
      height: calc(var(--control-xsmall-size) + var(--base-size-2));
      border-radius: var(--ld-radius-full);
      background: var(--control-bgColor-hover);
      color: var(--ld-fg-default);
      transition:
        background var(--motion-transition-stateChange),
        transform var(--motion-transition-stateChange);
    }

    .primary-action .nav-item:hover .nav-icon,
    .primary-action .nav-item:focus-visible .nav-icon {
      background: var(--ld-bg-selected);
      transform: rotate(-3deg) scale(1.06);
    }

    .history {
      display: grid;
      gap: var(--base-size-4);
      min-height: 0;
      padding-top: var(--base-size-8);
    }

    .history-label {
      overflow: hidden;
      margin:
        0
        var(--control-xsmall-paddingInline-normal)
        0
        calc(var(--control-xsmall-paddingInline-normal) + var(--ld-border-width));
      color: var(--fgColor-disabled);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      letter-spacing: 0;
    }

    .history-list {
      display: grid;
      gap: var(--base-size-2);
      min-height: 0;
    }

    .nav-item.history-item {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .history-title {
      overflow: hidden;
      min-width: 0;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .history-empty {
      padding: var(--base-size-4) var(--control-xsmall-paddingInline-normal);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      line-height: var(--ld-line-height-compact);
    }

    .pending-spinner {
      width: var(--ld-spinner-size-sm);
      height: var(--ld-spinner-size-sm);
      border: var(--ld-spinner-border-width) solid var(--ld-line-muted);
      border-top-color: var(--ld-fg-muted);
      border-radius: var(--ld-radius-full);
      animation: pending-spin var(--ld-duration-slow) linear infinite;
    }

    a,
    button {
      font: inherit;
    }

    .nav-item {
      position: relative;
      box-sizing: border-box;
      display: grid;
      grid-template-columns: calc(var(--control-xsmall-size) + var(--base-size-2)) minmax(0, 1fr) auto;
      min-height: var(--control-medium-size);
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
      background: var(--control-bgColor-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .nav-item[aria-current='page'] {
      border-color: transparent;
      background: var(--control-bgColor-hover);
      color: var(--ld-fg-default);
    }

    .nav-item[aria-current='page']::before {
      content: none;
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
      background: var(--control-bgColor-hover);
    }

    .avatar {
      display: grid;
      width: var(--control-xsmall-size);
      height: var(--control-xsmall-size);
      place-items: center;
      border-radius: 50%;
      background: var(--bgColor-neutral-muted);
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
      width: var(--ld-button-height);
      height: var(--ld-button-height);
      min-height: var(--ld-button-height);
      align-items: center;
      justify-content: center;
      gap: var(--base-size-8);
      border: var(--borderWidth-default) solid var(--ld-button-border-rest);
      border-radius: var(--ld-button-radius);
      background: var(--ld-button-bg-rest);
      color: var(--ld-button-fg-rest);
      cursor: pointer;
      padding: 0;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .theme-button:hover,
    .theme-button:focus-visible {
      border-color: var(--ld-button-border-hover);
      background: var(--ld-button-bg-hover);
      color: var(--ld-fg-default);
      outline: var(--focus-outline);
      outline-offset: var(--focus-outline-offset);
    }

    .theme-button {
      border-color: var(--ld-button-border-rest);
      background: var(--ld-button-bg-rest);
      color: var(--ld-button-fg-rest);
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
    :host([data-collapsed]) .history,
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
      width: calc(var(--ld-button-height) + var(--base-size-2));
      min-height: calc(var(--ld-button-height) + var(--base-size-2));
      height: calc(var(--ld-button-height) + var(--base-size-2));
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
        min-height: var(--control-large-size);
      }

      aside {
        position: relative;
        display: block;
        width: 100%;
        min-height: var(--control-large-size);
      }

      .brand {
        display: none;
      }

      .mobile-menu-button,
      .mobile-close-button {
        display: inline-grid;
        width: var(--ld-button-height-xs);
        height: var(--ld-button-height-xs);
        place-items: center;
        border: var(--ld-border-transparent);
        border-radius: var(--ld-button-radius);
        background: transparent;
        color: var(--ld-fg-muted);
        cursor: pointer;
        padding: 0;
      }

      .mobile-menu-button {
        margin: var(--base-size-8);
      }

      .mobile-menu-button:hover,
      .mobile-menu-button:focus-visible,
      .mobile-close-button:hover,
      .mobile-close-button:focus-visible {
        background: var(--control-bgColor-hover);
        color: var(--ld-fg-default);
        outline: var(--focus-outline);
        outline-offset: var(--focus-outline-offset);
      }

      .collapse-button,
      :host([data-collapsed]) .collapse-button {
        display: none;
      }

      .mobile-backdrop {
        position: fixed;
        z-index: var(--z-index-report-sidebar);
        inset: 0;
        display: block;
        border: 0;
        background: var(--ld-modal-backdrop);
        cursor: pointer;
        opacity: 0;
        pointer-events: none;
        transition: opacity var(--motion-transition-stateChange), visibility var(--motion-transition-stateChange);
        visibility: hidden;
      }

      nav {
        position: fixed;
        z-index: var(--z-index-sidebar);
        top: 0;
        bottom: 0;
        left: 0;
        box-sizing: border-box;
        display: grid;
        width: min(20rem, calc(100vw - var(--base-size-32)));
        min-height: 100svh;
        align-content: start;
        overflow-y: auto;
        border: 0;
        border-right: var(--ld-border-default);
        background: var(--ld-sidebar-bg);
        box-shadow: var(--ld-shadow-floating);
        padding: var(--base-size-12);
        pointer-events: none;
        transform: translateX(-100%);
        transition: transform var(--motion-transition-stateChange), visibility var(--motion-transition-stateChange);
        visibility: hidden;
      }

      aside[data-mobile-open] nav {
        pointer-events: auto;
        transform: translateX(0);
        visibility: visible;
      }

      aside[data-mobile-open] .mobile-backdrop {
        opacity: 1;
        pointer-events: auto;
        visibility: visible;
      }

      .mobile-drawer-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin-bottom: var(--base-size-8);
        border-bottom: var(--ld-border-muted);
        padding-bottom: var(--base-size-8);
      }

      .mobile-drawer-title {
        font-size: var(--ld-font-size-body-lg);
        font-weight: var(--ld-font-weight-strong);
      }

      .history,
      :host([data-collapsed]) .history {
        display: grid;
      }

      .nav-group,
      :host([data-collapsed]) .nav-group {
        display: grid;
        gap: var(--base-size-2);
        min-width: 0;
      }

      :host([data-collapsed]) nav {
        gap: var(--base-size-8);
        padding: var(--base-size-12);
      }

      .nav-item,
      :host([data-collapsed]) .nav-item {
        width: 100%;
        min-height: var(--control-medium-size);
        grid-template-columns: calc(var(--control-xsmall-size) + var(--base-size-2)) minmax(0, 1fr) auto;
        justify-items: stretch;
        gap: var(--base-size-8);
        padding: 0 var(--control-xsmall-paddingInline-normal);
      }

      :host([data-collapsed]) .nav-text {
        display: grid;
      }

      :host([data-collapsed]) .nav-icon {
        width: var(--control-xsmall-size);
        height: var(--control-xsmall-size);
      }

      .footer {
        display: none;
      }
    }

    @keyframes pending-spin {
      to {
        transform: rotate(360deg);
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    document.addEventListener('libredash-theme-applied', this.onThemeApplied as EventListener)
    document.addEventListener('keydown', this.onKeyDown)
    this.mobileMediaQuery = window.matchMedia('(max-width: 640px)')
    this.mobileMediaQuery.addEventListener('change', this.onMobileViewportChange)
    this.mode = storedThemeMode()
    this.syncCollapsedState()
  }

  disconnectedCallback(): void {
    document.removeEventListener('libredash-theme-applied', this.onThemeApplied as EventListener)
    document.removeEventListener('keydown', this.onKeyDown)
    this.mobileMediaQuery?.removeEventListener('change', this.onMobileViewportChange)
    this.mobileMediaQuery = undefined
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
      this.style.setProperty('--ld-sidebar-width', 'var(--ld-sidebar-width-collapsed)')
    } else {
      this.removeAttribute('data-collapsed')
      this.style.setProperty('--ld-sidebar-width', 'var(--ld-sidebar-width-expanded)')
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

  private get isMobileViewport(): boolean {
    return this.mobileMediaQuery?.matches ?? (typeof window !== 'undefined' && window.matchMedia('(max-width: 640px)').matches)
  }

  private onMobileViewportChange = (event: MediaQueryListEvent): void => {
    if (!event.matches) this.mobileOpen = false
    this.requestUpdate()
  }

  private toggleMobileNavigation(): void {
    this.mobileOpen = !this.mobileOpen
    if (!this.mobileOpen) return
    void this.updateComplete.then(() => this.shadowRoot?.querySelector<HTMLElement>('nav a')?.focus())
  }

  private closeMobileNavigation(restoreFocus = false): void {
    if (!this.mobileOpen) return
    this.mobileOpen = false
    if (restoreFocus) {
      void this.updateComplete.then(() => this.shadowRoot?.querySelector<HTMLButtonElement>('.mobile-menu-button')?.focus())
    }
  }

  private onKeyDown = (event: KeyboardEvent): void => {
    if (event.key !== 'Escape' || !this.mobileOpen) return
    event.preventDefault()
    this.closeMobileNavigation(true)
  }

  render() {
    const collapsed = this.effectiveCollapsed
    const mobileNavigationClosed = this.isMobileViewport && !this.mobileOpen
    return html`
      <aside aria-label="LibreDash workspace" ?data-mobile-open=${this.mobileOpen}>
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

        <button
          class="mobile-menu-button"
          type="button"
          aria-label="Open navigation"
          aria-hidden=${String(this.mobileOpen)}
          aria-controls="mobile-navigation"
          aria-expanded=${String(this.mobileOpen)}
          title="Open navigation"
          ?inert=${this.mobileOpen}
          @click=${this.toggleMobileNavigation}
        >
          ${icon(this.mobileOpen ? 'close' : 'menu')}
        </button>

        <div class="mobile-backdrop" aria-hidden="true" @click=${() => this.closeMobileNavigation(true)}></div>

        <nav id="mobile-navigation" aria-label="Primary" aria-hidden=${String(mobileNavigationClosed)} ?inert=${mobileNavigationClosed}>
          <div class="mobile-drawer-header">
            <strong class="mobile-drawer-title">LibreDash</strong>
            <button class="mobile-close-button" type="button" aria-label="Close navigation" title="Close navigation" @click=${() => this.closeMobileNavigation(true)}>
              ${icon('close')}
            </button>
          </div>
          ${this.config.primaryAction ? html`
            <section class="nav-group primary-action" aria-label="Chat action">
              ${this.renderLink({
                id: 'primary-action',
                label: this.config.primaryAction.label,
                href: this.config.primaryAction.href,
                icon: this.config.primaryAction.icon,
              })}
            </section>
          ` : null}
          ${this.config.groups.map((group) => html`
            <section class="nav-group" aria-label=${group.label}>
              ${group.items.map((item) => item.disabled ? this.renderDisabledItem(item) : this.renderLink(item))}
            </section>
          `)}
          ${this.renderHistory()}
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
    return html`
      <a class="nav-item" href=${item.href} aria-current=${current ? 'page' : 'false'} aria-label=${item.label} title=${item.label} @click=${(event: MouseEvent) => this.followInternalLink(event, item.href)}>
        <span class="nav-icon">${icon(item.icon)}</span>
        <span class="nav-text">
          <strong>${item.label}</strong>
        </span>
      </a>
    `
  }

  private renderDisabledItem(item: NavItem) {
    return html`
      <span class="nav-item disabled" aria-disabled="true" aria-label=${item.label} title=${item.label}>
        <span class="nav-icon">${icon(item.icon)}</span>
        <span class="nav-text">
          <strong>${item.label}</strong>
        </span>
      </span>
    `
  }

  private renderHistory() {
    const history = this.config.history
    if (!history) return null
    const items = Array.isArray(history.items) ? history.items : []
    return html`
      <section class="history" aria-label=${history.label || 'Chats'}>
        <strong class="history-label">${history.label || 'Chats'}</strong>
        <div class="history-list">
          ${items.length === 0 ? html`<span class="history-empty">${history.emptyText || 'No chats yet.'}</span>` : null}
          ${items.map((item) => this.renderHistoryItem(item))}
        </div>
      </section>
    `
  }

  private renderHistoryItem(item: SidebarHistoryItem) {
    const title = item.title || 'Conversation'
    return html`
      <a class="nav-item history-item" href=${item.href} aria-current=${item.active ? 'page' : 'false'} aria-label=${title} title=${title} @click=${(event: MouseEvent) => this.followInternalLink(event, item.href)}>
        <span class="history-title">${title}</span>
        ${item.pending ? html`<span class="pending-spinner" aria-label="Title loading"></span>` : null}
      </a>
    `
  }

  private followInternalLink(event: MouseEvent, href: string): void {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    const target = new URL(href, window.location.href)
    if (target.origin !== window.location.origin || target.href === window.location.href) return
    event.preventDefault()
    this.closeMobileNavigation()
    window.location.assign(target.href)
  }
}

function icon(name: string) {
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
    menu: Menu,
    close: X,
    plus: Plus,
  }

  return lucideIcon(icons[name as IconName] ?? Layers)
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
