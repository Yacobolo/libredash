import { LitElement, css, html, nothing } from 'lit'
import { property } from 'lit/decorators.js'
import type { AdminPageSignal, AdminContentSectionSignal, AdminStorageSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import '../navigation/sub-sidebar'
import '../shared/data-grid'
import './storage-explorer'

const emptyStorage: AdminStorageSignal = {
  summary: { duckdbDir: '', databaseCount: 0, totalSizeLabel: '', tableCount: 0 },
  status: '',
  warnings: [],
  tables: [],
  selectedKey: '',
  selectedTable: null,
}

class LibreDashAdminPage extends LitElement {
  @property({ converter: jsonAttribute<AdminPageSignal | null>(null) }) page: AdminPageSignal | null = null
  @property({ converter: jsonAttribute<AdminStorageSignal>(emptyStorage) }) storage: AdminStorageSignal = emptyStorage

  static styles = css`
    :host {
      display: block;
      min-width: 0;
      min-height: 100svh;
      color: var(--ld-fg-default);
      font-family: var(--ld-font-family-ui, var(--fontStack-system));
      background: var(--ld-bg-app);
    }

    .route {
      display: grid;
      min-height: 100svh;
      grid-template-columns: auto minmax(0, 1fr);
      background: var(--ld-bg-app);
    }

    .main {
      display: grid;
      min-width: 0;
      align-content: start;
      gap: var(--base-size-12);
      padding: var(--base-size-16);
    }

    header {
      display: grid;
      min-width: 0;
      gap: var(--base-size-4);
    }

    h1,
    h2,
    p {
      margin: 0;
    }

    .eyebrow {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      line-height: var(--ld-line-height-tight);
      text-transform: uppercase;
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

    .detail {
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-sm);
      line-height: var(--ld-line-height-compact);
    }

    .metrics {
      display: grid;
      max-width: var(--ld-workspace-detail-max-width, 72rem);
      grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
      gap: var(--base-size-12);
    }

    .metric,
    .panel {
      min-width: 0;
      overflow: hidden;
      border: var(--ld-border-muted);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-panel);
    }

    .metric {
      display: grid;
      align-content: start;
      gap: var(--base-size-4);
      padding: var(--base-size-16);
    }

    .metric .label {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      text-transform: uppercase;
    }

    .metric .value {
      overflow: hidden;
      color: var(--ld-fg-default);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-title-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .metric .meta,
    .empty {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .empty {
      padding: var(--base-size-12);
    }

    .warnings {
      display: grid;
      max-width: var(--ld-workspace-detail-max-width, 72rem);
      gap: var(--base-size-8);
    }

    .warning {
      border: var(--ld-border-attention, var(--ld-border-muted));
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-attention-muted, var(--ld-bg-panel-muted));
      padding: var(--base-size-10) var(--base-size-12);
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-medium);
    }

    ld-storage-explorer {
      max-width: min(100%, 88rem);
    }

    .section {
      display: grid;
      min-width: 0;
      align-content: start;
      gap: var(--base-size-12);
    }

    h2 {
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-body-sm);
      font-weight: var(--ld-font-weight-strong);
    }

    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
      gap: var(--base-size-12);
    }

    @media (max-width: 640px) {
      .route {
        grid-template-columns: 1fr;
      }

      .main {
        padding: var(--base-size-12);
      }
    }
  `

  updated(): void {
    checkSignalContract('admin page', this.page, { kind: 'required', title: 'required', sidebar: 'required' })
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    return html`
      <div class="route">
        <ld-sub-sidebar .config=${page.sidebar}></ld-sub-sidebar>
        <section class="main" aria-label="Admin">
          <header>
            <p class="eyebrow">Admin</p>
            <h1>${page.headerTitle || page.title}</h1>
            ${page.headerDetail ? html`<p class="detail">${page.headerDetail}</p>` : nothing}
          </header>
          ${page.empty && page.active !== 'storage' ? html`<div class="panel"><div class="empty">${page.empty}</div></div>` : nothing}
          ${page.metrics?.length ? html`
            <div class="metrics">
              ${page.metrics.map((metric) => html`
                <div class="metric">
                  <span class="label">${metric.label}</span>
                  <span class="value">${metric.value || '-'}</span>
                  ${metric.detail ? html`<span class="meta">${metric.detail}</span>` : nothing}
                </div>
              `)}
            </div>
          ` : nothing}
          ${page.active === 'storage' ? this.renderStorage(page) : page.sections?.map(renderSection)}
        </section>
      </div>
    `
  }

  private renderStorage(page: AdminPageSignal) {
    const storage = storageHasPayload(this.storage) ? this.storage : page.storage ?? emptyStorage
    return html`
      ${storage.warnings?.length ? html`
        <div class="warnings">
          ${storage.warnings.map((warning) => html`<p class="warning">${warning}</p>`)}
        </div>
      ` : nothing}
      <ld-storage-explorer .storage=${storage}></ld-storage-explorer>
    `
  }
}

function storageHasPayload(storage: AdminStorageSignal | null | undefined): storage is AdminStorageSignal {
  if (!storage) return false
  return Boolean(storage.tables?.length || storage.status || storage.selectedKey || storage.selectedTable || storage.warnings?.length)
}

function renderSection(section: AdminContentSectionSignal) {
  return html`
    <section class="section" aria-label=${section.title}>
      <h2>${section.title}</h2>
      ${section.grid?.columns?.length
        ? html`<div class="panel"><ld-data-grid .grid=${section.grid}></ld-data-grid></div>`
        : html`<div class="facts">${section.facts?.map((fact) => html`
          <div class="metric">
            <span class="label">${fact.label}</span>
            <span class="value">${fact.value || '-'}</span>
          </div>
        `)}</div>`}
    </section>
  `
}

if (!customElements.get('ld-admin-page')) customElements.define('ld-admin-page', LibreDashAdminPage)
