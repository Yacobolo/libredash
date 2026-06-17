/**
 * Datastar Inspector - Dev-only debugging tool
 *
 * A self-contained web component for inspecting Datastar signals.
 * Works in any Datastar project with zero configuration.
 */
import { LitElement, html, nothing } from 'lit'
import { customElement, state } from 'lit/decorators.js'

import type { InspectorState, SignalObject } from './types.js'
import {
  countSignals,
  filterObject,
  findChangedPaths,
  parseFilterPattern,
} from './utils.js'

const FLASH_DURATION = 400
const STORAGE_KEY = 'ds-inspector'

@customElement('datastar-inspector')
export class DatastarInspector extends LitElement {
  override createRenderRoot() {
    return this
  }

  @state() private expanded = false
  @state() private filter = ''
  @state() private signals: SignalObject = {}
  @state() private signalCount = 0
  @state() private changedPaths: Set<string> = new Set()
  @state() private expandedPaths: Set<string> = new Set()
  @state() private hasUnseenChanges = false

  private observer: MutationObserver | null = null
  private signalsElementId = `ds-inspector-signals-${Math.random().toString(36).slice(2, 9)}`
  private signalsElement: HTMLElement | null = null
  private previousSignals: SignalObject = {}
  private flashTimeout: number | null = null
  private parseFrame: number | null = null

  override connectedCallback() {
    super.connectedCallback()
    this.loadState()
  }

  override disconnectedCallback() {
    super.disconnectedCallback()
    this.observer?.disconnect()
    if (this.parseFrame !== null) {
      cancelAnimationFrame(this.parseFrame)
    }
    if (this.flashTimeout) {
      clearTimeout(this.flashTimeout)
    }
  }

  override firstUpdated() {
    this.setupSignalObserver()
  }

  private loadState() {
    try {
      const saved = sessionStorage.getItem(STORAGE_KEY)
      if (saved) {
        const state: InspectorState = JSON.parse(saved)
        this.expanded = state.expanded ?? false
        this.filter = state.filter ?? ''
        this.expandedPaths = new Set(state.expandedPaths ?? [])
      }
    } catch {
      /* ignore parse errors */
    }
  }

  private saveState() {
    const state: InspectorState = {
      expanded: this.expanded,
      filter: this.filter,
      expandedPaths: [...this.expandedPaths],
    }
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  }

  private setupSignalObserver() {
    const el = document.getElementById(this.signalsElementId)
    if (!el) return

    this.observer?.disconnect()
    this.signalsElement = el
    this.parseSignals(el.textContent || '{}', true)

    this.observer = new MutationObserver(() => {
      this.scheduleSignalParse()
    })
    this.observer.observe(el, { childList: true, characterData: true, subtree: true })
  }

  private scheduleSignalParse() {
    if (this.parseFrame !== null) {
      cancelAnimationFrame(this.parseFrame)
    }
    this.parseFrame = requestAnimationFrame(() => {
      this.parseFrame = null
      this.parseSignals(this.signalsElement?.textContent || '{}', false)
    })
  }

  private parseSignals(json: string, isInitial: boolean) {
    try {
      const newSignals = JSON.parse(json) as SignalObject

      if (!isInitial && Object.keys(this.previousSignals).length > 0) {
        const changed = findChangedPaths(this.previousSignals, newSignals)
        if (changed.size > 0) {
          this.changedPaths = changed

          if (!this.expanded) {
            this.hasUnseenChanges = true
          }

          if (this.flashTimeout) {
            clearTimeout(this.flashTimeout)
          }
          this.flashTimeout = window.setTimeout(() => {
            this.changedPaths = new Set()
            this.hasUnseenChanges = false
          }, FLASH_DURATION)
        }
      }

      this.previousSignals = newSignals
      this.signals = newSignals
      this.signalCount = countSignals(this.signals)
    } catch {
      this.signals = {}
      this.signalCount = 0
    }
  }

