import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { Mail, Search, Trash2, Users, X } from 'lucide'
import { lucideIcon } from './lucide-icons'

type Workspace = {
  id?: string
  title?: string
}

type Role = {
  name: string
}

type Binding = {
  principalId: string
  email: string
  displayName?: string
  role: string
}

type AccessStatus = {
  loading?: boolean
  error?: string
  message?: string
}

type WorkspaceAccess = {
  workspace?: Workspace
  roles?: Role[]
  bindings?: Binding[]
  canManage?: boolean
  status?: AccessStatus
}

type WorkspaceAccessInput = {
  workspace?: unknown
  roles?: unknown
  bindings?: unknown
  canManage?: unknown
  status?: unknown
}

type AccessCommand = {
  email?: string
  role?: string
  principalId?: string
}

const emptyAccess: WorkspaceAccess = {
  roles: [],
  bindings: [],
  canManage: false,
  status: {},
}

const focusableSelector = [
  'a[href]:not([tabindex="-1"])',
  'button:not([disabled]):not([tabindex="-1"])',
  'input:not([disabled]):not([tabindex="-1"])',
  'select:not([disabled]):not([tabindex="-1"])',
  'textarea:not([disabled]):not([tabindex="-1"])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

class WorkspaceAccessControl extends LitElement {
  @property({ attribute: false }) access: WorkspaceAccessInput | null = null
  @property({ attribute: 'access' }) accessAttribute = ''
  @property({ attribute: 'search' }) searchAttribute = ''

  @state() private open = false
  @state() private email = ''
  @state() private selectedRole = 'viewer'
  @state() private query = ''

  private previousFocus: HTMLElement | null = null
  private searchTimer: number | null = null

  static styles = css`
    :host {
      display: inline-block;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui);
    }

    button,
    input,
    select {
      font: inherit;
    }

    .trigger {
      display: inline-flex;
      min-height: var(--ld-control-medium);
      align-items: center;
      justify-content: center;
      gap: var(--base-size-6);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      cursor: pointer;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-tight);
      padding: 0 var(--base-size-10);
      transition:
        color var(--ld-transition-fast),
        background-color var(--ld-transition-fast),
        border-color var(--ld-transition-fast);
    }

    .trigger:hover,
    .trigger:focus-visible {
      border-color: var(--ld-line-default);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .icon {
      display: inline-flex;
      width: var(--ld-icon-sm);
      height: var(--ld-icon-sm);
      align-items: center;
      justify-content: center;
      color: currentColor;
    }

    .overlay {
      position: fixed;
      inset: 0;
      z-index: calc(var(--z-index-inspector) - 1);
      display: grid;
      place-items: center;
      background: var(--ld-modal-backdrop);
      padding: var(--base-size-32) var(--base-size-16);
    }

    .dialog {
      display: grid;
      width: min(38rem, calc(100vw - var(--base-size-32)));
      max-height: calc(100vh - var(--base-size-64));
      grid-template-rows: auto minmax(0, 1fr);
      overflow: hidden;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-large);
      background: var(--ld-bg-panel);
      box-shadow: var(--ld-shadow-floating-lg);
    }

    .header,
    .footer {
      border-bottom: var(--ld-border-muted);
      padding: var(--base-size-16) var(--base-size-20);
    }

    .header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: var(--base-size-16);
    }

    .title {
      margin: 0;
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-title-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-snug);
    }

    .subtitle {
      margin: var(--base-size-4) 0 0;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-normal);
      line-height: var(--ld-line-height-snug);
    }

    .close,
    .row-action {
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
    .close:focus-visible,
    .row-action:hover,
    .row-action:focus-visible {
      border-color: var(--ld-line-muted);
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .body {
      display: grid;
      gap: var(--base-size-24);
      min-height: 0;
      overflow: auto;
      padding: var(--base-size-20);
    }

    .card {
      display: grid;
      gap: var(--base-size-10);
    }

    .section-title {
      margin: 0;
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-snug);
    }

    .label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
    }

    .field-shell {
      display: flex;
      min-height: var(--ld-control-medium);
      min-width: 0;
      align-items: center;
      gap: var(--base-size-8);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      padding: 0 var(--base-size-10);
      transition:
        background-color var(--ld-transition-fast),
        border-color var(--ld-transition-fast);
    }

    .field-shell:focus-within,
    .field-shell:hover {
      border-color: var(--ld-line-accent);
      background: var(--ld-bg-control-hover);
    }

    .composer {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: var(--base-size-8);
      align-items: center;
    }

    .composer-shell {
      min-height: var(--ld-control-medium);
      border-radius: var(--ld-radius-tight);
      padding: var(--base-size-4) var(--base-size-6) var(--base-size-4) var(--base-size-12);
    }

    .composer-shell input {
      flex: 1 1 12rem;
    }

    .composer-role {
      width: auto;
      min-width: 7rem;
      flex: 0 0 auto;
      border: 0;
      border-left: var(--ld-border-muted);
      border-radius: 0;
      background: transparent;
      color: var(--ld-fg-default);
      padding-left: var(--base-size-12);
    }

    .composer-role:focus {
      outline-offset: var(--base-size-2);
    }

    input,
    select {
      min-height: var(--ld-control-medium);
      min-width: 0;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-snug);
      padding: 0 var(--base-size-8);
    }

    .field-shell input,
    .field-shell select {
      min-height: auto;
      border: 0;
      border-radius: 0;
      background: transparent;
      padding: 0;
      outline: 0;
    }

    .field-shell input {
      flex: 1 1 auto;
    }

    input::placeholder {
      color: var(--ld-fg-muted);
    }

    input:focus,
    select:focus {
      border-color: var(--ld-line-accent);
      outline: 2px solid var(--ld-line-accent-muted);
      outline-offset: 0;
    }

    .submit {
      min-height: var(--ld-control-medium);
      min-width: var(--base-size-80);
      border: 0;
      border-radius: var(--ld-radius-tight);
      background: var(--button-primary-bgColor-rest);
      color: var(--button-primary-fgColor-rest);
      cursor: pointer;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-tight);
      padding: 0 var(--base-size-16);
    }

    .submit:hover,
    .submit:focus-visible {
      background: var(--button-primary-bgColor-hover);
      outline: 0;
    }

    .submit:disabled,
    .row-action:disabled {
      cursor: not-allowed;
      opacity: var(--opacity-disabled);
    }

    .status {
      border-radius: var(--ld-radius-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-snug);
      padding: var(--base-size-8) var(--base-size-12);
    }

    .status-error {
      border: var(--ld-border-danger);
      background: var(--ld-bg-danger-muted);
      color: var(--ld-fg-danger);
    }

    .status-message {
      border: 1px solid var(--ld-line-success-muted);
      background: var(--ld-bg-success-muted);
      color: var(--ld-fg-success);
    }

    .toolbar {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(12rem, 18rem);
      align-items: center;
      gap: var(--base-size-12);
    }

    .search {
      width: 100%;
    }

    .list {
      display: grid;
      overflow: hidden;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-tight);
      background: var(--ld-bg-panel);
    }

    .row {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(8rem, 10rem) auto;
      align-items: center;
      gap: var(--base-size-12);
      border-bottom: var(--ld-border-muted);
      padding: var(--base-size-10) var(--base-size-12);
    }

    .person {
      display: grid;
      grid-template-columns: var(--base-size-32) minmax(0, 1fr);
      align-items: center;
      gap: var(--base-size-8);
      min-width: 0;
    }

    .principal-copy {
      min-width: 0;
    }

    .avatar {
      display: inline-flex;
      width: var(--base-size-28);
      height: var(--base-size-28);
      align-items: center;
      justify-content: center;
      border: var(--ld-border-muted);
      border-radius: 999px;
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      line-height: 1;
      text-transform: uppercase;
    }

    .name {
      overflow: hidden;
      margin: 0;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-snug);
    }

    .email {
      overflow: hidden;
      margin: var(--base-size-2) 0 0;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-normal);
      line-height: var(--ld-line-height-tight);
    }

    .empty {
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-tight);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
      padding: var(--base-size-20) var(--base-size-16);
      text-align: center;
    }

    @media (max-width: 44rem) {
      .overlay {
        align-items: end;
        padding: var(--base-size-8);
      }

      .dialog {
        width: 100%;
        max-height: calc(100vh - var(--base-size-16));
      }

      .composer,
      .row,
      .toolbar {
        grid-template-columns: minmax(0, 1fr);
      }

      .composer-shell {
        align-items: stretch;
        flex-wrap: wrap;
        padding: var(--base-size-8);
      }

      .composer-role {
        min-width: 100%;
        border-top: var(--ld-border-muted);
        border-left: 0;
        padding-left: var(--base-size-8);
      }

      .submit {
        justify-self: stretch;
      }
    }
  `

  updated(changed: Map<string, unknown>): void {
    if (changed.has('access') || changed.has('accessAttribute')) {
      this.ensureRole()
      const status = this.resolvedAccess.status
      if (status?.message && !status.error && !status.loading) {
        this.email = ''
      }
    }
    if (changed.has('searchAttribute') && this.searchAttribute !== this.query) {
      this.query = this.searchAttribute
    }
  }

  render() {
    const access = this.resolvedAccess
    if (!access.canManage) return nothing

    return html`
      <button class="trigger" type="button" aria-haspopup="dialog" aria-expanded=${String(this.open)} @click=${this.openDialog}>
        ${usersIcon()}
        <span>Manage access</span>
      </button>
      ${this.open ? this.renderModal(access) : nothing}
    `
  }

  private renderModal(access: WorkspaceAccess) {
    const status = access.status ?? {}
    return html`
      <div class="overlay" @click=${this.handleOverlayClick}>
        <section
          class="dialog"
          role="dialog"
          aria-modal="true"
          aria-labelledby="workspace-access-title"
          @keydown=${this.handleKeyDown}
        >
          <header class="header">
            <div>
              <h2 class="title" id="workspace-access-title">Manage access</h2>
              <p class="subtitle">${access.workspace?.title ?? 'Workspace'} roles apply to every published asset in this workspace.</p>
            </div>
            <button class="close" type="button" aria-label="Close workspace access" @click=${this.closeDialog}>
              ${xIcon()}
            </button>
          </header>
          <div class="body">
            <section class="card" aria-label="Add workspace access">
              <div class="label">Add people by email</div>
              ${status.error ? html`<div class="status status-error" role="alert">${status.error}</div>` : nothing}
              ${status.message && !status.error ? html`<div class="status status-message" role="status">${status.message}</div>` : nothing}
              <form class="composer" @submit=${this.handleSubmit}>
                <span class="field-shell composer-shell">
                  ${mailIcon()}
                  <input
                    type="email"
                    autocomplete="email"
                    placeholder="Search by email..."
                    aria-label="Email principal"
                    .value=${this.email}
                    ?disabled=${status.loading}
                    @input=${(event: Event) => { this.email = (event.currentTarget as HTMLInputElement).value }}
                  >
                  <select
                    class="composer-role"
                    aria-label="Role to assign"
                    .value=${this.selectedRole}
                    ?disabled=${status.loading}
                    @change=${(event: Event) => { this.selectedRole = (event.currentTarget as HTMLSelectElement).value }}
                  >
                    ${this.roles.map((role) => html`<option value=${role.name}>${roleLabel(role.name)}</option>`)}
                  </select>
                </span>
                <button class="submit" type="submit" ?disabled=${status.loading || !this.email.trim() || !this.selectedRole}>
                  ${status.loading ? 'Saving' : 'Assign'}
                </button>
              </form>
            </section>
            <section class="card" aria-label="Current workspace access">
              <div class="toolbar">
                <h3 class="section-title">People with access</h3>
                <span class="field-shell search">
                  ${searchIcon()}
                  <input
                    type="search"
                    placeholder="Search principals..."
                    .value=${this.query}
                    @input=${this.handleSearchInput}
                  >
                </span>
              </div>
              ${this.renderBindings(access)}
            </section>
          </div>
        </section>
      </div>
    `
  }

  private renderBindings(access: WorkspaceAccess) {
    const rows = this.filteredBindings(access.bindings ?? [])
    if (rows.length === 0) {
      return html`<div class="empty">${this.query ? 'No access entries match this search.' : 'No role bindings yet.'}</div>`
    }
    return html`
      <div class="list">
        ${rows.map((binding) => html`
          <div class="row">
            <div class="person">
              <span class="avatar" aria-hidden="true">${principalInitial(binding)}</span>
              <span class="principal-copy">
                <p class="name">${displayLabel(binding)}</p>
                <p class="email">${binding.email}</p>
              </span>
            </div>
            <select
              aria-label=${`Role for ${displayLabel(binding)}`}
              .value=${binding.role}
              ?disabled=${access.status?.loading}
              @change=${(event: Event) => this.updateBindingRole(binding, (event.currentTarget as HTMLSelectElement).value)}
            >
              ${this.roles.map((role) => html`<option value=${role.name}>${roleLabel(role.name)}</option>`)}
            </select>
            <button
              class="row-action"
              type="button"
              aria-label=${`Remove ${displayLabel(binding)}`}
              ?disabled=${access.status?.loading}
              @click=${() => this.removeBinding(binding)}
            >
              ${trashIcon()}
            </button>
          </div>
        `)}
      </div>
    `
  }

  private get resolvedAccess(): WorkspaceAccess {
    if (this.access) return normalizeAccess(this.access)
    if (this.accessAttribute) {
      try {
        return normalizeAccess(JSON.parse(this.accessAttribute) as WorkspaceAccessInput)
      } catch {
        return emptyAccess
      }
    }
    return emptyAccess
  }

  private get roles(): Role[] {
    return this.resolvedAccess.roles ?? []
  }

  private ensureRole(): void {
    const roles = this.roles
    if (roles.some((role) => role.name === this.selectedRole)) return
    this.selectedRole = roles.find((role) => role.name === 'viewer')?.name ?? roles[0]?.name ?? ''
  }

  private filteredBindings(bindings: Binding[]): Binding[] {
    const query = this.query.trim().toLowerCase()
    if (!query) return bindings
    return bindings.filter((binding) => {
      return `${binding.email} ${binding.role}`.toLowerCase().includes(query)
    })
  }

  private readonly handleSearchInput = (event: Event): void => {
    this.query = (event.currentTarget as HTMLInputElement).value
    if (this.searchTimer !== null) window.clearTimeout(this.searchTimer)
    this.searchTimer = window.setTimeout(() => {
      this.dispatchEvent(new CustomEvent('ld-workspace-access-search', {
        bubbles: true,
        composed: true,
        detail: { search: this.query },
      }))
    }, 200)
  }

  private readonly openDialog = (): void => {
    this.previousFocus = document.activeElement as HTMLElement | null
    this.open = true
    window.setTimeout(() => {
      const first = this.focusableElements()[0]
      first?.focus()
    }, 0)
  }

  private readonly closeDialog = (): void => {
    this.open = false
    if (this.searchTimer !== null) {
      window.clearTimeout(this.searchTimer)
      this.searchTimer = null
    }
    window.setTimeout(() => {
      if (this.previousFocus?.isConnected) this.previousFocus.focus()
      this.previousFocus = null
    }, 0)
  }

  private readonly handleOverlayClick = (event: Event): void => {
    if (event.target === event.currentTarget) this.closeDialog()
  }

  private readonly handleKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape') {
      event.preventDefault()
      this.closeDialog()
      return
    }
    if (event.key !== 'Tab') return
    const focusable = this.focusableElements()
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (event.shiftKey && this.shadowRoot?.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && this.shadowRoot?.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  private focusableElements(): HTMLElement[] {
    const dialog = this.renderRoot.querySelector<HTMLElement>('.dialog')
    if (!dialog) return []
    return Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
  }

  private readonly handleSubmit = (event: Event): void => {
    event.preventDefault()
    const command: AccessCommand = {
      email: this.email.trim(),
      role: this.selectedRole,
    }
    if (!command.email || !command.role) return
    this.dispatchEvent(new CustomEvent('ld-workspace-access-upsert', {
      bubbles: true,
      composed: true,
      detail: command,
    }))
  }

  private updateBindingRole(binding: Binding, role: string): void {
    if (!binding.email || !role || role === binding.role) return
    this.dispatchEvent(new CustomEvent('ld-workspace-access-upsert', {
      bubbles: true,
      composed: true,
      detail: {
        email: binding.email,
        role,
      },
    }))
  }

  private removeBinding(binding: Binding): void {
    if (!binding.principalId) return
    this.dispatchEvent(new CustomEvent('ld-workspace-access-remove', {
      bubbles: true,
      composed: true,
      detail: {
        principalId: binding.principalId,
      },
    }))
  }
}

function normalizeAccess(access: WorkspaceAccessInput): WorkspaceAccess {
  const raw = recordValue(access)
  return {
    workspace: normalizeWorkspace(access.workspace),
    roles: Array.isArray(access.roles) ? access.roles.map(normalizeRole).filter(isRole) : [],
    bindings: Array.isArray(access.bindings) ? access.bindings.map(normalizeBinding).filter(isBinding) : [],
    canManage: Boolean(access.canManage ?? raw.CanManage),
    status: normalizeStatus(access.status ?? raw.Status),
  }
}

function normalizeWorkspace(workspace: unknown): Workspace {
  const raw = recordValue(workspace)
  return {
    id: stringValue(raw.id ?? raw.ID),
    title: stringValue(raw.title ?? raw.Title),
  }
}

function normalizeRole(role: unknown): Role | null {
  if (typeof role === 'string') {
    const name = role.trim()
    return name ? { name } : null
  }
  const raw = recordValue(role)
  const name = stringValue(raw.name ?? raw.Name).trim()
  return name ? { name } : null
}

function normalizeBinding(binding: unknown): Binding | null {
  const raw = recordValue(binding)
  const email = stringValue(raw.email ?? raw.Email)
  const principalId = stringValue(raw.principalId ?? raw.PrincipalID)
  const role = stringValue(raw.role ?? raw.Role)
  if (!email && !principalId) return null
  return {
    principalId,
    email,
    displayName: stringValue(raw.displayName ?? raw.DisplayName),
    role,
  }
}

function normalizeStatus(status: unknown): AccessStatus {
  const raw = recordValue(status)
  return {
    loading: Boolean(raw.loading ?? raw.Loading),
    error: stringValue(raw.error ?? raw.Error),
    message: stringValue(raw.message ?? raw.Message),
  }
}

function isRole(role: Role | null): role is Role {
  return role !== null
}

function isBinding(binding: Binding | null): binding is Binding {
  return binding !== null
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function displayLabel(binding: Binding): string {
  return binding.email || 'Principal'
}

function principalInitial(binding: Binding): string {
  const label = displayLabel(binding).trim()
  return label ? label[0] : '?'
}

function roleLabel(role: string): string {
  return role.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function usersIcon() {
  return html`<span class="icon" aria-hidden="true">${lucideIcon(Users, { size: 16 })}</span>`
}

function xIcon() {
  return html`<span class="icon" aria-hidden="true">${lucideIcon(X, { size: 16 })}</span>`
}

function trashIcon() {
  return html`<span class="icon" aria-hidden="true">${lucideIcon(Trash2, { size: 16 })}</span>`
}

function searchIcon() {
  return html`<span class="icon" aria-hidden="true">${lucideIcon(Search, { size: 16 })}</span>`
}

function mailIcon() {
  return html`<span class="icon" aria-hidden="true">${lucideIcon(Mail, { size: 16 })}</span>`
}

customElements.define('ld-workspace-access-control', WorkspaceAccessControl)

declare global {
  interface HTMLElementTagNameMap {
    'ld-workspace-access-control': WorkspaceAccessControl
  }
}
