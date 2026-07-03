import { LitElement, css, html, nothing, type PropertyValues } from 'lit'
import { property, state } from 'lit/decorators.js'
import {
  ArrowLeft,
  BookOpen,
  Box,
  Boxes,
  Cable,
  ChartColumn,
  Component,
  ExternalLink,
  FileText,
  GalleryVerticalEnd,
  LayoutDashboard,
  ListFilter,
  PanelTop,
  Plug,
  RefreshCw,
  Ruler,
  Search,
  Sigma,
  SquareDashedMousePointer,
  Table2,
  TableProperties,
  Workflow,
  type IconNode,
} from 'lucide'
import type {
  ConnectionsPageSignal,
  DefinitionFactSignal,
  RecordTableSignal,
  WorkspaceAccessSignal,
  WorkspaceAssetPageSignal,
  WorkspaceAssetSummarySignal,
  WorkspaceDetailSectionSignal,
  WorkspacePageSignal,
  WorkspaceTabSignal,
} from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { checkSignalContract } from '../shared/signal-contract'
import { lucideIcon } from '../shared/lucide-icons'
import '../shared/record-table'
import '../shared/code-block'
import '../shared/workspace-access-control'

const emptyWorkspaceAccess: WorkspaceAccessSignal = {
  workspace: {},
  roles: [],
  bindings: [],
  canManage: false,
  status: { loading: false, error: '', message: '' },
  csrfToken: '',
  command: { email: '', role: '', principalId: '' },
  search: '',
}

class LibreDashWorkspacePage extends LitElement {
  @property({ converter: jsonAttribute<WorkspacePageSignal | null>(null) }) page: WorkspacePageSignal | null = null
  @property({ attribute: 'workspaceaccess', converter: jsonAttribute<WorkspaceAccessSignal>(emptyWorkspaceAccess) }) workspaceAccess: WorkspaceAccessSignal = emptyWorkspaceAccess
  @state() private assetQuery: string | null = null

  static get styles() {
    return workspaceStyles
  }

  updated(changedProperties: PropertyValues<this>): void {
    if (changedProperties.has('page')) this.assetQuery = null
    checkSignalContract('workspace page', this.page, { kind: 'required', title: 'required' })
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    if (page.cards?.length) return this.renderCatalog(page)
    if (!page.assetList?.searchHref && this.workspaceAccess?.canManage) return this.renderAccessPage(page)
    return this.renderAssetList(page, 'Workspace', 'Workspace assets')
  }

  private renderCatalog(page: WorkspacePageSignal) {
    return html`
      <section class="page catalog" aria-label="LibreDash workspaces">
        ${this.renderHeader('', page.title, page.description)}
        <div class="cards">
          ${page.cards?.map((card) => html`
            <article class="card">
              <div>
                <p class="eyebrow">Workspace</p>
                <h2>${card.title}</h2>
                <p class="muted">${card.description}</p>
              </div>
              <footer>
                ${card.deploymentLabel ? html`<span>${card.deploymentLabel}</span>` : html`<span></span>`}
                <a class="primary-link" href=${card.href}>${lucideIcon(ExternalLink)}<span>Open</span></a>
              </footer>
            </article>
          `)}
        </div>
      </section>
    `
  }

  private renderAssetList(page: WorkspacePageSignal, eyebrow: string, label: string) {
    const assetList = page.assetList
    const query = this.assetQuery ?? assetList?.query ?? ''
    const assets = filterAssetSummaries(assetList?.assets ?? [], query)
    return html`
      <section class="page" aria-label=${label}>
        ${this.renderHeader(eyebrow, page.title, page.description, this.renderAccessControl())}
        ${renderAssetToolbar(query, assetList?.activeType ?? '', assetList?.tabs ?? [], 'Search workspace assets...', (event: Event) => this.assetQuery = (event.currentTarget as HTMLInputElement).value)}
        ${renderAssetTable(assets, query ? 'No assets match this search.' : assetList?.empty ?? 'No assets match this view.')}
      </section>
    `
  }

  private renderAccessPage(page: WorkspacePageSignal) {
    return html`
      <section class="page" aria-label="Workspace permissions">
        ${this.renderHeader('Workspace', page.title, page.description, this.renderAccessControl())}
      </section>
    `
  }