  private getFilteredSignals(): SignalObject {
    if (!this.filter.trim()) return this.signals

    const regex = parseFilterPattern(this.filter.trim())
    return filterObject(this.signals, regex) as SignalObject
  }

  private toggle() {
    this.expanded = !this.expanded
    this.saveState()
    if (this.expanded) {
      this.hasUnseenChanges = false
    }
  }

  private close() {
    this.expanded = false
    this.saveState()
  }

  private handleFilterInput(e: Event) {
    this.filter = (e.target as HTMLInputElement).value
    this.saveState()
  }

  private clearFilter() {
    this.filter = ''
    this.saveState()
  }

  private togglePath(path: string) {
    const next = new Set(this.expandedPaths)
    if (next.has(path)) {
      next.delete(path)
    } else {
      next.add(path)
    }
    this.expandedPaths = next
    this.saveState()
  }

  override render() {
    return html`
      <pre id="${this.signalsElementId}" class="hidden" data-json-signals></pre>

      ${this.expanded ? this.renderOpenPanel() : this.renderToggle()}
    `
  }

  private renderToggle() {
    return html`
      <button
        class="btn bg-primary text-on-primary border-primary btn-circle btn-sm fixed bottom-4 right-4 z-[99999] shadow-xl ${this.hasUnseenChanges ? 'animate-pulse' : ''}"
        @click=${this.toggle}
        aria-label="Open Datastar Inspector"
      >
        DS
      </button>
    `
  }

  private renderOpenPanel() {
    const filteredSignals = this.getFilteredSignals()
    const filteredCount = countSignals(filteredSignals)
    const hasFilter = this.filter.trim().length > 0
    return this.renderPanel(filteredSignals, filteredCount, hasFilter)
  }

  private renderPanel(filteredSignals: SignalObject, filteredCount: number, hasFilter: boolean) {
    return html`
      <div class="fixed bottom-4 right-4 z-[99999] flex h-[32rem] w-96 max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-box border border-outline-variant bg-surface shadow-2xl">
        ${this.renderHeader(filteredCount, hasFilter)}
        ${this.renderContent(filteredSignals, hasFilter)}
      </div>
    `
  }

  private renderHeader(filteredCount: number, hasFilter: boolean) {
    const placeholder = hasFilter
      ? `${filteredCount}/${this.signalCount} match...`
      : `Filter ${this.signalCount} signals...`

    return html`
      <div class="flex items-center gap-2 border-b border-outline-variant bg-container-low px-3 py-2">
        <span class="badge badge-primary badge-sm">DS</span>
        <input
          type="text"
          class="input input-bordered input-xs w-full"
          placeholder="${placeholder}"
          .value=${this.filter}
          @input=${this.handleFilterInput}
        />
        ${hasFilter
          ? html`<button class="btn btn-ghost btn-xs" @click=${this.clearFilter} aria-label="Clear filter">&times;</button>`
          : nothing}
        <button class="btn btn-ghost btn-xs" @click=${this.close} aria-label="Close">&times;</button>
      </div>
    `
  }

  private renderContent(filteredSignals: SignalObject, hasFilter: boolean) {
    const isEmpty = Object.keys(filteredSignals).length === 0

    return html`
      <div class="flex-1 overflow-auto p-3">
        ${isEmpty
          ? html`<div class="flex h-full items-center justify-center text-sm text-on-surface-variant">
              ${hasFilter ? 'No signals match filter' : 'No signals found'}
            </div>`
          : this.renderJsonView(filteredSignals)}
      </div>
    `
  }

  private renderJsonView(signals: SignalObject) {
    return html`
      <div class="font-mono text-xs leading-6">
        ${Object.entries(signals).map(([key, value]) => this.renderTreeNode(key, value, key, 0, this.filter.trim().length > 0))}
      </div>
    `
  }

