import { LitElement, html, type PropertyValues } from 'lit'
import { property, state } from 'lit/decorators.js'
import { ChevronRight, Database, Search, Server, Table2, Waves } from 'lucide'
import type { AdminStorageSignal, AdminStorageTableSignal, RecordTableSignal } from '../../generated/signals'
import { jsonAttribute } from '../shared/json-attribute'
import { lucideIcon } from '../shared/lucide-icons'
import '../shared/record-table'

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

type DatabaseSelection = {
  databaseId: string
}

class StorageExplorer extends LitElement {
  @property({ converter: jsonAttribute<AdminStorageSignal>(emptyStorage) }) storage: AdminStorageSignal = emptyStorage
  @state() private search = ''
  @state() private selectedDatabase: DatabaseSelection | null = null
  @state() private selectedSchema: SchemaSelection | null = null
  @state() private localSelectedTable: AdminStorageTableSignal | null = null

  updated(changedProperties: PropertyValues<this>): void {
    if (!changedProperties.has('storage')) return
    const groups = groupTables(this.resolvedStorage.tables ?? [])
    if (this.selectedDatabase && !this.resolveSelectedDatabase(groups)) {
      this.selectedDatabase = null
    }
    if (this.selectedSchema && !this.resolveSelectedSchema(groups)) {
      this.selectedSchema = null
    }
    if (this.localSelectedTable) {
      this.localSelectedTable = this.findTableByKey(this.localSelectedTable.key)
    }
  }

  render() {
    const storage = this.resolvedStorage
    const tables = storage.tables ?? []
    const filtered = filterTables(tables, this.search)
    const selectedKey = this.selectedDatabase || this.selectedSchema ? '' : this.localSelectedTable?.key ?? storage.selectedKey ?? ''
    const selected = this.selectedDatabase || this.selectedSchema ? null : this.localSelectedTable ?? storage.selectedTable ?? null
    const selectedDatabase = this.resolveSelectedDatabase(groupTables(tables))
    const selectedSchema = this.resolveSelectedSchema(groupTables(tables))

    return html`
      <style>
        ${storageExplorerStyles}
      </style>
      <div class="storage-explorer" @ld-record-table-action=${this.handleRecordTableAction}>
        <div class="storage-explorer-header">
          <div class="storage-heading">
            <span class="storage-logo" aria-hidden="true">${lucideIcon(Database, { size: 18 })}</span>
            <div>
              <h2>Storage</h2>
              <p>DuckDB inventory · <span>${label(storage.summary?.duckdbDir)}</span></p>
            </div>
          </div>
        </div>
        ${storage.warnings?.length ? html`
          <div class="storage-warnings">
            ${storage.warnings.map((warning) => html`<p class="storage-warning">${warning}</p>`)}
          </div>
        ` : html`<div class="storage-warnings storage-warnings-empty" aria-hidden="true"></div>`}
        <aside class="storage-browser" aria-label="DuckDB table browser">
          <div class="storage-browser-menu">
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
          </div>
          <div class="storage-tree">
            ${storage.status && tables.length === 0
              ? html`<p class="storage-empty">${storage.status}</p>`
              : filtered.length === 0
                ? html`<p class="storage-empty">No matching tables.</p>`
                : this.renderDatabaseGroups(groupTables(filtered), selectedKey)}
          </div>
        </aside>
        <section class="storage-detail" aria-label="Selected DuckDB table details">
          ${selectedDatabase
            ? this.renderSelectedDatabase(selectedDatabase.database)
            : selectedSchema
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
        <span class="storage-table-size">${label(table.sizeLabel)}</span>
      </button>
    `
  }

  private renderSelectedDatabase(database: DatabaseGroup) {
    const tables = database.schemas.flatMap((schema) => schema.tables)
    return html`
      <div class="storage-detail-header">
        <nav aria-label="Selected database location">
          <span class="storage-breadcrumb-current">
            ${lucideIcon(Database, { size: 16 })}
            <strong>${label(database.name)}</strong>
          </span>
        </nav>
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
          <dt>Schemas</dt>
          <dd>${database.schemas.length}</dd>
        </div>
        <div>
          <dt>Known size</dt>
          <dd>${sumKnownSizes(tables)}</dd>
        </div>
      </dl>
      <div class="storage-columns">
        <div class="storage-columns-header">
          <h3>Schemas</h3>
        </div>
        <div class="storage-column-table-wrap">
          <ld-record-table .table=${this.databaseSchemasTable(database)}></ld-record-table>
        </div>
      </div>
    `
  }

