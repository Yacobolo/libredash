import { LitElement, css, html } from 'lit'
import { Monitor, Moon, Sun } from 'lucide'
import type { LoginPageSignal } from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'
import './topology-background'

type ThemeMode = 'system' | 'light' | 'dark'

const nextThemeMode: Record<ThemeMode, ThemeMode> = {
  system: 'light',
  light: 'dark',
  dark: 'system',
}

const themeLabels: Record<ThemeMode, string> = {
  system: 'System theme',
  light: 'Light theme',
  dark: 'Dark theme',
}

class LibreDashLoginPage extends DatastarLit(LitElement) {
  private themeMode: ThemeMode = currentThemeMode()
  private readonly handleThemeApplied = (event: Event) => {
    const detail = (event as CustomEvent<{ mode?: string }>).detail
    this.themeMode = normalizeThemeMode(detail?.mode)
    this.requestUpdate()
  }

  static styles = css`
    :host {
      position: relative;
      display: grid;
      width: 100%;
      height: 100svh;
      min-height: 100svh;
      place-items: center;
      place-content: center;
      overflow: hidden;
      background: var(--ld-bg-app);
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
      padding: var(--base-size-24);
      box-sizing: border-box;
    }

    ld-topology-background,
    .scrim {
      position: absolute;
      inset: 0;
    }

    ld-topology-background {
      display: block;
      background: var(--ld-bg-app);
    }

    .scrim {
      pointer-events: none;
      z-index: var(--zIndex-overlay, 20);
      background: var(--overlay-backdrop-bgColor);
    }

    .theme {
      position: absolute;
      top: var(--base-size-16);
      right: var(--base-size-16);
      z-index: var(--zIndex-modal, 30);
      display: inline-grid;
      width: var(--control-medium-size);
      height: var(--control-medium-size);
      place-items: center;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      cursor: pointer;
      box-shadow: var(--shadow-resting-small);
    }

    .theme:hover,
    .theme:focus-visible {
      background: var(--ld-bg-control-hover);
      color: var(--ld-fg-default);
      outline: 0;
    }

    .theme [hidden] {
      display: none;
    }

    .panel {
      position: relative;
      z-index: var(--zIndex-modal, 30);
      display: grid;
      width: min(100%, var(--ld-login-panel-width));
      justify-items: center;
      gap: var(--base-size-20);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--base-size-24);
      text-align: center;
      box-shadow: var(--shadow-resting-medium, var(--shadow-resting-small));
      box-sizing: border-box;
    }

    h1 {
      margin: 0;
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-title-md);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .provider {
      display: inline-grid;
      min-height: var(--control-xlarge-size);
      width: 100%;
      grid-template-columns: auto minmax(0, 1fr);
      align-items: center;
      gap: var(--base-size-12);
      border: var(--borderWidth-default) solid var(--ld-button-border-rest);
      border-radius: var(--ld-button-radius);
      background: var(--ld-button-bg-rest);
      color: var(--ld-button-fg-rest);
      cursor: pointer;
      padding: 0 var(--ld-button-padding-inline-spacious);
      font: inherit;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-medium);
      box-shadow: var(--ld-button-shadow-resting);
      text-decoration: none;
      box-sizing: border-box;
    }

    .provider:hover,
    .provider:focus-visible {
      border-color: var(--ld-button-border-hover);
      background: var(--ld-button-bg-hover);
      outline: var(--focus-outline, var(--ld-border-default));
      outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
      outline-offset: var(--focus-outline-offset, var(--base-size-2));
    }

    .provider-mark {
      display: grid;
      width: var(--base-size-20);
      height: var(--base-size-20);
      grid-template-columns: 1fr 1fr;
      grid-template-rows: 1fr 1fr;
      gap: 1px;
    }

    .provider-mark span:nth-child(1) { background: var(--ld-fg-danger); }
    .provider-mark span:nth-child(2) { background: var(--ld-fg-success); }
    .provider-mark span:nth-child(3) { background: var(--ld-accent); }
    .provider-mark span:nth-child(4) { background: var(--ld-fg-warning); }

    form {
      display: grid;
      width: 100%;
      gap: var(--base-size-12);
    }

    label {
      display: grid;
      gap: var(--base-size-6);
      text-align: left;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    input {
      width: 100%;
      min-height: var(--control-large-size);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      padding: 0 var(--base-size-12);
      font: inherit;
      font-size: var(--ld-font-size-body-md);
      box-sizing: border-box;
    }

    input:focus {
      outline: var(--focus-outline, var(--ld-border-default));
      outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
      outline-offset: var(--focus-outline-offset, var(--base-size-2));
    }

    .submit {
      display: inline-grid;
      min-height: var(--control-xlarge-size);
      width: 100%;
      place-items: center;
      border: var(--borderWidth-default) solid var(--ld-button-accent-border-rest);
      border-radius: var(--ld-button-radius);
      background: var(--ld-button-accent-bg-rest);
      color: var(--ld-button-accent-fg-rest);
      cursor: pointer;
      padding: 0 var(--ld-button-padding-inline-spacious);
      font: inherit;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-medium);
      box-shadow: var(--ld-button-shadow-resting);
    }

    .divider {
      width: 100%;
      border-top: var(--ld-border-muted);
    }

    @media (max-width: 520px) {
      :host {
        padding: var(--base-size-16);
      }

      .theme {
        top: var(--base-size-12);
        right: var(--base-size-12);
      }
    }
  `

