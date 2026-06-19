import { css, html, nothing, render as renderLit } from 'lit'
import {
  defaultControl,
  emptyFilters,
  filterConfigMap,
  filtersToURLParams,
  type DatePreset,
  type FilterConfig,
  type FilterControl,
  type FilterDefinition,
  type FiltersSignal,
} from './filter-url'

type FilterOption = {
  value: string
  label: string
}

const filterCardStyles = css`
    :host {
      display: block;
      height: 100%;
      color: var(--ld-fg-default);
      font-family: var(--fontStack-system);
    }

    .card {
      position: relative;
      display: grid;
      height: 100%;
      min-width: 0;
      align-content: center;
      gap: 4px;
      border: 0;
      background: transparent;
      padding: 8px 10px;
      box-sizing: border-box;
    }

    .label {
      overflow: hidden;
      color: var(--ld-fg-muted);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-none);
      text-transform: uppercase;
    }

    .trigger {
      display: flex;
      min-width: 0;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      border: 0;
      background: transparent;
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0;
      text-align: left;
      font: inherit;
    }

    .value {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--ld-font-size-body-md);
      font-weight: var(--ld-font-weight-strong);
      line-height: var(--ld-line-height-tight);
    }

    .chevron {
      flex: 0 0 auto;
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }

    .popover {
      position: absolute;
      top: calc(100% + 6px);
      left: 0;
      z-index: 30;
      display: grid;
      width: min(260px, max(100%, 220px));
      gap: 7px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-default);
      background: var(--ld-bg-overlay);
      box-shadow: var(--ld-shadow-floating-sm);
      padding: 8px;
    }

    button,
    input,
    select {
      font: inherit;
    }

    input,
    select {
      width: 100%;
      min-width: 0;
      min-height: 27px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-tight);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      padding: 0 7px;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-regular);
      outline-offset: 2px;
      box-sizing: border-box;
    }

    input:focus,
    select:focus,
    button:focus-visible {
      outline: var(--ld-border-width-focus) solid var(--ld-accent);
    }

    .chips {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 4px;
    }

    .chip,
    .action {
      min-height: 27px;
      border: var(--ld-border-default);
      border-radius: var(--ld-radius-tight);
      background: var(--ld-bg-control);
      color: var(--ld-fg-default);
      cursor: pointer;
      padding: 0 7px;
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-strong);
    }

    .chip.custom {
      grid-column: 1 / -1;
    }

    .chip[aria-pressed='true'] {
      border-color: var(--ld-accent);
      background: color-mix(in srgb, var(--ld-accent) 20%, var(--ld-bg-control));
    }

    .date-row,
    .actions {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 6px;
    }

    .actions.three {
      grid-template-columns: 1fr 1fr 1fr;
    }

    .action.primary {
      border-color: var(--ld-accent);
      background: var(--ld-accent);
      color: var(--ld-accent-fg);
    }

    .checks {
      display: grid;
      max-height: 152px;
      gap: 2px;
      overflow: auto;
    }

    .check {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: 6px;
      border-radius: var(--ld-radius-tight);
      padding: 4px;
      color: var(--ld-fg-default);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
    }

    .check:hover {
      background: var(--ld-bg-hover);
    }

    .check input {
      width: 13px;
      height: 13px;
      min-height: 0;
      accent-color: var(--ld-accent);
    }

    .empty {
      color: var(--ld-fg-muted);
      font-size: var(--ld-font-size-caption);
      font-weight: var(--ld-font-weight-medium);
      padding: 4px;
    }
  `

class FilterCard extends HTMLElement {
  static observedAttributes = ['filter-id', 'config', 'filters', 'options', 'loading']

  private open = false
  private search = ''
  private draftFrom = ''
  private draftTo = ''
  private customDate = false

  connectedCallback(): void {
    if (!this.shadowRoot) this.attachShadow({ mode: 'open' })
    this.renderCard()
  }

  attributeChangedCallback(): void {
    this.renderCard()
  }

  private renderCard(): void {
    if (!this.shadowRoot) return
    renderLit(this.template(), this.shadowRoot)
  }

  private template() {
    const definition = this.definition()
    if (!definition) return html`<style>${filterCardStyles}</style><slot></slot>`
    const control = this.control(definition)
    return html`
      <style>${filterCardStyles}</style>
      <section class="card" aria-label=${definition.label}>
        <div class="label">${definition.label}</div>
        <button class="trigger" type="button" ?disabled=${this.isLoading()} aria-expanded=${String(this.open)} @click=${() => this.toggle(definition, control)}>
          <span class="value">${this.summary(definition, control)}</span>
          <span class="chevron" aria-hidden="true">▾</span>
        </button>
        ${this.open ? this.renderPopover(definition, control) : nothing}
      </section>
    `
  }

