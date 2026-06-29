import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import { Monitor, Moon, Sun } from 'lucide'
import type { LoginPageSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'

class LibreDashLoginPage extends LitElement {
  @property({ converter: jsonAttribute<LoginPageSignal | null>(null) }) page: LoginPageSignal | null = null

  static styles = css`
    :host {
      position: relative;
      display: grid;
      min-height: 100svh;
      place-items: center;
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
      background: color-mix(in srgb, var(--ld-bg-app) 80%, transparent);
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

    .theme [data-theme-icon] {
      display: none;
    }

    .panel {
      position: relative;
      z-index: var(--zIndex-modal, 30);
      display: grid;
      width: min(100%, var(--ld-login-panel-width, 24rem));
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
      font-size: var(--ld-font-size-title-md, 1.25rem);
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
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0 var(--base-size-16);
      font: inherit;
      font-size: var(--ld-font-size-body-md, 1rem);
      font-weight: var(--ld-font-weight-medium);
      box-shadow: var(--shadow-resting-small);
    }

    .provider:hover,
    .provider:focus-visible {
      border-color: var(--ld-accent, #0969da);
      background: var(--ld-bg-control-hover);
      outline: 0;
    }

    .provider-mark {
      display: grid;
      width: var(--base-size-20);
      height: var(--base-size-20);
      grid-template-columns: 1fr 1fr;
      grid-template-rows: 1fr 1fr;
      gap: 1px;
    }

    .provider-mark span:nth-child(1) { background: var(--ld-danger, #cf222e); }
    .provider-mark span:nth-child(2) { background: var(--ld-success, #1a7f37); }
    .provider-mark span:nth-child(3) { background: var(--ld-accent, #0969da); }
    .provider-mark span:nth-child(4) { background: var(--ld-warning, #bf8700); }

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

  updated(): void {
    checkSignalContract('login page', this.page, { kind: 'required', title: 'required', providerLabel: 'required' })
  }

  render() {
    const page = this.page
    return html`
      <ld-topology-background
        data-login-background
        data-module-src=${page?.backgroundModuleSrc ?? '/static/topology-background.js?v=dev'}
      ></ld-topology-background>
      <div class="scrim" aria-hidden="true"></div>
      <button class="theme" type="button" data-theme-toggle aria-label="Toggle theme" title="Toggle theme">
        <span data-theme-icon="system">${lucideIcon(Monitor)}</span>
        <span data-theme-icon="light">${lucideIcon(Sun)}</span>
        <span data-theme-icon="dark">${lucideIcon(Moon)}</span>
      </button>
      <section class="panel" aria-label="LibreDash login">
        <h1>${page?.title ?? 'LibreDash'}</h1>
        <button class="provider" type="button">
          <span class="provider-mark" aria-hidden="true"><span></span><span></span><span></span><span></span></span>
          <span>${page?.providerLabel ?? 'Sign in with Azure Active Directory'}</span>
        </button>
      </section>
    `
  }
}

if (!customElements.get('ld-login-page')) customElements.define('ld-login-page', LibreDashLoginPage)