  connectedCallback(): void {
    super.connectedCallback()
    this.themeMode = currentThemeMode()
    document.addEventListener('libredash-theme-applied', this.handleThemeApplied)
  }

  disconnectedCallback(): void {
    document.removeEventListener('libredash-theme-applied', this.handleThemeApplied)
    super.disconnectedCallback()
  }

  updated(): void {
    checkSignalContract('login page', this.page, { kind: 'required', title: 'required', providerLabel: 'required' })
  }

  get page(): LoginPageSignal | null {
    return this.signal<LoginPageSignal | null>('page', null)
  }

  render() {
    const page = this.page
    const nextMode = nextThemeMode[this.themeMode]
    const themeLabel = `${themeLabels[this.themeMode]}. Switch to ${themeLabels[nextMode]}.`
    const localAuth = page?.localAuth ?? false
    const ssoAuth = page?.ssoAuth ?? true
    const mustChangePassword = page?.mustChangePassword ?? false
    return html`
      <ld-topology-background
        data-login-background
        data-module-src=${page?.backgroundModuleSrc ?? '/static/topology-background.js'}
      ></ld-topology-background>
      <div class="scrim" aria-hidden="true"></div>
      <button
        class="theme"
        type="button"
        data-theme-toggle
        data-theme-mode=${this.themeMode}
        aria-label=${themeLabel}
        title=${themeLabel}
        @click=${this.toggleTheme}
      >
        <span data-theme-icon="system" ?hidden=${this.themeMode !== 'system'}>${lucideIcon(Monitor)}</span>
        <span data-theme-icon="light" ?hidden=${this.themeMode !== 'light'}>${lucideIcon(Sun)}</span>
        <span data-theme-icon="dark" ?hidden=${this.themeMode !== 'dark'}>${lucideIcon(Moon)}</span>
      </button>
      <section class="panel" aria-label="LibreDash login">
        <h1>${page?.title ?? 'LibreDash'}</h1>
        ${mustChangePassword ? html`
          <form method="post" action="/auth/local/password">
            <input type="hidden" name="gorilla.csrf.Token" value=${csrfToken()}>
            <label>
              Temporary password
              <input name="currentPassword" type="password" autocomplete="current-password" required>
            </label>
            <label>
              New password
              <input name="newPassword" type="password" autocomplete="new-password" required>
            </label>
            <button class="submit" type="submit">Change password</button>
          </form>
        ` : localAuth ? html`
          <form method="post" action="/auth/local/login">
            <input type="hidden" name="gorilla.csrf.Token" value=${csrfToken()}>
            <label>
              Email
              <input name="email" type="email" autocomplete="username" required>
            </label>
            <label>
              Password
              <input name="password" type="password" autocomplete="current-password" required>
            </label>
            <button class="submit" type="submit">Sign in</button>
          </form>
        ` : ''}
        ${!mustChangePassword && localAuth && ssoAuth ? html`<div class="divider" aria-hidden="true"></div>` : ''}
        ${!mustChangePassword && ssoAuth ? html`
          <a class="provider" href="/auth/azureadv2">
            <span class="provider-mark" aria-hidden="true"><span></span><span></span><span></span><span></span></span>
            <span>${page?.providerLabel ?? 'Sign in with Azure Active Directory'}</span>
          </a>
        ` : ''}
      </section>
    `
  }

  private toggleTheme(): void {
    const mode = nextThemeMode[this.themeMode]
    this.themeMode = mode
    this.requestUpdate()
    document.dispatchEvent(new CustomEvent('libredash-theme-change', { detail: { mode } }))
  }
}

if (!customElements.get('ld-login-page')) customElements.define('ld-login-page', LibreDashLoginPage)

function currentThemeMode(): ThemeMode {
  try {
    return normalizeThemeMode(localStorage.getItem('libredash-color-mode'))
  } catch {
    const colorMode = document.documentElement.dataset.colorMode
    return colorMode === 'light' || colorMode === 'dark' ? colorMode : 'system'
  }
}

function csrfToken(): string {
  return document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content.trim() ?? ''
}

function normalizeThemeMode(mode: string | null | undefined): ThemeMode {
  return mode === 'light' || mode === 'dark' || mode === 'system' ? mode : 'system'
}
