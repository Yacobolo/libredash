import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'
import { ExternalLink } from 'lucide'
import type { CatalogPageSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'

class LibreDashCatalogPage extends LitElement {
  @property({ converter: jsonAttribute<CatalogPageSignal | null>(null) }) page: CatalogPageSignal | null = null

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 100svh;
      background: var(--ld-bg-app);
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
    }

    section {
      display: grid;
      width: min(100%, var(--ld-page-content-max-width, 72rem));
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
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-title-sm);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-compact);
    }

    .detail,
    .muted {
      margin-top: var(--base-size-4);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-snug);
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
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
      padding: var(--base-size-16);
      box-shadow: var(--ld-shadow-resting-sm, none);
    }

    .eyebrow {
      margin-bottom: var(--base-size-4);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
      text-transform: uppercase;
    }

    h2 {
      margin-top: var(--base-size-4);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-snug);
    }

    .tags {
      display: flex;
      flex-wrap: wrap;
      gap: var(--base-size-8);
      margin-top: var(--base-size-16);
    }

    .tag {
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-full);
      background: var(--ld-bg-panel-muted);
      color: var(--ld-fg-muted);
      padding: 0 var(--base-size-8);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-snug);
      text-transform: uppercase;
    }

    footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-12);
      margin-top: var(--base-size-16);
      border-top: var(--ld-border-muted);
      padding-top: var(--base-size-12);
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    a {
      display: inline-grid;
      min-height: var(--control-small-size, 28px);
      grid-auto-flow: column;
      place-items: center;
      gap: var(--base-size-6);
      border-radius: var(--ld-radius-default);
      background: var(--ld-accent, #0969da);
      color: var(--ld-accent-fg, #fff);
      padding: 0 var(--base-size-10);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      text-decoration: none;
    }
  `

  updated(): void {
    checkSignalContract('catalog page', this.page, { kind: 'required', dashboards: 'required' })
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    return html`
      <section aria-label="LibreDash dashboard catalog">
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
                <span>${dashboard.pageCount} pages</span>
                <a href=${dashboard.href}>${lucideIcon(ExternalLink)}<span>Open</span></a>
              </footer>
            </article>
          `)}
        </div>
      </section>
    `
  }
}

customElements.define('ld-catalog-page', LibreDashCatalogPage)
