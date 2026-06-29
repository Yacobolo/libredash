import { LitElement, css, html } from 'lit'
import { property } from 'lit/decorators.js'
import type { ChromeSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sidebar'

const emptyChrome: ChromeSignal = {
  sidebar: {
    workspaceTitle: '',
    active: '',
    dashboardId: '',
    dashboardTitle: '',
    pageTitle: '',
    modelId: '',
    modelTitle: '',
    compact: false,
    groups: [],
  },
}

class LibreDashAppShell extends LitElement {
  @property({ converter: jsonAttribute<ChromeSignal>(emptyChrome) }) chrome: ChromeSignal = emptyChrome

  static styles = css`
    :host {
      display: grid;
      min-height: 100svh;
      grid-template-columns: auto minmax(0, 1fr);
      background: var(--ld-bg-app);
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    ld-sidebar {
      border-right: var(--ld-border-default);
    }

    main {
      min-width: 0;
      min-height: 100svh;
    }

    ::slotted([slot='page']) {
      display: block;
      min-width: 0;
      min-height: 100svh;
    }

    @media (max-width: 640px) {
      :host {
        grid-template-columns: 1fr;
      }

      ld-sidebar {
        border-right: 0;
        border-bottom: var(--ld-border-default);
      }
    }
  `

  updated(): void {
    checkSignalContract('chrome', this.chrome, { sidebar: 'required' })
  }

  render() {
    return html`
      <ld-sidebar .config=${this.chrome.sidebar}></ld-sidebar>
      <main>
        <slot name="page"></slot>
      </main>
    `
  }
}

customElements.define('ld-app-shell', LibreDashAppShell)