  private renderPopover(definition: FilterDefinition, control: FilterControl) {
    switch (definition.type) {
      case 'date_range':
        return this.renderDate(definition, control)
      case 'multi_select':
        return this.renderMulti(definition, control)
      case 'text':
        return this.renderText(definition, control)
      default:
        return nothing
    }
  }

  private renderDate(definition: FilterDefinition, control: FilterControl) {
    const preset = control.preset || definition.default?.preset || 'all'
    const presets = [...(definition.presets ?? [])]
    if (definition.custom) presets.push({ value: 'custom', label: 'Custom' })
    const custom = this.customDate || preset === 'custom' || Boolean(control.from || control.to)
    return html`
      <div class="popover">
        <div class="chips">
          ${presets.map((item) => html`
            <button
              class=${`chip ${item.value === 'custom' ? 'custom' : ''}`}
              type="button"
              aria-pressed=${String((custom ? 'custom' : preset) === item.value)}
              @click=${() => this.pickDatePreset(control, item.value)}
            >${presetLabel(item)}</button>
          `)}
        </div>
        ${custom ? html`
          <div class="date-row">
            <input type="date" aria-label="${definition.label} from" .value=${this.draftFrom} @input=${(event: Event) => this.setDraft('from', event)} />
            <input type="date" aria-label="${definition.label} to" .value=${this.draftTo} @input=${(event: Event) => this.setDraft('to', event)} />
          </div>
          <div class="actions three">
            <button class="action" type="button" @click=${() => this.clear()}>Clear</button>
            <button class="action" type="button" @click=${() => this.close()}>Cancel</button>
            <button class="action primary" type="button" @click=${() => this.applyDate(control)}>Apply</button>
          </div>
        ` : nothing}
      </div>
    `
  }

  private renderMulti(definition: FilterDefinition, control: FilterControl) {
    const selected = new Set(control.values ?? [])
    const search = this.search.toLowerCase()
    const options = (this.currentOptions()[this.currentFilterId()] ?? []).filter((option) => option.label.toLowerCase().includes(search) || option.value.toLowerCase().includes(search))
    return html`
      <div class="popover">
        <input type="search" placeholder="Search ${definition.label.toLowerCase()}..." .value=${this.search} @input=${(event: Event) => this.setSearch(event)} />
        <div class="checks">
          ${options.length === 0 ? html`<div class="empty">No values loaded</div>` : nothing}
          ${options.map((option) => html`
            <label class="check">
              <input type="checkbox" .checked=${selected.has(option.value)} @change=${() => this.toggleValue(control, option.value)} />
              <span>${option.label}</span>
            </label>
          `)}
        </div>
        <div class="actions">
          <button class="action" type="button" @click=${() => this.clear()}>Clear</button>
          <button class="action" type="button" @click=${() => this.close()}>Close</button>
        </div>
      </div>
    `
  }

  private renderText(definition: FilterDefinition, control: FilterControl) {
    return html`
      <div class="popover">
        <select aria-label="${definition.label} operator" .value=${control.operator ?? definition.defaultOperator ?? 'contains'} @change=${(event: Event) => this.update({ ...control, type: 'text', operator: (event.currentTarget as HTMLSelectElement).value })}>
          ${(definition.operators ?? ['contains']).map((operator) => html`<option value=${operator}>${operatorLabel(operator)}</option>`)}
        </select>
        <input type="search" placeholder="Search..." .value=${control.value ?? ''} @input=${(event: Event) => this.update({ ...control, type: 'text', value: (event.currentTarget as HTMLInputElement).value })} />
        <div class="actions">
          <button class="action" type="button" @click=${() => this.clear()}>Clear</button>
          <button class="action" type="button" @click=${() => this.close()}>Close</button>
        </div>
      </div>
    `
  }

  private definition(): FilterDefinition | undefined {
    return this.currentConfig()[this.currentFilterId()]
  }

  private control(definition: FilterDefinition): FilterControl {
    return this.currentFilters().controls?.[this.currentFilterId()] ?? defaultControl(definition)
  }

  private summary(definition: FilterDefinition, control: FilterControl): string {
    switch (definition.type) {
      case 'date_range':
        return dateSummary(definition, control)
      case 'multi_select': {
        const count = control.values?.length ?? 0
        if (count === 0) return allValuesLabel(definition)
        if (count === 1) return control.values?.[0] ?? ''
        return `${count} selected`
      }
      case 'text':
        return control.value?.trim() || `Any ${definition.label.toLowerCase()}`
      default:
        return definition.label
    }
  }

  private toggle(definition: FilterDefinition, control: FilterControl): void {
    this.open = !this.open
    if (this.open && definition.type === 'date_range') {
      this.draftFrom = control.from ?? ''
      this.draftTo = control.to ?? ''
      this.customDate = control.preset === 'custom' || Boolean(control.from || control.to)
    }
    this.renderCard()
  }