  private renderAccessControl() {
    if (!this.workspaceAccess?.canManage) return nothing
    return html`
      <ld-workspace-access-control
        .access=${this.workspaceAccess}
        search=${this.workspaceAccess.search ?? ''}
      ></ld-workspace-access-control>
    `
  }

  private renderHeader(eyebrow: string, title: string, detail = '', actions = nothing) {
    return html`
      <header class="header">
        <div class="title-block">
          ${eyebrow ? html`<p class="eyebrow">${eyebrow}</p>` : nothing}
          <h1>${title}</h1>
          ${detail ? html`<p class="detail">${detail}</p>` : nothing}
        </div>
        <div class="actions">${actions}</div>
      </header>
    `
  }
}

class LibreDashConnectionsPage extends LitElement {
  @property({ converter: jsonAttribute<ConnectionsPageSignal | null>(null) }) page: ConnectionsPageSignal | null = null
  @state() private assetQuery: string | null = null

  static get styles() {
    return workspaceStyles
  }

  updated(changedProperties: PropertyValues<this>): void {
    if (changedProperties.has('page')) this.assetQuery = null
    checkSignalContract('connections page', this.page, { kind: 'required', title: 'required', assetList: 'required' })
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    const assetList = page.assetList
    const query = this.assetQuery ?? assetList?.query ?? ''
    const assets = filterAssetSummaries(assetList?.assets ?? [], query)
    return html`
      <section class="page" aria-label="Connections and sources">
        <header class="header">
          <div class="title-block">
            <p class="eyebrow">Data access</p>
            <h1>${page.title}</h1>
            ${page.description ? html`<p class="detail">${page.description}</p>` : nothing}
          </div>
        </header>
        ${renderAssetToolbar(query, assetList?.activeType ?? '', assetList?.tabs ?? [], 'Search connections and sources...', (event: Event) => this.assetQuery = (event.currentTarget as HTMLInputElement).value)}
        ${renderAssetTable(assets, query ? 'No connection assets match this search.' : assetList?.empty ?? 'No connection assets match this view.')}
      </section>
    `
  }
}

class LibreDashWorkspaceAssetPage extends LitElement {
  @property({ converter: jsonAttribute<WorkspaceAssetPageSignal | null>(null) }) page: WorkspaceAssetPageSignal | null = null

  static get styles() {
    return workspaceStyles
  }

  updated(): void {
    checkSignalContract('workspace asset page', this.page, { title: 'required', breadcrumbs: 'required', tabs: 'required' })
  }

  render() {
    const page = this.page
    if (!page) return html`<slot></slot>`
    return html`
      <section class="asset-page" aria-label="Workspace asset detail">
        <header class="breadcrumb-header">
          <nav aria-label="Breadcrumb">
            <ol>
              ${page.breadcrumbs.map((crumb) => html`
                <li>
                  ${crumb.current
                    ? html`<h1>${assetTypeGlyph(page.asset.type, 'inline')}<span>${crumb.label}</span></h1>`
                    : html`<a href=${crumb.href}>${crumb.label}</a>`}
                </li>
              `)}
            </ol>
          </nav>
          <div class="actions">
            ${page.actions?.map((action) => this.renderAction(action, page))}
          </div>
        </header>
        <div class="asset-body">
          ${renderTabs(page.tabs)}
          <div class=${page.activeSection === 'lineage' ? 'section-body lineage-body' : page.activeSection === 'details' && page.details?.semanticModelGraph ? 'section-body graph-details-body' : 'section-body'}>
            ${page.activeSection === 'lineage'
              ? this.renderLineage(page)
              : page.activeSection === 'refreshes'
                ? this.renderRefreshes(page)
                : this.renderDetails(page)}
          </div>
        </div>
      </section>
    `
  }

