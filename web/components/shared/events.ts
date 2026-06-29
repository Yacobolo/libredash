export const domainEvents = {
  chatSubmit: 'ld-chat-submit',
  visualAction: 'ld-visual-action',
  visualSelect: 'ld-visual-select',
  tableWindow: 'ld-table-window',
  tableSort: 'ld-table-sort',
  filterChange: 'ld-filter-change',
  filterClear: 'ld-filter-clear',
} as const

export type DomainEventName = typeof domainEvents[keyof typeof domainEvents]

export function emitDomainEvent<T>(target: EventTarget, name: DomainEventName, detail: T): boolean {
  return target.dispatchEvent(new CustomEvent(name, {
    bubbles: true,
    composed: true,
    detail,
  }))
}
