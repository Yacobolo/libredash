import { LitElement, css, html, nothing } from 'lit'
import { ExternalLink } from 'lucide'
import type { CatalogPageSignal } from '../../generated/signals'
import { DatastarLit } from '../shared/datastar-lit'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'

class LeapViewCatalogPage extends DatastarLit(LitElement) {
  static styles = css`
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
    h2,
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

    .detail,
    .muted {
      margin-top: var(--base-size-4);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-body-sm);
      line-height: var(--lv-line-height-snug);
    }

    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 22rem));
      gap: var(--base-size-16);
      align-items: start;
      justify-content: start;
    }

    article {
      display: grid;
      min-height: 10rem;
      min-width: 0;
      grid-template-rows: minmax(0, 1fr) auto;
      overflow: hidden;
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-panel);
      padding: var(--base-size-16);
      box-shadow: var(--lv-shadow-resting-sm, none);
    }

    .eyebrow {
      margin-bottom: var(--base-size-4);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
      line-height: var(--lv-line-height-tight);
      text-transform: uppercase;
    }

    h2 {
      margin-top: var(--base-size-4);
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-body-md);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-snug);
    }

    .tags {
      display: flex;
      flex-wrap: wrap;
      gap: var(--base-size-8);
      margin-top: var(--base-size-16);
    }

    .tag {
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-full);
      background: var(--lv-bg-panel-muted);
      color: var(--lv-fg-muted);
      padding: 0 var(--base-size-8);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
      line-height: var(--lv-line-height-snug);
      text-transform: uppercase;
    }

    footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-12);
      margin-top: var(--base-size-16);
      border-top: var(--lv-border-muted);
      padding-top: var(--base-size-12);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
    }

    a {
      display: inline-grid;
      min-height: var(--lv-button-height-sm);
      grid-auto-flow: column;
      place-items: center;
      gap: var(--base-size-6);
      border: var(--borderWidth-default) solid var(--lv-button-accent-border-rest);
      border-radius: var(--lv-button-radius);
      background: var(--lv-button-accent-bg-rest);
      color: var(--lv-button-accent-fg-rest);
      padding: 0 var(--lv-button-padding-inline-sm);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      text-decoration: none;
    }
  `

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
        <div class="grid">
          ${page.dashboards.map((dashboard) => html`
            <article>
              <div>
                <p class="eyebrow">${dashboard.semanticModel || 'Dashboard'}</p>
                <h2>${dashboard.title}</h2>
                ${dashboard.description ? html`<p class="muted">${dashboard.description}</p>` : nothing}
                ${dashboard.tags?.length ? html`
                  <div class="tags">${dashboard.tags.map((tag) => html`<span class="tag">${tag}</span>`)}</div>
                ` : nothing}
              </div>
              <footer>
                <span>${dashboard.pageCount} ${dashboard.pageCount === 1 ? 'page' : 'pages'}</span>
                <a href=${dashboard.href}>${lucideIcon(ExternalLink)}<span>Open</span></a>
              </footer>
            </article>
          `)}
        </div>
      </section>
    `
  }
}

customElements.define('lv-catalog-page', LeapViewCatalogPage)