  private renderSelectedSchema(database: DatabaseGroup, schema: SchemaGroup) {
    return html`
      <div class="storage-detail-header">
        <nav aria-label="Selected schema location">
          <button type="button" class="storage-breadcrumb-button" data-breadcrumb-kind="database" @click=${() => this.selectDatabase(database.id)}>
            ${label(database.name)}
          </button>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <span class="storage-breadcrumb-current">
            ${lucideIcon(Server, { size: 16 })}
            <strong>${label(schema.schema)}</strong>
          </span>
        </nav>
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
        </div>
        <div class="storage-column-table-wrap">
          <ld-record-table .table=${this.schemaTablesTable(schema)}></ld-record-table>
        </div>
      </div>
    `
  }

  private renderSelectedTable(table: AdminStorageTableSignal) {
    const columns = table.columns ?? []
    return html`
      <div class="storage-detail-header">
        <nav aria-label="Selected table location">
          <button type="button" class="storage-breadcrumb-button" data-breadcrumb-kind="database" @click=${() => this.selectDatabase(table.databaseId)}>
            ${label(table.databaseName)}
          </button>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <button type="button" class="storage-breadcrumb-button" data-breadcrumb-kind="schema" @click=${() => this.selectSchemaByID(table.databaseId, table.schema)}>
            ${label(table.schema)}
          </button>
          <span class="storage-breadcrumb-separator" aria-hidden="true">${lucideIcon(ChevronRight, { size: 15 })}</span>
          <span class="storage-breadcrumb-current">
            ${lucideIcon(table.type === 'view' ? Waves : Table2, { size: 16 })}
            <strong>${label(table.name)}</strong>
          </span>
        </nav>
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
        </div>
        ${columns.length === 0
          ? html`<p class="storage-empty">No column metadata available.</p>`
          : html`
            <div class="storage-column-table-wrap">
              <ld-record-table .table=${this.tableColumnsTable(table)}></ld-record-table>
            </div>
          `}
      </div>
    `
  }

  private databaseSchemasTable(database: DatabaseGroup): RecordTableSignal {
    return {
      columns: [
        { id: 'index', header: '#', kind: 'number', align: 'right', width: '64px' },
        { id: 'schema', header: 'Schema', kind: 'button', width: '220px' },
        { id: 'tables', header: 'Tables', kind: 'number', align: 'right', width: '120px' },
        { id: 'rows', header: 'Known rows', align: 'right', width: '150px' },
        { id: 'size', header: 'Known size', align: 'right', width: '150px' },
      ],
      rows: database.schemas.map((schema, index) => ({
        index: index + 1,
        schema: { label: schema.schema, icon: 'schema', action: 'select-schema' },
        tables: schema.tables.length,
        rows: sumKnownRows(schema.tables),
        size: sumKnownSizes(schema.tables),
        databaseId: database.id,
        schemaName: schema.schema,
      })),
      empty: 'No schemas found.',
      minWidth: '700px',
    }
  }

  private schemaTablesTable(schema: SchemaGroup): RecordTableSignal {
    return {
      columns: [
        { id: 'index', header: '#', kind: 'number', align: 'right', width: '64px' },
        { id: 'table', header: 'Table', kind: 'button', width: '240px' },
        { id: 'type', header: 'Type', width: '110px' },
        { id: 'rows', header: 'Rows', align: 'right', width: '130px' },
        { id: 'columns', header: 'Columns', kind: 'number', align: 'right', width: '120px' },
        { id: 'size', header: 'Estimated size', align: 'right', width: '150px' },
      ],
      rows: schema.tables.map((table, index) => ({
        index: index + 1,
        table: { label: table.name, icon: table.type === 'view' ? 'view' : 'table', action: 'select-table' },
        type: label(table.type),
        rows: label(table.rowCountLabel),
        columns: table.columnCount ?? '-',
        size: label(table.sizeLabel),
        tableKey: table.key,
      })),
      empty: 'No tables found.',
      minWidth: '820px',
    }
  }

