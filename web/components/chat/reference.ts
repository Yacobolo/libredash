import { BarChart3, Boxes, Columns3, Database, File, Filter, LayoutDashboard, PanelsTopLeft, Plug, Search, Sigma, Table2 } from 'lucide'
import type { AgentContextSignal, AgentReferenceSignal } from '../../generated/signals'
import { lucideIcon } from '../shared/lucide-icons'

export type ChatContextReference = AgentReferenceSignal

export interface ChatReferenceSearchDetail {
  query: string
  requestId: number
}

export interface ChatReferencesChangeDetail {
  references: AgentReferenceSignal[]
}

export const defaultAgentReferenceLimit = 12

export function normalizeReferenceLimit(limit: number | null | undefined): number {
  return Number.isFinite(limit) && Number(limit) > 0
    ? Math.floor(Number(limit))
    : defaultAgentReferenceLimit
}

export function referenceIdentity(reference: AgentReferenceSignal): string {
  return `${reference.workspaceId ?? ''}:${reference.kind}:${reference.id || reference.componentId || reference.visualId || reference.title}`
}

export function uniqueReferences(references: AgentReferenceSignal[]): AgentReferenceSignal[] {
  const seen = new Set<string>()
  return references.filter((reference) => {
    const key = referenceIdentity(reference)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

const mentionStopWords = new Set(['a', 'an', 'and', 'by', 'for', 'in', 'of', 'on', 'the', 'to'])

export function normalizedReferenceQuery(query: string): string {
  return query.trim().toLocaleLowerCase()
}

export function matchesReferenceQuery(reference: AgentReferenceSignal, query: string): boolean {
  const tokens = normalizedReferenceQuery(query)
    .split(/[^\p{L}\p{N}_]+/u)
    .filter((token) => token !== '' && !mentionStopWords.has(token))
  if (tokens.length === 0) return true
  const haystack = `${reference.title} ${reference.description ?? ''} ${reference.kind}`.toLocaleLowerCase()
  return tokens.every((token) => haystack.includes(token))
}

export function isOnPageReference(reference: AgentReferenceSignal, context: AgentContextSignal | null): boolean {
  return Boolean(
    context?.workspaceId
    && context.dashboardId
    && context.pageId
    && reference.workspaceId === context.workspaceId
    && reference.dashboardId === context.dashboardId
    && reference.pageId === context.pageId,
  )
}

export function mergeReferences(...groups: AgentReferenceSignal[][]): AgentReferenceSignal[] {
  return uniqueReferences(groups.flat())
}

export function referenceIcon(kind: string) {
  switch (kind) {
    case 'dashboard': return lucideIcon(LayoutDashboard)
    case 'page': return lucideIcon(PanelsTopLeft)
    case 'visual': return lucideIcon(BarChart3)
    case 'filter': return lucideIcon(Filter)
    case 'semantic_model': return lucideIcon(Boxes)
    case 'dataset':
    case 'semantic_table': return lucideIcon(Database)
    case 'measure': return lucideIcon(Sigma)
    case 'field': return lucideIcon(Columns3)
    case 'source': return lucideIcon(Plug)
    case 'table': return lucideIcon(Table2)
    case 'asset': return lucideIcon(File)
    default: return lucideIcon(Search)
  }
}
