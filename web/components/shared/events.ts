export const domainEvents = {
  chatSubmit: 'lv-chat-submit',
  chatReferenceSearch: 'lv-chat-reference-search',
  chatReferencesChange: 'lv-chat-references-change',
  agentReferencesChange: 'lv-agent-references-change',
  chatNew: 'lv-chat-new',
  chatRestore: 'lv-chat-restore',
  chatDrawerClose: 'lv-chat-drawer-close',
  visualAction: 'lv-visual-action',
  visualSelect: 'lv-visual-select',
  tableWindow: 'lv-table-window',
  tableSort: 'lv-table-sort',
  filterChange: 'lv-filter-change',
  filterClear: 'lv-filter-clear',
} as const

export type DomainEventName = typeof domainEvents[keyof typeof domainEvents]

export function emitDomainEvent<T>(target: EventTarget, name: DomainEventName, detail: T): boolean {
  return target.dispatchEvent(new CustomEvent(name, {
    bubbles: true,
    composed: true,
    detail,
  }))
}