  private renderAction(action: NonNullable<WorkspaceAssetPageSignal['actions']>[number], page: WorkspaceAssetPageSignal) {
    if (action.command === 'refresh-materializations') {
      return html`
        <button
          type="button"
          class="icon-link"
          title=${action.label}
          aria-label=${action.label}
          ?disabled=${Boolean(action.disabled || page.refresh?.running)}
          @click=${() => this.dispatchEvent(new CustomEvent('ld-refresh-materializations', { bubbles: true, composed: true }))}
        >
          ${lucideIcon(RefreshCw, { className: page.refresh?.running ? 'spin' : '' })}
        </button>
      `
    }
    const icon = action.icon === 'open' ? ExternalLink : ArrowLeft
    return html`
      <a class="icon-link" href=${action.href ?? '#'} title=${action.label} aria-label=${action.label}>
        ${lucideIcon(icon)}
      </a>
    `
  }

  private renderDetails(page: WorkspaceAssetPageSignal) {
    return html`
      <section class="details" id="details" aria-label="Asset details">
        ${page.details?.semanticModelGraph ? renderSemanticModelGraph(page.details.semanticModelGraph, page) : nothing}
        <div class="details-content">
          ${renderFacts('Overview', page.details?.overview ?? [], true)}
          ${(page.details?.sections ?? []).map(renderDetailSection)}
        </div>
      </section>
    `
  }

  private renderLineage(page: WorkspaceAssetPageSignal) {
    return html`
      <section class="lineage" id="lineage" aria-label="Asset lineage">
        <ld-asset-lineage-graph class="lineage-graph" .graph=${page.lineage?.graph ?? { nodes: [], edges: [] }}></ld-asset-lineage-graph>
        <div class="lineage-grids">
          ${renderRecordTableSection('Uses', page.lineage?.usesTable)}
          ${renderRecordTableSection('Used by', page.lineage?.usedByTable)}
        </div>
      </section>
    `
  }

  private renderRefreshes(page: WorkspaceAssetPageSignal) {
    return html`
      <section class="details" id="refreshes" aria-label="Refresh runs">
        ${renderRecordTableSection('Refreshes', page.refresh?.runsTable)}
      </section>
    `
  }
}

function renderAssetToolbar(query: string, activeType: string, tabs: WorkspaceTabSignal[], placeholder: string, onSearch: (event: Event) => void) {
  return html`
    <div class="toolbar">
      <form class="search" @submit=${preventSubmit}>
        <input
          type="search"
          name="q"
          .value=${query}
          placeholder=${placeholder}
          autocomplete="off"
          @input=${onSearch}
        />
        ${activeType ? html`<input type="hidden" name="type" value=${activeType} />` : nothing}
        <span class="search-icon" aria-hidden="true">${lucideIcon(Search)}</span>
      </form>
      ${renderTabs(tabs)}
    </div>
  `
}

function preventSubmit(event: Event) {
  event.preventDefault()
}

function filterAssetSummaries(assets: WorkspaceAssetSummarySignal[], query: string) {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return assets
  return assets.filter((asset) => [
    asset.title,
    asset.description,
    asset.typeLabel,
    asset.type,
    asset.key,
    asset.parentTitle,
  ].some((value) => String(value ?? '').toLowerCase().includes(normalized)))
}

function renderAssetTable(assets: WorkspaceAssetSummarySignal[], empty: string) {
  if (!assets.length) return html`<div class="panel"><div class="empty">${empty}</div></div>`
  const table: RecordTableSignal = {
    columns: [
      { id: 'name', header: 'Name', kind: 'entity', width: '42%' },
      { id: 'type', header: 'Type', width: '150px' },
      { id: 'key', header: 'Key', kind: 'code', width: '180px' },
      { id: 'actions', header: 'Actions', kind: 'actions', align: 'right', width: '104px', sortable: false } as any,
    ],
    rows: assets.map((asset) => ({
      name: {
        label: asset.title,
        description: asset.description,
        href: asset.detailHref,
        icon: asset.type,
      },
      type: asset.typeLabel,
      key: asset.key,
      actions: [
        { label: 'View details', href: asset.detailHref, icon: 'details' },
        { label: 'Open asset', href: asset.openHref, icon: 'open' },
      ],
    })),
    empty,
    minWidth: '840px',
  }
  return html`
    <div class="panel">
      <ld-record-table variant="primary" .table=${table}></ld-record-table>
    </div>
  `
}

