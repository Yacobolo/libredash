import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { Send } from 'lucide'
import { lucideIcon } from '../shared/lucide-icons'

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
      width: min(calc(100% - var(--ld-space-lg) - var(--ld-space-lg)), var(--ld-chat-stack-width));
      margin-inline: auto;
      padding: var(--ld-space-lg) var(--ld-space-lg) var(--ld-space-md);
    }

    .composer-surface {
      display: grid;
      gap: var(--ld-space-md);
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-large);
      background: var(--ld-bg-panel);
      padding: var(--ld-space-md);
      box-shadow: var(--ld-shadow-floating-sm);
      transition:
        background var(--ld-transition-fast),
        border-color var(--ld-transition-fast),
        box-shadow var(--ld-transition-fast);
    }

    .composer-surface:hover:not(.is-disabled) {
      border-color: var(--ld-line-muted);
      box-shadow: var(--ld-shadow-floating-sm);
    }

    .composer-surface:focus-within {
      border-color: var(--ld-line-accent-muted);
      box-shadow:
        var(--ld-shadow-floating-sm),
        0 0 0 var(--ld-border-width-focus) var(--ld-bg-accent-muted);
    }

    .composer-surface.is-disabled {
      background: var(--ld-bg-control);
      color: var(--ld-fg-muted);
      box-shadow: none;
    }

    textarea {
      box-sizing: border-box;
      min-height: 36px;
      max-height: 160px;
      width: 100%;
      resize: none;
      overflow-y: auto;
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

    textarea:focus {
      outline: 0;
    }

    textarea::placeholder {
      color: var(--ld-fg-muted);
    }

    .actions {
      display: flex;
      min-height: var(--ld-control-medium);
      align-items: center;
      justify-content: flex-end;
    }

    button {
      display: inline-flex;
      width: var(--ld-button-height, var(--ld-control-medium));
      height: var(--ld-button-height, var(--ld-control-medium));
      min-width: var(--ld-button-height, var(--ld-control-medium));
      align-items: center;
      justify-content: center;
      border: var(--borderWidth-default, var(--ld-border-width)) solid var(--ld-button-accent-border-rest, var(--ld-accent));
      border-radius: var(--ld-button-radius, var(--ld-radius-default));
      background: var(--ld-button-accent-bg-rest, var(--ld-accent));
      color: var(--ld-button-accent-fg-rest, var(--ld-accent-fg));
      cursor: pointer;
      font: inherit;
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
      padding: 0;
      box-shadow: var(--ld-button-shadow-resting, var(--shadow-resting-small));
      transition:
        background var(--duration-fast) var(--ease-ld),
        border-color var(--duration-fast) var(--ease-ld),
        color var(--duration-fast) var(--ease-ld),
        transform var(--duration-fast) var(--ease-ld);
    }

    button svg {
      width: var(--ld-button-icon-size, var(--base-size-16));
      height: var(--ld-button-icon-size, var(--base-size-16));
    }

    button:hover:not(:disabled) {
      border-color: var(--ld-button-accent-border-hover, var(--ld-accent));
      background: var(--ld-button-accent-bg-hover, var(--ld-accent));
      transform: translateY(-1px);
    }

    button:focus-visible {
      outline: var(--focus-outline, var(--ld-border-default));
      outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
      outline-offset: var(--focus-outline-offset, var(--ld-space-xs));
    }

    .spinner {
      width: 14px;
      height: 14px;
      border: var(--borderWidth-thick) solid transparent;
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
      border-color: var(--ld-button-accent-border-disabled, var(--ld-line-default));
      background: var(--ld-button-accent-bg-disabled, var(--ld-bg-control));
      color: var(--ld-button-accent-fg-disabled, var(--ld-fg-muted));
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
      form {
        width: min(calc(100% - var(--ld-space-md) - var(--ld-space-md)), var(--ld-chat-stack-width));
        padding-inline: var(--ld-space-md);
      }
    }
  `

  updated(changed: Map<string, unknown>) {
    if (changed.has('value')) {
      this.draft = this.value || ''
      void this.updateComplete.then(() => this.resizeTextarea())
    }
  }

  connectedCallback() {
    super.connectedCallback()
    this.draft = this.value || ''
  }

  protected firstUpdated() {
    this.resizeTextarea()
  }

  render() {
    const blocked = this.disabled || this.pending
    return html`
      <form @submit=${this.submit}>
        <div class=${['composer-surface', blocked ? 'is-disabled' : ''].filter(Boolean).join(' ')}>
          <textarea
            .value=${this.draft}
            ?disabled=${this.disabled}
            placeholder=${this.placeholder}
            rows="1"
            @input=${this.input}
            @keydown=${this.keydown}
          ></textarea>
          <div class="actions">
            <button
              type="submit"
              aria-label=${this.pending ? 'Sending' : 'Send'}
              title="Send"
              ?disabled=${this.disabled || this.pending || this.draft.trim() === ''}
            >
              ${this.pending ? html`<span class="spinner" aria-hidden="true"></span>` : lucideIcon(Send)}
            </button>
          </div>
        </div>
      </form>
    `
  }

  private input(event: Event) {
    const textarea = event.target as HTMLTextAreaElement
    this.draft = textarea.value
    this.resizeTextarea(textarea)
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

  private resizeTextarea(textarea = this.shadowRoot?.querySelector('textarea') as HTMLTextAreaElement | null) {
    if (!textarea) return
    const maxHeight = 160
    textarea.style.height = 'auto'
    const height = Math.min(textarea.scrollHeight, maxHeight)
    textarea.style.height = `${height}px`
    textarea.style.overflowY = textarea.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }
}

if (!customElements.get('ld-chat-composer')) customElements.define('ld-chat-composer', ChatComposer)
