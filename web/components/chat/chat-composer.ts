import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { Send } from 'lucide'
import { lucideIcon } from '../shared/lucide-icons'
import '../shared/loading-spinner'

class ChatComposer extends LitElement {
  @property({ type: String }) value = ''
  @property({ type: Boolean, reflect: true }) disabled = false
  @property({ type: Boolean, reflect: true }) pending = false
  @property({ type: String }) placeholder = 'Ask about dashboards, metrics, or models...'
  @state() private draft = ''

  static styles = css`
    :host {
      display: block;
      color: var(--lv-fg-default);
      font-family: var(--fontStack-system);
    }

    form {
      width: min(calc(100% - var(--lv-space-lg) - var(--lv-space-lg)), var(--lv-chat-stack-width));
      margin-inline: auto;
      padding: var(--lv-space-lg) var(--lv-space-lg) var(--lv-space-md);
    }

    .composer-surface {
      display: grid;
      gap: var(--lv-space-md);
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-large);
      background: var(--lv-bg-panel);
      padding: var(--lv-space-md);
      box-shadow: var(--lv-shadow-floating-sm);
      transition:
        background var(--lv-transition-fast),
        border-color var(--lv-transition-fast),
        box-shadow var(--lv-transition-fast);
    }

    .composer-surface:hover:not(.is-disabled) {
      border-color: var(--lv-line-muted);
      box-shadow: var(--lv-shadow-floating-sm);
    }

    .composer-surface:focus-within {
      border-color: var(--lv-line-accent-muted);
      box-shadow:
        var(--lv-shadow-floating-sm),
        0 0 0 var(--lv-border-width-focus) var(--lv-bg-accent-muted);
    }

    .composer-surface.is-disabled {
      background: var(--lv-bg-control);
      color: var(--lv-fg-muted);
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
      border-radius: calc(var(--lv-radius-default) - var(--lv-space-2xs));
      background: transparent;
      color: var(--lv-fg-default);
      font: inherit;
      font-size: var(--lv-font-size-body-sm);
      line-height: var(--lv-line-height-normal);
      padding: var(--lv-space-sm) var(--lv-space-md);
      outline: 0;
    }

    textarea:focus {
      outline: 0;
    }

    textarea::placeholder {
      color: var(--lv-fg-muted);
    }

    .actions {
      display: flex;
      min-height: var(--lv-control-medium);
      align-items: center;
      justify-content: flex-end;
    }

    button {
      display: inline-flex;
      width: var(--lv-button-height, var(--lv-control-medium));
      height: var(--lv-button-height, var(--lv-control-medium));
      min-width: var(--lv-button-height, var(--lv-control-medium));
      align-items: center;
      justify-content: center;
      border: var(--borderWidth-default, var(--lv-border-width)) solid var(--lv-button-accent-border-rest, var(--lv-accent));
      border-radius: var(--lv-button-radius, var(--lv-radius-default));
      background: var(--lv-button-accent-bg-rest, var(--lv-accent));
      color: var(--lv-button-accent-fg-rest, var(--lv-accent-fg));
      cursor: pointer;
      font: inherit;
      font-size: var(--lv-font-size-body-sm);
      font-weight: var(--lv-font-weight-strong);
      padding: 0;
      box-shadow: var(--lv-button-shadow-resting, var(--shadow-resting-small));
      transition:
        background var(--duration-fast) var(--ease-lv),
        border-color var(--duration-fast) var(--ease-lv),
        color var(--duration-fast) var(--ease-lv),
        transform var(--duration-fast) var(--ease-lv);
    }

    button svg {
      width: var(--lv-button-icon-size, var(--base-size-16));
      height: var(--lv-button-icon-size, var(--base-size-16));
    }

    button:hover:not(:disabled) {
      border-color: var(--lv-button-accent-border-hover, var(--lv-accent));
      background: var(--lv-button-accent-bg-hover, var(--lv-accent));
      transform: translateY(-1px);
    }

    button:focus-visible {
      outline: var(--focus-outline, var(--lv-border-default));
      outline-color: var(--borderColor-accent-emphasis, var(--lv-line-accent));
      outline-offset: var(--focus-outline-offset, var(--lv-space-xs));
    }

    button:disabled {
      border-color: var(--lv-button-accent-border-disabled, var(--lv-line-default));
      background: var(--lv-button-accent-bg-disabled, var(--lv-bg-control));
      color: var(--lv-button-accent-fg-disabled, var(--lv-fg-muted));
      cursor: not-allowed;
      opacity: 1;
      box-shadow: none;
    }

    textarea:disabled {
      cursor: not-allowed;
      color: var(--lv-fg-muted);
      opacity: 1;
    }
    @media (max-width: 560px) {
      form {
        width: min(calc(100% - var(--lv-space-md) - var(--lv-space-md)), var(--lv-chat-stack-width));
        padding-inline: var(--lv-space-md);
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
              ${this.pending ? html`<lv-loading-spinner aria-hidden="true"></lv-loading-spinner>` : lucideIcon(Send)}
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
    this.dispatchEvent(new CustomEvent('lv-chat-submit', {
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

if (!customElements.get('lv-chat-composer')) customElements.define('lv-chat-composer', ChatComposer)
