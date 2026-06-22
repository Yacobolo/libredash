import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'

class ChatComposer extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) disabled = false
  @property({ type: String }) placeholder = 'Ask about dashboards, metrics, or models...'
  @state() private draft = ''

  static styles = css`
    :host {
      display: block;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    form {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 8px;
      padding: 12px;
    }

    textarea {
      box-sizing: border-box;
      min-height: 44px;
      max-height: 160px;
      width: 100%;
      resize: vertical;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      color: var(--ld-fg-default);
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-normal);
      padding: 10px 11px;
      outline: 0;
    }

    textarea:focus {
      border-color: var(--ld-line-accent-muted);
      box-shadow: 0 0 0 2px var(--ld-bg-accent-muted);
    }

    button {
      min-height: 44px;
      align-self: end;
      border: 0;
      border-radius: var(--ld-radius-default);
      background: var(--ld-button-primary-bg, var(--ld-fg-accent));
      color: var(--ld-button-primary-fg, var(--ld-bg-panel));
      cursor: pointer;
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      padding: 0 14px;
    }

    button:disabled,
    textarea:disabled {
      cursor: not-allowed;
      opacity: var(--ld-opacity-disabled, 0.55);
    }

    @media (max-width: 560px) {
      form {
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
    return html`
      <form @submit=${this.submit}>
        <textarea
          .value=${this.draft}
          ?disabled=${this.disabled}
          placeholder=${this.placeholder}
          rows="2"
          @input=${this.input}
          @keydown=${this.keydown}
        ></textarea>
        <button type="submit" ?disabled=${this.disabled || this.draft.trim() === ''}>Send</button>
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
    if (this.disabled || input === '') return
    this.dispatchEvent(new CustomEvent('ld-chat-submit', {
      bubbles: true,
      composed: true,
      detail: { input },
    }))
  }
}

if (!customElements.get('ld-chat-composer')) customElements.define('ld-chat-composer', ChatComposer)