function renderTabs(tabs: WorkspaceTabSignal[]) {
  if (!tabs.length) return nothing
  return html`
    <nav class="tabs" aria-label="Asset sections">
      ${tabs.map((tab) => html`
        <a class=${tab.active ? 'active' : ''} href=${tab.href} aria-current=${tab.active ? 'page' : nothing}>
          <span>${tab.label}</span>
          ${tab.count ? html`<span class="count">${tab.count}</span>` : nothing}
        </a>
      `)}
    </nav>
  `
}

function renderDetailSection(section: WorkspaceDetailSectionSignal) {
  if (section.code) {
    return html`
      <section class="detail-section" aria-label=${section.title}>
        <h2>${section.title}</h2>
        <ld-code-block language=${section.lang || 'text'} .code=${section.code}></ld-code-block>
      </section>
    `
  }
  if (section.table?.columns?.length) return renderRecordTableSection(section.title, section.table)
  return renderFacts(section.title, section.facts ?? [], false)
}

function renderSemanticModelGraph(graph: NonNullable<NonNullable<WorkspaceAssetPageSignal['details']>['semanticModelGraph']>, page: WorkspaceAssetPageSignal) {
  return html`
    <section class="semantic-model-section" aria-label="Data model graph">
      <ld-semantic-model-graph class="semantic-model-graph" .graph=${graph} storagekey=${`${page.workspaceId}:${page.assetId}`}></ld-semantic-model-graph>
    </section>
  `
}

function renderFacts(title: string, facts: DefinitionFactSignal[], overview: boolean) {
  const filtered = facts.filter((fact) => fact.value?.trim())
  return html`
    <section class="detail-section" aria-label=${title}>
      <h2>${title}</h2>
      ${filtered.length
        ? html`
          <div class=${overview ? 'facts overview' : 'facts'}>
            ${filtered.map((fact) => html`
              <div class=${fact.wide ? 'wide' : ''}>
                <span>${fact.label}</span>
                ${fact.code ? html`<code>${fact.value}</code>` : html`<p>${fact.value}</p>`}
              </div>
            `)}
          </div>
        `
        : html`<div class="empty">No details are available.</div>`}
    </section>
  `
}

function renderRecordTableSection(title: string, table?: RecordTableSignal) {
  return html`
    <section class="detail-section" aria-label=${title}>
      <h2>${title}</h2>
      <ld-record-table .table=${table ?? null}></ld-record-table>
    </section>
  `
}

function assetTypeGlyph(type: string, size: 'table' | 'inline' = 'table') {
  return html`
    <span class=${`asset-glyph asset-kind-${assetPresentationToken(type)} ${size === 'inline' ? 'inline' : ''}`} aria-hidden="true">
      ${lucideIcon(assetIconNode(type), { size: size === 'inline' ? 14 : 16, strokeWidth: 1.75 })}
    </span>
  `
}

function assetIconNode(type: string): IconNode {
  switch (type) {
    case 'catalog':
      return BookOpen
    case 'connection':
      return Plug
    case 'dashboard':
      return LayoutDashboard
    case 'field':
      return Ruler
    case 'filter':
      return ListFilter
    case 'measure':
      return Sigma
    case 'model_table':
    case 'semantic_table':
      return TableProperties
    case 'page':
      return PanelTop
    case 'page_item':
      return Component
    case 'relationship':
      return Workflow
    case 'semantic_model':
      return Box
    case 'source':
      return Cable
    case 'table':
      return Table2
    case 'visual':
      return ChartColumn
    case 'visual_element':
      return SquareDashedMousePointer
    case 'workspace':
      return Boxes
    case 'workspace_group':
      return GalleryVerticalEnd
    default:
      return Component
  }
}

function assetPresentationToken(type: string): string {
  switch (type) {
    case 'catalog':
    case 'workspace':
    case 'workspace_group':
      return 'catalog'
    case 'connection':
      return 'connection'
    case 'dashboard':
      return 'dashboard'
    case 'field':
    case 'relationship':
      return 'dimension'
    case 'filter':
      return 'filter'
    case 'measure':
      return 'measure'
    case 'model_table':
    case 'semantic_table':
      return 'model-table'
    case 'page':
    case 'page_item':
      return 'page'
    case 'semantic_model':
      return 'semantic-model'
    case 'source':
      return 'source'
    case 'table':
      return 'table'
    case 'visual':
    case 'visual_element':
      return 'visual'
    default:
      return 'default'
  }
}

