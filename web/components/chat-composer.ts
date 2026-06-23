import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'

class ChatComposer extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) disabled = false
  @property({ type: Boolean, reflect: true }) pending = false
  @property({ type: String }) placeholder = 'Ask about dashboards, metrics, or models...'
  @state() private draft = ''

  static styles = css`
    :host {
      display: block;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    form {
      width: min(100%, var(--ld-chat-stack-width));
      margin-inline: auto;
      padding: var(--ld-space-md) var(--ld-space-lg);
    }

    .composer {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: end;
      gap: var(--ld-space-sm);
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--ld-space-sm);
      transition:
        border-color var(--ld-transition-fast),
        box-shadow var(--ld-transition-fast);
    }

    .composer:focus-within {
      border-color: var(--ld-line-accent-muted);
      box-shadow: 0 0 0 2px var(--ld-bg-accent-muted);
    }

    .composer.is-disabled {
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
    }

    textarea {
      box-sizing: border-box;
      min-height: 40px;
      max-height: 160px;
      width: 100%;
      resize: vertical;
      border: 0;
      border-radius: calc(var(--ld-radius-default) - var(--ld-space-2xs));
      background: transparent;
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-normal);
      padding: var(--ld-space-sm) var(--ld-space-md);
      outline: 0;
    }

    textarea::placeholder {
      color: var(--ld-fg-muted);
    }

    button {
      display: inline-flex;
      min-height: 40px;
      min-width: 68px;
      align-items: center;
      justify-content: center;
      gap: 8px;
      align-self: end;
      border: 1px solid var(--ld-accent);
      border-radius: var(--ld-radius-default);
      background: var(--ld-accent);
      color: var(--ld-accent-fg);
      cursor: pointer;
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      padding: 0 var(--ld-space-lg);
      box-shadow: var(--shadow-resting-small);
      transition:
        background var(--duration-fast) var(--ease-ld),
        border-color var(--duration-fast) var(--ease-ld),
        color var(--duration-fast) var(--ease-ld);
    }

    button:hover:not(:disabled) {
      background: color-mix(in srgb, var(--ld-accent), var(--ld-bg-panel) 10%);
    }

    button:focus-visible {
      outline: var(--ld-border-width-focus) solid var(--ld-line-accent);
      outline-offset: var(--ld-space-xs);
    }

    .spinner {
      width: 14px;
      height: 14px;
      border: 2px solid color-mix(in srgb, currentColor 28%, transparent);
      border-top-color: currentColor;
      border-radius: 999px;
      animation: spin 0.8s linear infinite;
    }

    @keyframes spin {
      to {
        transform: rotate(360deg);
      }
    }

    button:disabled {
      border-color: var(--ld-line-default);
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      cursor: not-allowed;
      opacity: 1;
      box-shadow: none;
    }

    textarea:disabled {
      cursor: not-allowed;
      color: var(--ld-fg-muted);
      opacity: 1;
    }

    @media (max-width: 560px) {
      .composer {
        grid-template-columns: 1fr;
      }
      button {
        width: 100%;
      }
    }
  `

  updated(changed: Map<string, unknown>) {
    if (changed.has('value')) {
      this.draft = this.value || ''
    }
  }

  connectedCallback() {
    super.connectedCallback()
    this.draft = this.value || ''
  }

  render() {
    const blocked = this.disabled || this.pending
    return html`
      <form @submit=${this.submit}>
        <div class=${['composer', blocked ? 'is-disabled' : ''].filter(Boolean).join(' ')}>
          <textarea
            .value=${this.draft}
            ?disabled=${this.disabled}
            placeholder=${this.placeholder}
            rows="2"
            @input=${this.input}
            @keydown=${this.keydown}
          ></textarea>
          <button type="submit" ?disabled=${this.disabled || this.pending || this.draft.trim() === ''}>
            ${this.pending ? html`<span class="spinner" aria-hidden="true"></span><span>Sending</span>` : 'Send'}
          </button>
        </div>
      </form>
    `
  }

  private input(event: Event) {
    this.draft = (event.target as HTMLTextAreaElement).value
  }

  private keydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.shiftKey) return
    event.preventDefault()
    this.dispatchSubmit()
  }

  private submit(event: Event) {
    event.preventDefault()
    this.dispatchSubmit()
  }

  private dispatchSubmit() {
    const input = this.draft.trim()
    if (this.disabled || this.pending || input === '') return
    this.dispatchEvent(new CustomEvent('ld-chat-submit', {
      bubbles: true,
      composed: true,
      detail: { input },
    }))
  }
}

if (!customElements.get('ld-chat-composer')) customElements.define('ld-chat-composer', ChatComposer)