  private pickDatePreset(control: FilterControl, value: string): void {
    if (value === 'custom') {
      this.draftFrom = control.from ?? ''
      this.draftTo = control.to ?? ''
      this.customDate = true
      this.renderCard()
      return
    }
    this.customDate = false
    this.update({ ...control, type: 'date_range', preset: value, from: '', to: '' })
    this.open = false
    this.renderCard()
  }

  private setDraft(key: 'from' | 'to', event: Event): void {
    const value = (event.currentTarget as HTMLInputElement).value
    if (key === 'from') this.draftFrom = value
    if (key === 'to') this.draftTo = value
    if (this.draftFrom && this.draftTo && this.draftTo < this.draftFrom) {
      const from = this.draftTo
      this.draftTo = this.draftFrom
      this.draftFrom = from
    }
    this.renderCard()
  }

  private setSearch(event: Event): void {
    this.search = (event.currentTarget as HTMLInputElement).value
    this.renderCard()
  }

  private applyDate(control: FilterControl): void {
    this.update({ ...control, type: 'date_range', preset: 'custom', from: this.draftFrom, to: this.draftTo })
    this.customDate = false
    this.open = false
    this.renderCard()
  }

  private toggleValue(control: FilterControl, value: string): void {
    const selected = new Set(control.values ?? [])
    if (selected.has(value)) {
      selected.delete(value)
    } else {
      selected.add(value)
    }
    this.update({ ...control, type: 'multi_select', operator: 'in', values: [...selected].sort() })
    this.renderCard()
  }

  private clear(): void {
    const definition = this.definition()
    if (!definition) return
    this.draftFrom = ''
    this.draftTo = ''
    this.search = ''
    this.customDate = false
    this.update(defaultControl(definition))
    this.open = false
    this.renderCard()
  }

  private update(control: FilterControl): void {
    const filtersSignal = this.currentFilters()
    const filters: FiltersSignal = {
      controls: { ...(filtersSignal.controls ?? {}), [this.currentFilterId()]: control },
      visualSelections: [...(filtersSignal.visualSelections ?? [])],
    }
    const config = this.currentFilterConfig()
    this.dispatchEvent(new CustomEvent('ld-filters-change', {
      detail: { filters, urlParams: filtersToURLParams(config, filters) },
      bubbles: true,
      composed: true,
    }))
  }

  private currentFilterId(): string {
    return this.getAttribute('filter-id') || ''
  }

  private currentConfig(): Record<string, FilterDefinition> {
    return filterConfigMap(this.currentFilterConfig())
  }

  private currentFilterConfig(): FilterConfig {
    return readJSONAttribute<FilterConfig>(this, 'config', [])
  }

  private currentFilters(): FiltersSignal {
    return readJSONAttribute(this, 'filters', emptyFilters)
  }

  private currentOptions(): Record<string, FilterOption[]> {
    return readJSONAttribute(this, 'options', {})
  }

  private isLoading(): boolean {
    const loading = this.getAttribute('loading')
    return loading !== null && loading !== 'false'
  }

  private close(): void {
    this.open = false
    this.renderCard()
  }
}

function presetLabel(preset: DatePreset): string {
  if (preset.value === 'all') return 'All'
  if (preset.relativeDays) return `${preset.relativeDays}d`
  return preset.label
}

function dateSummary(definition: FilterDefinition, control: FilterControl): string {
  if (control.from || control.to) {
    if (control.from && control.to) return `${formatDate(control.from)} - ${formatDate(control.to)}`
    if (control.from) return `From ${formatDate(control.from)}`
    return `Until ${formatDate(control.to ?? '')}`
  }
  const preset = control.preset || definition.default?.preset || 'all'
  return (definition.presets ?? []).find((item) => item.value === preset)?.label ?? 'Custom range'
}

function allValuesLabel(definition: FilterDefinition): string {
  const label = definition.label.toLowerCase()
  if (label === 'state') return 'All states'
  if (label.endsWith('y')) return `All ${label.slice(0, -1)}ies`
  if (label.endsWith('s')) return `All ${label}`
  return `All ${label}s`
}

function formatDate(value: string): string {
  const [year, month, day] = value.split('-').map((part) => Number(part))
  if (!year || !month || !day) return value
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric' }).format(new Date(year, month - 1, day))
}

function operatorLabel(operator: string): string {
  switch (operator) {
    case 'equals':
      return 'Equals'
    case 'starts_with':
      return 'Starts with'
    case 'not_contains':
      return 'Does not contain'
    default:
      return 'Contains'
  }
}

function readJSONAttribute<T>(element: Element, name: string, fallback: T): T {
  const value = element.getAttribute(name)
  if (!value) return fallback
  try {
    return JSON.parse(value) as T
  } catch {
    return fallback
  }
}

if (!window.customElements.get('ld-filter-card')) window.customElements.define('ld-filter-card', FilterCard)