  private renderTreeNode(key: string, value: unknown, path: string, depth: number, hasFilter: boolean) {
    const changed = this.isChanged(path)
    const rowClass = changed ? 'bg-warning' : ''
    const rowStyle = this.treeRowStyle(depth)

    if (!this.isBranch(value)) {
      return html`
        <div class="flex min-w-0 items-baseline gap-1 rounded px-1 py-0.5 whitespace-nowrap ${rowClass}" style=${rowStyle}>
          <span class="font-medium text-on-surface-variant opacity-80">${key}</span>
          <span class="text-on-surface-variant opacity-70">:</span>
          ${this.renderPrimitive(value)}
        </div>
      `
    }

    const count = countSignals(value)
    const expanded = hasFilter || this.expandedPaths.has(path)
    const marker = Array.isArray(value) ? `[${count}]` : `{${count}}`

    return html`
      <div>
        <button
          type="button"
          class="flex min-w-0 cursor-pointer items-center gap-1 rounded border-0 bg-transparent px-1 py-0.5 text-left font-mono hover:bg-container-low ${rowClass}"
          style=${rowStyle}
          aria-expanded=${String(expanded)}
          aria-label=${expanded ? `Collapse ${key}` : `Expand ${key}`}
          @click=${() => this.togglePath(path)}
        >
          <span class="inline-block w-4 text-center text-on-surface-variant opacity-75">${expanded ? '▾' : '▸'}</span>
          <span class="font-medium text-on-surface-variant opacity-90">${key}</span>
          <span
            class="rounded border border-outline-variant bg-container-low px-1.5 text-[0.62rem] font-medium leading-4 text-on-surface-variant opacity-80"
          >
            ${marker}
          </span>
        </button>
        ${expanded
          ? this.childEntries(value).map(([childKey, childValue]) =>
              this.renderTreeNode(childKey, childValue, this.childPath(path, childKey, value), depth + 1, hasFilter)
            )
          : nothing}
      </div>
    `
  }

  private treeRowStyle(depth: number) {
    const indent = depth * 0.85
    const guide = depth > 0 ? 'border-left: 1px solid var(--borderColor-muted); padding-left: 0.55rem;' : ''
    return `margin-left: ${indent}rem; width: calc(100% - ${indent}rem); ${guide}`
  }

  private renderPrimitive(value: unknown) {
    if (value === null) {
      return html`<span class="text-on-surface-variant opacity-85">null</span>`
    }
    if (typeof value === 'boolean') {
      return html`<span class="text-primary opacity-90">${String(value)}</span>`
    }
    if (typeof value === 'number') {
      return html`<span class="text-warning opacity-90">${value}</span>`
    }
    if (typeof value === 'string') {
      return html`
        <span class="min-w-0 max-w-64 truncate text-success opacity-85">
          "${value}"
        </span>
      `
    }
    return html`<span class="min-w-0 max-w-64 truncate text-on-surface-variant opacity-85">${String(value)}</span>`
  }

  private isBranch(value: unknown): value is Record<string, unknown> | unknown[] {
    return typeof value === 'object' && value !== null
  }

  private childEntries(value: Record<string, unknown> | unknown[]): Array<[string, unknown]> {
    if (Array.isArray(value)) {
      return value.map((item, index) => [String(index), item])
    }
    return Object.entries(value)
  }

  private childPath(path: string, key: string, parent: Record<string, unknown> | unknown[]) {
    return Array.isArray(parent) ? `${path}[${key}]` : `${path}.${key}`
  }

  private isChanged(path: string) {
    if (this.changedPaths.has(path)) return true
    for (const changedPath of this.changedPaths) {
      if (changedPath.startsWith(`${path}.`) || changedPath.startsWith(`${path}[`)) {
        return true
      }
    }
    return false
  }

}

declare global {
  interface HTMLElementTagNameMap {
    'datastar-inspector': DatastarInspector
  }
}
