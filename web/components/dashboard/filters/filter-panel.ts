import { LitElement, css, html, nothing } from 'lit'
import { property, state } from 'lit/decorators.js'
import { ChevronDown, ChevronLeft, ChevronRight, X } from 'lucide'
import { lucideIcon } from '../../shared/lucide-icons'
import {
  defaultControl,
  emptyFilters,
  filterConfigEntries,
  filterConfigMap,
  filtersToURLParams,
  interactionSelectionLabel,
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

type DateDraft = {
  filter: string
  from: string
  to: string
  month: string
}

type CalendarDay = {
  value: string
  day: number
  month: number
}

const jsonConverter = <T,>(fallback: T) => ({
  fromAttribute(value: string | null): T {
    if (!value) return fallback
    try {
      return JSON.parse(value) as T
    } catch {
      return fallback
    }
  },
  toAttribute(value: T | null): string {
    return JSON.stringify(value ?? fallback)
  },
})

class FilterPanel extends LitElement {
  @property({ attribute: 'config', converter: jsonConverter<FilterConfig>([]) }) config: FilterConfig = []
  @property({ attribute: 'filters', converter: jsonConverter<FiltersSignal>(emptyFilters) }) filters: FiltersSignal = emptyFilters
  @property({ attribute: 'options', converter: jsonConverter<Record<string, FilterOption[]>>({}) }) options: Record<string, FilterOption[]> = {}
  @property({ type: Boolean, reflect: true }) loading = false
  @state() private searches: Record<string, string> = {}
  @state() private openDate: string | null = null
  @state() private dateDraft: DateDraft | null = null

  static styles = css`
    :host {
      display: block;
      color: var(--lv-fg-default);
      font-family: var(--fontStack-system);
    }

    .panel {
      display: grid;
      gap: var(--base-size-8);
      font-size: var(--lv-font-size-caption);
    }

    header,
    .header-title,
    .summary {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
    }

    header {
      border-bottom: var(--lv-border-default);
      padding-bottom: var(--base-size-8);
    }

    .header-title {
      min-width: 0;
    }

    h2 {
      margin: 0;
      font-size: var(--lv-font-size-body-md);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-tight);
    }

    .count {
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-full);
      background: var(--lv-bg-panel-muted);
      color: var(--lv-fg-muted);
      padding: var(--base-size-2) var(--base-size-6);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-none);
      white-space: nowrap;
    }

    .close {
      display: inline-grid;
      width: var(--lv-button-height-xs);
      height: var(--lv-button-height-xs);
      place-items: center;
      border: var(--borderWidth-default) solid var(--lv-button-border-rest);
      border-radius: var(--lv-radius-tight);
      background: var(--lv-button-bg-rest);
      color: var(--lv-button-fg-rest);
      cursor: pointer;
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      line-height: var(--lv-line-height-none);
    }

    .close:hover {
      border-color: var(--lv-button-border-hover);
      background: var(--lv-button-bg-hover);
      color: var(--lv-fg-default);
    }

    .close svg,
    .date-trigger svg,
    .calendar-nav svg {
      width: var(--base-size-16);
      height: var(--base-size-16);
    }

    .card {
      display: grid;
      gap: var(--base-size-6);
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-panel);
      padding: var(--base-size-8);
    }

    .card-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
    }

    h3 {
      margin: 0;
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      text-transform: uppercase;
    }

    button,
    input,
    select {
      font: inherit;
    }

    .clear,
    .reset {
      min-height: var(--lv-button-height-xs);
      border: var(--borderWidth-default) solid var(--lv-button-border-rest);
      border-radius: var(--lv-radius-tight);
      background: var(--lv-button-bg-rest);
      color: var(--lv-button-fg-rest);
      cursor: pointer;
      padding: 0 var(--lv-button-padding-inline-xs);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
    }

    .clear:disabled,
    .reset:disabled,
    .refresh:disabled,
    .preset:disabled,
    .date-trigger:disabled,
    .calendar-nav:disabled,
    .day:disabled,
    .popover-action:disabled {
      cursor: default;
      opacity: var(--opacity-disabled);
    }

    .input-row {
      display: grid;
      grid-template-columns: 1fr;
      gap: var(--base-size-6);
    }

    .date-filter {
      position: relative;
      display: grid;
      gap: var(--base-size-6);
    }

    .preset-row {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: var(--base-size-4);
    }

    .preset,
    .date-trigger,
    .calendar-nav,
    .day,
    .popover-action {
      border: var(--borderWidth-default) solid var(--lv-button-border-rest);
      border-radius: var(--lv-radius-tight);
      background: var(--lv-button-bg-rest);
      color: var(--lv-button-fg-rest);
      cursor: pointer;
      font-weight: var(--lv-font-weight-strong);
    }

    .preset {
      min-width: 0;
      min-height: var(--lv-button-height-sm);
      overflow: hidden;
      padding: 0 var(--lv-button-padding-inline-xs);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--lv-font-size-caption);
    }

    .preset.custom {
      grid-column: 1 / -1;
    }

    .preset[aria-pressed='true'] {
      border-color: var(--borderColor-accent-muted);
      background: var(--bgColor-accent-muted);
      color: var(--lv-fg-default);
    }

    .date-trigger {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
      min-height: var(--control-medium-size);
      padding: 0 var(--control-small-paddingInline-normal);
      text-align: left;
      font-size: var(--lv-font-size-caption);
    }

    .date-trigger span:first-child {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .date-popover {
      position: absolute;
      top: calc(100% + var(--base-size-4));
      left: 0;
      right: 0;
      z-index: var(--zIndex-dropdown);
      display: grid;
      gap: var(--base-size-6);
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-default);
      background: var(--lv-bg-overlay);
      box-shadow: var(--shadow-floating-small);
      padding: var(--base-size-8);
    }

    .calendar-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--base-size-8);
    }

    .calendar-title {
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
    }

    .calendar-nav {
      width: var(--control-xsmall-size);
      height: var(--control-xsmall-size);
      padding: 0;
      font-size: var(--lv-font-size-body-md);
    }

    .calendar-grid {
      display: grid;
      grid-template-columns: repeat(7, minmax(0, 1fr));
      gap: var(--base-size-2);
    }

    .weekday {
      color: var(--lv-fg-muted);
      text-align: center;
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      text-transform: uppercase;
    }

    .day {
      aspect-ratio: 1;
      min-height: 0;
      padding: 0;
      border-color: transparent;
      background: transparent;
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-caption);
    }

    .day.outside {
      color: var(--lv-fg-muted);
      opacity: 0.45;
    }

    .day.in-range {
      background: var(--bgColor-accent-muted);
    }

    .day.selected {
      border-color: var(--lv-button-accent-border-rest);
      background: var(--lv-button-accent-bg-rest);
      color: var(--lv-button-accent-fg-rest);
    }

    .date-row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: var(--base-size-6);
    }

    .date-field {
      display: grid;
      gap: var(--base-size-4);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
      text-transform: uppercase;
    }

    .popover-actions {
      display: grid;
      grid-template-columns: 1fr 1fr 1fr;
      gap: var(--base-size-4);
    }

    .popover-action {
      min-height: var(--lv-button-height-xs);
      padding: 0 var(--lv-button-padding-inline-xs);
      font-size: var(--lv-font-size-caption);
    }

    .popover-action.primary {
      border-color: var(--lv-button-accent-border-rest);
      background: var(--lv-button-accent-bg-rest);
      color: var(--lv-button-accent-fg-rest);
    }

    input,
    select {
      width: 100%;
      min-width: 0;
      min-height: var(--control-xsmall-size);
      border: var(--lv-border-default);
      border-radius: var(--lv-radius-tight);
      background: var(--lv-bg-control);
      color: var(--lv-fg-default);
      padding: 0 var(--control-xsmall-paddingInline-normal);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-regular);
      outline-offset: var(--base-size-2);
    }

    input:focus,
    select:focus {
      outline: var(--lv-border-width-focus) solid var(--lv-accent);
    }

    .checks {
      display: grid;
      max-height: 138px;
      gap: var(--base-size-2);
      overflow: auto;
    }

    label.check {
      display: flex;
      min-width: 0;
      align-items: center;
      gap: var(--base-size-6);
      border-radius: var(--lv-radius-tight);
      padding: var(--base-size-4);
      color: var(--lv-fg-default);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
    }

    label.check:hover {
      background: var(--lv-bg-panel-muted);
    }

    label.check input {
      width: var(--base-size-12);
      height: var(--base-size-12);
      min-height: 0;
      accent-color: var(--lv-accent);
    }

    .empty {
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
      padding: var(--base-size-4);
    }

    .chips {
      display: flex;
      flex-wrap: wrap;
      gap: var(--base-size-4);
    }

    .chip {
      max-width: 100%;
      overflow: hidden;
      border: var(--lv-border-muted);
      border-radius: var(--lv-radius-full);
      background: var(--lv-bg-panel-muted);
      color: var(--lv-fg-muted);
      padding: var(--base-size-2) var(--base-size-6);
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
    }

    .summary {
      min-height: var(--control-xsmall-size);
      color: var(--lv-fg-muted);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-medium);
    }

    .refresh {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: var(--base-size-4);
      min-height: var(--lv-button-height-sm);
      width: 100%;
      cursor: pointer;
      border: var(--borderWidth-default) solid var(--lv-button-accent-border-rest);
      border-radius: var(--lv-radius-tight);
      background: var(--lv-button-accent-bg-rest);
      color: var(--lv-button-accent-fg-rest);
      font-size: var(--lv-font-size-caption);
      font-weight: var(--lv-font-weight-strong);
    }
  `

  render() {
    const entries = filterConfigEntries(this.config)
    const activeCount = this.activeCount()
    return html`
      <section class="panel" aria-label="Filters">
        <header>
          <div class="header-title">
            <h2>Filters</h2>
            <span class="count">${activeCount} active</span>
          </div>
          <button class="close" type="button" aria-label="Collapse filters" title="Collapse filters" @click=${this.close}>${lucideIcon(X)}</button>
        </header>
        ${entries.map(([name, definition]) => this.renderFilter(name, definition))}
        ${this.renderInteractionSelections()}
        <div class="summary">
          <span>${activeCount} total filter${activeCount === 1 ? '' : 's'} applied</span>
          <button class="reset" type="button" ?disabled=${this.loading || activeCount === 0} @click=${this.reset}>Reset</button>
        </div>
        <button class="refresh" type="button" ?disabled=${this.loading} @click=${this.refresh}>Refresh</button>
      </section>
    `
  }

  private renderFilter(name: string, definition: FilterDefinition) {
    const control = this.control(name, definition)
    return html`
      <article class="card">
        <div class="card-head">
          <h3>${definition.label}</h3>
          <button class="clear" type="button" ?disabled=${!this.isActive(name, definition)} @click=${() => this.clearFilter(name)}>Clear</button>
        </div>
        ${definition.type === 'date_range' ? this.renderDate(name, definition, control) : nothing}
        ${definition.type === 'multi_select' ? this.renderMulti(name, definition, control) : nothing}
        ${definition.type === 'text' ? this.renderText(name, definition, control) : nothing}
      </article>
    `
  }

  private renderDate(name: string, definition: FilterDefinition, control: FilterControl) {
    const preset = control.preset || definition.default?.preset || 'all'
    const showCustom = definition.custom && (preset === 'custom' || control.from || control.to || this.openDate === name)
    const presets = [...(definition.presets ?? [])]
    if (definition.custom) {
      presets.push({ value: 'custom', label: 'Custom' })
    }
    return html`
      <div class="date-filter">
        <button
          class="date-trigger"
          type="button"
          aria-expanded=${this.openDate === name ? 'true' : 'false'}
          ?disabled=${this.loading || !definition.custom}
          @click=${() => this.toggleDatePopover(name, definition, control)}
        >
          <span>${dateSummary(definition, control)}</span>
          ${lucideIcon(ChevronDown)}
        </button>
        <div class="preset-row" role="group" aria-label=${definition.label}>
          ${presets.map((item) => html`
            <button
              class=${`preset ${item.value === 'custom' ? 'custom' : ''}`}
              type="button"
              aria-pressed=${(showCustom ? 'custom' : preset) === item.value ? 'true' : 'false'}
              ?disabled=${this.loading}
              @click=${() => this.pickDatePreset(name, item.value)}
            >${presetShortLabel(item)}</button>
          `)}
        </div>
        ${showCustom
          ? this.renderDatePopover(name, definition, control)
          : nothing}
      </div>
    `
  }

  private renderDatePopover(name: string, definition: FilterDefinition, control: FilterControl) {
    if (this.openDate !== name) return nothing
    const draft = this.activeDateDraft(name, definition, control)
    const month = parseMonth(draft.month)
    const days = calendarDays(month)
    return html`
      <div class="date-popover" @keydown=${(event: KeyboardEvent) => this.handleDatePopoverKey(name, event)}>
        <div class="calendar-head">
          <button class="calendar-nav" type="button" aria-label="Previous month" @click=${() => this.shiftDraftMonth(-1)}>${lucideIcon(ChevronLeft)}</button>
          <div class="calendar-title">${monthTitle(month)}</div>
          <button class="calendar-nav" type="button" aria-label="Next month" @click=${() => this.shiftDraftMonth(1)}>${lucideIcon(ChevronRight)}</button>
        </div>
        <div class="calendar-grid">
          ${['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((day) => html`<div class="weekday">${day}</div>`)}
          ${days.map((day) => this.renderCalendarDay(day, month, draft))}
        </div>
        <div class="date-row">
          <label class="date-field">
            From
            <input type="date" .value=${draft.from} @input=${(event: Event) => this.setDraftDate('from', event)} />
          </label>
          <label class="date-field">
            To
            <input type="date" .value=${draft.to} @input=${(event: Event) => this.setDraftDate('to', event)} />
          </label>
        </div>
        <div class="popover-actions">
          <button class="popover-action" type="button" @click=${() => this.clearDateDraft(name, definition)}>Clear</button>
          <button class="popover-action" type="button" @click=${this.cancelDateDraft}>Cancel</button>
          <button class="popover-action primary" type="button" @click=${() => this.applyDateDraft(name)}>Apply</button>
        </div>
      </div>
    `
  }

  private renderCalendarDay(day: CalendarDay, month: Date, draft: DateDraft) {
    const selected = day.value === draft.from || day.value === draft.to
    const inRange = isInRange(day.value, draft.from, draft.to)
    const classes = ['day']
    if (day.month !== month.getMonth()) classes.push('outside')
    if (inRange) classes.push('in-range')
    if (selected) classes.push('selected')
    return html`
      <button class=${classes.join(' ')} type="button" @click=${() => this.pickDraftDay(day.value)}>
        ${day.day}
      </button>
    `
  }

  private renderMulti(name: string, definition: FilterDefinition, control: FilterControl) {
    const search = this.searches[name]?.toLowerCase() ?? ''
    const selected = new Set(control.values ?? [])
    const options = (this.options[name] ?? []).filter((option) => option.label.toLowerCase().includes(search) || option.value.toLowerCase().includes(search))
    return html`
      <div class="input-row">
        <input type="search" placeholder="Search ${definition.label.toLowerCase()}..." .value=${this.searches[name] ?? ''} @input=${(event: Event) => this.setSearch(name, event)} />
        <div class="checks">
          ${options.length === 0 ? html`<div class="empty">No values loaded</div>` : nothing}
          ${options.map((option) => html`
            <label class="check">
              <input type="checkbox" .checked=${selected.has(option.value)} @change=${() => this.toggleValue(name, option.value)} />
              <span>${option.label}</span>
            </label>
          `)}
        </div>
      </div>
    `
  }

  private renderText(name: string, definition: FilterDefinition, control: FilterControl) {
    return html`
      <div class="input-row">
        <select aria-label="${definition.label} operator" .value=${control.operator ?? definition.defaultOperator ?? 'contains'} @change=${(event: Event) => this.setOperator(name, event)}>
          ${(definition.operators ?? ['contains']).map((operator) => html`<option value=${operator}>${operatorLabel(operator)}</option>`)}
        </select>
        <input type="search" placeholder="health, watches, furniture..." .value=${control.value ?? ''} @input=${(event: Event) => this.setTextValue(name, event)} />
      </div>
    `
  }

  private renderInteractionSelections() {
    const selections = this.filters.selections ?? []
    if (selections.length === 0) return nothing
    return html`
      <article class="card">
        <div class="card-head">
          <h3>Selections</h3>
          <button class="clear" type="button" @click=${this.clearSelections}>Clear</button>
        </div>
        <div class="chips">
          ${selections.map((selection) => html`<span class="chip">${interactionSelectionLabel(selection)}</span>`)}
        </div>
      </article>
    `
  }

  private control(name: string, definition: FilterDefinition): FilterControl {
    return this.filters.controls?.[name] ?? defaultControl(definition)
  }

  private nextFilters(): FiltersSignal {
    return {
      controls: { ...(this.filters.controls ?? {}) },
      selections: [...(this.filters.selections ?? [])],
    }
  }

  private emitChange(filters: FiltersSignal): void {
    this.dispatchEvent(new CustomEvent('lv-filters-change', { detail: { filters, urlParams: filtersToURLParams(this.config, filters) }, bubbles: true, composed: true }))
  }

  private updateControl(name: string, control: FilterControl): void {
    const filters = this.nextFilters()
    filters.controls[name] = control
    this.emitChange(filters)
  }

  private pickDatePreset(name: string, value: string): void {
    const definition = this.configMap()[name]
    const control = this.control(name, definition)
    if (value === 'custom') {
      this.toggleDatePopover(name, definition, { ...control, type: 'date_range', preset: 'custom' })
      return
    }
    this.openDate = null
    this.dateDraft = null
    this.updateControl(name, {
      ...control,
      type: 'date_range',
      preset: value,
      from: '',
      to: '',
    })
  }

  private toggleDatePopover(name: string, definition: FilterDefinition, control: FilterControl): void {
    if (!definition.custom) return
    if (this.openDate === name) {
      this.openDate = null
      this.dateDraft = null
      return
    }
    this.openDate = name
    this.dateDraft = this.createDateDraft(name, definition, control)
  }

  private activeDateDraft(name: string, definition: FilterDefinition, control: FilterControl): DateDraft {
    if (this.dateDraft?.filter === name) return this.dateDraft
    const draft = this.createDateDraft(name, definition, control)
    this.dateDraft = draft
    return draft
  }

  private createDateDraft(name: string, definition: FilterDefinition, control: FilterControl): DateDraft {
    const from = control.from ?? ''
    const to = control.to ?? ''
    return {
      filter: name,
      from,
      to,
      month: monthSeed(definition, from, to),
    }
  }

  private shiftDraftMonth(delta: number): void {
    if (!this.dateDraft) return
    const month = parseMonth(this.dateDraft.month)
    month.setMonth(month.getMonth() + delta)
    this.dateDraft = { ...this.dateDraft, month: formatMonth(month) }
  }

  private pickDraftDay(value: string): void {
    if (!this.dateDraft) return
    const { from, to } = this.dateDraft
    if (!from || (from && to)) {
      this.dateDraft = { ...this.dateDraft, from: value, to: '' }
      return
    }
    if (value < from) {
      this.dateDraft = { ...this.dateDraft, from: value, to: from }
      return
    }
    this.dateDraft = { ...this.dateDraft, to: value }
  }

  private setDraftDate(key: 'from' | 'to', event: Event): void {
    if (!this.dateDraft) return
    const value = (event.currentTarget as HTMLInputElement).value
    const next = { ...this.dateDraft, [key]: value }
    if (value) {
      next.month = value.slice(0, 7)
    }
    if (next.from && next.to && next.to < next.from) {
      const from = next.to
      next.to = next.from
      next.from = from
    }
    this.dateDraft = next
  }

  private clearDateDraft(name: string, definition: FilterDefinition): void {
    this.openDate = null
    this.dateDraft = null
    this.updateControl(name, defaultControl(definition))
  }

  private cancelDateDraft = (): void => {
    this.openDate = null
    this.dateDraft = null
  }

  private applyDateDraft(name: string): void {
    if (!this.dateDraft) return
    const definition = this.configMap()[name]
    const control = this.control(name, definition)
    this.openDate = null
    this.updateControl(name, {
      ...control,
      type: 'date_range',
      preset: 'custom',
      from: this.dateDraft.from,
      to: this.dateDraft.to,
    })
    this.dateDraft = null
  }

  private handleDatePopoverKey(name: string, event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.stopPropagation()
      this.openDate = null
      this.dateDraft = null
    }
    if (event.key === 'Enter' && event.metaKey) {
      event.stopPropagation()
      this.applyDateDraft(name)
    }
  }

  private toggleValue(name: string, value: string): void {
    const definition = this.configMap()[name]
    const control = this.control(name, definition)
    const selected = new Set(control.values ?? [])
    if (selected.has(value)) {
      selected.delete(value)
    } else {
      selected.add(value)
    }
    this.updateControl(name, { ...control, type: 'multi_select', operator: 'in', values: [...selected].sort() })
  }

  private setOperator(name: string, event: Event): void {
    const definition = this.configMap()[name]
    const control = this.control(name, definition)
    this.updateControl(name, { ...control, type: 'text', operator: (event.currentTarget as HTMLSelectElement).value })
  }

  private setTextValue(name: string, event: Event): void {
    const definition = this.configMap()[name]
    const control = this.control(name, definition)
    this.updateControl(name, { ...control, type: 'text', value: (event.currentTarget as HTMLInputElement).value })
  }

  private setSearch(name: string, event: Event): void {
    this.searches = { ...this.searches, [name]: (event.currentTarget as HTMLInputElement).value }
  }

  private clearFilter(name: string): void {
    const definition = this.configMap()[name]
    if (this.openDate === name) {
      this.openDate = null
      this.dateDraft = null
    }
    this.updateControl(name, defaultControl(definition))
  }

  private clearSelections = (): void => {
    this.dispatchEvent(new CustomEvent('lv-selection-clear', { bubbles: true, composed: true }))
  }

  private reset = (): void => {
    const filters: FiltersSignal = { controls: {}, selections: [] }
    for (const [name, definition] of filterConfigEntries(this.config)) {
      filters.controls[name] = defaultControl(definition)
    }
    this.openDate = null
    this.dateDraft = null
    this.dispatchEvent(new CustomEvent('lv-filters-reset', { detail: { filters, urlParams: filtersToURLParams(this.config, filters) }, bubbles: true, composed: true }))
  }

  private refresh = (): void => {
    this.dispatchEvent(new CustomEvent('lv-filters-refresh', { bubbles: true, composed: true }))
  }

  private close = (): void => {
    this.dispatchEvent(new CustomEvent('lv-filters-close', { bubbles: true, composed: true }))
  }

  private activeCount(): number {
    let count = this.filters.selections?.length ?? 0
    for (const [name, definition] of filterConfigEntries(this.config)) {
      if (this.isActive(name, definition)) count += 1
    }
    return count
  }

  private isActive(name: string, definition: FilterDefinition): boolean {
    const control = this.control(name, definition)
    switch (definition.type) {
      case 'date_range':
        return Boolean(control.from || control.to || ((control.preset || definition.default?.preset || 'all') !== (definition.default?.preset || 'all')))
      case 'multi_select':
        return (control.values ?? []).length > 0
      case 'text':
        return Boolean((control.value ?? '').trim())
      default:
        return false
    }
  }

  private configMap(): Record<string, FilterDefinition> {
    return filterConfigMap(this.config)
  }
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

function presetShortLabel(preset: DatePreset): string {
  if (preset.value === 'all') return 'All'
  if (preset.relativeDays) return `${preset.relativeDays}d`
  return preset.label.replace(/^Latest\s+/i, '')
}

function dateSummary(definition: FilterDefinition, control: FilterControl): string {
  if (control.from || control.to) {
    if (control.from && control.to) return `${formatReadableDate(control.from)} - ${formatReadableDate(control.to)}`
    if (control.from) return `From ${formatReadableDate(control.from)}`
    return `Until ${formatReadableDate(control.to ?? '')}`
  }
  const preset = control.preset || definition.default?.preset || 'all'
  return (definition.presets ?? []).find((item) => item.value === preset)?.label ?? 'Custom range'
}

function monthSeed(definition: FilterDefinition, from: string, to: string): string {
  if (from) return from.slice(0, 7)
  if (to) return to.slice(0, 7)
  const datedPreset = (definition.presets ?? []).find((item) => item.from)
  if (datedPreset?.from) return datedPreset.from.slice(0, 7)
  return formatMonth(new Date())
}

function parseMonth(month: string): Date {
  const [year, index] = month.split('-').map((part) => Number(part))
  if (!year || !index) return new Date(new Date().getFullYear(), new Date().getMonth(), 1)
  return new Date(year, index - 1, 1)
}

function formatMonth(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
}

function monthTitle(date: Date): string {
  return new Intl.DateTimeFormat(undefined, { month: 'short', year: 'numeric' }).format(date)
}

function calendarDays(month: Date): CalendarDay[] {
  const first = new Date(month.getFullYear(), month.getMonth(), 1)
  const mondayOffset = (first.getDay() + 6) % 7
  const start = new Date(first)
  start.setDate(first.getDate() - mondayOffset)
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(start)
    date.setDate(start.getDate() + index)
    return {
      value: formatDate(date),
      day: date.getDate(),
      month: date.getMonth(),
    }
  })
}

function formatDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function formatReadableDate(value: string): string {
  if (!value) return ''
  const [year, month, day] = value.split('-').map((part) => Number(part))
  if (!year || !month || !day) return value
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric' }).format(new Date(year, month - 1, day))
}

function isInRange(value: string, from: string, to: string): boolean {
  if (!from || !to) return false
  return value > from && value < to
}

customElements.define('lv-filter-panel', FilterPanel)