const workspaceStyles = css`
  :host {
    display: block;
    min-width: 0;
    min-height: 100svh;
    color: var(--ld-fg-default);
    font-family: var(--ld-font-family-ui, var(--fontStack-system));
    background: var(--ld-bg-app);
  }

  .page,
  .asset-page {
    display: grid;
    width: min(100%, var(--ld-page-content-max-width));
    min-width: 0;
    min-height: 100svh;
    align-content: start;
    gap: var(--base-size-12);
    box-sizing: border-box;
    margin-inline: auto;
    background: var(--ld-bg-app);
    padding: var(--base-size-16);
  }

  .asset-page {
    width: 100%;
    grid-template-rows: auto minmax(0, 1fr);
    gap: 0;
    height: 100svh;
    margin-inline: 0;
    padding: 0;
    overflow: hidden;
  }

  .catalog {
    gap: var(--base-size-16);
  }

  .header,
  .breadcrumb-header {
    display: grid;
    min-width: 0;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--base-size-8);
  }

  .breadcrumb-header {
    border-bottom: var(--ld-border-muted);
    padding: var(--ld-space-control) var(--base-size-16);
  }

  .title-block {
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

  h2 {
    color: var(--ld-fg-default);
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-strong);
  }

  .eyebrow {
    margin-bottom: var(--base-size-4);
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    line-height: var(--ld-line-height-tight);
    text-transform: uppercase;
  }

  .detail,
  .muted {
    margin-top: var(--base-size-4);
    overflow: hidden;
    color: var(--ld-fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-sm);
    line-height: var(--ld-line-height-compact);
  }

  .actions,
  .row-actions {
    display: inline-flex;
    min-width: 0;
    align-items: center;
    justify-content: flex-end;
    gap: var(--base-size-8);
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 22rem));
    gap: var(--base-size-16);
    align-items: start;
    justify-content: start;
  }

  .card,
  .panel {
    min-width: 0;
    overflow: hidden;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel);
  }

  .card {
    display: grid;
    min-height: 10rem;
    grid-template-rows: 1fr auto;
    padding: var(--base-size-16);
  }

  .card > div {
    min-width: 0;
  }

  .card footer {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
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

  .primary-link,
  .icon-link,
  .icon-button {
    display: inline-grid;
    place-items: center;
    border-radius: var(--ld-radius-default);
    text-decoration: none;
  }

  .primary-link {
    min-height: var(--ld-button-height-sm);
    grid-auto-flow: column;
    gap: var(--base-size-6);
    border: var(--borderWidth-default) solid var(--ld-button-accent-border-rest);
    background: var(--ld-button-accent-bg-rest);
    color: var(--ld-button-accent-fg-rest);
    padding: 0 var(--ld-button-padding-inline-sm);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-strong);
  }

  .icon-link,
  .icon-button {
    width: var(--control-medium-size);
    height: var(--control-medium-size);
    border: var(--ld-border-muted);
    padding: 0;
  }

  .icon-link {
    border-color: transparent;
    background: transparent;
    color: var(--ld-fg-muted);
    cursor: pointer;
  }

  .icon-link:hover,
  .icon-link:focus-visible {
    border-color: var(--ld-line-muted);
    background: var(--ld-bg-control-hover);
    color: var(--ld-fg-default);
    outline: 0;
  }

  .icon-link:disabled {
    opacity: 0.6;
    cursor: wait;
  }

  .spin {
    animation: ld-spin 900ms linear infinite;
  }

  @keyframes ld-spin {
    to {
      transform: rotate(360deg);
    }
  }

  .icon-button {
    background: var(--ld-bg-panel);
    color: var(--ld-fg-default);
  }

  button,
  input {
    font: inherit;
  }

  .toolbar {
    display: grid;
    min-width: 0;
    gap: var(--base-size-12);
    border-bottom: var(--ld-border-default);
    padding-top: var(--base-size-12);
  }

  .search {
    position: relative;
    display: block;
    max-width: 34rem;
    min-width: 0;
  }

  input[type='search'] {
    min-width: 0;
    min-height: var(--control-medium-size);
    width: 100%;
    border: var(--ld-border-default);
    border-radius: var(--ld-radius-tight);
    background: var(--ld-bg-control);
    color: var(--ld-fg-default);
    padding: 0 calc(var(--base-size-24) + var(--base-size-12)) 0 var(--base-size-12);
  }

  input[type='search']:focus {
    border-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
    background: var(--ld-bg-panel);
    outline: var(--focus-outline, var(--ld-border-default));
    outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
    outline-offset: var(--focus-outline-offset, var(--base-size-2));
  }

  .search-icon {
    position: absolute;
    top: 50%;
    right: var(--ld-space-control);
    display: grid;
    width: var(--base-size-16);
    height: var(--base-size-16);
    place-items: center;
    color: var(--ld-fg-muted);
    pointer-events: none;
    transform: translateY(-50%);
  }

  .tabs {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    gap: var(--base-size-24);
    border-bottom: var(--ld-border-default);
  }

  .toolbar .tabs {
    border-bottom: 0;
  }

  .tabs a {
    display: inline-flex;
    min-height: var(--control-xlarge-size);
    align-items: center;
    gap: var(--base-size-8);
    border-bottom: 2px solid transparent;
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-medium);
    text-decoration: none;
  }

  .tabs a.active {
    border-bottom-color: var(--ld-accent);
    color: var(--ld-fg-default);
    font-weight: var(--ld-font-weight-strong);
  }

  .count {
    display: inline-grid;
    min-width: var(--base-size-16);
    place-items: center;
    border-radius: var(--ld-radius-full);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
    padding: 0 var(--base-size-6);
    font-size: var(--ld-font-size-caption);
  }

  code {
    color: var(--ld-fg-muted);
    font-family: var(--fontStack-monospace, ui-monospace, SFMono-Regular, Consolas, monospace);
    font-size: var(--ld-font-size-caption);
  }

  .asset-glyph {
    display: inline-grid;
    width: var(--control-medium-size);
    height: var(--control-medium-size);
    flex: 0 0 auto;
    place-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-fg-muted);
  }

  .asset-glyph.inline {
    width: var(--base-size-20);
    height: var(--base-size-20);
  }

  .asset-kind-catalog {
    background: var(--ld-asset-catalog-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-catalog-border, var(--ld-line-muted));
    color: var(--ld-asset-catalog-accent, var(--ld-fg-muted));
  }

  .asset-kind-connection {
    background: var(--ld-asset-connection-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-connection-border, var(--ld-line-muted));
    color: var(--ld-asset-connection-accent, var(--ld-fg-muted));
  }

  .asset-kind-dashboard {
    background: var(--ld-asset-dashboard-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-dashboard-border, var(--ld-line-muted));
    color: var(--ld-asset-dashboard-accent, var(--ld-fg-muted));
  }

  .asset-kind-dimension {
    background: var(--ld-asset-dimension-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-dimension-border, var(--ld-line-muted));
    color: var(--ld-asset-dimension-accent, var(--ld-fg-muted));
  }

  .asset-kind-filter {
    background: var(--ld-asset-filter-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-filter-border, var(--ld-line-muted));
    color: var(--ld-asset-filter-accent, var(--ld-fg-muted));
  }

  .asset-kind-measure {
    background: var(--ld-asset-measure-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-measure-border, var(--ld-line-muted));
    color: var(--ld-asset-measure-accent, var(--ld-fg-muted));
  }

  .asset-kind-model-table {
    background: var(--ld-asset-model-table-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-model-table-border, var(--ld-line-muted));
    color: var(--ld-asset-model-table-accent, var(--ld-fg-muted));
  }

  .asset-kind-page {
    background: var(--ld-asset-page-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-page-border, var(--ld-line-muted));
    color: var(--ld-asset-page-accent, var(--ld-fg-muted));
  }

  .asset-kind-semantic-model {
    background: var(--ld-asset-semantic-model-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-semantic-model-border, var(--ld-line-muted));
    color: var(--ld-asset-semantic-model-accent, var(--ld-fg-muted));
  }

  .asset-kind-source {
    background: var(--ld-asset-source-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-source-border, var(--ld-line-muted));
    color: var(--ld-asset-source-accent, var(--ld-fg-muted));
  }

  .asset-kind-table {
    background: var(--ld-asset-table-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-table-border, var(--ld-line-muted));
    color: var(--ld-asset-table-accent, var(--ld-fg-muted));
  }

  .asset-kind-visual {
    background: var(--ld-asset-visual-bg, var(--ld-bg-panel-muted));
    border-color: var(--ld-asset-visual-border, var(--ld-line-muted));
    color: var(--ld-asset-visual-accent, var(--ld-fg-muted));
  }

  .empty {
    color: var(--ld-fg-muted);
    padding: var(--base-size-12);
    font-size: var(--ld-font-size-body-sm);
  }

  .breadcrumb-header ol {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--base-size-6);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--ld-font-size-body-sm);
    font-weight: var(--ld-font-weight-medium);
  }

  .breadcrumb-header li:not(:last-child)::after {
    content: '/';
    margin-left: var(--base-size-6);
    color: var(--ld-fg-muted);
  }

  .breadcrumb-header a {
    color: var(--ld-fg-muted);
    text-decoration: none;
  }

  .breadcrumb-header h1 {
    display: inline-flex;
    min-width: 0;
    align-items: center;
    gap: var(--base-size-8);
  }

  .asset-body {
    display: grid;
    min-width: 0;
    min-height: 0;
    grid-template-rows: auto minmax(0, 1fr);
  }

  .asset-body > .tabs {
    padding-inline: var(--base-size-16);
  }

  .section-body {
    min-height: 0;
    overflow: auto;
    padding: var(--base-size-16);
  }

  .lineage-body {
    padding: 0;
  }

  .graph-details-body {
    padding: 0;
  }

  .details,
  .details-content,
  .lineage-grids {
    display: grid;
    align-content: start;
    gap: var(--base-size-24);
  }

  .details-content {
    padding: var(--base-size-16);
  }

  .lineage {
    display: grid;
    min-height: 0;
    align-content: start;
  }

  .lineage-graph {
    display: block;
    height: var(--ld-lineage-graph-height);
    min-height: 0;
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
  }

  .semantic-model-section {
    min-height: 0;
  }

  .semantic-model-graph {
    display: block;
    height: min(72svh, 48rem);
    min-height: 0;
    overflow: hidden;
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
  }

  .lineage-grids {
    padding: var(--base-size-16);
  }

  .detail-section {
    display: grid;
    min-width: 0;
    align-content: start;
    gap: var(--base-size-12);
    border-bottom: var(--ld-border-muted);
    padding-bottom: var(--base-size-20);
  }

  .detail-section:last-child {
    border-bottom: 0;
  }

  .facts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: var(--base-size-12) var(--base-size-20);
  }

  .facts.overview {
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
  }

  .facts .wide {
    grid-column: span 2;
  }

  .facts div {
    display: grid;
    min-width: 0;
    gap: var(--base-size-4);
  }

  .facts span:first-child {
    color: var(--ld-fg-muted);
    font-size: var(--ld-font-size-caption);
    font-weight: var(--ld-font-weight-medium);
    text-transform: uppercase;
  }

  .facts p,
  .facts code {
    overflow: hidden;
    color: var(--ld-fg-default);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-body-sm);
  }

  .facts .wide p,
  .facts .wide code {
    white-space: pre-wrap;
  }

  @media (max-width: 720px) {
    .page {
      padding: var(--base-size-12);
    }

    .header,
    .breadcrumb-header {
      grid-template-columns: 1fr;
    }

    .asset-page {
      height: auto;
      min-height: 100svh;
      overflow: visible;
    }

    .section-body {
      overflow: visible;
    }

    .graph-details-body {
      overflow: visible;
    }

    .semantic-model-graph {
      height: 32rem;
    }
  }
`

if (!customElements.get('ld-workspace-page')) customElements.define('ld-workspace-page', LibreDashWorkspacePage)
if (!customElements.get('ld-workspace-asset-page')) customElements.define('ld-workspace-asset-page', LibreDashWorkspaceAssetPage)
if (!customElements.get('ld-connections-page')) customElements.define('ld-connections-page', LibreDashConnectionsPage)
