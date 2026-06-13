import { LitElement, css, html, nothing } from 'lit';
import { createRef, ref } from 'lit/directives/ref.js';
import { flexRender, getCoreRowModel, TableController } from '@tanstack/lit-table';
import { VirtualizerController } from '@tanstack/lit-virtual';

const emptyTable = {
  title: 'Orders',
  columns: [],
  rows: [],
  totalRows: 0,
  window: { offset: 0, limit: 120 },
  sort: { key: 'purchase_date', direction: 'desc' },
  loading: false,
  error: '',
};

const tableConverter = {
  fromAttribute(value) {
    if (!value) return emptyTable;
    try {
      return { ...emptyTable, ...JSON.parse(value) };
    } catch {
      return { ...emptyTable, error: 'Could not parse table signal.' };
    }
  },
  toAttribute(value) {
    return JSON.stringify(value ?? emptyTable);
  },
};

function formatCell(value, column) {
  if (value === null || value === undefined || value === '') return '-';
  if (column.key === 'revenue' && Number.isFinite(Number(value))) {
    return `R$ ${Number(value).toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
  }
  if (column.key === 'review_score' && Number.isFinite(Number(value))) {
    return Number(value).toFixed(2);
  }
  if (column.key === 'delivery_days' && Number.isFinite(Number(value))) {
    return `${Number(value)}d`;
  }
  return String(value);
}

function defaultDirection(column) {
  return ['revenue', 'review_score', 'delivery_days', 'purchase_date'].includes(column.key) ? 'desc' : 'asc';
}

class DataTable extends LitElement {
  static properties = {
    table: { attribute: 'table', converter: tableConverter },
  };

  static styles = css`
    :host {
      display: block;
      color: var(--fgColor-default);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    }

    .shell {
      display: grid;
      grid-template-rows: auto auto minmax(320px, 52vh);
      min-height: 480px;
    }

    .toolbar {
      display: flex;
      align-items: end;
      justify-content: space-between;
      gap: 16px;
      border-bottom: 1px solid var(--borderColor-default);
      padding: 14px 16px 12px;
    }

    .eyebrow {
      margin: 0 0 3px;
      color: var(--fgColor-muted);
      font-size: 0.72rem;
      font-weight: 900;
      letter-spacing: 0;
      text-transform: uppercase;
    }

    h2 {
      margin: 0;
      font-size: 1.35rem;
      font-weight: 900;
      letter-spacing: 0;
    }

    .meta {
      display: flex;
      flex-wrap: wrap;
      justify-content: end;
      gap: 8px;
      color: var(--fgColor-muted);
      font-size: 0.78rem;
      font-weight: 850;
    }

    .pill {
      border: 1px solid var(--borderColor-default);
      background: var(--bgColor-muted);
      padding: 6px 9px;
      white-space: nowrap;
    }

    .error {
      border-bottom: 1px solid var(--borderColor-danger-emphasis);
      background: var(--bgColor-danger-muted);
      color: var(--fgColor-danger);
      padding: 10px 16px;
      font-size: 0.86rem;
      font-weight: 850;
    }

    .head,
    .row {
      display: grid;
      grid-template-columns: var(--ld-table-columns);
      min-width: 1040px;
    }

    .head {
      border-bottom: 1px solid var(--borderColor-emphasis);
      background: var(--bgColor-muted);
      color: var(--fgColor-muted);
    }

    .header-cell,
    .cell {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .header-cell {
      border-right: 1px solid var(--borderColor-default);
    }

    .header-cell:last-child {
      border-right: 0;
    }

    button.header-button {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      width: 100%;
      min-height: 42px;
      border: 0;
      background: transparent;
      color: inherit;
      cursor: pointer;
      padding: 0 10px;
      font: inherit;
      font-size: 0.74rem;
      font-weight: 950;
      letter-spacing: 0;
      text-transform: uppercase;
    }

    button.header-button:hover,
    button.header-button:focus-visible {
      background: var(--control-transparent-bgColor-hover);
      color: var(--fgColor-default);
      outline: 0;
    }

    .sort {
      color: var(--fgColor-accent);
      font-size: 0.8rem;
      opacity: 0;
    }

    .sorted .sort {
      opacity: 1;
    }

    .viewport {
      position: relative;
      overflow: auto;
      background: var(--bgColor-default);
    }

    .canvas {
      position: relative;
      min-width: 1040px;
    }

    .row {
      position: absolute;
      inset-inline: 0;
      height: 38px;
      border-bottom: 1px solid var(--borderColor-muted);
      background: var(--bgColor-default);
      color: var(--fgColor-default);
    }

    .row:nth-child(even) {
      background: var(--bgColor-muted);
    }

    .cell {
      display: flex;
      align-items: center;
      border-right: 1px solid var(--borderColor-muted);
      padding: 0 10px;
      font-size: 0.82rem;
      font-weight: 700;
    }

    .cell:last-child {
      border-right: 0;
    }

    .right {
      justify-content: end;
      font-variant-numeric: tabular-nums;
    }

    .empty {
      display: grid;
      min-height: 240px;
      place-items: center;
      color: var(--fgColor-muted);
      font-size: 0.9rem;
      font-weight: 850;
    }

    .loading {
      position: absolute;
      inset-inline: 0;
      top: 0;
      height: 3px;
      overflow: hidden;
      background: var(--bgColor-accent-muted);
    }

    .loading::after {
      content: '';
      display: block;
      width: 34%;
      height: 100%;
      background: var(--fgColor-accent);
      animation: load 900ms ease-in-out infinite;
    }

    @keyframes load {
      0% { transform: translateX(-100%); }
      100% { transform: translateX(310%); }
    }

    @media (max-width: 760px) {
      .shell {
        grid-template-rows: auto auto minmax(300px, 62vh);
      }

      .toolbar {
        align-items: stretch;
        flex-direction: column;
      }

      .meta {
        justify-content: start;
      }
    }
  `;

  constructor() {
    super();
    this.table = emptyTable;
    this.scrollElementRef = createRef();
    this.tableController = new TableController(this);
    this.virtualizerController = new VirtualizerController(this, {
      getScrollElement: () => this.scrollElementRef.value,
      count: 0,
      estimateSize: () => 38,
      overscan: 8,
    });
    this.pendingKey = '';
  }

  updated() {
    const key = this.requestKey(this.table?.window?.offset, this.table?.sort);
    if (!this.table?.loading && this.pendingKey === key) {
      this.pendingKey = '';
    }
  }

  get rows() {
    return Array.isArray(this.table?.rows) ? this.table.rows : [];
  }

  get columns() {
    return Array.isArray(this.table?.columns) ? this.table.columns : [];
  }

  get gridTemplate() {
    const widths = {
      order_id: 'minmax(190px,1.35fr)',
      purchase_date: 'minmax(118px,.75fr)',
      status: 'minmax(120px,.75fr)',
      state: 'minmax(74px,.45fr)',
      category: 'minmax(180px,1.1fr)',
      revenue: 'minmax(112px,.7fr)',
      review_score: 'minmax(92px,.55fr)',
      delivery_days: 'minmax(96px,.55fr)',
    };
    return this.columns.map((column) => widths[column.key] ?? 'minmax(120px,1fr)').join(' ');
  }

  requestKey(offset, sort = this.table?.sort) {
    return `${offset ?? 0}:${this.table?.window?.limit ?? 120}:${sort?.key ?? ''}:${sort?.direction ?? ''}`;
  }

  emitWindow(offset, sort = this.table?.sort) {
    const limit = this.table?.window?.limit ?? 120;
    const maxOffset = Math.max(0, (this.table?.totalRows ?? 0) - limit);
    const nextOffset = Math.max(0, Math.min(offset, maxOffset));
    const nextSort = sort?.key ? sort : { key: 'purchase_date', direction: 'desc' };
    const key = this.requestKey(nextOffset, nextSort);
    if (this.pendingKey === key || this.table?.loading) return;

    this.pendingKey = key;
    this.dispatchEvent(new CustomEvent('ld-table-window-change', {
      bubbles: true,
      composed: true,
      detail: {
        table: 'orders',
        offset: nextOffset,
        limit,
        sort: nextSort,
      },
    }));
  }

  handleScroll(event) {
    const target = event.currentTarget;
    const offset = this.table?.window?.offset ?? 0;
    const limit = this.table?.window?.limit ?? 120;
    const total = this.table?.totalRows ?? 0;
    const nearBottom = target.scrollTop + target.clientHeight > target.scrollHeight - 160;
    const nearTop = target.scrollTop < 80;

    if (nearBottom && offset + limit < total) {
      this.emitWindow(offset + limit);
    } else if (nearTop && offset > 0) {
      this.emitWindow(offset - limit);
    }
  }

  sortColumn(column) {
    const current = this.table?.sort ?? {};
    const direction = current.key === column.key
      ? current.direction === 'asc' ? 'desc' : 'asc'
      : defaultDirection(column);
    this.emitWindow(0, { key: column.key, direction });
  }

  render() {
    const rows = this.rows;
    const columns = this.columns;
    const table = this.tableController.table({
      data: rows,
      columns: columns.map((column) => ({
        id: column.key,
        accessorKey: column.key,
        header: () => column.label,
        cell: (info) => formatCell(info.getValue(), column),
      })),
      getCoreRowModel: getCoreRowModel(),
      renderFallbackValue: '-',
      manualSorting: true,
    });

    const rowModel = table.getRowModel().rows;
    const virtualizer = this.virtualizerController.getVirtualizer();
    virtualizer.setOptions({
      ...virtualizer.options,
      count: rowModel.length,
      estimateSize: () => 38,
      overscan: 8,
    });
    const virtualRows = virtualizer.getVirtualItems();
    const totalSize = virtualizer.getTotalSize();
    const first = (this.table?.window?.offset ?? 0) + 1;
    const last = Math.min((this.table?.window?.offset ?? 0) + rows.length, this.table?.totalRows ?? 0);

    return html`
      <section class="shell" style=${`--ld-table-columns:${this.gridTemplate}`}>
        <div class="toolbar">
          <div>
            <p class="eyebrow">Windowed read model</p>
            <h2>${this.table?.title ?? 'Orders'}</h2>
          </div>
          <div class="meta">
            <span class="pill">${this.table?.totalRows ? `${first.toLocaleString()}-${last.toLocaleString()} of ${this.table.totalRows.toLocaleString()}` : 'No rows'}</span>
            <span class="pill">${this.table?.sort?.key ?? 'purchase_date'} ${this.table?.sort?.direction ?? 'desc'}</span>
          </div>
        </div>
        ${this.table?.error ? html`<div class="error">${this.table.error}</div>` : nothing}
        <div>
          <div class="head" role="row">
            ${columns.map((column) => {
              const sorted = this.table?.sort?.key === column.key;
              const sortMark = this.table?.sort?.direction === 'asc' ? '↑' : '↓';
              return html`
                <div class=${`header-cell ${sorted ? 'sorted' : ''}`} role="columnheader">
                  <button class="header-button" type="button" @click=${() => this.sortColumn(column)}>
                    <span>${column.label}</span>
                    <span class="sort">${sortMark}</span>
                  </button>
                </div>
              `;
            })}
          </div>
          <div class="viewport" ${ref(this.scrollElementRef)} @scroll=${this.handleScroll} role="table" aria-label=${this.table?.title ?? 'Orders'}>
            ${this.table?.loading ? html`<div class="loading" aria-hidden="true"></div>` : nothing}
            ${rows.length === 0 && !this.table?.loading ? html`<div class="empty">Waiting for table data</div>` : html`
              <div class="canvas" style=${`height:${totalSize}px`}>
                ${virtualRows.map((virtualRow) => {
                  const row = rowModel[virtualRow.index];
                  return html`
                    <div
                      class="row"
                      role="row"
                      style=${`transform:translateY(${virtualRow.start}px)`}
                    >
                      ${row.getVisibleCells().map((cell) => {
                        const column = columns.find((item) => item.key === cell.column.id) ?? {};
                        return html`
                          <div class=${`cell ${column.align === 'right' ? 'right' : ''}`} role="cell" title=${String(cell.getValue() ?? '')}>
                            ${flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </div>
                        `;
                      })}
                    </div>
                  `;
                })}
              </div>
            `}
          </div>
        </div>
      </section>
    `;
  }
}

customElements.define('ld-data-table', DataTable);
