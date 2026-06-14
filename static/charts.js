import { LitElement, css, html, svg } from 'https://cdn.jsdelivr.net/npm/lit@3/+esm';

const chartStyles = css`
  :host {
    display: block;
    height: 100%;
    min-height: 286px;
    color: var(--fgColor-default);
    --chart-line: var(--ld-chart-1, var(--data-blue-color-emphasis));
    --chart-line-fill: var(--ld-chart-1-muted, var(--data-blue-color-muted));
    --chart-bar-1: var(--ld-chart-1, var(--data-blue-color-emphasis));
    --chart-bar-2: var(--ld-chart-2, var(--data-green-color-emphasis));
    --chart-bar-3: var(--ld-chart-3, var(--data-purple-color-emphasis));
    --chart-bar-4: var(--ld-chart-4, var(--data-coral-color-emphasis));
    --chart-bar-5: var(--ld-chart-5, var(--data-pine-color-emphasis));
    --chart-bar-6: var(--ld-chart-6, var(--data-pink-color-emphasis));
    font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  }

  .chart {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100%;
    min-height: 286px;
    background: var(--bgColor-default);
  }

  header {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    align-items: baseline;
    min-height: 42px;
    padding: 10px 12px 8px;
  }

  h2 {
    margin: 0;
    font-size: 0.98rem;
    font-weight: 850;
    letter-spacing: 0;
  }

  .unit {
    color: var(--fgColor-muted);
    font-size: 0.72rem;
    font-weight: 900;
    text-transform: uppercase;
  }

  .empty {
    display: grid;
    place-items: center;
    margin: 12px;
    min-height: 210px;
    border: 1px dashed var(--borderColor-default);
    background: var(--bgColor-muted);
    color: var(--fgColor-muted);
    font-weight: 800;
  }

  svg {
    width: 100%;
    height: 224px;
    padding: 12px;
    overflow: visible;
    box-sizing: border-box;
  }

  .grid {
    stroke: var(--ld-chart-grid, var(--borderColor-muted));
    stroke-width: 1;
    opacity: 0.8;
  }

  .selectable {
    cursor: pointer;
    outline: none;
  }

  .selectable.dimmed {
    opacity: 0.38;
  }

  .selectable:focus-visible .mark,
  .selected .mark {
    filter: drop-shadow(0 0 0.22rem color-mix(in srgb, var(--fgColor-accent), transparent 45%));
    stroke: var(--fgColor-accent);
    stroke-width: 3;
  }

  text {
    fill: var(--ld-chart-axis, var(--fgColor-muted));
    font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    font-size: 10px;
    font-weight: 750;
  }
`;

class BaseChart extends LitElement {
  static properties = {
    data: { type: Array },
    chartTitle: { type: String, attribute: 'chart-title' },
    unit: { type: String },
    visualId: { type: String, attribute: 'visual-id' },
    field: { type: String },
    selection: { type: Array },
  };

  constructor() {
    super();
    this.data = [];
    this.chartTitle = 'Chart';
    this.unit = '';
    this.visualId = '';
    this.field = '';
    this.selection = [];
  }

  selectedLabels() {
    return new Set([...(this.selection ?? []), ...(this.data ?? []).filter((d) => d.selected).map((d) => d.label)]);
  }

  selectPoint(point) {
    if (!this.visualId || !this.field || !point?.label) return;
    this.dispatchEvent(new CustomEvent('ld-chart-select', {
      bubbles: true,
      composed: true,
      detail: {
        visualId: this.visualId,
        field: this.field,
        value: point.label,
        label: point.label,
        mode: 'toggle',
      },
    }));
  }

  selectFromKeyboard(event, point) {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    this.selectPoint(point);
  }
}

