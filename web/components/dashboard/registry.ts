import { defineElementOnce } from '../shared/lazy-registry'

type DashboardComponentKind =
  | 'lv-filter-card'
  | string

const dashboardComponentLoaders: Record<string, () => Promise<unknown>> = {
  'lv-filter-panel': () => import('./filters/filter-panel'),
  'lv-filter-card': () => import('./filters/filter-card'),
}

export function loadDashboardComponent(kind: DashboardComponentKind): Promise<void> {
  const loader = dashboardComponentLoaders[kind]
  if (!loader) return Promise.reject(new Error(`unsupported dashboard component kind ${kind}`))
  return defineElementOnce(kind, loader)
}

export function dashboardComponentKinds(): string[] {
  return Object.keys(dashboardComponentLoaders)
}