  private tableColumnsTable(table: AdminStorageTableSignal): RecordTableSignal {
    return {
      columns: [
        { id: 'ordinal', header: '#', kind: 'number', align: 'right', width: '64px' },
        { id: 'name', header: 'Name', kind: 'code', width: '220px' },
        { id: 'type', header: 'Type', kind: 'code', width: '180px' },
        { id: 'nullable', header: 'Nullable', width: '120px' },
        { id: 'default', header: 'Default', kind: 'code' },
      ],
      rows: (table.columns ?? []).map((column) => ({
        ordinal: column.ordinal ?? '',
        name: label(column.name),
        type: label(column.type),
        nullable: label(column.nullable),
        default: column.default || '-',
      })),
      empty: 'No column metadata available.',
      minWidth: '760px',
    }
  }

  private handleRecordTableAction(event: CustomEvent): void {
    const action = String(event.detail?.action ?? '')
    const row = event.detail?.row ?? {}
    if (action === 'select-schema') {
      this.selectSchemaByID(String(row.databaseId ?? ''), String(row.schemaName ?? ''))
      return
    }
    if (action === 'select-table') {
      const table = this.findTableByKey(String(row.tableKey ?? ''))
      if (table) this.selectTable(table)
    }
  }

  private selectTable(table: AdminStorageTableSignal): void {
    this.selectedDatabase = null
    this.selectedSchema = null
    this.localSelectedTable = table
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

  private selectDatabase(databaseId: string): void {
    this.selectedDatabase = { databaseId }
    this.selectedSchema = null
    this.localSelectedTable = null
  }

  private selectSchema(event: Event, databaseId: string, schema: string): void {
    if ((event.target as HTMLElement).closest('.storage-chevron')) {
      return
    }
    event.preventDefault()
    this.selectSchemaByID(databaseId, schema)
  }

  private selectSchemaByID(databaseId: string, schema: string): void {
    this.selectedDatabase = null
    this.selectedSchema = { databaseId, schema }
    this.localSelectedTable = null
  }

  private isSelectedSchema(databaseId: string, schema: string): boolean {
    return this.selectedSchema?.databaseId === databaseId && this.selectedSchema.schema === schema
  }

  private resolveSelectedDatabase(groups: DatabaseGroup[]): { database: DatabaseGroup } | null {
    const selection = this.selectedDatabase
    if (!selection) return null
    const database = groups.find((group) => group.id === selection.databaseId)
    if (!database) return null
    return { database }
  }

  private resolveSelectedSchema(groups: DatabaseGroup[]): { database: DatabaseGroup; schema: SchemaGroup } | null {
    const selection = this.selectedSchema
    if (!selection) return null
    const database = groups.find((group) => group.id === selection.databaseId)
    const schema = database?.schemas.find((item) => item.schema === selection.schema)
    if (!database || !schema) return null
    return { database, schema }
  }

  private findTableByKey(key: string | undefined): AdminStorageTableSignal | null {
    if (!key) return null
    return this.resolvedStorage.tables?.find((table) => table.key === key) ?? null
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
    height: 100%;
    max-width: 100%;
  }

  .storage-explorer {
    display: grid;
    height: 100svh;
    min-height: 0;
    min-width: 0;
    grid-template-columns: minmax(18rem, 22rem) minmax(0, 1fr);
    grid-template-rows: auto auto minmax(0, 1fr);
    grid-template-areas:
      "header header"
      "warnings warnings"
      "browser detail";
    overflow: hidden;
    border: 0;
    border-radius: 0;
    background: var(--ld-bg-panel);
  }

  .storage-explorer-header {
    grid-area: header;
    display: grid;
    min-width: 0;
    align-items: center;
    border-bottom: var(--ld-border-muted);
    padding: var(--base-size-12, 0.75rem) var(--base-size-16, 1rem);
    background: var(--ld-bg-panel);
  }

  .storage-browser,
  .storage-detail {
    min-width: 0;
    min-height: 0;
  }

  .storage-browser {
    grid-area: browser;
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    gap: 0;
    border-right: var(--ld-border-muted);
    background: var(--ld-bg-panel);
  }

  .storage-heading {
    display: grid;
    min-width: 0;
    grid-template-columns: minmax(0, 1fr);
    align-items: center;
    gap: var(--base-size-4, 0.25rem);
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
    font-size: var(--ld-font-size-title-sm, 1rem);
    line-height: var(--ld-line-height-tight, 1.2);
    font-weight: var(--ld-font-weight-strong, 600);
  }

  .storage-heading p {
    overflow: hidden;
    color: var(--ld-fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--ld-font-size-caption, 0.75rem);
    font-weight: var(--ld-font-weight-medium, 500);
    line-height: var(--ld-line-height-tight, 1.2);
  }

  .storage-heading p span {
    color: var(--ld-fg-default);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
    font-weight: var(--ld-font-weight-medium, 500);
  }

  h3 {
    color: var(--ld-fg-default);
    font-size: 0.8125rem;
    line-height: 1.3;
    font-weight: 700;
  }

  .storage-logo {
    display: none;
    width: 1.875rem;
    height: 1.875rem;
    place-items: center;
    border: var(--ld-border-muted);
    border-radius: var(--ld-radius-small);
    color: var(--ld-fg-success);
    background: var(--ld-bg-panel);
  }

  .storage-search {
    position: relative;
    display: block;
    min-width: 0;
  }

  .storage-browser-menu {
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
    padding: var(--base-size-4, 0.25rem) var(--base-size-8, 0.5rem);
  }

  .storage-search-icon {
    position: absolute;
    left: 0.5rem;
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
    border: 0;
    border-radius: 0;
    background: transparent;
    padding: 0 0.625rem 0 2rem;
    color: var(--ld-fg-default);
    font: inherit;
    font-size: 0.875rem;
    outline: 0;
  }

  .storage-search input:focus {
    background: var(--ld-bg-control-hover);
  }

  .storage-warnings {
    grid-area: warnings;
    display: grid;
    gap: var(--base-size-8, 0.5rem);
    border-bottom: var(--ld-border-muted);
    background: var(--ld-bg-panel);
    padding: var(--base-size-8, 0.5rem) var(--base-size-12, 0.75rem);
  }

  .storage-warnings-empty {
    display: none;
  }

  .storage-warning {
    border: var(--ld-border-attention, var(--ld-border-muted));
    border-radius: var(--ld-radius-default);
    background: var(--ld-bg-attention-muted, var(--ld-bg-panel-muted));
    padding: var(--base-size-8, 0.5rem) var(--base-size-12, 0.75rem);
    color: var(--ld-fg-default);
    font-size: var(--ld-font-size-body-sm, 0.875rem);
    font-weight: var(--ld-font-weight-medium, 500);
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
    grid-template-columns: 0.875rem 1rem minmax(0, 1fr);
    margin-bottom: 0.125rem;
  }

  .storage-schema > summary {
    grid-template-columns: 0.875rem 1rem minmax(0, 1fr) auto;
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
    grid-template-columns: 1rem minmax(0, 1fr) max-content;
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

  .storage-table-size {
    overflow: hidden;
    max-width: 4.75rem;
    color: var(--ld-fg-muted);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    font-weight: var(--ld-font-weight-medium, 500);
    text-align: right;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .storage-detail {
    grid-area: detail;
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

  .storage-detail-header nav > span:not(.storage-breadcrumb-separator):not(.storage-breadcrumb-current),
  .storage-breadcrumb-button {
    overflow: hidden;
    color: var(--ld-fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .storage-breadcrumb-button {
    min-width: 0;
    border: 0;
    background: transparent;
    padding: 0;
    font: inherit;
    font-weight: inherit;
    text-align: left;
    cursor: pointer;
  }

  .storage-breadcrumb-button:hover,
  .storage-breadcrumb-button:focus-visible {
    color: var(--ld-fg-link);
    outline: 0;
    text-decoration: underline;
    text-underline-offset: 0.125rem;
  }

  .storage-breadcrumb-separator {
    display: grid;
    flex: none;
    place-items: center;
    color: var(--ld-fg-muted);
  }

  .storage-breadcrumb-current {
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
      grid-template-rows: auto auto auto minmax(0, 1fr);
      grid-template-areas:
        "header"
        "warnings"
        "browser"
        "detail";
      height: auto;
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
