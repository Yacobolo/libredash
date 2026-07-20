import { LitElement, css, html } from 'lit'
import { ChevronRight, LayoutDashboard } from 'lucide'
import type { CatalogPageSignal } from '../../generated/signals'
import { catalogListStyles } from '../shared/catalog-list-styles'
import { DatastarLit } from '../shared/datastar-lit'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'

class LeapViewCatalogPage extends DatastarLit(LitElement) {
  static styles = [catalogListStyles, css`
    :host {
      display: block;
      min-width: 0;
      min-height: 100svh;
      background: var(--lv-bg-app);
      color: var(--lv-fg-default);
      font-family: var(--lv-font-family-ui, var(--fontStack-system));
    }

    section {
      display: grid;
      width: min(100%, var(--lv-page-content-max-width));
      min-width: 0;
      min-height: 100svh;
      align-content: start;
      gap: var(--base-size-16);
      box-sizing: border-box;
      margin-inline: auto;
      padding: var(--base-size-16);
    }

    header {
      min-width: 0;
    }

    h1,
    p {
      margin: 0;
    }

    h1 {
      overflow: hidden;
      color: var(--lv-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--lv-font-size-title-sm);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-compact);
    }

    .detail {
      margin-top: var(--base-size-4);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-body-sm);
      line-height: var(--lv-line-height-snug);
    }

    .dashboard-icon {
      border-color: var(--lv-asset-dashboard-border);
      background: var(--lv-asset-dashboard-bg);
      color: var(--lv-asset-dashboard-accent);
    }

    .empty {
      display: grid;
      min-height: 10rem;
      place-content: center;
      gap: var(--base-size-4);
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-panel);
      color: var(--lv-fg-muted);
      padding: var(--base-size-20);
      text-align: center;
    }

    .empty strong {
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-body-md);
    }
  `]

  updated(): void {
    const page = this.page
    if (!page) return
    checkSignalContract('catalog page', page, { kind: 'required', dashboards: 'required' })
  }

  get page(): CatalogPageSignal | null {
    return this.signal<CatalogPageSignal | null>('page', null)
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    return html`
      <section aria-label="LeapView dashboard catalog">
        <header>
          <h1>${page.title}</h1>
          <p class="detail">${page.description}</p>
        </header>
        ${page.dashboards.length ? html`<ul class="catalog-list dashboard-list" aria-label="Published dashboards">
          ${page.dashboards.map((dashboard) => html`
            <li>
              <a class="catalog-row dashboard-row" href=${dashboard.href}>
                <span class="catalog-icon dashboard-icon">${lucideIcon(LayoutDashboard)}</span>
                <span class="catalog-copy dashboard-copy">
                  <span class="catalog-title dashboard-title">${dashboard.title}</span>
                  <span class="catalog-description dashboard-description">${dashboard.description || dashboard.semanticModel || 'Dashboard'}</span>
                </span>
                <span class="catalog-trailing">
                  <span class="catalog-meta dashboard-pages">${dashboard.pageCount} ${dashboard.pageCount === 1 ? 'page' : 'pages'}</span>
                  <span class="catalog-chevron dashboard-chevron">${lucideIcon(ChevronRight)}</span>
                </span>
              </a>
            </li>
          `)}
        </ul>` : html`
          <div class="empty" role="status">
            <strong>No dashboards are available.</strong>
            <span>Deploy a project with a dashboard to see it here.</span>
          </div>
        `}
      </section>
    `
  }
}

customElements.define('lv-catalog-page', LeapViewCatalogPage)
