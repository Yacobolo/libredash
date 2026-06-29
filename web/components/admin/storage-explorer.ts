import { LitElement, html, type PropertyValues } from 'lit'
import { property, state } from 'lit/decorators.js'
import { ChevronRight, Database, Search, Server, Table2, Waves } from 'lucide'
import type { AdminStorageSignal, AdminStorageTableSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { lucideIcon } from '../shared/lucide-icons'

const emptyStorage: AdminStorageSignal = {
  summary: { duckdbDir: '', databaseCount: 0, totalSizeLabel: '', tableCount: 0 },
  status: '',
  warnings: [],
  tables: [],
  selectedKey: '',
  selectedTable: null,
}

type SchemaGroup = {
  schema: string
  tables: AdminStorageTableSignal[]
}

type DatabaseGroup = {
  id: string
  name: string
  model: string
  schemas: SchemaGroup[]
}

type SchemaSelection = {
  databaseId: string
  schema: string
}

class StorageExplorer extends LitElement {
  @property({ converter: jsonAttribute<AdminStorageSignal>(emptyStorage) }) storage: AdminStorageSignal = emptyStorage
  @state() private search = ''
  @state() private selectedSchema: SchemaSelection | null = null

  updated(changedProperties: PropertyValues<this>): void {
    if (!changedProperties.has('storage')) return
    if (!this.selectedSchema) return
    if (!this.resolveSelectedSchema(groupTables(this.resolvedStorage.tables ?? []))) {
      this.selectedSchema = null
    }
  }

  render() {
    const storage = this.resolvedStorage
    const tables = storage.tables ?? []
    const filtered = filterTables(tables, this.search)
    const selected = storage.selectedTable ?? null
    const selectedSchema = this.resolveSelectedSchema(groupTables(tables))

    return html`
      <style>
        ${storageExplorerStyles}
      </style>
      <div class="storage-explorer">
        <aside class="storage-browser" aria-label="DuckDB table browser">
          <div class="storage-browser-header">
            <span class="storage-logo" aria-hidden="true">${lucideIcon(Database, { size: 18 })}</span>
            <h2>Storage</h2>
            <span>${filtered.length}/${tables.length}</span>
          </div>
          <label class="storage-search">
            <span class="storage-search-icon" aria-hidden="true">${lucideIcon(Search, { size: 15 })}</span>
            <input
              type="search"
              .value=${this.search}
              @input=${this.onSearch}
              placeholder="Schema, table, model"
              autocomplete="off"
            />
          </label>
          <div class="storage-tree">
            ${storage.status && tables.length === 0
              ? html`<p class="storage-empty">${storage.status}</p>`
              : filtered.length === 0
                ? html`<p class="storage-empty">No matching tables.</p>`
                : this.renderDatabaseGroups(groupTables(filtered), storage.selectedKey ?? '')}
          </div>
        </aside>
        <section class="storage-detail" aria-label="Selected DuckDB table details">
          ${selectedSchema
            ? this.renderSelectedSchema(selectedSchema.database, selectedSchema.schema)
            : selected
              ? this.renderSelectedTable(selected)
              : html`<p class="storage-empty">Select a schema or table to inspect storage metadata.</p>`}
        </section>
      </div>
    `
  }

  private get resolvedStorage(): AdminStorageSignal {
    return this.storage ?? emptyStorage
  }

  private onSearch(event: Event): void {
    this.search = (event.target as HTMLInputElement).value
  }

  private renderDatabaseGroups(groups: DatabaseGroup[], selectedKey: string) {
    return groups.map((database) => html`
      <details class="storage-db" open>
        <summary>
          <span class="storage-chevron" aria-hidden="true">${lucideIcon(ChevronRight, { size: 14 })}</span>
          <span class="storage-node-icon" aria-hidden="true">${lucideIcon(Database, { size: 15 })}</span>
          <span>${label(database.name)}</span>
          <em>${database.schemas.reduce((count, schema) => count + schema.tables.length, 0)}</em>
        </summary>
        ${database.schemas.map((schema) => html`
          <details class="storage-schema" open>
            <summary
              class=${this.isSelectedSchema(database.id, schema.schema) ? 'is-selected-schema' : ''}
              @click=${(event: Event) => this.selectSchema(event, database.id, schema.schema)}
            >
              <span class="storage-chevron" aria-hidden="true">${lucideIcon(ChevronRight, { size: 14 })}</span>
              <span class="storage-node-icon" aria-hidden="true">${lucideIcon(Server, { size: 14 })}</span>
              <span>${label(schema.schema)}</span>
              <em>${schema.tables.length}</em>
            </summary>
            <div class="storage-table-list">
              ${schema.tables.map((table) => this.renderTableButton(table, selectedKey))}
            </div>
          </details>
        `)}
      </details>
    `)
  }

  private renderTableButton(table: AdminStorageTableSignal, selectedKey: string) {
    const isSelected = table.key === selectedKey
    return html`
      <button
        type="button"
        class=${isSelected ? 'storage-table-button is-selected' : 'storage-table-button'}
        aria-pressed=${isSelected ? 'true' : 'false'}
        @click=${() => this.selectTable(table)}
      >
        <span class=${table.type === 'view' ? 'storage-table-icon storage-table-icon-view' : 'storage-table-icon'} aria-hidden="true">
          ${lucideIcon(table.type === 'view' ? Waves : Table2, { size: 14 })}
        </span>
        <span>
          <strong>${label(table.name)}</strong>
          <small>${label(table.schema)}.${label(table.name)}</small>
        </span>
      </button>
    `
  }

  private renderSelectedSchema(database: DatabaseGroup, schema: SchemaGroup) {
    return html`
      <div class="storage-detail-header">
        <nav aria-label="Selected schema location">
          <span>${label(database.name)}</span>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <span class="storage-breadcrumb-table">
            ${lucideIcon(Server, { size: 16 })}
            <strong>${label(schema.schema)}</strong>
          </span>
        </nav>
        <span>${schema.tables.length} ${schema.tables.length === 1 ? 'table' : 'tables'}</span>
      </div>
      <dl class="storage-metrics">
        <div>
          <dt>Database</dt>
          <dd>${label(database.name)}</dd>
        </div>
        <div>
          <dt>Model</dt>
          <dd>${label(database.model)}</dd>
        </div>
        <div>
          <dt>Known rows</dt>
          <dd>${sumKnownRows(schema.tables)}</dd>
        </div>
        <div>
          <dt>Known size</dt>
          <dd>${sumKnownSizes(schema.tables)}</dd>
        </div>
      </dl>
      <div class="storage-columns">
        <div class="storage-columns-header">
          <h3>Tables</h3>
          <span>${schema.tables.length}</span>
        </div>
        <div class="storage-column-table-wrap">
          <table class="storage-column-table storage-schema-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Table</th>
                <th>Type</th>
                <th>Rows</th>
                <th>Columns</th>
                <th>Estimated size</th>
              </tr>
            </thead>
            <tbody>
              ${schema.tables.map((table, index) => html`
                <tr>
                  <td>${index + 1}</td>
                  <td>
                    <button type="button" class="storage-schema-table-link" @click=${() => this.selectTable(table)}>
                      ${lucideIcon(table.type === 'view' ? Waves : Table2, { size: 14 })}
                      <span>${label(table.name)}</span>
                    </button>
                  </td>
                  <td>${label(table.type)}</td>
                  <td>${label(table.rowCountLabel)}</td>
                  <td>${table.columnCount ?? '-'}</td>
                  <td>${label(table.sizeLabel)}</td>
                </tr>
              `)}
            </tbody>
          </table>
        </div>
      </div>
    `
  }

  private renderSelectedTable(table: AdminStorageTableSignal) {
    const columns = table.columns ?? []
    return html`
      <div class="storage-detail-header">
        <nav aria-label="Selected table location">
          <span>${label(table.databaseName)}</span>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <span>${label(table.schema)}</span>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <span class="storage-breadcrumb-table">
            ${lucideIcon(table.type === 'view' ? Waves : Table2, { size: 16 })}
            <strong>${label(table.name)}</strong>
          </span>
        </nav>
        <span>${label(table.type)}</span>
      </div>
      <dl class="storage-metrics">
        <div>
          <dt>Model</dt>
          <dd>${label(table.modelName)}</dd>
        </div>
        <div>
          <dt>Rows</dt>
          <dd>${label(table.rowCountLabel)}</dd>
        </div>
        <div>
          <dt>Columns</dt>
          <dd>${table.columnCount ?? columns.length}</dd>
        </div>
        <div>
          <dt>Estimated size</dt>
          <dd>${label(table.sizeLabel)}</dd>
        </div>
      </dl>
      <div class="storage-columns">
        <div class="storage-columns-header">
          <h3>Columns</h3>
          <span>${columns.length}</span>
        </div>
        ${columns.length === 0
          ? html`<p class="storage-empty">No column metadata available.</p>`
          : html`
            <div class="storage-column-table-wrap">
              <table class="storage-column-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Name</th>
                    <th>Type</th>
                    <th>Nullable</th>
                    <th>Default</th>
                  </tr>
                </thead>
                <tbody>
                  ${columns.map((column) => html`
                    <tr>
                      <td>${column.ordinal ?? ''}</td>
                      <td><code>${label(column.name)}</code></td>
                      <td><code>${label(column.type)}</code></td>
                      <td>${label(column.nullable)}</td>
                      <td>${column.default ? html`<code>${column.default}</code>` : html`<span class="storage-muted">-</span>`}</td>
                    </tr>
                  `)}
                </tbody>
              </table>
            </div>
          `}
      </div>
    `
  }

  private selectTable(table: AdminStorageTableSignal): void {
    this.selectedSchema = null
    this.dispatchEvent(new CustomEvent('ld-storage-table-select', {
      bubbles: true,
      composed: true,
      detail: {
        databaseId: table.databaseId ?? '',
        schema: table.schema ?? '',
        table: table.name ?? '',
      },
    }))
  }

  private selectSchema(event: Event, databaseId: string, schema: string): void {
    if ((event.target as HTMLElement).closest('.storage-chevron')) {
      return
    }
    event.preventDefault()
    this.selectedSchema = { databaseId, schema }
  }

  private isSelectedSchema(databaseId: string, schema: string): boolean {
    return this.selectedSchema?.databaseId === databaseId && this.selectedSchema.schema === schema
  }

  private resolveSelectedSchema(groups: DatabaseGroup[]): { database: DatabaseGroup; schema: SchemaGroup } | null {
    const selection = this.selectedSchema
    if (!selection) return null
    const database = groups.find((group) => group.id === selection.databaseId)
    const schema = database?.schemas.find((item) => item.schema === selection.schema)
    if (!database || !schema) return null
    return { database, schema }
  }
}

function filterTables(tables: AdminStorageTableSignal[], query: string): AdminStorageTableSignal[] {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return tables
  return tables.filter((table) => [
    table.databaseName,
    table.modelName,
    table.schema,
    table.name,
    table.type,
  ].some((value) => String(value ?? '').toLowerCase().includes(normalized)))
}

function groupTables(tables: AdminStorageTableSignal[]): DatabaseGroup[] {
  const databases = new Map<string, DatabaseGroup>()
  for (const table of tables) {
    const id = table.databaseId || table.databaseName || 'database'
    let database = databases.get(id)
    if (!database) {
      database = {
        id,
        name: table.databaseName || id,
        model: table.modelName || table.modelId || '-',
        schemas: [],
      }
      databases.set(id, database)
    }
    let schema = database.schemas.find((item) => item.schema === (table.schema || '-'))
    if (!schema) {
      schema = { schema: table.schema || '-', tables: [] }
      database.schemas.push(schema)
    }
    schema.tables.push(table)
  }
  return [...databases.values()]
}

function label(value: unknown): string {
  if (value == null || value === '') return '-'
  return String(value)
}

function sumKnownRows(tables: AdminStorageTableSignal[]): string {
  let total = 0
  let known = 0
  for (const table of tables) {
    const value = Number(String(table.rowCountLabel ?? '').replaceAll(',', ''))
    if (Number.isFinite(value)) {
      total += value
      known += 1
    }
  }
  if (known === 0) return '-'
  const label = total.toLocaleString('en-US')
  return known < tables.length ? `${label} + unknown` : label
}

function sumKnownSizes(tables: AdminStorageTableSignal[]): string {
  let total = 0
  let known = 0
  for (const table of tables) {
    const bytes = parseSizeLabel(table.sizeLabel)
    if (bytes != null) {
      total += bytes
      known += 1
    }
  }
  if (known === 0) return 'Unknown'
  const label = formatBytes(total)
  return known < tables.length ? `${label} + unknown` : label
}

function parseSizeLabel(value: string | undefined): number | null {
  if (!value || value === 'Unknown' || value === '-') return null
  const match = value.match(/^([\d.]+)\s*(B|KiB|MiB|GiB|TiB)$/)
  if (!match) return null
  const amount = Number(match[1])
  if (!Number.isFinite(amount)) return null
  const units: Record<string, number> = {
    B: 1,
    KiB: 1024,
    MiB: 1024 ** 2,
    GiB: 1024 ** 3,
    TiB: 1024 ** 4,
  }
  return amount * units[match[2]]
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${Math.round(bytes)} B`
  let value = bytes
  for (const suffix of ['KiB', 'MiB', 'GiB', 'TiB']) {
    value /= 1024
    if (value < 1024) return `${value.toFixed(1)} ${suffix}`
  }
  return `${(value / 1024).toFixed(1)} PiB`
}

const storageExplorerStyles = `
  :host {
    display: block;
    min-width: 0;
    max-width: 100%;
  }

  .storage-explorer {
    display: grid;
    height: min(46rem, calc(100svh - 13rem));
    min-height: 34rem;
    min-width: 0;
    grid-template-columns: minmax(18rem, 22rem) minmax(0, 1fr);
    overflow: hidden;
    border: var(--ld-border-default);
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-panel);
  }

  .storage-browser,
  .storage-detail {
    min-width: 0;
    min-height: 0;
  }

  .storage-browser {
    display: grid;
    grid-template-rows: auto auto minmax(0, 1fr);
    gap: 0;
    border-right: var(--ld-border-muted);
    background: var(--ld-bg-panel);
  }

  .storage-browser-header {
    display: grid;
    min-height: 3rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.5rem;
    border-bottom: var(--ld-border-muted);
    padding: 0.5rem 0.625rem;
  }

  .storage-detail-header,
  .storage-columns-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  h2,
  h3,
  p,
  dl {
    margin: 0;
  }

  h2 {
    color: var(--ld-fg-default);
    font-size: 1rem;
    line-height: 1.25;
    font-weight: 800;
  }

  h3 {
    color: var(--ld-fg-default);
    font-size: 0.8125rem;
    line-height: 1.3;
    font-weight: 700;
  }

  .storage-logo {
    display: grid;
    width: 1.875rem;
    height: 1.875rem;
    place-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-small);
    color: var(--ld-fg-success);
    background: var(--ld-bg-panel);
  }

  .storage-browser-header > span:not(.storage-logo),
  .storage-detail-header > span,
  .storage-columns-header > span {
    flex: none;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-small);
    background: var(--ld-bg-panel);
    padding: 0.1875rem 0.4375rem;
    color: var(--ld-fg-muted);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
  }

  .storage-search {
    position: relative;
    display: block;
    border-bottom: var(--ld-border-muted);
    padding: 0.625rem;
  }

  .storage-search-icon {
    position: absolute;
    left: 1.125rem;
    top: 50%;
    display: grid;
    width: 1rem;
    height: 1rem;
    place-items: center;
    color: var(--ld-fg-muted);
    transform: translateY(-50%);
  }

  .storage-search input {
    min-height: 2.125rem;
    width: 100%;
    border: var(--ld-border-default);
    border-radius: var(--ld-radius-small);
    background: var(--ld-bg-panel);
    padding: 0 0.625rem 0 2rem;
    color: var(--ld-fg-default);
    font: inherit;
    font-size: 0.875rem;
    outline: 0;
  }

  .storage-search input:focus {
    border-color: var(--ld-line-accent);
    background: var(--ld-bg-control-hover);
  }

  .storage-tree {
    min-height: 0;
    overflow: auto;
    padding: 0.25rem;
  }

  .storage-db {
    display: grid;
  }

  .storage-db + .storage-db {
    margin-top: 0.125rem;
  }

  summary {
    display: grid;
    min-height: 1.9rem;
    grid-template-columns: 0.875rem 1rem minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.45rem;
    border-radius: var(--ld-radius-small);
    padding: 0 0.5rem;
    cursor: pointer;
    color: var(--ld-fg-default);
    font-size: 0.875rem;
    font-weight: 750;
    list-style: none;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary:hover,
  summary:focus-visible {
    background: var(--ld-bg-hover);
    outline: 0;
  }

  .storage-db > summary {
    margin-bottom: 0.125rem;
  }

  .storage-schema > summary {
    min-height: 1.75rem;
    font-size: 0.8125rem;
    font-weight: 700;
  }

  .storage-schema > summary.is-selected-schema {
    background: var(--ld-bg-accent-muted);
    color: var(--ld-fg-default);
  }

  summary span:not(.storage-chevron):not(.storage-node-icon) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  summary em {
    border-radius: var(--ld-radius-small);
    background: var(--ld-bg-panel-muted);
    padding: 0.125rem 0.375rem;
    color: var(--ld-fg-muted);
    font-size: 0.6875rem;
    font-style: normal;
    font-weight: 750;
    line-height: 1;
  }

  .storage-chevron,
  .storage-node-icon {
    display: grid;
    place-items: center;
    color: var(--ld-fg-muted);
  }

  details[open] > summary .storage-chevron {
    transform: rotate(90deg);
  }

  .storage-schema {
    display: grid;
    margin-left: 1rem;
  }

  .storage-table-list {
    display: grid;
    gap: 0.0625rem;
    margin-left: 1.55rem;
    padding: 0.125rem 0 0.25rem;
  }

  .storage-table-button {
    display: grid;
    min-height: 1.75rem;
    width: 100%;
    grid-template-columns: 1rem minmax(0, 1fr);
    align-items: center;
    gap: 0.45rem;
    border: 0;
    border-left: 2px solid transparent;
    border-radius: var(--ld-radius-small);
    background: transparent;
    padding: 0 0.5rem;
    color: var(--ld-fg-default);
    text-align: left;
    font: inherit;
    cursor: pointer;
  }

  .storage-table-button:hover,
  .storage-table-button:focus-visible {
    background: var(--ld-bg-control-hover);
    outline: 0;
  }

  .storage-table-button.is-selected {
    border-left-color: var(--ld-line-accent);
    background: var(--ld-bg-accent-muted);
  }

  .storage-table-icon {
    display: grid;
    width: 1rem;
    height: 1rem;
    place-items: center;
    color: var(--ld-fg-muted);
  }

  .storage-table-icon-view {
    color: var(--ld-fg-link);
  }

  .storage-table-button span {
    min-width: 0;
  }

  .storage-table-button strong,
  .storage-table-button small {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .storage-table-button strong {
    font-size: 0.8125rem;
    line-height: 1.25;
    font-weight: 550;
  }

  .storage-table-button small {
    display: none;
  }

  .storage-detail {
    display: grid;
    grid-template-rows: auto auto minmax(0, 1fr);
    gap: 0;
    overflow: hidden;
    background: var(--ld-bg-panel);
  }

  .storage-detail-header {
    min-height: 3rem;
    border-bottom: var(--ld-border-muted);
    padding: 0.5rem 0.75rem;
  }

  .storage-detail-header nav {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.375rem;
    color: var(--ld-fg-default);
    font-size: 1rem;
    font-weight: 750;
  }

  .storage-detail-header nav > span {
    min-width: 0;
  }

  .storage-detail-header nav > span:not(.storage-breadcrumb-separator):not(.storage-breadcrumb-table) {
    overflow: hidden;
    color: var(--ld-fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .storage-breadcrumb-separator {
    display: grid;
    flex: none;
    place-items: center;
    color: var(--ld-fg-muted);
  }

  .storage-breadcrumb-table {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
  }

  .storage-metrics {
    display: flex;
    min-height: 2.5rem;
    align-items: center;
    gap: 1rem;
    overflow-x: auto;
    border-bottom: var(--ld-border-muted);
    padding: 0 0.75rem;
  }

  .storage-metrics div {
    display: flex;
    min-width: max-content;
    align-items: baseline;
    gap: 0.375rem;
  }

  dt {
    color: var(--ld-fg-muted);
    font-size: 0.6875rem;
    font-weight: 750;
    text-transform: uppercase;
  }

  dd {
    margin: 0;
    overflow: hidden;
    color: var(--ld-fg-default);
    font-size: 0.8125rem;
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .storage-columns {
    display: grid;
    min-height: 0;
    grid-template-rows: auto minmax(0, 1fr);
    gap: 0;
  }

  .storage-columns-header {
    min-height: 2.125rem;
    border-bottom: var(--ld-border-muted);
    padding: 0 0.75rem;
  }

  .storage-column-table-wrap {
    min-height: 0;
    overflow: auto;
  }

  .storage-column-table {
    width: 100%;
    min-width: 42rem;
    border-collapse: collapse;
    table-layout: fixed;
  }

  .storage-schema-table {
    min-width: 50rem;
  }

  th,
  td {
    border-bottom: var(--ld-border-muted);
    padding: 0.4375rem 0.75rem;
    text-align: left;
    vertical-align: top;
  }

  th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--ld-bg-panel);
    color: var(--ld-fg-muted);
    font-size: 0.6875rem;
    font-weight: 700;
    text-transform: uppercase;
  }

  td {
    color: var(--ld-fg-default);
    font-size: 0.8125rem;
  }

  th:first-child,
  td:first-child {
    width: 4rem;
    color: var(--ld-fg-muted);
    text-align: right;
  }

  .storage-schema-table th:nth-child(4),
  .storage-schema-table td:nth-child(4),
  .storage-schema-table th:nth-child(5),
  .storage-schema-table td:nth-child(5),
  .storage-schema-table th:nth-child(6),
  .storage-schema-table td:nth-child(6) {
    text-align: right;
  }

  .storage-schema-table-link {
    display: inline-flex;
    max-width: 100%;
    align-items: center;
    gap: 0.375rem;
    border: 0;
    background: transparent;
    padding: 0;
    color: var(--ld-fg-default);
    font: inherit;
    font-weight: 650;
    text-align: left;
    cursor: pointer;
  }

  .storage-schema-table-link:hover,
  .storage-schema-table-link:focus-visible {
    color: var(--ld-fg-link);
    outline: 0;
  }

  .storage-schema-table-link span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  code {
    overflow-wrap: anywhere;
    color: var(--ld-fg-default);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
    font-size: 0.8125rem;
  }

  .storage-muted,
  .storage-empty {
    color: var(--ld-fg-muted);
  }

  .storage-empty {
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-small);
    background: var(--ld-bg-panel-muted);
    padding: 0.75rem;
    font-size: 0.8125rem;
  }

  @media (max-width: 820px) {
    .storage-explorer {
      grid-template-columns: minmax(0, 1fr);
      min-height: 0;
    }

    .storage-browser {
      max-height: 22rem;
      border-right: 0;
      border-bottom: var(--ld-border-muted);
    }

    .storage-detail {
      min-height: 28rem;
    }
  }
`

if (!customElements.get('ld-storage-explorer')) customElements.define('ld-storage-explorer', StorageExplorer)

declare global {
  interface HTMLElementTagNameMap {
    'ld-storage-explorer': StorageExplorer
  }
}