function format(value) {
  if (!Number.isFinite(value)) return '-';
  if (Math.abs(value) >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`;
  if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
  return value.toLocaleString(undefined, { maximumFractionDigits: 0 });
}

class LineChart extends BaseChart {
  static styles = chartStyles;

  render() {
    const data = this.data ?? [];
    const max = Math.max(...data.map((d) => d.value), 1);
    const width = 760;
    const height = 224;
    const pad = 28;
    const step = data.length > 1 ? (width - pad * 2) / (data.length - 1) : 0;
    const points = data.map((d, index) => {
      const x = pad + index * step;
      const y = height - pad - (d.value / max) * (height - pad * 2);
      return { ...d, x, y };
    });
    const selected = this.selectedLabels();
    const hasSelection = selected.size > 0;
    const path = points.map((p, index) => `${index === 0 ? 'M' : 'L'}${p.x},${p.y}`).join(' ');
    const area = points.length ? `${path} L${points.at(-1).x},${height - pad} L${points[0].x},${height - pad} Z` : '';

    return html`
      <section class="chart">
        <header>
          <h2>${this.chartTitle ?? 'Chart'}</h2>
          <span class="unit">${this.unit ?? ''}</span>
        </header>
        ${data.length === 0 ? html`<div class="empty">Waiting for signal data</div>` : svg`
          <svg viewBox="0 0 ${width} ${height}" role="img" aria-label=${this.chartTitle ?? 'Line chart'}>
            <line class="grid" x1=${pad} x2=${width - pad} y1=${height - pad} y2=${height - pad}></line>
            <line class="grid" x1=${pad} x2=${width - pad} y1=${pad} y2=${pad}></line>
            <path d=${area} fill="var(--chart-line-fill)"></path>
            <path d=${path} fill="none" stroke="var(--chart-line)" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"></path>
            ${points.map((p) => {
              const isSelected = selected.has(p.label);
              return svg`
                <g
                  class=${`selectable ${isSelected ? 'selected' : ''} ${hasSelection && !isSelected ? 'dimmed' : ''}`}
                  tabindex="0"
                  role="button"
                  aria-label=${`Filter ${this.chartTitle} by ${p.label}`}
                  @click=${() => this.selectPoint(p)}
                  @keydown=${(event) => this.selectFromKeyboard(event, p)}
                >
                  <circle class="mark" cx=${p.x} cy=${p.y} r=${isSelected ? '5.2' : '3.8'} fill="var(--bgColor-default)" stroke="var(--chart-line)" stroke-width="2.4"><title>${p.label}: ${format(p.value)}</title></circle>
                </g>
              `;
            })}
            ${points.filter((_, index) => index === 0 || index === points.length - 1 || index % Math.ceil(points.length / 6) === 0).map((p) => svg`<text x=${p.x} y=${height - 4} text-anchor="middle">${p.label}</text>`)}
          </svg>
        `}
      </section>
    `;
  }
}

class BarChart extends BaseChart {
  static styles = chartStyles;

  render() {
    const data = this.data ?? [];
    const max = Math.max(...data.map((d) => d.value), 1);
    const width = 760;
    const rowHeight = 28;
    const height = Math.max(230, data.length * rowHeight + 32);
    const selected = this.selectedLabels();
    const hasSelection = selected.size > 0;

    return html`
      <section class="chart">
        <header>
          <h2>${this.chartTitle ?? 'Chart'}</h2>
          <span class="unit">${this.unit ?? ''}</span>
        </header>
        ${data.length === 0 ? html`<div class="empty">Waiting for signal data</div>` : svg`
          <svg viewBox="0 0 ${width} ${height}" role="img" aria-label=${this.chartTitle ?? 'Bar chart'}>
            ${data.map((d, index) => {
              const y = 14 + index * rowHeight;
              const barWidth = Math.max(2, (d.value / max) * 470);
              const tones = ['var(--chart-bar-1)', 'var(--chart-bar-2)', 'var(--chart-bar-3)', 'var(--chart-bar-4)', 'var(--chart-bar-5)', 'var(--chart-bar-6)'];
              const tone = tones[index % tones.length];
              const isSelected = selected.has(d.label);
              return svg`
                <g
                  class=${`selectable ${isSelected ? 'selected' : ''} ${hasSelection && !isSelected ? 'dimmed' : ''}`}
                  tabindex="0"
                  role="button"
                  aria-label=${`Filter ${this.chartTitle} by ${d.label}`}
                  @click=${() => this.selectPoint(d)}
                  @keydown=${(event) => this.selectFromKeyboard(event, d)}
                >
                  <text x="0" y=${y + 16}>${d.label}</text>
                  <rect class="mark" x="210" y=${y} width=${barWidth} height="16" rx="1.5" fill=${tone}></rect>
                  <text x=${220 + barWidth} y=${y + 15}>${format(d.value)}</text>
                </g>
              `;
            })}
          </svg>
        `}
      </section>
    `;
  }
}

class KPIStrip extends LitElement {
  static properties = {
    items: { type: Array },
  };

  constructor() {
    super();
    this.items = [];
  }

  static styles = css`
    :host {
      display: block;
    }

    .strip {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
    }

    .kpi {
      position: relative;
      min-height: 104px;
      border: 1px solid var(--borderColor-default);
      border-radius: 6px;
      background: var(--bgColor-default);
      box-shadow: var(--shadow-resting-small);
      padding: 12px 14px 12px 16px;
      overflow: hidden;
    }

    .kpi::before {
      content: '';
      position: absolute;
      inset-block: 0;
      left: 0;
      width: 5px;
      background: var(--borderColor-muted);
    }

    .label {
      color: var(--fgColor-muted);
      font-size: 0.72rem;
      font-weight: 900;
      text-transform: uppercase;
    }

    .value {
      margin: 8px 0 4px;
      font-size: clamp(1.72rem, 3.5vw, 2.65rem);
      font-weight: 850;
      line-height: 1;
      letter-spacing: 0;
    }

    .note {
      color: var(--fgColor-muted);
      font-size: 0.85rem;
      font-weight: 700;
    }

    .green::before { background: var(--ld-chart-2, var(--data-green-color-emphasis)); }
    .amber::before { background: var(--ld-accent, var(--data-yellow-color-emphasis)); }
    .coral::before { background: var(--ld-chart-4, var(--data-coral-color-emphasis)); }
    .ink::before { background: var(--ld-chart-1, var(--data-blue-color-emphasis)); }
    .neutral::before { background: var(--borderColor-muted); }

    @media (max-width: 760px) {
      .strip {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }

    @media (max-width: 440px) {
      .strip {
        grid-template-columns: 1fr;
      }
    }
  `;

  render() {
    const kpis = this.items ?? [];
    return html`
      <section class="strip" aria-label="Key metrics">
        ${(kpis.length ? kpis : [{ label: 'Orders', value: '-', note: 'Waiting for stream', tone: 'neutral' }]).map((item) => html`
          <article class="kpi ${item.tone ?? 'neutral'}">
            <div class="label">${item.label}</div>
            <div class="value">${item.value}</div>
            <div class="note">${item.note}</div>
          </article>
        `)}
      </section>
    `;
  }
}

customElements.define('ld-line-chart', LineChart);
customElements.define('ld-bar-chart', BarChart);
customElements.define('ld-kpi-strip', KPIStrip);
