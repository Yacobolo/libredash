import { LitElement, css, html } from 'lit'
import { property, state } from 'lit/decorators.js'
import { SlidersHorizontal } from 'lucide'
import type { DashboardFilters, DashboardStatus, ReportFilterConfig } from '../../../generated/signals'
import { jsonAttribute } from '../../shared/json-attribute'
import { lucideIcon } from '../../shared/lucide-icons'

const emptyFilters: DashboardFilters = { controls: {}, selections: [] }

class LibreDashFilterDock extends LitElement {
  @property({ converter: jsonAttribute<ReportFilterConfig[]>([]) }) config: ReportFilterConfig[] = []
  @property({ converter: jsonAttribute<DashboardFilters>(emptyFilters) }) filters: DashboardFilters = emptyFilters
  @property({ converter: jsonAttribute<Record<string, unknown>>({}) }) options: Record<string, unknown> = {}
  @property({ type: Boolean, reflect: true }) loading: DashboardStatus['loading'] = false

  @state() private open = storedFilterDockOpen()

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 0;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    aside {
      display: grid;
      width: var(--ld-dashboard-filter-width);
      box-sizing: border-box;
      min-width: 0;
      min-height: 0;
      height: 100%;
      overflow: hidden;
      border-left: var(--ld-border-default);
      background: var(--ld-bg-panel-muted);
      transition:
        width var(--ld-duration-fast) var(--motion-easing-move),
        background-color var(--ld-duration-fast) var(--motion-easing-move);
    }

    aside[data-open] {
      grid-template-rows: minmax(0, 1fr);
      width: var(--ld-dashboard-filter-open-width);
      background: var(--ld-bg-app);
    }

    button {
      font: inherit;
    }

    .rail {
      display: flex;
      width: 100%;
      height: 100%;
      min-width: 0;
      min-height: 0;
      box-sizing: border-box;
      align-items: center;
      align-content: start;
      flex-direction: column;
      justify-items: center;
      justify-content: flex-start;
      gap: var(--base-size-8);
      border: 0;
      background: transparent;
      color: var(--ld-fg-muted);
      cursor: pointer;
      padding: var(--base-size-16) 0;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      text-transform: uppercase;
    }

    .rail:hover,
    .rail:focus-visible {
      color: var(--ld-fg-default);
      outline: 0;
    }

    aside[data-open] .rail {
      display: none;
    }

    .rail span {
      writing-mode: vertical-rl;
      line-height: var(--ld-line-height-none, 1);
    }

    .panel {
      display: none;
      min-width: 0;
      min-height: 0;
      overflow: auto;
      padding: var(--base-size-12);
    }

    aside[data-open] .panel {
      display: block;
    }

    aside[data-open] .rail span {
      writing-mode: horizontal-tb;
      transform: none;
    }

    @media (max-width: 640px) {
      aside,
      aside[data-open] {
        width: 100%;
        border-left: 0;
        border-top: var(--ld-border-default);
      }

      .rail span,
      aside[data-open] .rail span {
        writing-mode: horizontal-tb;
        transform: none;
      }
    }
  `

  render() {
    return html`
      <aside ?data-open=${this.open} aria-label="Report filters">
        <button class="rail" type="button" title="Toggle filters" aria-expanded=${String(this.open)} @click=${this.toggle}>
          ${lucideIcon(SlidersHorizontal)}
          <span>Filters</span>
        </button>
        <div class="panel">
          <ld-filter-panel
            .config=${this.config}
            .filters=${this.filters}
            .options=${this.options}
            .loading=${this.loading}
            @ld-filters-close=${this.close}
          ></ld-filter-panel>
        </div>
      </aside>
    `
  }

  private toggle = (): void => {
    this.open = !this.open
    storeFilterDockOpen(this.open)
  }

  private close = (): void => {
    this.open = false
    storeFilterDockOpen(false)
  }
}

const filterDockStorageKey = 'libredash:filters-open'

function storedFilterDockOpen(): boolean {
  try {
    return localStorage.getItem(filterDockStorageKey) === 'open'
  } catch {
    return false
  }
}

function storeFilterDockOpen(open: boolean): void {
  try {
    localStorage.setItem(filterDockStorageKey, open ? 'open' : 'closed')
  } catch {
    // The in-memory state is enough when storage is unavailable.
  }
}

if (!customElements.get('ld-filter-dock')) customElements.define('ld-filter-dock', LibreDashFilterDock)
